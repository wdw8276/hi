package proxy

import (
	"bufio"
	"encoding/json"
	"github.com/mars-base/hi/pkg/logx"
	"io"
	"net/http"
	"strings"
)

// transformRequestBody remaps model names and strips thinking blocks before forwarding.
func (ps *ProxyState) transformRequestBody(body []byte, isModel bool, activeName string, backend Backend) []byte {
	if !isModel || len(body) == 0 {
		return body
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		// Not valid JSON — pass through.
		return body
	}

	changed := false

	// Remap model name.
	if model, ok := parsed["model"].(string); ok {
		mapped := backend.MapModel(model)
		if mapped != model {
			parsed["model"] = mapped
			changed = true
			logx.Debug("Model remap: %s → %s", model, mapped)
		}
	}

	// Strip top-level thinking config. Some backends (e.g. DeepSeek) require
	// once thinking is enabled, ALL subsequent requests must also have it —
	// but Claude Code's tool-use requests (web search, fetch) may omit it.
	// Removing the top-level config avoids the "thinking options type cannot
	// be disabled when reasoning_effort is set" error.
	if backend.StripTopLevelThinking() {
		if _, ok := parsed["thinking"]; ok {
			delete(parsed, "thinking")
			changed = true
			logx.Debug("Stripped top-level thinking config (backend: %s)", activeName)
		}
	}

	// Inject output_config.effort for deepseek backends if configured.
	if eff := backend.ReasoningEffort(); eff != "" {
		parsed["output_config"] = map[string]interface{}{"effort": eff}
		changed = true
		logx.Debug("Injected output_config.effort=%s (backend: %s)", eff, activeName)
	}

	// Strip thinking blocks from message content.
	if backend.NeedsThinkingStrip() {
		if messages, ok := parsed["messages"].([]interface{}); ok {
			for _, msg := range messages {
				if msgMap, ok := msg.(map[string]interface{}); ok {
					if content, ok := msgMap["content"].([]interface{}); ok {
						filtered := make([]interface{}, 0, len(content))
						for _, block := range content {
							if blockMap, ok := block.(map[string]interface{}); ok {
								if blockMap["type"] == "thinking" {
									changed = true
									continue
								}
							}
							filtered = append(filtered, block)
						}
						msgMap["content"] = filtered
					}
				}
			}
		}
	}

	if !changed {
		return body
	}

	newBody, err := json.Marshal(parsed)
	if err != nil {
		return body
	}
	return newBody
}

// processResponse handles the upstream response — normalizing SSE, tracking costs,
// and writing back to the client.
func (ps *ProxyState) processResponse(w http.ResponseWriter, r *http.Request, resp *http.Response, backend Backend) {
	contentType := resp.Header.Get("Content-Type")

	isSSE := strings.Contains(contentType, "text/event-stream")
	isJSON := strings.Contains(contentType, "application/json")

	backendName := backend.Name()

	logx.Debug("processResponse: content-type=%s sse=%v json=%v", contentType, isSSE, isJSON)

	if isSSE {
		ps.processSSEResponse(w, resp, backendName)
	} else if isJSON {
		ps.processJSONResponse(w, resp, backendName)
	} else {
		ps.processPassthrough(w, resp)
	}
}

// processSSEResponse streams an SSE response with usage normalization.
func (ps *ProxyState) processSSEResponse(w http.ResponseWriter, resp *http.Response, backendName string) {
	// Copy headers.
	ps.copyHeaders(w, resp)
	w.WriteHeader(resp.StatusCode)

	flusher, ok := w.(http.Flusher)
	if !ok {
		// Can't flush — just copy.
		io.Copy(w, resp.Body)
		return
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024) // 1MB max scan token.

	var inputTokens, outputTokens int64
	var eventBuf strings.Builder

	for scanner.Scan() {
		line := scanner.Text()
		eventBuf.WriteString(line)
		eventBuf.WriteString("\n")

		// SSE events are separated by blank lines.
		if line == "" {
			event := eventBuf.String()
			eventBuf.Reset()

			fixedEvent := ps.normalizeSSEEvent(event, &inputTokens, &outputTokens)
			w.Write([]byte(fixedEvent))
			flusher.Flush()
		}
	}

	// Flush remaining buffer.
	if eventBuf.Len() > 0 {
		fixedEvent := ps.normalizeSSEEvent(eventBuf.String(), &inputTokens, &outputTokens)
		w.Write([]byte(fixedEvent))
		flusher.Flush()
	}

	if err := scanner.Err(); err != nil {
		logx.Warn("SSE stream error: %v", err)
	}

	// Record usage if we got any tokens.
	if inputTokens > 0 || outputTokens > 0 {
		ps.costTracker.Record(backendName, inputTokens, outputTokens)
		logx.Info("#%d tokens: in=%d out=%d", ps.RequestCount(), inputTokens, outputTokens)
	}
}

