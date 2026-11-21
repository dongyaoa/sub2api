//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/require"
)

func TestRunCheckForModel_OpenAIResponsesStream(t *testing.T) {
	client := &http.Client{Transport: upstreamModelsTransport(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/relay/v1/responses", r.URL.Path)
		require.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		require.Equal(t, "text/event-stream", r.Header.Get("Accept"))
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, true, body["stream"])
		require.Equal(t, "gpt-test", body["model"])
		require.NotEmpty(t, body["instructions"])
		require.NotEmpty(t, body["input"])

		answer := answerFromOpenAIRequest(body)
		stream := fmt.Sprintf(": keepalive\r\n\r\ndata: {\"type\":\"response.output_text.delta\",\"delta\":%q}\r\n\r\n", answer)
		stream += "event: response.completed\r\ndata: {\"response\":{\"status\":\"completed\",\r\ndata: \"usage\":{\"input_tokens\":13,\"output_tokens\":2,\"input_tokens_details\":{\"cached_tokens\":3}}}}\r\n\r\n"
		// Reading beyond response.completed would wait for a keepalive connection to close.
		reader := io.MultiReader(strings.NewReader(stream), iotest.ErrReader(errors.New("read past terminal event")))
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream; charset=utf-8"}}, Body: io.NopCloser(reader)}, nil
	})}

	res := runCheckForModel(context.Background(), MonitorProviderOpenAI, "https://relay.example/relay/v1", "test-key", "gpt-test",
		&CheckOptions{APIMode: MonitorAPIModeResponses, StreamResponses: true, HTTPClient: client})
	require.Equal(t, MonitorStatusOperational, res.Status, res.Message)
	require.Equal(t, 200, *res.HTTPStatus)
	require.Equal(t, &UsageTokens{InputTokens: 10, OutputTokens: 2, CacheReadTokens: 3}, res.Usage)
}

func TestRunCheckForModel_OpenAIResponsesStreamFailures(t *testing.T) {
	const delta = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"1\"}\n\n"
	for _, tc := range []struct {
		name, mediaType, wire, status, message string
		httpStatus                             int
	}{
		{"plain JSON", "application/json", "{\"output_text\":\"1\"}", MonitorStatusError, "text/event-stream", 200},
		{"JSON disguised as SSE", "text/event-stream", "{\"output_text\":\"1\"}", MonitorStatusError, "ended before completion", 200},
		{"truncated", "text/event-stream", delta, MonitorStatusError, "ended before completion", 200},
		{"early DONE", "text/event-stream", delta + "data: [DONE]\n\n", MonitorStatusError, "ended before completion", 200},
		{"event error", "text/event-stream", delta + "data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"message\":\"rejected private-fixture-key\"}}}\n\n", MonitorStatusError, "rejected", 200},
		{"incomplete", "text/event-stream", delta + "data: {\"type\":\"response.incomplete\"}\n\n", MonitorStatusError, "incomplete", 200},
		{"canceled", "text/event-stream", delta + "data: {\"type\":\"response.canceled\"}\n\n", MonitorStatusError, "incomplete", 200},
		{"bad event", "text/event-stream", "data: {bad}\n\n", MonitorStatusError, "invalid JSON", 200},
		{"wire limit", "text/event-stream", strings.Repeat(": heartbeat\n\n", monitorResponseMaxBytes/12+1), MonitorStatusError, "exceeded", 200},
		{"wrong answer", "text/event-stream", "data: {\"type\":\"response.completed\",\"response\":{\"output_text\":\"wrong answer\"}}\n\n", MonitorStatusFailed, "challenge mismatch", 200},
		{"HTTP error", "application/json", "{\"error\":{\"message\":\"rate limit\"}}", MonitorStatusError, "rate limit", 429},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: upstreamModelsTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.httpStatus, Header: http.Header{"Content-Type": {tc.mediaType}}, Body: io.NopCloser(strings.NewReader(tc.wire))}, nil
			})}
			res := runCheckForModel(context.Background(), MonitorProviderOpenAI, "https://relay.example", "private-fixture-key", "gpt-test",
				&CheckOptions{APIMode: MonitorAPIModeResponses, StreamResponses: true, HTTPClient: client})
			require.Equal(t, tc.httpStatus, *res.HTTPStatus)
			require.Equal(t, tc.status, res.Status)
			require.Contains(t, res.Message, tc.message)
			require.NotContains(t, res.Message, "private-fixture-key")
		})
	}
}
