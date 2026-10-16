// OpenAI Responses: POST /v1/responses (streaming SSE + non-streaming).
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type respToolDef struct {
	Type       string          `json:"type"`
	Name       string          `json:"name"`
	Parameters json.RawMessage `json:"parameters"`
}

func respInputText(item json.RawMessage) (role, text string, isOutput bool, output string) {
	var probe struct {
		Type    string          `json:"type"`
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
		Input   json.RawMessage `json:"input"`
		Output  json.RawMessage `json:"output"`
		Text    string          `json:"text"`
	}
	if err := json.Unmarshal(item, &probe); err != nil {
		return "", "", false, ""
	}
	switch probe.Type {
	case "function_call_output":
		return "", "", true, rawToText(probe.Output)
	case "message":
		return probe.Role, contentPartsText(probe.Content), false, ""
	}
	if probe.Role != "" {
		// Plain {role, content} item.
		return probe.Role, contentPartsText(probe.Content), false, ""
	}
	return "", "", false, ""
}

func rawToText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return string(raw)
}

type respCall struct {
	Name string
	Args string
}

type respIDs struct {
	fc   string
	call string
}

func contentPartsText(raw json.RawMessage) string {
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
			b.WriteString(p.Text)
		}
		return b.String()
	}
	return ""
}

