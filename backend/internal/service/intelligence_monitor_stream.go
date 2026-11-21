package service

import (
	"bufio"
	"bytes"
	"io"
	"strings"

	"github.com/tidwall/gjson"
)

func intelligenceLooksLikeSSE(raw []byte) bool {
	raw = bytes.TrimSpace(raw)
	return bytes.HasPrefix(raw, []byte("data:")) || bytes.HasPrefix(raw, []byte("event:")) || bytes.HasPrefix(raw, []byte(":"))
}

// Read while the remote generator works, and stop at its terminal event even
// if the proxy keeps the connection open. A JSON-only compatible endpoint is
// still accepted, including a gateway with a stale text/event-stream header.
func readIntelligenceGenerationBody(body io.Reader, mode, contentType, key string, waitForChatDone bool) (string, string) {
	limited := &io.LimitedReader{R: body, N: intelligenceResponseMaxBytes + 1}
	reader := bufio.NewReader(limited)
	for {
		first, err := reader.Peek(1)
		if err != nil {
			if limited.N <= 0 {
				return "", "generation response exceeded the 4 MiB limit"
			}
			if err == io.EOF {
				return "", "model returned no text"
			}
			return "", "generation response was interrupted"
		}
		if !strings.ContainsRune(" \t\r\n", rune(first[0])) {
			break
		}
		_, _ = reader.Discard(1)
	}
	first, _ := reader.Peek(1)
	jsonBody := first[0] == '{' || first[0] == '['
	if !jsonBody && (strings.Contains(strings.ToLower(contentType), "text/event-stream") || first[0] == ':' || first[0] == 'd' || first[0] == 'e') {
		text, message := readIntelligenceEventStream(reader, mode, key, waitForChatDone)
		if limited.N <= 0 {
			return "", "generation response exceeded the 4 MiB limit"
		}
		return text, message
	}
	raw, err := io.ReadAll(reader)
	if limited.N <= 0 {
		return "", "generation response exceeded the 4 MiB limit"
	}
	if err != nil {
		return "", "generation response was interrupted"
	}
	return extractIntelligenceModelTextWithKey(raw, mode, false, key)
}

func readIntelligenceEventStream(body io.Reader, mode, key string, waitForChatDone bool) (string, string) {
	limited := &io.LimitedReader{R: body, N: intelligenceResponseMaxBytes + 1}
	text, _, message := readMonitorEventStream(limited, mode, key, waitForChatDone)
	if limited.N <= 0 {
		return "", "generation response exceeded the 4 MiB limit"
	}
	return text, message
}

// The terminal Responses object carries the token counts used for upstream probe billing.
func readMonitorEventStream(limited *io.LimitedReader, mode, key string, waitForChatDone bool) (final, responseJSON, message string) {
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 4096), int(limited.N))
	var deltas, data strings.Builder
	eventName := ""
	terminal := false
	chatFinished := false
	var anthropic intelligenceAnthropicStream
	consume := func() {
		value := strings.TrimSpace(data.String())
		data.Reset()
		name := eventName
		eventName = ""
		if value == "" {
			return
		}
		if value == "[DONE]" {
			terminal = true
			if mode == MonitorAPIModeChatCompletions && chatFinished {
				return
			}
			// Transport termination alone does not prove a complete generation.
			// Require a Responses terminal or CC finish before this sentinel;
			// truncated proxies sometimes emit only the transport marker.
			message = "generation stream ended before completion"
			return
		}
		if !gjson.Valid(value) {
			terminal, message = true, "generation event stream contained invalid JSON"
			return
		}
		parsed := gjson.Parse(value)
		event := parsed.Get("type").String()
		if event == "" {
			event = name
		}
		if failure := parsed.Get("error"); failure.Exists() && failure.Type != gjson.Null {
			terminal = true
			message = intelligenceGenerationEventError("model generation returned an error", []byte(value), key)
			return
		}
		if mode == IntelligenceMonitorAPIModeMessages {
			terminal, message = anthropic.consume(event, parsed, &deltas)
			return
		}
		switch event {
		case "response.output_text.delta":
			_, _ = deltas.WriteString(parsed.Get("delta").String())
		case "response.completed", "response.done":
			terminal = true
			response := parsed.Get("response")
			responseJSON = response.Raw
			final = extractOpenAIResponsesText([]byte(responseJSON))
			status := response.Get("status").String()
			failure := response.Get("error")
			if (status != "" && status != "completed") || (failure.Exists() && failure.Type != gjson.Null) {
				message = intelligenceGenerationEventError("model generation failed or returned an incomplete result", []byte(value), key)
			}
		case "response.failed", "response.incomplete", "response.cancelled", "response.canceled", "error":
			terminal = true
			responseJSON = parsed.Get("response").Raw
			final = extractOpenAIResponsesText([]byte(responseJSON))
			message = intelligenceGenerationEventError("model generation failed or returned an incomplete result", []byte(value), key)
		default:
			if mode == MonitorAPIModeChatCompletions {
				_, _ = deltas.WriteString(parsed.Get("choices.0.delta.content").String())
				finish := parsed.Get("choices.0.finish_reason").String()
				if finish != "" {
					// The local gateway still needs the usage chunk after finish
					// for billing, even for truncated or filtered generations.
					// Closing its bound request early cancels that upstream tail.
					chatFinished = true
					terminal = !waitForChatDone
					if finish != "stop" {
						message = "model output was truncated or filtered"
					}
				}
			}
		}
	}
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			consume()
			if terminal {
				break
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "event":
			eventName = value
		case "data":
			if data.Len() > 0 {
				_ = data.WriteByte('\n')
			}
			_, _ = data.WriteString(value)
		}
	}
	if !terminal && scanner.Err() == nil {
		consume()
	}
	if !terminal {
		message = "generation stream ended before completion"
		if scanner.Err() != nil {
			message = "generation response was interrupted"
		}
	}
	if final == "" {
		final = deltas.String()
	}
	if final == "" && message == "" {
		message = "model returned no text"
	}
	return final, responseJSON, message
}
