// OpenAI Chat Completions: POST /v1/chat/completions.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type chatToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type chatReq struct {
	Model         string            `json:"model"`
	Messages      []json.RawMessage `json:"messages"`
	Tools         []json.RawMessage `json:"tools"`
	Stream        bool              `json:"stream"`
	StreamOptions *struct {
		IncludeUsage bool `json:"include_usage"`
	} `json:"stream_options"`
	ReasoningEffort any `json:"reasoning_effort"`
	Reasoning       any `json:"reasoning"`
}

type chatMsgView struct {
	Role      string
	Text      string
	ToolCalls []chatToolCall
	IsTool    bool
}

func parseChatMessage(raw json.RawMessage) chatMsgView {
	var probe struct {
		Role       string          `json:"role"`
		Content    json.RawMessage `json:"content"`
		ToolCalls  []chatToolCall  `json:"tool_calls"`
		ToolCallID string          `json:"tool_call_id"`
	}
	var v chatMsgView
	if err := json.Unmarshal(raw, &probe); err != nil {
		return v
	}
	v.Role = probe.Role
	v.ToolCalls = probe.ToolCalls
	v.IsTool = probe.Role == "tool" || probe.ToolCallID != ""
	v.Text = chatContentText(probe.Content)
	return v
}

func chatContentText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err == nil {
		var b strings.Builder
		for _, p := range parts {
			if p.Type == "text" || p.Text != "" {
				b.WriteString(p.Text)
			}
		}
		return b.String()
	}
	return ""
}

func chatToolNames(raw []json.RawMessage) ([]string, map[string]bool) {
	var names []string
	set := map[string]bool{}
	for _, t := range raw {
		var o struct {
			Type     string `json:"type"`
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		}
		if err := json.Unmarshal(t, &o); err != nil {
			continue
		}
		if o.Function.Name != "" {
			names = append(names, o.Function.Name)
			set[o.Function.Name] = true
		}
	}
	return names, set
}

