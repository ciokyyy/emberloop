// Google Gemini: generateContent / streamGenerateContent under
// /v1beta/models/ and /v1/models/.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type gemPart struct {
	Text             string       `json:"text"`
	Thought          bool         `json:"thought"`
	FunctionCall     *gemFuncCall `json:"functionCall"`
	FunctionResponse *gemFuncResp `json:"functionResponse"`
}

type gemFuncCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

type gemFuncResp struct {
	Name     string          `json:"name"`
	Response json.RawMessage `json:"response"`
}

type gemContent struct {
	Role  string    `json:"role"`
	Parts []gemPart `json:"parts"`
}

type gemCall struct {
	Name string
	Args json.RawMessage
}

func (s *Server) handleGemini(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	// Path forms:
	//   /v1beta/models/{model}:generateContent
	//   /v1beta/models/{model}:streamGenerateContent
	// (same under /v1/models/).
	path := r.URL.Path
	stream := strings.HasSuffix(path, ":streamGenerateContent")
	if !stream && !strings.HasSuffix(path, ":generateContent") {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "unknown gemini method"})
		return
	}
	model := gemModelFromPath(path)
	if !s.applyLatency(r) {
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 16<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unreadable body"})
		return
	}
	var req struct {
		Contents []gemContent `json:"contents"`
		Tools    []struct {
			FunctionDeclarations []struct {
				Name       string          `json:"name"`
				Parameters json.RawMessage `json:"parameters"`
			} `json:"functionDeclarations"`
		} `json:"tools"`
		GenerationConfig json.RawMessage `json:"generationConfig"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON"})
		return
	}
	toolNames := []string{}
	toolSet := map[string]bool{}
	for _, t := range req.Tools {
		for _, fd := range t.FunctionDeclarations {
			if fd.Name != "" {
				toolNames = append(toolNames, fd.Name)
				toolSet[fd.Name] = true
			}
		}
	}

	lastUser, toolResults, lastTextIdx, lastResultIdx := gemLastUser(req.Contents)
	d := ParseDirectives(lastUser)
	if lastResultIdx > lastTextIdx && len(toolResults) > 0 {
		d = Directives{Raw: lastUser}
	}
	wantThink := gemWantsThinking(body)
	if model == "" {
		model = "google/gemini-fake-1"
	}
	seq := s.nextReqSeq()
	fault := s.selectFault(d, seq)
	if fault == faultStall && d.StallMs == 0 {
		d.StallAfter, d.StallMs = 1, 500
	}
	if fault == faultTTFB && d.TtfbMs == 0 {
		d.TtfbMs = 300
	}
	d.Fault = fault
	s.record(r, model, toolNames, stream, fault)

	if isHTTPFault(fault) {
		writeHTTPFault(w, "gemini", fault, model)
		return
	}
	switch fault {
	case faultHang:
		if stream {
			hangOpen(w, r, "data: "+mustJSON(gemChunk(model, "par", false))+"\n\n")
		} else {
			hangNonStream(w, r)
		}
		return
	case faultCrash:
		if stream {
			crashConn(w, r, "data: "+mustJSON(gemChunk(model, "par", false))+"\n\n")
		} else {
			hj, ok := w.(http.Hijacker)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"candidates":[{"content":{"role":`)
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
		if stream {
			writeSSEHeader(w)
			sseData(w, gemChunk(model, "par", false))
			fmt.Fprint(w, `data: {"error":{"message":"stream interrupted","code":500}}`+"\n\n")
			flush(w)
		} else {
			writeHTTPFault(w, "gemini", faultHTTP500, model)
		}
		return
	case faultTruncated:
		if stream {
			writeSSEHeader(w)
			fmt.Fprint(w, `data: {"candidates":[{"content":{"role":"model","parts":[{"text":"par`)
			flush(w)
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"candidates":[{"content":{"role":`)
			flush(w)
		}
		return
	case faultInvalidSSE:
		if stream {
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
		if stream {
			s.streamGemText(w, r, model, "", wantThink, d)
		} else {
			s.writeGemText(w, r, model, "", wantThink, lastUser)
		}
		return
	case faultLength:
		if stream {
			s.streamGemLength(w, r, model, d)
		} else {
			s.writeGemLength(w, model, lastUser, wantThink)
		}
		return
	}
	if fault == faultTTFB {
		if !streamFaultTTFB(r, d.TtfbMs) {
			return
		}
	}

	var calls []gemCall
	for _, t := range d.Tools {
		if toolSet[t.Name] {
			calls = append(calls, gemCall{t.Name, t.Args})
		}
	}
	if len(calls) == 0 && len(d.Tools) == 0 && len(toolResults) == 0 &&
		!d.HasEcho && !d.HasBig && !d.HasSlow && d.Fault == "" && !d.Hang && !d.Crash &&
		s.cooperateEnabled(r) && lastUser != "" {
		if name := matchCooperate(lastUser, toolNames); name != "" {
			var props map[string]json.RawMessage
			for _, t := range req.Tools {
				for _, fd := range t.FunctionDeclarations {
					if fd.Name == name && len(fd.Parameters) > 0 {
						var o struct {
							Properties map[string]json.RawMessage `json:"properties"`
						}
						if json.Unmarshal(fd.Parameters, &o) == nil {
							props = o.Properties
						}
					}
				}
			}
			calls = append(calls, gemCall{name, json.RawMessage(cooperateArgs(name, props, lastUser))})
		}
	}
	if len(d.Tools) > 0 && len(calls) == 0 {
		reply := "MISSING_TOOL:" + d.Tools[0].Name
		if stream {
			s.streamGemText(w, r, model, reply, wantThink, d)
		} else {
			s.writeGemText(w, r, model, reply, wantThink, lastUser)
		}
		return
	}
	if len(calls) > 0 {
		if stream {
			s.streamGemTools(w, r, model, calls, d)
		} else {
			s.writeGemTools(w, r, model, calls, lastUser)
		}
		return
	}
	reply := d.ReplyText(toolResults)
	if stream {
		s.streamGemText(w, r, model, reply, wantThink, d)
		return
	}
	s.writeGemText(w, r, model, reply, wantThink, lastUser)
}

func gemModelFromPath(path string) string {
	// .../models/{model}:verb
	i := strings.Index(path, "/models/")
	if i < 0 {
		return ""
	}
	rest := path[i+len("/models/"):]
	if j := strings.Index(rest, ":"); j >= 0 {
		rest = rest[:j]
	}
	return rest
}

func gemLastUser(contents []gemContent) (string, []string, int, int) {
	var lastUser string
	var results []string
	lastTextIdx, lastResultIdx := -1, -1
	for i, c := range contents {
		if c.Role == "" || c.Role == "user" {
			for _, p := range c.Parts {
				if p.Text != "" {
					lastUser = p.Text
					lastTextIdx = i
				}
				if p.FunctionResponse != nil {
					lastResultIdx = i
					results = append(results, string(p.FunctionResponse.Response))
				}
			}
		}
	}
	return lastUser, results, lastTextIdx, lastResultIdx
}

func gemWantsThinking(body []byte) bool {
	var m struct {
		GenerationConfig struct {
			ThinkingConfig json.RawMessage `json:"thinkingConfig"`
			Thinking       json.RawMessage `json:"thinking"`
		} `json:"generationConfig"`
	}
	if err := json.Unmarshal(body, &m); err != nil {
		return false
	}
	return len(m.GenerationConfig.ThinkingConfig) > 0 || len(m.GenerationConfig.Thinking) > 0
}

func gemChunk(model, text string, thought bool) map[string]any {
	part := map[string]any{"text": text}
	if thought {
		part["thought"] = true
	}
	pt := 8
	ct := countTokens(text)
	return map[string]any{
		"candidates": []any{map[string]any{
			"content":      map[string]any{"role": "model", "parts": []any{part}},
			"finishReason": "STOP",
			"index":        0,
		}},
		"usageMetadata": map[string]any{
			"promptTokenCount": pt, "candidatesTokenCount": ct, "totalTokenCount": pt + ct,
		},
		"modelVersion": model,
	}
}

func (s *Server) writeGemText(w http.ResponseWriter, r *http.Request, model, reply string, wantThink bool, prompt string) {
	if d := ParseDirectives(prompt); d.HasSlow {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(time.Duration(d.SlowMs) * time.Millisecond):
		}
	}
	parts := []any{}
	if wantThink {
		parts = append(parts, map[string]any{"thought": true, "text": "THINKING: plan for " + truncate(reply, 120)})
	}
	parts = append(parts, map[string]any{"text": reply})
	pt := countTokens(prompt) + 8
	ct := countTokens(reply) + 3
	writeJSON(w, http.StatusOK, map[string]any{
		"candidates": []any{map[string]any{
			"content":      map[string]any{"role": "model", "parts": parts},
			"finishReason": "STOP", "index": 0,
		}},
		"usageMetadata": map[string]any{
			"promptTokenCount": pt, "candidatesTokenCount": ct, "totalTokenCount": pt + ct,
		},
		"modelVersion": model,
	})
}

func (s *Server) writeGemLength(w http.ResponseWriter, model, prompt string, wantThink bool) {
	parts := []any{}
	if wantThink {
		parts = append(parts, map[string]any{"thought": true, "text": "THINKING: plan"})
	}
	parts = append(parts, map[string]any{"text": "partial-"})
	pt := countTokens(prompt) + 8
	writeJSON(w, http.StatusOK, map[string]any{
		"candidates": []any{map[string]any{
			"content":      map[string]any{"role": "model", "parts": parts},
			"finishReason": "MAX_TOKENS", "index": 0,
		}},
		"usageMetadata": map[string]any{
			"promptTokenCount": pt, "candidatesTokenCount": 3, "totalTokenCount": pt + 3,
		},
		"modelVersion": model,
	})
}

func (s *Server) writeGemTools(w http.ResponseWriter, r *http.Request, model string, calls []gemCall, prompt string) {
	parts := []any{}
	argTokens := 0
	for _, c := range calls {
		var v any
		_ = json.Unmarshal(c.Args, &v)
		if v == nil {
			v = map[string]any{}
		}
		argTokens += countTokens(string(c.Args))
		parts = append(parts, map[string]any{
			"functionCall": map[string]any{"name": c.Name, "args": v},
		})
	}
	pt := countTokens(prompt) + 8
	ct := 5 + argTokens
	writeJSON(w, http.StatusOK, map[string]any{
		"candidates": []any{map[string]any{
			"content":      map[string]any{"role": "model", "parts": parts},
			"finishReason": "STOP", "index": 0,
		}},
		"usageMetadata": map[string]any{
			"promptTokenCount": pt, "candidatesTokenCount": ct, "totalTokenCount": pt + ct,
		},
		"modelVersion": model,
	})
}

func (s *Server) streamGemText(w http.ResponseWriter, r *http.Request, model, reply string, wantThink bool, d Directives) {
	writeSSEHeader(w)
	if wantThink {
		sseData(w, gemChunk(model, "THINKING: plan", true))
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
		sseData(w, gemChunk(model, ch, false))
		if dup {
			sseData(w, gemChunk(model, ch, false))
		}
	}
	if stallAfter == len(chunks) {
		if !stallPoint(r, d) {
			return
		}
	}
}

func (s *Server) streamGemLength(w http.ResponseWriter, r *http.Request, model string, d Directives) {
	writeSSEHeader(w)
	pt := 8
	sseData(w, map[string]any{
		"candidates": []any{map[string]any{
			"content": map[string]any{"role": "model", "parts": []any{
				map[string]any{"text": "partial-"},
			}},
			"finishReason": "MAX_TOKENS", "index": 0,
		}},
		"usageMetadata": map[string]any{
			"promptTokenCount": pt, "candidatesTokenCount": 3, "totalTokenCount": pt + 3,
		},
		"modelVersion": model,
	})
}

func (s *Server) streamGemTools(w http.ResponseWriter, r *http.Request, model string, calls []gemCall, d Directives) {
	writeSSEHeader(w)
	for _, c := range calls {
		if !sleepChunks(r, d.SlowMs) {
			return
		}
		select {
		case <-r.Context().Done():
			return
		default:
		}
		var v any
		_ = json.Unmarshal(c.Args, &v)
		if v == nil {
			v = map[string]any{}
		}
		pt := 8
		ct := 5 + countTokens(string(c.Args))
		sseData(w, map[string]any{
			"candidates": []any{map[string]any{
				"content": map[string]any{"role": "model", "parts": []any{
					map[string]any{"functionCall": map[string]any{"name": c.Name, "args": v}},
				}},
				"finishReason": "STOP", "index": 0,
			}},
			"usageMetadata": map[string]any{
				"promptTokenCount": pt, "candidatesTokenCount": ct, "totalTokenCount": pt + ct,
			},
			"modelVersion": model,
		})
	}
}
