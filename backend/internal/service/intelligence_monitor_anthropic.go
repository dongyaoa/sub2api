package service

import (
	"strings"

	"github.com/tidwall/gjson"
)

// Anthropic numbers content blocks in order. Only text blocks contribute to
// the artwork, so thinking, signatures and tool payloads never reach preview.
type intelligenceAnthropicStream struct {
	started    bool
	blockOpen  bool
	textBlock  bool
	nextIndex  int64
	stopReason string
}

func (s *intelligenceAnthropicStream) consume(event string, payload gjson.Result, output *strings.Builder) (terminal bool, message string) {
	const invalid = "generation stream contained invalid content blocks"
	switch event {
	case "message_start":
		if s.started {
			return true, invalid
		}
		s.started = true
	case "content_block_start", "content_block_delta", "content_block_stop":
		index := payload.Get("index")
		if !s.started || index.Type != gjson.Number || index.Num != float64(s.nextIndex) || s.stopReason != "" {
			return true, invalid
		}
		switch event {
		case "content_block_start":
			kind := payload.Get("content_block.type")
			if s.blockOpen || kind.Type != gjson.String || kind.Str == "" {
				return true, invalid
			}
			s.blockOpen, s.textBlock = true, kind.Str == "text"
			if s.textBlock {
				output.WriteString(payload.Get("content_block.text").String())
			}
		case "content_block_delta":
			if !s.blockOpen {
				return true, invalid
			}
			if s.textBlock && payload.Get("delta.type").String() == "text_delta" {
				text := payload.Get("delta.text")
				if text.Type != gjson.String {
					return true, invalid
				}
				output.WriteString(text.Str)
			}
		case "content_block_stop":
			if !s.blockOpen {
				return true, invalid
			}
			s.blockOpen, s.textBlock = false, false
			s.nextIndex++
		}
	case "message_delta":
		if reason := payload.Get("delta.stop_reason"); reason.Type == gjson.String {
			s.stopReason = reason.Str
		}
	case "message_stop":
		if !s.started || s.blockOpen {
			return true, invalid
		}
		return true, intelligenceAnthropicStopError(s.stopReason)
	case "error":
		return true, "model generation returned an error"
	}
	return false, ""
}

func intelligenceAnthropicStopError(reason string) string {
	switch reason {
	case "end_turn", "stop_sequence":
		return ""
	case "max_tokens", "model_context_window_exceeded", "refusal":
		return "model output was truncated or filtered"
	default:
		return "model generation returned an incomplete result"
	}
}

func extractIntelligenceAnthropicMessage(message gjson.Result) (string, string) {
	if message.Get("type").String() != "message" || !message.Get("content").IsArray() {
		return "", "generation endpoint did not return an Anthropic message"
	}
	var output strings.Builder
	message.Get("content").ForEach(func(_, block gjson.Result) bool {
		if text := block.Get("text"); block.Get("type").String() == "text" && text.Type == gjson.String {
			output.WriteString(text.Str)
		}
		return true
	})
	if issue := intelligenceAnthropicStopError(message.Get("stop_reason").String()); issue != "" {
		return output.String(), issue
	}
	if output.Len() == 0 {
		return "", "model returned no text"
	}
	return output.String(), ""
}
