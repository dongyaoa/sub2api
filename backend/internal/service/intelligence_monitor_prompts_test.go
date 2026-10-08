//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type intelligencePromptTestRepo struct {
	*intelligenceLocalMonitorRepo
	channels []IntelligenceLocalChannel
	err      error
	groupID  int64
}

func (r *intelligencePromptTestRepo) ListIntelligenceLocalChannels(_ context.Context, groupID int64) ([]IntelligenceLocalChannel, error) {
	r.groupID = groupID
	return r.channels, r.err
}

func intelligencePromptFixture() (*IntelligenceMonitorService, *intelligencePromptTestRepo) {
	svc, base, _ := newLocalIntelligenceKeyTestService()
	repo := &intelligencePromptTestRepo{intelligenceLocalMonitorRepo: base, channels: []IntelligenceLocalChannel{{AccountID: 11, Name: "First", Status: StatusActive}, {AccountID: 12, Name: "Paused", Status: StatusDisabled}}}
	svc.repo = repo
	return svc, repo
}

func TestIntelligencePromptsSaveAndSnapshotPreserveActualInput(t *testing.T) {
	svc, repo := intelligencePromptFixture()
	custom := "  创建一个 HTML：画一只戴帽子的鹈鹕。\n  "
	channels := []IntelligenceChannelPrompt{{AccountID: 11, Prompt: "  创建一个 HTML：画红色自行车。  "}, {AccountID: 12, Prompt: ""}}
	in := localIntelligenceInput()
	in.CustomPrompt, in.ChannelPrompts = &custom, &channels
	plan, err := svc.SavePlan(context.Background(), 0, 7, in)
	require.NoError(t, err)
	require.Equal(t, strings.TrimSpace(custom), plan.Prompt)
	require.Equal(t, plan.Prompt, plan.CustomPrompt)
	require.Equal(t, []IntelligenceChannelPrompt{{AccountID: 11, Prompt: "创建一个 HTML：画红色自行车。"}}, plan.ChannelPrompts)
	require.Equal(t, int64(8), repo.groupID)
	plan.ID, plan.CandyEnabled = 3, true
	repo.plan = plan
	run, err := svc.Enqueue(context.Background(), 3)
	require.NoError(t, err)
	require.Equal(t, plan.Prompt, run.Prompt)
	require.Equal(t, plan.ChannelPrompts, run.SourceSnapshot["channel_prompts"])
	plan.ChannelPrompts[0].Prompt = "changed after enqueue"
	require.Equal(t, "创建一个 HTML：画红色自行车。", run.SourceSnapshot["channel_prompts"].([]IntelligenceChannelPrompt)[0].Prompt)
	candy, err := svc.EnqueueCandy(context.Background(), 3)
	require.NoError(t, err)
	require.Equal(t, IntelligenceMonitorCandyPrompt, candy.Prompt)
	require.NotContains(t, candy.SourceSnapshot, "channel_prompts")

	// Omitting optional fields and toggling scheduling must preserve settings.
	paused := false
	_, err = svc.SavePlan(context.Background(), 3, 7, IntelligenceMonitorInput{Enabled: &paused})
	require.NoError(t, err)
	require.Equal(t, plan.CustomPrompt, repo.saved.CustomPrompt)
	require.Equal(t, plan.ChannelPrompts, repo.saved.ChannelPrompts)
	blank, empty := "", []IntelligenceChannelPrompt{}
	cleared, err := svc.SavePlan(context.Background(), 3, 7, IntelligenceMonitorInput{CustomPrompt: &blank, ChannelPrompts: &empty})
	require.NoError(t, err)
	require.Equal(t, IntelligenceMonitorPrompt, cleared.Prompt)
	require.Empty(t, cleared.CustomPrompt)
	require.Empty(t, cleared.ChannelPrompts)
}