func (s *Server) handleResponses(w http.ResponseWriter, r *http.Request) {
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
	var req struct {
		Model     string          `json:"model"`
		Input     json.RawMessage `json:"input"`
		Tools     []respToolDef   `json:"tools"`
		Stream    bool            `json:"stream"`
		Reasoning json.RawMessage `json:"reasoning"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON"})
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

	lastUser, toolResults, lastUserIdx, lastOutIdx := respLastUser(req.Input)
	d := ParseDirectives(lastUser)
	if lastOutIdx > lastUserIdx && len(toolResults) > 0 {
		d = Directives{Raw: lastUser}
	}
	model := req.Model
	if model == "" {
		model = "fake-small"
	}
	wantReason := len(req.Reasoning) > 0 && string(req.Reasoning) != "null"
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
		writeHTTPFault(w, "openai", fault, model)
		return
	}
	switch fault {
	case faultHang:
		if req.Stream {
			hangOpen(w, r, "event: response.created\ndata: "+mustJSON(respShell(model))+"\n\n")
		} else {
			hangNonStream(w, r)
		}
		return
	case faultCrash:
		if req.Stream {
			crashConn(w, r, "event: response.created\ndata: "+mustJSON(respShell(model))+"\n\n")
		} else {
			hj, ok := w.(http.Hijacker)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"id":"resp_fake","object":"response","created_at":`)
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
			sseEvent(w, "response.created", respShell(model))
			fmt.Fprint(w, `data: {"error":{"message":"stream interrupted","type":"server_error"}}`+"\n\n")
			flush(w)
		} else {
			writeHTTPFault(w, "openai", faultHTTP500, model)
		}
		return
	case faultTruncated:
		if req.Stream {
			writeSSEHeader(w)
			fmt.Fprint(w, "event: response.created\ndata: {\"id\": \"resp_fake\", \"object\":")
			flush(w)
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"id":"resp_fake","object":"response","created_at":`)
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
			s.streamRespText(w, r, model, "", wantReason, d)
		} else {
			s.writeRespText(w, r, model, "", wantReason, lastUser)
		}
		return
	case faultLength:
		if req.Stream {
			s.streamRespLength(w, r, model, d)
		} else {
			s.writeRespLength(w, model, lastUser)
		}
		return
	}
	if fault == faultTTFB {
		if !streamFaultTTFB(r, d.TtfbMs) {
			return
		}
	}

	var calls []respCall
	for _, t := range d.Tools {
		if toolSet[t.Name] {
			calls = append(calls, respCall{t.Name, string(t.Args)})
		}
	}
	if len(calls) == 0 && len(d.Tools) == 0 && len(toolResults) == 0 &&
		!d.HasEcho && !d.HasBig && !d.HasSlow && d.Fault == "" && !d.Hang && !d.Crash &&
		s.cooperateEnabled(r) && lastUser != "" {
		if name := matchCooperate(lastUser, toolNames); name != "" {
			var props map[string]json.RawMessage
			for _, td := range req.Tools {
				if td.Name == name && len(td.Parameters) > 0 {
					var o struct {
						Properties map[string]json.RawMessage `json:"properties"`
					}
					if json.Unmarshal(td.Parameters, &o) == nil {
						props = o.Properties
					}
					break
				}
			}
			calls = append(calls, respCall{name, cooperateArgs(name, props, lastUser)})
		}
	}
	if len(d.Tools) > 0 && len(calls) == 0 {
		reply := "MISSING_TOOL:" + d.Tools[0].Name
		if req.Stream {
			s.streamRespText(w, r, model, reply, wantReason, d)
		} else {
			s.writeRespText(w, r, model, reply, wantReason, lastUser)
		}
		return
	}
	if len(calls) > 0 {
		idList := make([]respIDs, len(calls))
		for i := range calls {
			idList[i] = respIDs{s.nextToolID("fc_"), s.nextToolID("call_")}
		}
		if req.Stream {
			s.streamRespTools(w, r, model, calls, idList, d)
		} else {
			s.writeRespTools(w, r, model, calls, idList, lastUser)
		}
		return
	}
	reply := d.ReplyText(toolResults)
	if req.Stream {
		s.streamRespText(w, r, model, reply, wantReason, d)
		return
	}
	s.writeRespText(w, r, model, reply, wantReason, lastUser)
}

func respLastUser(input json.RawMessage) (string, []string, int, int) {
	if len(input) == 0 || string(input) == "null" {
		return "", nil, -1, -1
	}
	var s string
	if err := json.Unmarshal(input, &s); err == nil {
		return s, nil, 0, -1
	}
	var items []json.RawMessage
	if err := json.Unmarshal(input, &items); err != nil {
		return "", nil, -1, -1
	}
	var lastUser string
	var results []string
	lastUserIdx, lastOutIdx := -1, -1
	for i, it := range items {
		role, text, isOut, out := respInputText(it)
		if isOut {
			lastOutIdx = i
			results = append(results, out)
			continue
		}
		if role == "user" && text != "" {
			lastUser = text
			lastUserIdx = i
		}
	}
	return lastUser, results, lastUserIdx, lastOutIdx
}

func respShell(model string) map[string]any {
	return map[string]any{"id": "resp_fake", "object": "response", "created_at": time.Now().Unix(), "model": model}
}

func (s *Server) writeRespText(w http.ResponseWriter, r *http.Request, model, reply string, wantReason bool, prompt string) {
	if d := ParseDirectives(prompt); d.HasSlow {
		// Note: prompt here is the raw last-user text; slow applies too.
		select {
		case <-r.Context().Done():
			return
		case <-time.After(time.Duration(d.SlowMs) * time.Millisecond):
		}
	}
	output := []any{}
	if wantReason {
		output = append(output, map[string]any{
			"type": "reasoning", "id": "rs_fake",
			"summary": []any{map[string]any{"type": "summary_text", "text": "THINKING: plan for " + truncate(reply, 120)}},
		})
	}
	output = append(output, map[string]any{
		"type": "message", "id": "msg_fake", "role": "assistant",
		"content": []any{map[string]any{"type": "output_text", "text": reply}},
	})
	pt := countTokens(prompt) + 8
	ct := countTokens(reply) + 3
	writeJSON(w, http.StatusOK, map[string]any{
		"id": "resp_fake", "object": "response", "created_at": time.Now().Unix(),
		"model": model, "status": "completed", "output": output,
		"usage": map[string]any{"input_tokens": pt, "output_tokens": ct, "total_tokens": pt + ct},
	})
}

func (s *Server) writeRespLength(w http.ResponseWriter, model, prompt string) {
	pt := countTokens(prompt) + 8
	writeJSON(w, http.StatusOK, map[string]any{
		"id": "resp_fake", "object": "response", "created_at": time.Now().Unix(),
		"model": model, "status": "incomplete", "incomplete_details": map[string]any{"reason": "max_output_tokens"},
		"output": []any{map[string]any{"type": "message", "id": "msg_fake", "role": "assistant",
			"content": []any{map[string]any{"type": "output_text", "text": "partial-"}}}},
		"usage": map[string]any{"input_tokens": pt, "output_tokens": 3, "total_tokens": pt + 3},
	})
}

func (s *Server) writeRespTools(w http.ResponseWriter, r *http.Request, model string, calls []respCall, idList []respIDs, prompt string) {
	output := []any{}
	argTokens := 0
	for i, c := range calls {
		argTokens += countTokens(c.Args)
		output = append(output, map[string]any{
			"type": "function_call", "id": idList[i].fc,
			"call_id": idList[i].call, "name": c.Name, "arguments": c.Args,
		})
	}
	pt := countTokens(prompt) + 8
	ct := 5 + argTokens
	writeJSON(w, http.StatusOK, map[string]any{
		"id": "resp_fake", "object": "response", "created_at": time.Now().Unix(),
		"model": model, "status": "completed", "output": output,
		"usage": map[string]any{"input_tokens": pt, "output_tokens": ct, "total_tokens": pt + ct},
	})
}

func (s *Server) streamRespText(w http.ResponseWriter, r *http.Request, model, reply string, wantReason bool, d Directives) {
	writeSSEHeader(w)
	sseEvent(w, "response.created", respShell(model))
	if wantReason {
		sseEvent(w, "response.reasoning_summary_text.delta", map[string]any{"delta": "THINKING: plan"})
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
		sseEvent(w, "response.output_text.delta", map[string]any{"delta": ch})
		if dup {
			sseEvent(w, "response.output_text.delta", map[string]any{"delta": ch})
		}
	}
	if stallAfter == len(chunks) {
		if !stallPoint(r, d) {
			return
		}
	}
	sseEvent(w, "response.completed", map[string]any{
		"id": "resp_fake", "object": "response", "model": model, "status": "completed",
		"output": []any{map[string]any{"type": "message", "role": "assistant",
			"content": []any{map[string]any{"type": "output_text", "text": reply}}}},
	})
}

func (s *Server) streamRespLength(w http.ResponseWriter, r *http.Request, model string, d Directives) {
	writeSSEHeader(w)
	sseEvent(w, "response.created", respShell(model))
	sseEvent(w, "response.output_text.delta", map[string]any{"delta": "partial-"})
	sseEvent(w, "response.incomplete", map[string]any{
		"id": "resp_fake", "object": "response", "model": model, "status": "incomplete",
	})
}

func (s *Server) streamRespTools(w http.ResponseWriter, r *http.Request, model string, calls []respCall, idList []respIDs, d Directives) {
	writeSSEHeader(w)
	sseEvent(w, "response.created", respShell(model))
	for i, c := range calls {
		if !sleepChunks(r, d.SlowMs) {
			return
		}
		select {
		case <-r.Context().Done():
			return
		default:
		}
		sseEvent(w, "response.output_item.added", map[string]any{
			"output_index": i,
			"item":         map[string]any{"type": "function_call", "id": idList[i].fc, "name": c.Name},
		})
		sseEvent(w, "response.function_call_arguments.delta", map[string]any{
			"output_index": i, "delta": c.Args,
		})
	}
	sseEvent(w, "response.completed", map[string]any{
		"id": "resp_fake", "object": "response", "model": model, "status": "completed",
	})
}
