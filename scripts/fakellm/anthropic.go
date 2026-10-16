// Anthropic Messages: POST /v1/messages (streaming SSE + non-streaming).
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type anthToolDef struct {
	Name        string          `json:"name"`
	InputSchema json.RawMessage `json:"input_schema"`
}

func anthContentText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []struct {
		Type        string          `json:"type"`
		Text        string          `json:"text"`
		Thinking    string          `json:"thinking"`
		PartialJSON string          `json:"partial_json"`
		Content     json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var b strings.Builder
		for _, bl := range blocks {
			b.WriteString(bl.Text)
			if bl.Text == "" && len(bl.Content) > 0 {
				b.WriteString(anthContentText(bl.Content))
			}
		}
		return b.String()
	}
	return ""
}

// anthToolResults pulls tool_result block texts from a user message.
func anthToolResults(raw json.RawMessage) []string {
	var blocks []struct {
		Type    string          `json:"type"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil
	}
	var out []string
	for _, bl := range blocks {
		if bl.Type != "tool_result" {
			continue
		}
		out = append(out, anthContentText(bl.Content))
	}
	return out
}

type anthCall struct {
	Name string
	Args json.RawMessage
}

func (s *Server) handleAnthropic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": map[string]any{"message": "method not allowed"}})
		return
	}
	if !s.applyLatency(r) {
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 16<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"message": "unreadable body"}})
		return
	}
	var req struct {
		Model     string            `json:"model"`
		Messages  []json.RawMessage `json:"messages"`
		Tools     []anthToolDef     `json:"tools"`
		Stream    bool              `json:"stream"`
		Thinking  json.RawMessage   `json:"thinking"`
		MaxTokens int               `json:"max_tokens"`
		System    json.RawMessage   `json:"system"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"message": "invalid JSON"}})
		return
	}
	toolNames := make([]string, 0, len(req.Tools))
	toolSet := map[string]bool{}
	for _, t := range req.Tools {
		if t.Name != "" {
			toolNames = append(toolNames, t.Name)
			toolSet[t.Name] = true
		}
	}

	var lastUser string
	var toolResults []string
	lastTextIdx, lastResultIdx := -1, -1
	for i, m := range req.Messages {
		var msg struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
		if err := json.Unmarshal(m, &msg); err != nil {
			continue
		}
		if msg.Role == "user" {
			if t := anthContentText(msg.Content); t != "" {
				lastUser = t
				lastTextIdx = i
			}
			if res := anthToolResults(msg.Content); len(res) > 0 {
				lastResultIdx = i
				toolResults = append(toolResults, res...)
			}
		}
	}
	d := ParseDirectives(lastUser)
	if lastResultIdx > lastTextIdx && len(toolResults) > 0 {
		d = Directives{Raw: lastUser}
	}
	model := req.Model
	if model == "" {
		model = "anthropic/claude-fake-1"
	}
	wantThink := len(req.Thinking) > 0 && string(req.Thinking) != "null"
	seq := s.nextReqSeq()
	fault := s.selectFault(d, seq)
	if fault == faultStall && d.StallMs == 0 {
		d.StallAfter, d.StallMs = 1, 500
	}
	if fault == faultTTFB && d.TtfbMs == 0 {
		d.TtfbMs = 300
	}
	d.Fault = fault
	s.record(r, model, toolNames, req.Stream, fault)

	if isHTTPFault(fault) {
		writeHTTPFault(w, "anthropic", fault, model)
		return
	}
	switch fault {
	case faultHang:
		if req.Stream {
			hangOpen(w, r, "event: message_start\ndata: "+mustJSON(anthMessageStart(model, countTokens(lastUser)+8))+"\n\n")
		} else {
			hangNonStream(w, r)
		}
		return
	case faultCrash:
		if req.Stream {
			crashConn(w, r, "event: message_start\ndata: "+mustJSON(anthMessageStart(model, countTokens(lastUser)+8))+"\n\n")
		} else {
			hj, ok := w.(http.Hijacker)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"id":"msg_fake","type":"message","role":`)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			if ok {
				if conn, _, herr := hj.Hijack(); herr == nil {
					if tl, ok := conn.(interface{ SetLinger(int) error }); ok {
						_ = tl.SetLinger(0)
					}
					conn.Close()
					return
				}
			}
		}
		return
	case faultSSEError:
		if req.Stream {
			writeSSEHeader(w)
			sseEvent(w, "message_start", anthMessageStartEvent(model, countTokens(lastUser)+8))
			fmt.Fprint(w, "event: error\ndata: "+mustJSON(map[string]any{
				"type":  "error",
				"error": map[string]any{"type": "overloaded_error", "message": "Overloaded"},
			})+"\n\n")
			flush(w)
		} else {
			writeHTTPFault(w, "anthropic", faultHTTP529, model)
		}
		return
	case faultTruncated:
		if req.Stream {
			writeSSEHeader(w)
			fmt.Fprint(w, "event: message_start\ndata: {\"type\": \"message_start\", \"message\": {\"id\":")
			flush(w)
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"id":"msg_fake","type":"message","role":`)
			flush(w)
		}
		return
	case faultInvalidSSE:
		if req.Stream {
			writeSSEHeader(w)
			fmt.Fprint(w, "not-an-sse-event\n!!! bogus {{{\n\n")
			flush(w)
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `not json{{{`)
		}
		return
	case faultEmpty:
		if req.Stream {
			s.streamAnthText(w, r, model, "", wantThink, d)
		} else {
			s.writeAnthText(w, r, model, "", wantThink, lastUser)
		}
		return
	case faultLength:
		if req.Stream {
			s.streamAnthLength(w, r, model, wantThink, d)
		} else {
			s.writeAnthLength(w, r, model, lastUser, wantThink)
		}
		return
	}
	if fault == faultTTFB {
		if !streamFaultTTFB(r, d.TtfbMs) {
			return
		}
	}

	var calls []anthCall
	for _, t := range d.Tools {
		if toolSet[t.Name] {
			calls = append(calls, anthCall{t.Name, t.Args})
		}
	}
	if len(calls) == 0 && len(d.Tools) == 0 && len(toolResults) == 0 &&
		!d.HasEcho && !d.HasBig && !d.HasSlow && d.Fault == "" && !d.Hang && !d.Crash &&
		s.cooperateEnabled(r) && lastUser != "" {
		if name := matchCooperate(lastUser, toolNames); name != "" {
			var props map[string]json.RawMessage
			for _, td := range req.Tools {
				if td.Name == name && len(td.InputSchema) > 0 {
					var o struct {
						Properties map[string]json.RawMessage `json:"properties"`
					}
					if json.Unmarshal(td.InputSchema, &o) == nil {
						props = o.Properties
					}
					break
				}
			}
			calls = append(calls, anthCall{name, json.RawMessage(cooperateArgs(name, props, lastUser))})
		}
	}
	if len(d.Tools) > 0 && len(calls) == 0 {
		reply := "MISSING_TOOL:" + d.Tools[0].Name
		if req.Stream {
			s.streamAnthText(w, r, model, reply, wantThink, d)
		} else {
			s.writeAnthText(w, r, model, reply, wantThink, lastUser)
		}
		return
	}
	if len(calls) > 0 {
		// Assign process-unique ids now so stream + non-stream agree.
		ids := make([]string, len(calls))
		for i := range calls {
			ids[i] = s.nextToolID("toolu_")
		}
		if req.Stream {
			s.streamAnthTools(w, r, model, calls, ids, wantThink, d)
		} else {
			s.writeAnthTools(w, r, model, calls, ids, wantThink, lastUser)
		}
		return
	}
	reply := d.ReplyText(toolResults)
	if req.Stream {
		s.streamAnthText(w, r, model, reply, wantThink, d)
		return
	}
	s.writeAnthText(w, r, model, reply, wantThink, lastUser)
}

