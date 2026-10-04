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
	"time"

	"github.com/stretchr/testify/require"
)

func TestIntelligenceStreamingCompletesWithoutWaitingForConnectionClose(t *testing.T) {
	for _, mode := range []string{MonitorAPIModeResponses, MonitorAPIModeChatCompletions} {
		for _, source := range []string{"external", "upstream", "local_group"} {
			t.Run(source+"_"+mode, func(t *testing.T) {
				reader, writer := io.Pipe()
				defer writer.Close()
				defer reader.Close()
				svc := NewIntelligenceMonitorService(nil, nil, nil, nil, nil, nil, nil)
				calls := 0
				client := &http.Client{Transport: upstreamModelsTransport(func(req *http.Request) (*http.Response, error) {
					calls++
					var body map[string]any
					require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
					require.Equal(t, true, body["stream"])
					require.Contains(t, req.Header.Get("Accept"), "text/event-stream")
					if source == "local_group" {
						require.True(t, req.URL.IsAbs())
						require.Equal(t, "127.0.0.1", req.URL.Hostname())
						require.True(t, intelligencePermitExists(req.Header.Get(intelligenceLocalRequestHeader)))
					} else {
						require.Empty(t, req.Header.Get(intelligenceLocalRequestHeader))
					}
					return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: reader}, nil
				})}
				svc.externalClient, svc.localClient = client, client
				writes := make(chan error, 1)
				go func() {
					_, err := io.WriteString(writer, ": keepalive\n\n")
					if err == nil {
						// The body remains open after the terminal event; the client
						// must finish instead of waiting for EOF or its 15m timeout.
						chunk := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"<html><svg>private-fixture-key</svg></html>\"}\n\n"
						terminal := "event: response.completed\ndata: {\"response\":{\"status\":\"completed\",\"output\":[]}}\n\n"
						if mode == MonitorAPIModeChatCompletions {
							chunk = "data: {\"choices\":[{\"delta\":{\"content\":\"<html><svg>private-fixture-key</svg></html>\"},\"finish_reason\":null}]}\n\n"
							terminal = "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"
						}
						_, err = io.WriteString(writer, chunk+terminal)
						if err == nil && source == "local_group" && mode == MonitorAPIModeChatCompletions {
							// A separate write proves the consumer did not close at stop
							// and cancel the gateway before its billing tail arrived.
							_, err = io.WriteString(writer, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":20}}\n\ndata: [DONE]\n\n")
						}
					}
					writes <- err
				}()
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				stopClose := context.AfterFunc(ctx, func() { _ = reader.CloseWithError(ctx.Err()) })
				defer stopClose()
				status, text, message := svc.generate(ctx, &IntelligenceMonitorRun{SourceType: source, SourceEndpoint: "https://8.8.8.8", APIMode: mode, SourceSnapshot: map[string]any{"local_api_key_id": int64(72)}}, "private-fixture-key")
				require.NoError(t, ctx.Err(), "must finish on terminal event, before cancellation")
				require.Empty(t, message)
				require.Equal(t, 200, *status)
				require.Contains(t, text, "<svg>")
				require.NotContains(t, text, "private-fixture-key")
				require.Equal(t, 1, calls)
				require.NoError(t, <-writes)
			})
		}
	}
}

