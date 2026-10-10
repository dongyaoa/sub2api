//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
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

func TestIntelligencePromptsUpstreamAndExternalPersistAndExecute(t *testing.T) {
	for _, source := range []string{"upstream", "external"} {
		for _, mode := range []string{MonitorAPIModeResponses, MonitorAPIModeChatCompletions} {
			t.Run(source+"/"+mode, func(t *testing.T) {
				ctx := context.Background()
				repo := &intelligenceTestRepository{}
				target := &UpstreamTarget{ID: 7, Name: "API group", Provider: MonitorProviderOpenAI, Endpoint: "https://8.8.8.8", APIKeyEncrypted: "encrypted:secret"}
				svc := NewIntelligenceMonitorService(repo, upstreamTestEncryptor{}, &upstreamTestRepo{target: target}, nil, nil, nil, nil)
				name, key, custom := "Artwork", "secret", "  创建 HTML，用 SVG 画一只红色鹈鹕。  "
				channels := []IntelligenceChannelPrompt{{AccountID: 99, Prompt: "must not apply outside a local group"}}
				in := IntelligenceMonitorInput{Name: &name, SourceType: &source, Endpoint: &target.Endpoint, APIKey: &key, APIMode: &mode, UpstreamTargetID: json.RawMessage(`7`), CustomPrompt: &custom, ChannelPrompts: &channels}
				plan, err := svc.SavePlan(ctx, 0, 1, in)
				require.NoError(t, err)
				require.Equal(t, strings.TrimSpace(custom), plan.CustomPrompt)
				require.Equal(t, plan.CustomPrompt, plan.Prompt)
				require.Empty(t, plan.ChannelPrompts)
				plan.ID, plan.CandyEnabled = 3, true
				repo.plan = plan
				run, err := svc.Enqueue(ctx, plan.ID)
				require.NoError(t, err)
				require.Equal(t, plan.CustomPrompt, run.Prompt)
				require.NotContains(t, run.SourceSnapshot, "channel_prompts")
				// Editing a plan after queueing must not change the queued request.
				changed := "changed after enqueue"
				_, err = svc.SavePlan(ctx, plan.ID, 1, IntelligenceMonitorInput{CustomPrompt: &changed})
				require.NoError(t, err)
				svc.externalClient = &http.Client{Transport: upstreamModelsTransport(func(request *http.Request) (*http.Response, error) {
					body, err := io.ReadAll(request.Body)
					require.NoError(t, err)
					path := "input"
					response := `{"status":"completed","output_text":"<html><svg></svg></html>"}`
					if mode == MonitorAPIModeChatCompletions {
						path = "messages.0.content"
						response = `{"choices":[{"message":{"content":"<html><svg></svg></html>"},"finish_reason":"stop"}]}`
					}
					require.Equal(t, strings.TrimSpace(custom), gjson.GetBytes(body, path).String())
					require.True(t, gjson.GetBytes(body, "stream").Bool())
					return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(response))}, nil
				})}
				status, artwork, message := svc.generate(ctx, run, key)
				require.Empty(t, message)
				require.Equal(t, http.StatusOK, *status)
				require.Contains(t, artwork, "<svg>")
				candy, err := svc.EnqueueCandy(ctx, plan.ID)
				require.NoError(t, err)
				require.Equal(t, IntelligenceMonitorCandyPrompt, candy.Prompt)
				updated, err := svc.SavePlan(ctx, plan.ID, 1, IntelligenceMonitorInput{Notes: &name})
				require.NoError(t, err)
				require.Equal(t, strings.TrimSpace(custom), updated.CustomPrompt, "omitting the field preserves the prompt")
				blank := ""
				updated, err = svc.SavePlan(ctx, plan.ID, 1, IntelligenceMonitorInput{CustomPrompt: &blank})
				require.NoError(t, err)
				require.Equal(t, IntelligenceMonitorPrompt, updated.Prompt)
			})
		}
	}
}

func TestIntelligencePromptsNonLocalValidationAndFallback(t *testing.T) {
	svc := &IntelligenceMonitorService{}
	for _, source := range []string{"upstream", "external"} {
		for _, prompt := range []string{strings.Repeat("鹈", 8001), "draw\x00pelican", string([]byte{0xff})} {
			plan := &IntelligenceMonitorPlan{SourceType: source}
			err := svc.configureIntelligencePrompts(context.Background(), plan, nil, IntelligenceMonitorInput{CustomPrompt: &prompt})
			require.ErrorIs(t, err, ErrIntelligenceInvalid)
			_, _, valid := intelligenceTestRequestDefinition(&IntelligenceMonitorRun{SourceType: source, Prompt: prompt})
			require.False(t, valid)
		}
		prompt, _, valid := intelligenceTestRequestDefinition(&IntelligenceMonitorRun{SourceType: source, Prompt: " \n "})
		require.True(t, valid)
		require.Equal(t, IntelligenceMonitorPrompt, prompt)
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