// anthMessageStart builds the message_start message object including usage,
// which strict SDKs (claude-cli>=2.1, mistral-vibe) require.
func anthMessageStart(model string, inputTokens int) map[string]any {
	return map[string]any{"type": "message_start",
		"message": map[string]any{"id": "msg_fake", "type": "message", "role": "assistant", "model": model,
			"usage": map[string]any{"input_tokens": inputTokens, "output_tokens": 0}}}
}

func anthMessageStartEvent(model string, inputTokens int) map[string]any {
	return anthMessageStart(model, inputTokens)
}

func (s *Server) writeAnthText(w http.ResponseWriter, r *http.Request, model, reply string, wantThink bool, prompt string) {
	if d := ParseDirectives(prompt); d.HasSlow {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(time.Duration(d.SlowMs) * time.Millisecond):
		}
	}
	content := []any{}
	if wantThink {
		content = append(content, map[string]any{"type": "thinking", "thinking": "THINKING: plan for " + truncate(reply, 120)})
	}
	content = append(content, map[string]any{"type": "text", "text": reply})
	pt := countTokens(prompt) + 8
	ct := countTokens(reply) + 3
	writeJSON(w, http.StatusOK, map[string]any{
		"id": "msg_fake", "type": "message", "role": "assistant", "model": model,
		"content": content, "stop_reason": "end_turn",
		"usage": map[string]any{"input_tokens": pt, "output_tokens": ct},
	})
}

