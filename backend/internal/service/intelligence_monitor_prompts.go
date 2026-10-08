package service

import (
	"context"
	"strings"
	"unicode/utf8"
)

const IntelligenceMonitorMaxPromptCharacters = 8000
const IntelligenceMonitorMaxChannelPrompts = 200
const IntelligenceMonitorMaxTotalPromptCharacters = 64000

type IntelligenceChannelPrompt struct {
	AccountID int64  `json:"account_id"`
	Prompt    string `json:"prompt"`
}

// This deliberately contains no credentials or account configuration.
type IntelligenceLocalChannel struct {
	AccountID int64  `json:"account_id"`
	Name      string `json:"name"`
	Platform  string `json:"platform"`
	Type      string `json:"type"`
	Status    string `json:"status"`
}

type IntelligenceMonitorLocalChannelsRepository interface {
	ListIntelligenceLocalChannels(context.Context, int64) ([]IntelligenceLocalChannel, error)
}

func (s *IntelligenceMonitorService) ListLocalChannels(ctx context.Context, groupID int64) ([]IntelligenceLocalChannel, error) {
	if groupID <= 0 || s.groups == nil {
		return nil, ErrIntelligenceInvalid.WithMetadata(map[string]string{"field": "group_id"})
	}
	group, err := s.groups.GetByID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if group == nil || (group.Platform != PlatformOpenAI && group.Platform != PlatformComposite) {
		return nil, ErrIntelligenceInvalid.WithMetadata(map[string]string{"field": "group_id", "detail": "choose an OpenAI or composite group"})
	}
	repo, ok := s.repo.(IntelligenceMonitorLocalChannelsRepository)
	if !ok {
		return nil, ErrIntelligenceFilterUnavailable
	}
	channels, err := repo.ListIntelligenceLocalChannels(ctx, groupID)
	if channels == nil && err == nil {
		channels = []IntelligenceLocalChannel{}
	}
	return channels, err
}

func intelligencePlanPrompt(p *IntelligenceMonitorPlan) string {
	if p != nil && p.SourceType == "local_group" && strings.TrimSpace(p.CustomPrompt) != "" {
		return strings.TrimSpace(p.CustomPrompt)
	}
	return IntelligenceMonitorPrompt
}

func (s *IntelligenceMonitorService) configureIntelligencePrompts(ctx context.Context, p, old *IntelligenceMonitorPlan, in IntelligenceMonitorInput) error {
	if p.SourceType != "local_group" {
		p.CustomPrompt, p.ChannelPrompts = "", []IntelligenceChannelPrompt{}
		return nil
	}
	if old != nil && (old.SourceType != "local_group" || !sameUpstreamSupplier(old.GroupID, p.GroupID)) {
		p.CustomPrompt, p.ChannelPrompts = "", nil
	}
	if in.CustomPrompt != nil {
		p.CustomPrompt = strings.TrimSpace(*in.CustomPrompt)
	}
	if in.ChannelPrompts != nil {
		p.ChannelPrompts = append([]IntelligenceChannelPrompt(nil), (*in.ChannelPrompts)...)
	}
	invalid := func(field, detail string) error {
		return ErrIntelligenceInvalid.WithMetadata(map[string]string{"field": field, "detail": detail})
	}
	valid := func(prompt string) bool {
		return utf8.ValidString(prompt) && !strings.ContainsRune(prompt, '\x00') && utf8.RuneCountInString(prompt) <= IntelligenceMonitorMaxPromptCharacters
	}
	if !valid(p.CustomPrompt) {
		return invalid("custom_prompt", "prompt must contain at most 8000 characters and no null characters")
	}
	if len(p.ChannelPrompts) > IntelligenceMonitorMaxChannelPrompts {
		return invalid("channel_prompts", "configure at most 200 channel prompts")
	}
	count := utf8.RuneCountInString(p.CustomPrompt)
	seen := make(map[int64]bool, len(p.ChannelPrompts))
	channels := make([]IntelligenceChannelPrompt, 0, len(p.ChannelPrompts))
	for _, channel := range p.ChannelPrompts {
		channel.Prompt = strings.TrimSpace(channel.Prompt)
		if channel.AccountID <= 0 || seen[channel.AccountID] || !valid(channel.Prompt) {
			return invalid("channel_prompts", "channel IDs must be positive and unique; each prompt allows at most 8000 characters")
		}
		seen[channel.AccountID] = true
		count += utf8.RuneCountInString(channel.Prompt)
		if channel.Prompt != "" {
			channels = append(channels, channel)
		}
	}
	if count > IntelligenceMonitorMaxTotalPromptCharacters {
		return invalid("channel_prompts", "all prompts together allow at most 64000 characters")
	}
	if len(channels) > 0 {
		if p.GroupID == nil || *p.GroupID <= 0 {
			return invalid("group_id", "choose a group before configuring channel prompts")
		}
		available, err := s.ListLocalChannels(ctx, *p.GroupID)
		if err != nil {
			return err
		}
		members := make(map[int64]bool, len(available))
		for _, channel := range available {
			members[channel.AccountID] = true
		}
		for _, channel := range channels {
			if !members[channel.AccountID] {
				return invalid("channel_prompts", "a configured channel no longer belongs to the selected group; remove its prompt before saving")
			}
		}
	}
	p.ChannelPrompts = channels
	return nil
}