func TestIntelligenceStreamingRejectsFailureAndInterruptedResults(t *testing.T) {
	for _, tc := range []struct{ name, mode, body, want string }{
		{"failed", "responses", `data: {"type":"response.failed","response":{"error":{"message":"model unavailable","code":"model_not_found"}}}` + "\n\n", "model_not_found"},
		{"incomplete", "responses", `data: {"type":"response.incomplete","response":{"incomplete_details":{"reason":"max_output_tokens"}}}` + "\n\n", "max_output_tokens"},
		{"completed_failed_status", "responses", `data: {"type":"response.completed","response":{"status":"failed","error":{"message":"upstream failure"},"output_text":"<html></html>"}}` + "\n\n", "upstream failure"},
		{"error_without_event_type", "chat_completions", `data: {"error":{"message":"upstream lost connection private-fixture-key"}}` + "\n\ndata: [DONE]\n\n", "upstream lost connection"},
		{"length", "chat_completions", `data: {"choices":[{"delta":{"content":"partial"},"finish_reason":"length"}]}` + "\n\n", "truncated"},
		{"no_terminal", "responses", `data: {"type":"response.output_text.delta","delta":"<html></html>"}` + "\n\n", "before completion"},
		{"done_without_responses_terminal", "responses", "data: [DONE]\n\n", "before completion"},
		{"done_without_cc_stop", "chat_completions", "data: [DONE]\n\n", "before completion"},
		{"bad_json", "responses", "data: {broken\n\n", "invalid JSON"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prefix := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n"
			_, message := readIntelligenceGenerationBody(strings.NewReader(prefix+tc.body), tc.mode, "text/event-stream", "private-fixture-key", false)
			require.Contains(t, message, tc.want)
			require.NotContains(t, message, "private-fixture-key")
		})
	}
}

func TestIntelligenceStreamingSupportsJSONFallbackMultilineDataAndFinalText(t *testing.T) {
	for _, tc := range []struct{ name, contentType, mode, body, want string }{
		{"json_fallback", "application/json", "responses", `{"status":"completed","output_text":"full HTML","error":null}`, "full HTML"},
		{"stale_sse_header", "text/event-stream", "responses", `{"status":"completed","output_text":"full HTML"}`, "full HTML"},
		{"cc_json_fallback", "application/json", "chat_completions", `{"choices":[{"message":{"content":"cc HTML"},"finish_reason":"stop"}]}`, "cc HTML"},
		{"missing_sse_header", "application/json", "responses", ": heartbeat\n\ndata: {\"type\":\"response.completed\",\"response\":{\"output_text\":\"final HTML\"}}\n\n", "final HTML"},
		{"multiline_event_data", "text/event-stream", "responses", "event: response.completed\ndata: {\"response\":\ndata: {\"output_text\":\"multiline HTML\",\"status\":\"completed\"}}\n\n", "multiline HTML"},
		{"authoritative_final", "text/event-stream", "responses", "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"output_text\":\"final HTML\"}}\n\n", "final HTML"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text, message := readIntelligenceGenerationBody(strings.NewReader(tc.body), tc.mode, tc.contentType, "", false)
			require.Empty(t, message)
			require.Equal(t, tc.want, text)
		})
	}
}

func TestIntelligenceStreamingBoundsWireBytesIncludingCommentsAndWhitespace(t *testing.T) {
	for _, body := range []string{
		strings.Repeat(" ", intelligenceResponseMaxBytes+1),
		strings.Repeat(": heartbeat\n\n", intelligenceResponseMaxBytes/12+1),
		`{"output_text":"` + strings.Repeat("x", intelligenceResponseMaxBytes) + `"}`,
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"" + strings.Repeat("x", intelligenceResponseMaxBytes) + "\"}\n\n",
	} {
		text, message := readIntelligenceGenerationBody(strings.NewReader(body), "responses", "text/event-stream", "", false)
		require.Empty(t, text)
		require.Contains(t, message, "4 MiB")
	}
}

func TestIntelligenceGenerationDoesNotRetryHTTPOrStreamFailure(t *testing.T) {
	for _, status := range []int{502, 200} {
		calls := 0
		svc := NewIntelligenceMonitorService(nil, nil, nil, nil, nil, nil, nil)
		svc.externalClient = &http.Client{Transport: upstreamModelsTransport(func(req *http.Request) (*http.Response, error) {
			calls++
			body := `{"error":{"message":"upstream unavailable","type":"server_error"}}`
			if status == 200 {
				body = "data: " + body + "\n\n"
			}
			return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"text/event-stream"}, "X-Request-Id": {"req-fixture"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
		})}
		gotStatus, _, message := svc.generate(context.Background(), &IntelligenceMonitorRun{SourceType: "external", SourceEndpoint: "https://8.8.8.8", APIMode: "responses"}, "private-fixture-key")
		require.Equal(t, status, *gotStatus)
		require.Contains(t, message, "upstream unavailable")
		require.Equal(t, 1, calls)
	}
}