func chatWantsReasoning(body []byte) bool {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return false
	}
	for _, k := range []string{"reasoning_effort", "reasoning", "reasoning_content", "include_reasoning", "enable_thinking"} {
		if _, ok := m[k]; ok {
			return true
		}
	}
	return false
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if !s.applyLatency(r) {
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 16<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unreadable body"})
		return
	}
	var req chatReq
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON"})
		return
	}
	toolNames, toolSet := chatToolNames(req.Tools)

	var lastUser string
	var toolResults []string
	lastUserIdx, lastToolIdx := -1, -1
	for i, m := range req.Messages {
		v := parseChatMessage(m)
		if v.Role == "user" && v.Text != "" {
			lastUser = v.Text
			lastUserIdx = i
		}
		if v.IsTool {
			lastToolIdx = i
			if v.Text != "" {
				toolResults = append(toolResults, v.Text)
			}
		}
	}
	d := ParseDirectives(lastUser)
	if lastToolIdx > lastUserIdx && len(toolResults) > 0 {
		// Tool results are newer than any user directive: this is the
		// second half of a tool loop, not a fresh TOOL: request.
		d = Directives{Raw: lastUser}
	}
	model := req.Model
	if model == "" {
		model = "fake-small"
	}
	wantReason := chatWantsReasoning(body)
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

	// Terminal failure faults.
	if isHTTPFault(fault) {
		writeHTTPFault(w, "openai", fault, model)
		return
	}
	switch fault {
	case faultHang:
		if req.Stream {
			hangOpen(w, r, fmt.Sprintf("data: %s\n\n",
				mustJSON(map[string]any{"id": "chatcmpl-fake", "object": "chat.completion.chunk",
					"created": time.Now().Unix(), "model": model,
					"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "content": "par"}}}})))
		} else {
			hangNonStream(w, r)
		}
		return
	case faultCrash:
		if req.Stream {
			crashConn(w, r, fmt.Sprintf("data: %s\n\n",
				mustJSON(map[string]any{"id": "chatcmpl-fake", "object": "chat.completion.chunk",
					"created": time.Now().Unix(), "model": model,
					"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "content": "par"}}}})))
		} else {
			hj, ok := w.(http.Hijacker)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"id":"chatcmpl-fake","object":"chat.completion","created":`)
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
			created := time.Now().Unix()
			sseData(w, map[string]any{
				"id": "chatcmpl-fake", "object": "chat.completion.chunk",
				"created": created, "model": model,
				"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "content": "par"}, "finish_reason": nil}},
			})
			fmt.Fprint(w, `data: {"error":{"message":"stream interrupted","type":"server_error","code":500}}`+"\n\n")
			flush(w)
		} else {
			writeHTTPFault(w, "openai", faultHTTP500, model)
		}
		return
	case faultTruncated:
		if req.Stream {
			writeSSEHeader(w)
			fmt.Fprint(w, `data: {"id":"chatcmpl-fake","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"par`)
			flush(w)
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"id":"chatcmpl-fake","object":"chat.completion","created":`)
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
			s.streamChatText(w, r, model, "", wantReason, d)
		} else {
			s.writeChatText(w, model, "", wantReason, lastUser)
		}
		return
	case faultLength:
		if req.Stream {
			s.streamChatLength(w, r, model, d)
		} else {
			s.writeChatLength(w, model, lastUser)
		}
		return
	}
	// Passthrough faults (stall, ttfb, duplicate, "") continue below.
	if fault == faultTTFB {
		if !streamFaultTTFB(r, d.TtfbMs) {
			return
		}
	}

	// Legacy Crash/Hang directives that selectFault maps are handled above
	// via fault; keep the old explicit paths unreachable but harmless.
	if d.Crash && fault == "" {
		if req.Stream {
			crashConn(w, r, fmt.Sprintf("data: %s\n\n",
				mustJSON(map[string]any{"id": "chatcmpl-fake", "object": "chat.completion.chunk",
					"created": time.Now().Unix(), "model": model,
					"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "content": "par"}}}})))
		} else {
			hj, ok := w.(http.Hijacker)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"id":"chatcmpl-fake","object":"chat.completion","created":`)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			if ok {
				if conn, _, herr := hj.Hijack(); herr == nil {
					conn.Close()
					return
				}
			}
		}
		return
	}
	if d.Hang && fault == "" {
		if req.Stream {
			hangOpen(w, r, fmt.Sprintf("data: %s\n\n",
				mustJSON(map[string]any{"id": "chatcmpl-fake", "object": "chat.completion.chunk",
					"created": time.Now().Unix(), "model": model,
					"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "content": "par"}}}})))
		} else {
			hangNonStream(w, r)
		}
		return
	}

	// Tool turn: emit calls for the requested tools that are listed.
	var calls []chatToolCall
	for _, t := range d.Tools {
		if !toolSet[t.Name] {
			continue
		}
		var c chatToolCall
		c.ID = s.nextToolID("call_")
		c.Type = "function"
		c.Function.Name = t.Name
		c.Function.Arguments = string(t.Args)
		calls = append(calls, c)
	}
	// COOPERATE mode: natural-language prompt, no directives, no tool
	// results yet, prompt mentions an available tool.
	if len(calls) == 0 && len(d.Tools) == 0 && len(toolResults) == 0 &&
		!d.HasEcho && !d.HasBig && !d.HasSlow && d.Fault == "" && !d.Hang && !d.Crash &&
		s.cooperateEnabled(r) && lastUser != "" {
		if name := matchCooperate(lastUser, toolNames); name != "" {
			var props map[string]json.RawMessage
			for _, raw := range req.Tools {
				var o struct {
					Function struct {
						Name string `json:"name"`
					} `json:"function"`
				}
				if json.Unmarshal(raw, &o) == nil && o.Function.Name == name {
					props = schemaProps(raw)
					break
				}
			}
			var c chatToolCall
			c.ID = s.nextToolID("call_")
			c.Type = "function"
			c.Function.Name = name
			c.Function.Arguments = cooperateArgs(name, props, lastUser)
			calls = append(calls, c)
		}
	}
	if len(d.Tools) > 0 && len(calls) == 0 {
		// Named tools were requested but none is in the tool list.
		missing := d.Tools[0].Name
		reply := "MISSING_TOOL:" + missing
		if req.Stream {
			s.streamChatText(w, r, model, reply, wantReason, d)
		} else {
			if d.HasSlow {
				select {
				case <-r.Context().Done():
					return
				case <-time.After(time.Duration(d.SlowMs) * time.Millisecond):
				}
			}
			s.writeChatText(w, model, reply, wantReason, lastUser)
		}
		return
	}

	if len(calls) > 0 {
		if req.Stream {
			s.streamChatTools(w, r, model, calls, d)
		} else {
			if d.HasSlow {
				select {
				case <-r.Context().Done():
					return
				case <-time.After(time.Duration(d.SlowMs) * time.Millisecond):
				}
			}
			s.writeChatTools(w, model, calls, lastUser)
		}
		return
	}

	reply := d.ReplyText(toolResults)
	if req.Stream {
		s.streamChatText(w, r, model, reply, wantReason, d)
		return
	}
	if d.HasSlow {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(time.Duration(d.SlowMs) * time.Millisecond):
		}
	}
	s.writeChatText(w, model, reply, wantReason, lastUser)
}

func (s *Server) writeChatText(w http.ResponseWriter, model, reply string, wantReason bool, prompt string) {
	msg := map[string]any{"role": "assistant", "content": reply}
	if wantReason {
		msg["reasoning_content"] = "THINKING: plan for " + truncate(reply, 120)
	}
	pt := countTokens(prompt) + 8
	ct := countTokens(reply) + 3
	writeJSON(w, http.StatusOK, map[string]any{
		"id":      "chatcmpl-fake",
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []any{map[string]any{
			"index": 0, "message": msg, "finish_reason": "stop",
		}},
		"usage": map[string]any{"prompt_tokens": pt, "completion_tokens": ct, "total_tokens": pt + ct},
	})
}

func (s *Server) writeChatTools(w http.ResponseWriter, model string, calls []chatToolCall, prompt string) {
	enc := make([]any, 0, len(calls))
	argTokens := 0
	for i, c := range calls {
		argTokens += countTokens(c.Function.Arguments)
		enc = append(enc, map[string]any{
			"id": c.ID, "type": "function", "index": i,
			"function": map[string]any{"name": c.Function.Name, "arguments": c.Function.Arguments},
		})
	}
	pt := countTokens(prompt) + 8
	ct := 5 + argTokens
	writeJSON(w, http.StatusOK, map[string]any{
		"id":      "chatcmpl-fake",
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []any{map[string]any{
			"index": 0,
			"message": map[string]any{
				"role": "assistant", "content": nil, "tool_calls": enc,
			},
			"finish_reason": "tool_calls",
		}},
		"usage": map[string]any{"prompt_tokens": pt, "completion_tokens": ct, "total_tokens": pt + ct},
	})
}

func (s *Server) writeChatLength(w http.ResponseWriter, model, prompt string) {
	reply := "partial-"
	pt := countTokens(prompt) + 8
	ct := countTokens(reply) + 3
	writeJSON(w, http.StatusOK, map[string]any{
		"id":      "chatcmpl-fake",
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []any{map[string]any{
			"index": 0,
			"message": map[string]any{
				"role": "assistant", "content": reply,
			},
			"finish_reason": "length",
		}},
		"usage": map[string]any{"prompt_tokens": pt, "completion_tokens": ct, "total_tokens": pt + ct},
	})
}

func (s *Server) streamChatText(w http.ResponseWriter, r *http.Request, model, reply string, wantReason bool, d Directives) {
	if d.Fault == faultTTFB {
		// Already slept in the handler; kept for direct callers.
	}
	writeSSEHeader(w)
	created := time.Now().Unix()
	sseData(w, map[string]any{
		"id": "chatcmpl-fake", "object": "chat.completion.chunk",
		"created": created, "model": model,
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant"}, "finish_reason": nil}},
	})
	if wantReason {
		sseData(w, map[string]any{
			"id": "chatcmpl-fake", "object": "chat.completion.chunk",
			"created": created, "model": model,
			"choices": []any{map[string]any{"index": 0,
				"delta":         map[string]any{"reasoning_content": "THINKING: plan"},
				"finish_reason": nil}},
		})
	}
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
			sseData(w, map[string]any{
				"id": "chatcmpl-fake", "object": "chat.completion.chunk",
				"created": created, "model": model,
				"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": ch}, "finish_reason": nil}},
			})
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
	pt := countTokens(d.Raw) + 8
	ct := countTokens(reply) + 3
	sseData(w, map[string]any{
		"id": "chatcmpl-fake", "object": "chat.completion.chunk",
		"created": created, "model": model,
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}},
		"usage":   map[string]any{"prompt_tokens": pt, "completion_tokens": ct, "total_tokens": pt + ct},
	})
	fmt.Fprint(w, "data: [DONE]\n\n")
	flush(w)
}

func (s *Server) streamChatLength(w http.ResponseWriter, r *http.Request, model string, d Directives) {
	writeSSEHeader(w)
	created := time.Now().Unix()
	sseData(w, map[string]any{
		"id": "chatcmpl-fake", "object": "chat.completion.chunk",
		"created": created, "model": model,
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant"}, "finish_reason": nil}},
	})
	sseData(w, map[string]any{
		"id": "chatcmpl-fake", "object": "chat.completion.chunk",
		"created": created, "model": model,
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": "partial-"}, "finish_reason": nil}},
	})
	sseData(w, map[string]any{
		"id": "chatcmpl-fake", "object": "chat.completion.chunk",
		"created": created, "model": model,
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "length"}},
	})
	fmt.Fprint(w, "data: [DONE]\n\n")
	flush(w)
}

func splitArgs(s string, n int) []string {
	if s == "" {
		return nil
	}
	if n <= 0 {
		n = 12
	}
	var out []string
	for len(s) > n {
		out = append(out, s[:n])
		s = s[n:]
	}
	out = append(out, s)
	return out
}

func (s *Server) streamChatTools(w http.ResponseWriter, r *http.Request, model string, calls []chatToolCall, d Directives) {
	writeSSEHeader(w)
	created := time.Now().Unix()
	sseData(w, map[string]any{
		"id": "chatcmpl-fake", "object": "chat.completion.chunk",
		"created": created, "model": model,
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant"}, "finish_reason": nil}},
	})
	for i, c := range calls {
		if !sleepChunks(r, d.SlowMs) {
			return
		}
		select {
		case <-r.Context().Done():
			return
		default:
		}
		// First chunk carries the realistic id/type/function name with an
		// empty arguments seed; argument bytes follow as deltas.
		sseData(w, map[string]any{
			"id": "chatcmpl-fake", "object": "chat.completion.chunk",
			"created": created, "model": model,
			"choices": []any{map[string]any{"index": 0, "delta": map[string]any{
				"tool_calls": []any{map[string]any{
					"index": i, "id": c.ID, "type": "function",
					"function": map[string]any{"name": c.Function.Name, "arguments": ""},
				}},
			}, "finish_reason": nil}},
		})
		for _, piece := range splitArgs(c.Function.Arguments, 12) {
			if !sleepChunks(r, d.SlowMs) {
				return
			}
			sseData(w, map[string]any{
				"id": "chatcmpl-fake", "object": "chat.completion.chunk",
				"created": created, "model": model,
				"choices": []any{map[string]any{"index": 0, "delta": map[string]any{
					"tool_calls": []any{map[string]any{
						"index":    i,
						"function": map[string]any{"arguments": piece},
					}},
				}, "finish_reason": nil}},
			})
		}
	}
	pt := countTokens(d.Raw) + 8
	argTokens := 0
	for _, c := range calls {
		argTokens += countTokens(c.Function.Arguments)
	}
	ct := 5 + argTokens
	sseData(w, map[string]any{
		"id": "chatcmpl-fake", "object": "chat.completion.chunk",
		"created": created, "model": model,
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "tool_calls"}},
		"usage":   map[string]any{"prompt_tokens": pt, "completion_tokens": ct, "total_tokens": pt + ct},
	})
	fmt.Fprint(w, "data: [DONE]\n\n")
	flush(w)
}

func mustJSON(v any) string {
	enc, _ := json.Marshal(v)
	return string(enc)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