func (s *Server) writeAnthLength(w http.ResponseWriter, r *http.Request, model, prompt string, wantThink bool) {
	content := []any{}
	if wantThink {
		content = append(content, map[string]any{"type": "thinking", "thinking": "THINKING: tool plan"})
	}
	content = append(content, map[string]any{"type": "text", "text": "partial-"})
	pt := countTokens(prompt) + 8
	writeJSON(w, http.StatusOK, map[string]any{
		"id": "msg_fake", "type": "message", "role": "assistant", "model": model,
		"content": content, "stop_reason": "max_tokens",
		"usage": map[string]any{"input_tokens": pt, "output_tokens": 3},
	})
}

func (s *Server) writeAnthTools(w http.ResponseWriter, r *http.Request, model string, calls []anthCall, ids []string, wantThink bool, prompt string) {
	content := []any{}
	if wantThink {
		content = append(content, map[string]any{"type": "thinking", "thinking": "THINKING: tool plan"})
	}
	argTokens := 0
	for i, c := range calls {
		var v any
		_ = json.Unmarshal(c.Args, &v)
		if v == nil {
			v = map[string]any{}
		}
		argTokens += countTokens(string(c.Args))
		content = append(content, map[string]any{
			"type": "tool_use", "id": ids[i],
			"name": c.Name, "input": v,
		})
	}
	pt := countTokens(prompt) + 8
	ct := 5 + argTokens
	writeJSON(w, http.StatusOK, map[string]any{
		"id": "msg_fake", "type": "message", "role": "assistant", "model": model,
		"content": content, "stop_reason": "tool_use",
		"usage": map[string]any{"input_tokens": pt, "output_tokens": ct},
	})
}

func (s *Server) streamAnthText(w http.ResponseWriter, r *http.Request, model, reply string, wantThink bool, d Directives) {
	writeSSEHeader(w)
	pt := countTokens(d.Raw) + 8
	ct := countTokens(reply) + 3
	idx := 0
	sseEvent(w, "message_start", anthMessageStartEvent(model, pt))
	if wantThink {
		sseEvent(w, "content_block_start", map[string]any{"type": "content_block_start",
			"index": idx, "content_block": map[string]any{"type": "thinking"}})
		sseEvent(w, "content_block_delta", map[string]any{"type": "content_block_delta",
			"index": idx, "delta": map[string]any{"type": "thinking_delta", "thinking": "THINKING: plan"}})
		sseEvent(w, "content_block_stop", map[string]any{"type": "content_block_stop", "index": idx})
		idx++
	}
	sseEvent(w, "content_block_start", map[string]any{"type": "content_block_start",
		"index": idx, "content_block": map[string]any{"type": "text", "text": ""}})
	chunks := splitChunks(reply)
	stallAfter := -1
	if d.Fault == faultStall {
		stallAfter = d.StallAfter
	}
	dup := d.Fault == faultDuplicate
	for n, ch := range chunks {
		if stallAfter == n {
			if !stallPoint(r, d) {
				return
			}
		}
		if !sleepChunks(r, d.SlowMs) {
			return
		}
		select {
		case <-r.Context().Done():
			return
		default:
		}
		emit := func() {
			sseEvent(w, "content_block_delta", map[string]any{"type": "content_block_delta",
				"index": idx, "delta": map[string]any{"type": "text_delta", "text": ch}})
		}
		emit()
		if dup {
			emit()
		}
	}
	if stallAfter == len(chunks) {
		if !stallPoint(r, d) {
			return
		}
	}
	sseEvent(w, "content_block_stop", map[string]any{"type": "content_block_stop", "index": idx})
	sseEvent(w, "message_delta", map[string]any{"type": "message_delta",
		"delta": map[string]any{"stop_reason": "end_turn"},
		"usage": map[string]any{"output_tokens": ct}})
	sseEvent(w, "message_stop", map[string]any{"type": "message_stop"})
}

