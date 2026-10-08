//go:build unit

package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestIntelligenceChannelPromptFollowsActualAttemptAndPersistsPrompt(t *testing.T) {
	for _, mode := range []string{MonitorAPIModeResponses, MonitorAPIModeChatCompletions} {
		for _, finalID := range []int64{20, 30} {
			t.Run(mode+"/"+map[int64]string{20: "channel", 30: "monitor_fallback"}[finalID], func(t *testing.T) {
				worker, cancel := context.WithTimeout(context.Background(), time.Minute)
				defer cancel()
				key := &APIKey{ID: 72, Key: "monitor-secret"}
				run := &IntelligenceMonitorRun{SourceType: "local_group", TestKind: IntelligenceMonitorTestPelican, Prompt: "绘制蓝色鹈鹕的 HTML 动画", APIMode: mode,
					SourceSnapshot: map[string]any{"local_api_key_id": key.ID, "channel_prompts": []IntelligenceChannelPrompt{
						{AccountID: 10, Prompt: "绘制红色鹈鹕的 HTML 动画"}, {AccountID: 20, Prompt: "绘制绿色鹈鹕的 HTML 动画"},
					}}}
				// Model queue persistence: another process sees generic JSON values.
				encoded, err := json.Marshal(run.SourceSnapshot)
				require.NoError(t, err)
				require.NoError(t, json.Unmarshal(encoded, &run.SourceSnapshot))
				want := run.Prompt
				if finalID == 20 {
					want = "绘制绿色鹈鹕的 HTML 动画"
				}
				svc := &IntelligenceMonitorService{localEndpoint: "http://127.0.0.1:8081"}
				svc.localClient = &http.Client{Transport: upstreamModelsTransport(func(request *http.Request) (*http.Response, error) {
					body, err := io.ReadAll(request.Body)
					require.NoError(t, err)
					original := string(body)
					inbound := request.Clone(context.Background())
					inbound.RemoteAddr = "127.0.0.1:54321"
					bound, release := BindIntelligenceLocalRequest(inbound, key)
					defer release()
					for _, id := range []int64{10, finalID} {
						account := &Account{ID: id}
						RecordIntelligenceExecutionAccount(bound, account)
						updated, err := ApplyIntelligenceChannelPrompt(bound, account, body, mode)
						require.NoError(t, err)
						path := "input"
						if mode == MonitorAPIModeChatCompletions {
							path = "messages.0.content"
						}
						prompt := want
						if id == 10 {
							prompt = "绘制红色鹈鹕的 HTML 动画"
						}
						require.Equal(t, prompt, gjson.GetBytes(updated, path).String())
						require.True(t, gjson.GetBytes(updated, "stream").Bool())
						require.Equal(t, IntelligenceMonitorModel, gjson.GetBytes(updated, "model").String())
						require.Equal(t, original, string(body), "a retry must not mutate the shared original body")
					}
					response := `{"status":"completed","output_text":"<html><body>pelican</body></html>"}`
					if mode == MonitorAPIModeChatCompletions {
						response = `{"choices":[{"message":{"content":"<html><body>pelican</body></html>"},"finish_reason":"stop"}]}`
					}
					return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(response)), Request: request}, nil
				})}
				status, text, message := svc.generate(worker, run, key.Key)
				require.Empty(t, message)
				require.Equal(t, 200, *status)
				require.Contains(t, text, "pelican")
				require.Equal(t, want, run.Prompt, "history must record the prompt of the final actual channel")
				require.Equal(t, finalID, run.SourceSnapshot["execution_account_id"])
			})
		}
	}
}

func TestIntelligenceChannelPromptOnlyAppliesToAuthenticatedLocalArtwork(t *testing.T) {
	worker, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	worker, trace := newIntelligenceExecutionTrace(worker)
	run := &IntelligenceMonitorRun{SourceType: "local_group", TestKind: IntelligenceMonitorTestPelican}
	require.NoError(t, trace.configurePrompts(run, IntelligenceMonitorPrompt))
	account := &Account{ID: 20}
	body := []byte(`{"model":"gpt-6-astra","input":"original","stream":true}`)
	for _, ctx := range []context.Context{context.Background(), worker, context.WithValue(context.Background(), intelligenceGenerationContextKey{}, true)} {
		got, err := ApplyIntelligenceChannelPrompt(ctx, account, body, MonitorAPIModeResponses)
		require.NoError(t, err)
		require.Equal(t, body, got)
	}
	key := &APIKey{ID: 72, Key: "monitor-secret"}
	request := newIntelligenceLocalPermitRequest(t, worker, key)
	replay := request.Clone(context.Background())
	bound, release := BindIntelligenceLocalRequest(request, key)
	defer release()
	got, err := ApplyIntelligenceChannelPrompt(bound, account, body, MonitorAPIModeResponses)
	require.NoError(t, err)
	require.Equal(t, IntelligenceMonitorPrompt, gjson.GetBytes(got, "input").String())
	replayed, cleanup := BindIntelligenceLocalRequest(replay, key)
	defer cleanup()
	got, err = ApplyIntelligenceChannelPrompt(replayed, account, body, MonitorAPIModeResponses)
	require.NoError(t, err)
	require.Equal(t, body, got)
}

func TestIntelligenceChannelPromptLeavesCandyAndNonLocalDefinitionsFixed(t *testing.T) {
	for _, source := range []string{"local_group", "external", "upstream", "openai_oauth"} {
		run := &IntelligenceMonitorRun{SourceType: source, TestKind: IntelligenceMonitorTestCandy, Prompt: "custom artwork", SourceSnapshot: map[string]any{"channel_prompts": "ignored for candy"}}
		prompt, _, valid := intelligenceTestRequestDefinition(run)
		require.True(t, valid)
		require.Equal(t, IntelligenceMonitorCandyPrompt, prompt)
		trace := &intelligenceExecutionTrace{}
		require.NoError(t, trace.configurePrompts(run, prompt))
		require.Nil(t, trace.promptConfig)
		if source != "local_group" {
			run.TestKind = IntelligenceMonitorTestPelican
			prompt, _, valid = intelligenceTestRequestDefinition(run)
			require.True(t, valid)
			require.Equal(t, IntelligenceMonitorPrompt, prompt)
		}
	}
}

func TestIntelligenceChannelPromptRejectsInvalidSnapshotsBeforeSending(t *testing.T) {
	for _, raw := range []any{
		"malformed",
		[]IntelligenceChannelPrompt{{AccountID: -1, Prompt: "draw"}},
		[]IntelligenceChannelPrompt{{AccountID: 1, Prompt: strings.Repeat("字", IntelligenceMonitorMaxPromptCharacters+1)}},
		[]IntelligenceChannelPrompt{{AccountID: 1, Prompt: "one"}, {AccountID: 1, Prompt: "two"}},
	} {
		trace := &intelligenceExecutionTrace{}
		run := &IntelligenceMonitorRun{SourceType: "local_group", SourceSnapshot: map[string]any{"channel_prompts": raw}}
		require.Error(t, trace.configurePrompts(run, IntelligenceMonitorPrompt))
	}
	for _, prompt := range []string{"", " \n "} {
		value, _, valid := intelligenceTestRequestDefinition(&IntelligenceMonitorRun{SourceType: "local_group", Prompt: prompt})
		require.True(t, valid)
		require.Equal(t, IntelligenceMonitorPrompt, value)
	}
	_, _, valid := intelligenceTestRequestDefinition(&IntelligenceMonitorRun{SourceType: "local_group", Prompt: strings.Repeat("字", IntelligenceMonitorMaxPromptCharacters+1)})
	require.False(t, valid)
}