func TestIntelligencePromptsValidateLimitsAndCurrentMembership(t *testing.T) {
	for _, tc := range []struct {
		name     string
		custom   string
		channels []IntelligenceChannelPrompt
	}{
		{name: "custom too long", custom: strings.Repeat("鹈", 8001)},
		{name: "null character", custom: "draw\x00pelican"},
		{name: "channel too long", channels: []IntelligenceChannelPrompt{{AccountID: 11, Prompt: strings.Repeat("a", 8001)}}},
		{name: "foreign channel", channels: []IntelligenceChannelPrompt{{AccountID: 99, Prompt: "draw"}}},
		{name: "duplicate channel", channels: []IntelligenceChannelPrompt{{AccountID: 11, Prompt: "draw"}, {AccountID: 11, Prompt: "other"}}},
		{name: "negative channel", channels: []IntelligenceChannelPrompt{{AccountID: -1, Prompt: "draw"}}},
		{name: "too many overrides", channels: make([]IntelligenceChannelPrompt, 201)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo := intelligencePromptFixture()
			in := localIntelligenceInput()
			in.CustomPrompt, in.ChannelPrompts = &tc.custom, &tc.channels
			_, err := svc.SavePlan(context.Background(), 0, 7, in)
			require.ErrorIs(t, err, ErrIntelligenceInvalid)
			require.Nil(t, repo.saved)
		})
	}
	svc, repo := intelligencePromptFixture()
	custom := strings.Repeat("鹈", 8000)
	in := localIntelligenceInput()
	in.CustomPrompt = &custom
	_, err := svc.SavePlan(context.Background(), 0, 7, in)
	require.NoError(t, err, "character limit must not count UTF-8 bytes")
	channels := make([]IntelligenceChannelPrompt, 8)
	for i := range channels {
		channels[i] = IntelligenceChannelPrompt{AccountID: int64(i + 1), Prompt: strings.Repeat("鹈", 8000)}
	}
	in.ChannelPrompts = &channels
	repo.saved = nil
	_, err = svc.SavePlan(context.Background(), 0, 7, in)
	require.ErrorIs(t, err, ErrIntelligenceInvalid, "total limit includes the plan prompt")
	require.Nil(t, repo.saved)
}

func TestIntelligencePromptsClearOldSourceAndGroupSettings(t *testing.T) {
	svc, _ := intelligencePromptFixture()
	group, other := int64(8), int64(9)
	old := &IntelligenceMonitorPlan{SourceType: "local_group", GroupID: &group, CustomPrompt: "old prompt", ChannelPrompts: []IntelligenceChannelPrompt{{AccountID: 11, Prompt: "old channel"}}}
	for _, next := range []*IntelligenceMonitorPlan{
		{SourceType: "local_group", GroupID: &other, CustomPrompt: old.CustomPrompt, ChannelPrompts: old.ChannelPrompts},
		{SourceType: "external", CustomPrompt: old.CustomPrompt, ChannelPrompts: old.ChannelPrompts},
	} {
		require.NoError(t, svc.configureIntelligencePrompts(context.Background(), next, old, IntelligenceMonitorInput{}))
		require.Empty(t, next.CustomPrompt)
		require.Empty(t, next.ChannelPrompts)
	}
}

func TestIntelligenceLocalChannelListPreservesDisabledAndHidesCredentials(t *testing.T) {
	svc, repo := intelligencePromptFixture()
	channels, err := svc.ListLocalChannels(context.Background(), 8)
	require.NoError(t, err)
	require.Equal(t, repo.channels, channels)
	data, err := json.Marshal(channels)
	require.NoError(t, err)
	require.NotContains(t, string(data), "credentials")
	require.Contains(t, string(data), "disabled")
	_, err = svc.ListLocalChannels(context.Background(), 0)
	require.ErrorIs(t, err, ErrIntelligenceInvalid)
	repo.err = errors.New("database unavailable")
	_, err = svc.ListLocalChannels(context.Background(), 8)
	require.ErrorIs(t, err, repo.err)
	// A group lookup failure does not block monitors that use only their prompt.
	in := localIntelligenceInput()
	custom := "draw a pelican in HTML"
	in.CustomPrompt = &custom
	_, err = svc.SavePlan(context.Background(), 0, 7, in)
	require.NoError(t, err)
}