func (s *Server) streamAnthLength(w http.ResponseWriter, r *http.Request, model string, wantThink bool, d Directives) {
	writeSSEHeader(w)
	pt := countTokens(d.Raw) + 8
	sseEvent(w, "message_start", anthMessageStartEvent(model, pt))
	idx := 0
	if wantThink {
		sseEvent(w, "content_block_start", map[string]any{"type": "content_block_start",
			"index": idx, "content_block": map[string]any{"type": "thinking"}})
		sseEvent(w, "content_block_stop", map[string]any{"type": "content_block_stop", "index": idx})
		idx++
	}
	sseEvent(w, "content_block_start", map[string]any{"type": "content_block_start",
		"index": idx, "content_block": map[string]any{"type": "text", "text": ""}})
	sseEvent(w, "content_block_delta", map[string]any{"type": "content_block_delta",
		"index": idx, "delta": map[string]any{"type": "text_delta", "text": "partial-"}})
	sseEvent(w, "content_block_stop", map[string]any{"type": "content_block_stop", "index": idx})
	sseEvent(w, "message_delta", map[string]any{"type": "message_delta",
		"delta": map[string]any{"stop_reason": "max_tokens"},
		"usage": map[string]any{"output_tokens": 3}})
	sseEvent(w, "message_stop", map[string]any{"type": "message_stop"})
}

func (s *Server) streamAnthTools(w http.ResponseWriter, r *http.Request, model string, calls []anthCall, ids []string, wantThink bool, d Directives) {
	writeSSEHeader(w)
	pt := countTokens(d.Raw) + 8
	argTokens := 0
	for _, c := range calls {
		argTokens += countTokens(string(c.Args))
	}
	ct := 5 + argTokens
	sseEvent(w, "message_start", anthMessageStartEvent(model, pt))
	idx := 0
	if wantThink {
		sseEvent(w, "content_block_start", map[string]any{"type": "content_block_start",
			"index": idx, "content_block": map[string]any{"type": "thinking"}})
		sseEvent(w, "content_block_delta", map[string]any{"type": "content_block_delta",
			"index": idx, "delta": map[string]any{"type": "thinking_delta", "thinking": "THINKING: tool plan"}})
		sseEvent(w, "content_block_stop", map[string]any{"type": "content_block_stop", "index": idx})
		idx++
	}
	for i, c := range calls {
		if !sleepChunks(r, d.SlowMs) {
			return
		}
		select {
		case <-r.Context().Done():
			return
		default:
		}
		// tool_use start must carry input:{} for strict SDKs.
		sseEvent(w, "content_block_start", map[string]any{"type": "content_block_start",
			"index": idx, "content_block": map[string]any{
				"type": "tool_use", "id": ids[i], "name": c.Name, "input": map[string]any{}}})
		sseEvent(w, "content_block_delta", map[string]any{"type": "content_block_delta",
			"index": idx, "delta": map[string]any{"type": "input_json_delta", "partial_json": string(c.Args)}})
		sseEvent(w, "content_block_stop", map[string]any{"type": "content_block_stop", "index": idx})
		idx++
	}
	sseEvent(w, "message_delta", map[string]any{"type": "message_delta",
		"delta": map[string]any{"stop_reason": "tool_use"},
		"usage": map[string]any{"output_tokens": ct}})
	sseEvent(w, "message_stop", map[string]any{"type": "message_stop"})
}
