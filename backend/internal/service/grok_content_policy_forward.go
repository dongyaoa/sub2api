package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// A refusal is a billable result even when forwarding returns an error. The
// existing partial-result billing path persists it without retrying the call.
func (s *OpenAIGatewayService) grokContentPolicyErrorResponse(resp *http.Response, c *gin.Context, account *Account, body []byte, model string) (*OpenAIForwardResult, error) {
	message := extractUpstreamErrorMessage(body)
	setOpsUpstreamError(c, resp.StatusCode, message, truncateString(string(body), 2048))
	appendOpsUpstreamHTTPError(c, account, resp, message)
	writeOpenAIPassthroughResponseHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	MarkResponseCommitted(c)
	c.Data(resp.StatusCode, contentType, body)
	ctx := context.Background()
	if c.Request != nil {
		ctx = c.Request.Context()
	}
	return &OpenAIForwardResult{
		RequestID:       resolveUsageBillingRequestID(ctx, firstNonEmpty(resp.Header.Get("x-request-id"), resp.Header.Get("xai-request-id"))),
		UpstreamHeaders: resp.Header.Clone(), ResponseHeaders: resp.Header.Clone(),
		Model: model, UpstreamModel: model, GrokContentPolicyRejection: true,
	}, fmt.Errorf("grok content policy rejection: %s", message)
}

// Observe bytes as the normal response parser consumes them: no extra read,
// changed client payload, or wait for EOF on a live SSE connection is needed.
func (s *OpenAIGatewayService) observeGrokContentPolicyResponse(resp *http.Response, account *Account, model string, stream bool, started time.Time, result **OpenAIForwardResult, resultErr *error) func() {
	if account == nil || !account.IsGrok() || resp == nil || resp.Body == nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return func() {}
	}
	observer := &grokContentPolicyBody{ReadCloser: resp.Body, stream: strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream"), limit: resolveUpstreamResponseReadLimit(s.cfg), tails: make(map[string]string)}
	resp.Body = observer
	return func() {
		if !observer.rejected() {
			return
		}
		if *result == nil {
			*result = &OpenAIForwardResult{Model: model, UpstreamModel: model, Stream: stream, Duration: time.Since(started)}
		}
		(*result).GrokContentPolicyRejection = true
		if (*result).RequestID == "" {
			(*result).RequestID = resolveUsageBillingRequestID(context.Background(), firstNonEmpty(resp.Header.Get("x-request-id"), resp.Header.Get("xai-request-id")))
		}
		(*result).UpstreamHeaders = resp.Header.Clone()
		if *resultErr != nil {
			// The refusal already consumed a billable upstream call. Never let an
			// empty/failed terminal event or protocol fallback trigger a retry.
			*resultErr = fmt.Errorf("grok content policy rejection: %v", *resultErr)
		}
	}
}

type grokContentPolicyBody struct {
	io.ReadCloser
	mu        sync.Mutex
	stream    bool
	decided   bool
	limit     int64
	buffer    []byte
	oversized bool
	event     string
	tails     map[string]string
	hit       bool
	excluded  bool
}

// Capture the observer before a protocol adapter wraps the body again.
func grokContentPolicyResponseCheck(resp *http.Response) func() bool {
	if resp != nil {
		if observer, ok := resp.Body.(*grokContentPolicyBody); ok {
			return observer.rejected
		}
	}
	return func() bool { return false }
}

func (b *grokContentPolicyBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.mu.Lock()
	defer b.mu.Unlock()
	if n > 0 {
		b.observe(p[:n])
	}
	if err == io.EOF && b.stream && len(b.buffer) > 0 {
		b.line(b.buffer)
		b.buffer = nil
	}
	return n, err
}

func (b *grokContentPolicyBody) observe(data []byte) {
	if !b.decided {
		// Some relays omit the SSE Content-Type, including on non-streaming
		// client requests. Detect the upstream framing before buffering JSON.
		b.buffer = append(b.buffer, data...)
		prefix := bytes.TrimSpace(b.buffer)
		if len(prefix) < 6 && int64(len(b.buffer)) <= b.limit {
			return
		}
		b.decided = true
		b.stream = b.stream || bytes.HasPrefix(prefix, []byte("data:")) || bytes.HasPrefix(prefix, []byte("event:")) || bytes.HasPrefix(prefix, []byte(":"))
		data = b.buffer
		b.buffer = nil
	}
	if !b.stream {
		if int64(len(b.buffer)+len(data)) > b.limit {
			b.oversized = true
			b.buffer = nil
		}
		if !b.oversized {
			b.buffer = append(b.buffer, data...)
		}
		return
	}
	for len(data) > 0 {
		index := bytes.IndexByte(data, '\n')
		part := data
		if index >= 0 {
			part = data[:index]
		}
		if int64(len(b.buffer)+len(part)) > b.limit {
			b.oversized = true
			b.buffer = nil
		}
		if !b.oversized {
			b.buffer = append(b.buffer, part...)
		}
		if index < 0 {
			return
		}
		if !b.oversized {
			b.line(b.buffer)
		}
		b.buffer = nil
		b.oversized = false
		data = data[index+1:]
	}
}

func (b *grokContentPolicyBody) line(line []byte) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		b.event = ""
		return
	}
	if value, ok := bytes.CutPrefix(line, []byte("event:")); ok {
		b.event = strings.TrimSpace(string(value))
		return
	}
	data, ok := bytes.CutPrefix(line, []byte("data:"))
	if !ok || !gjson.ValidBytes(data) {
		return
	}
	if b.event != "" && !gjson.GetBytes(data, "type").Exists() {
		data, _ = sjson.SetBytes(data, "type", b.event)
	}
	root := gjson.ParseBytes(data)
	b.excluded = b.excluded || grokContentPolicyResponseFailure(data)
	if choices := root.Get("choices"); choices.IsArray() {
		choices.ForEach(func(_, choice gjson.Result) bool {
			// Keep choices separate: fragments from independent completions
			// must not combine into a sentence the model never produced.
			payload := []byte(`{"choices":[` + choice.Raw + `]}`)
			b.text("choice:"+choice.Get("index").String(), grokContentPolicyOutputText(payload), choice.Get("delta").Exists())
			return true
		})
		return
	}
	kind := root.Get("type").String()
	key := kind + ":" + root.Get("output_index").String() + ":" + root.Get("content_index").String() + ":" + root.Get("item_id").String() + ":" + root.Get("index").String()
	b.text(key, grokContentPolicyOutputText(data), strings.HasSuffix(kind, ".delta") || kind == "content_block_delta")
}

func (b *grokContentPolicyBody) text(key, text string, delta bool) {
	if text == "" {
		return
	}
	if delta {
		text = b.tails[key] + text
		// Only short phrases span chunks. Bound retained text per output.
		if len(b.tails) < 128 || b.tails[key] != "" {
			tail := text
			if len(tail) > 2048 {
				tail = tail[len(tail)-2048:]
			}
			b.tails[key] = tail
		}
	}
	b.excluded = b.excluded || grokContentPolicyAccountOrBrowserFailure([]byte(text))
	b.hit = b.hit || grokContentPolicyRefusalText(text)
}

func (b *grokContentPolicyBody) rejected() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.stream {
		return !b.oversized && isGrokBillableContentPolicyRejection(http.StatusOK, b.buffer)
	}
	return b.hit && !b.excluded
}