func TestIntelligenceLocalStreamingKeepsTrustedDeadline(t *testing.T) {
	svc := NewIntelligenceMonitorService(nil, nil, nil, nil, nil, nil, nil)
	svc.localClient = &http.Client{Transport: upstreamModelsTransport(func(req *http.Request) (*http.Response, error) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
		require.Equal(t, true, body["stream"])
		require.Contains(t, req.Header.Get("Accept"), "text/event-stream")
		require.NotEmpty(t, req.Header.Get(intelligenceLocalRequestHeader))
		deadline, ok := req.Context().Deadline()
		require.True(t, ok)
		require.InDelta(t, 900, time.Until(deadline).Seconds(), 2)
		bodySSE := "data: {\"type\":\"response.completed\",\"response\":{\"output_text\":\"<html></html>\",\"status\":\"completed\"}}\n\n"
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(bodySSE))}, nil
	})}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	_, text, message := svc.generate(ctx, &IntelligenceMonitorRun{SourceType: "local_group", APIMode: "responses", SourceSnapshot: map[string]any{"local_api_key_id": int64(72)}}, "private-fixture-key")
	require.Empty(t, message)
	require.Equal(t, "<html></html>", text)
}

func TestIntelligenceStreamingReadFailurePreservesFailure(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	go func() {
		_, _ = io.WriteString(writer, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"<html></html>\"}\n\n")
		_ = writer.CloseWithError(errors.New("interrupted private-fixture-key"))
	}()
	text, message := readIntelligenceGenerationBody(reader, "responses", "text/event-stream", "private-fixture-key", false)
	require.Equal(t, "<html></html>", text)
	require.Equal(t, "generation response was interrupted", message)
}

func TestIntelligenceLocalChatStreamingRequiresCompleteBillingTail(t *testing.T) {
	stop := "data: {\"choices\":[{\"delta\":{\"content\":\"21\"},\"finish_reason\":\"stop\"}]}\n\n"
	for _, tc := range []struct{ name, body, want string }{
		{"completed", stop + "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":20}}\n\ndata: [DONE]\n\n", ""},
		{"missing_done", stop, "before completion"},
		{"missing_stop", "data: [DONE]\n\n", "before completion"},
		{"error_after_stop", stop + "data: {\"error\":{\"message\":\"upstream interrupted private-fixture-key\"}}\n\ndata: [DONE]\n\n", "upstream interrupted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text, message := readIntelligenceGenerationBody(strings.NewReader(tc.body), MonitorAPIModeChatCompletions, "text/event-stream", "private-fixture-key", true)
			if tc.want == "" {
				require.Empty(t, message)
				require.Equal(t, "21", text)
			} else {
				require.Contains(t, message, tc.want)
				require.NotContains(t, message, "private-fixture-key")
			}
		})
	}
}

func TestIntelligenceLocalChatStreamingDrainsUsageOnTruncatedOutput(t *testing.T) {
	for _, finish := range []string{"length", "content_filter"} {
		t.Run(finish, func(t *testing.T) {
			reader, writer := io.Pipe()
			defer reader.Close()
			defer writer.Close()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			stopClose := context.AfterFunc(ctx, func() { _ = reader.CloseWithError(ctx.Err()) })
			defer stopClose()
			writes := make(chan error, 1)
			go func() {
				_, err := io.WriteString(writer, "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"},\"finish_reason\":\""+finish+"\"}]}\n\n")
				if err == nil {
					_, err = io.WriteString(writer, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":20}}\n\ndata: [DONE]\n\n")
				}
				writes <- err
			}()
			text, message := readIntelligenceGenerationBody(reader, MonitorAPIModeChatCompletions, "text/event-stream", "", true)
			_ = reader.Close()
			require.NoError(t, <-writes, "must consume the usage tail before closing the response")
			require.NoError(t, ctx.Err())
			require.Equal(t, "partial", text)
			require.Contains(t, message, "truncated or filtered")
		})
	}
}