// normalizeSSEEvent ensures usage fields exist in message_start and message_delta events.
func (ps *ProxyState) normalizeSSEEvent(event string, inputTokens *int64, outputTokens *int64) string {
	// Find the data: line.
	dataPrefix := "data: "
	idx := strings.Index(event, dataPrefix)
	if idx < 0 {
		return event
	}

	dataStart := idx + len(dataPrefix)
	dataEnd := strings.IndexByte(event[dataStart:], '\n')
	if dataEnd < 0 {
		dataEnd = len(event) - dataStart
	}

	dataStr := event[dataStart : dataStart+dataEnd]
	if strings.TrimSpace(dataStr) == "[DONE]" {
		return event
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(dataStr), &parsed); err != nil {
		return event
	}

	changed := false

	eventType, _ := parsed["type"].(string)

	switch eventType {
	case "message_start":
		if msg, ok := parsed["message"].(map[string]interface{}); ok {
			if _, hasUsage := msg["usage"]; !hasUsage {
				msg["usage"] = map[string]interface{}{
					"input_tokens":  0,
					"output_tokens": 0,
				}
				changed = true
			}
			if usage, ok := msg["usage"].(map[string]interface{}); ok {
				if it, ok := usage["input_tokens"].(float64); ok {
					*inputTokens = int64(it)
				}
			}
		}

	case "message_delta":
		if _, hasUsage := parsed["usage"]; !hasUsage {
			parsed["usage"] = map[string]interface{}{
				"output_tokens": 0,
			}
			changed = true
		}
		if usage, ok := parsed["usage"].(map[string]interface{}); ok {
			if ot, ok := usage["output_tokens"].(float64); ok {
				*outputTokens = int64(ot)
			}
		}

	case "message_stop":
		// An SSE "message_stop" event has a "usage" field at the **event** level
		// that may contain the final token count. If present, prefer it.
		if usage, ok := parsed["usage"].(map[string]interface{}); ok {
			if it, ok := usage["input_tokens"].(float64); ok && int64(it) > *inputTokens {
				*inputTokens = int64(it)
			}
			if ot, ok := usage["output_tokens"].(float64); ok && int64(ot) > *outputTokens {
				*outputTokens = int64(ot)
			}
		}
	}

	if !changed {
		return event
	}

	newData, err := json.Marshal(parsed)
	if err != nil {
		return event
	}

	// Reconstruct the event with the fixed data line.
	return strings.Replace(event, dataStr, string(newData), 1)
}

// processJSONResponse handles non-streaming JSON responses.
func (ps *ProxyState) processJSONResponse(w http.ResponseWriter, resp *http.Response, backendName string) {
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		ps.copyHeaders(w, resp)
		w.WriteHeader(resp.StatusCode)
		return
	}

	// Normalize: ensure usage block exists.
	var parsed map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &parsed); err == nil {
		if parsed["type"] == "message" {
			if _, ok := parsed["usage"]; !ok {
				parsed["usage"] = map[string]interface{}{
					"input_tokens":  0,
					"output_tokens": 0,
				}
			}
		}
		// Track usage.
		if usage, ok := parsed["usage"].(map[string]interface{}); ok {
			var inputTokens, outputTokens int64
			if it, ok := usage["input_tokens"].(float64); ok {
				inputTokens = int64(it)
			}
			if ot, ok := usage["output_tokens"].(float64); ok {
				outputTokens = int64(ot)
				ps.costTracker.Record(backendName, inputTokens, outputTokens)
				logx.Info("#%d tokens: in=%d out=%d", ps.RequestCount(), inputTokens, outputTokens)
			}
		}

		newBody, err := json.Marshal(parsed)
		if err == nil {
			bodyBytes = newBody
		}
	}

	ps.copyHeaders(w, resp)
	w.Header().Set("Content-Length", itoa(len(bodyBytes)))
	w.WriteHeader(resp.StatusCode)
	w.Write(bodyBytes)
}

// processPassthrough copies the response without modification.
func (ps *ProxyState) processPassthrough(w http.ResponseWriter, resp *http.Response) {
	ps.copyHeaders(w, resp)
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// copyHeaders copies response headers from upstream to downstream.
func (ps *ProxyState) copyHeaders(w http.ResponseWriter, resp *http.Response) {
	for key, values := range resp.Header {
		for _, v := range values {
			w.Header().Add(key, v)
		}
	}
	stripHopByHop(w.Header())
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(byte('0'+n%10)) + s
		n /= 10
	}
	return s
}
