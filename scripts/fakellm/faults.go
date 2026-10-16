// Faults: failure-injection classes shared by every protocol.
//
// A fault can be triggered three ways (first match wins):
//  1. inline directive in the last user message, e.g. "FAULT:500"
//  2. --faults by-counter rule: request N (1-based, global LLM order) fails
//  3. --fault-rate p with --fault-seed: each request fails with probability p
//
// Every injected fault is counted in /__stats by_fault and logged in the
// JSONL log as "fault", so a benchmark can correlate agent behaviour.
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Canonical fault classes.
const (
	faultHTTP429    = "http429"
	faultHTTP500    = "http500"
	faultHTTP502    = "http502"
	faultHTTP503    = "http503"
	faultHTTP529    = "http529"
	faultHTTP401    = "http401"
	faultSSEError   = "sse_error"
	faultStall      = "stall"
	faultTTFB       = "ttfb"
	faultTruncated  = "truncated"
	faultInvalidSSE = "invalid_sse"
	faultReset      = "reset"
	faultEmpty      = "empty"
	faultLength     = "length"
	faultDuplicate  = "duplicate"
	faultHang       = "hang"
	faultCrash      = "crashconn"
)

// randomFaultClasses is the soak pool for --fault-rate.
var randomFaultClasses = []string{
	faultHTTP429, faultHTTP500, faultHTTP502, faultHTTP503,
	faultHTTP529, faultHTTP401, faultSSEError, faultStall,
	faultTTFB, faultTruncated, faultInvalidSSE, faultReset,
	faultEmpty, faultLength, faultDuplicate,
}

// canonicalFault normalises aliases to a canonical class, "" if unknown.
func canonicalFault(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "429", "http429", "http_429", "rate_limit", "ratelimit", "err429", "fault429":
		return faultHTTP429
	case "500", "http500", "err500", "internal":
		return faultHTTP500
	case "502", "http502", "err502", "bad_gateway":
		return faultHTTP502
	case "503", "http503", "err503", "unavailable":
		return faultHTTP503
	case "529", "http529", "err529", "overloaded", "overloaded_error":
		return faultHTTP529
	case "401", "http401", "err401", "unauthorized", "invalid_api_key":
		return faultHTTP401
	case "sse_error", "sseerror", "stream_error", "midstream", "mid_stream",
		"streamerror", "sse-error", "stream-error":
		return faultSSEError
	case "stall", "stall_tokens", "stall-tokens", "pause", "stallafter":
		return faultStall
	case "hang":
		return faultHang
	case "ttfb", "slow_first_byte", "slow-first-byte", "first_byte":
		return faultTTFB
	case "truncated", "truncated_json", "truncated-json", "trunc", "cutoff":
		return faultTruncated
	case "invalid_sse", "invalid-sse", "bad_sse", "badsse", "invalidsse":
		return faultInvalidSSE
	case "reset", "rst", "resetconn", "connreset", "crashconn", "crash_conn", "crash":
		return faultCrash
	case "empty", "empty_completion", "empty-completion", "zero":
		return faultEmpty
	case "length", "finish_length", "max_tokens", "too_long":
		return faultLength
	case "duplicate", "dup", "dup_chunks", "duplicate_chunks", "dupchunks":
		return faultDuplicate
	}
	return ""
}

// isHTTPFault reports whether the class is a plain HTTP status error.
func isHTTPFault(f string) bool {
	switch f {
	case faultHTTP429, faultHTTP500, faultHTTP502, faultHTTP503, faultHTTP529, faultHTTP401:
		return true
	}
	return false
}

// parseFaultsConfig parses --faults: a JSON object mapping 1-based request
// number to fault class ({"3":"http429"}), a {"faults":{...}} or
// {"rules":[{"at":3,"fault":"http429"}]} wrapper, or a JSON array of
// {"at":N,"fault":...} rules. A plain file path is read first.
func parseFaultsConfig(s string) (map[int]string, error) {
	out := map[int]string{}
	t := strings.TrimSpace(s)
	if t == "" {
		return out, nil
	}
	if b, err := os.ReadFile(t); err == nil {
		t = strings.TrimSpace(string(b))
	}
	var asMap map[string]string
	if err := json.Unmarshal([]byte(t), &asMap); err == nil {
		for k, v := range asMap {
			n, err := strconv.Atoi(strings.TrimSpace(k))
			if err != nil || n <= 0 {
				continue
			}
			if c := canonicalFault(v); c != "" {
				out[n] = c
			}
		}
		if len(out) > 0 || len(asMap) == 0 {
			return out, nil
		}
	}
	var wrapped struct {
		Faults map[string]string `json:"faults"`
		Rules  []struct {
			At    int    `json:"at"`
			Nth   int    `json:"nth"`
			Fault string `json:"fault"`
		} `json:"rules"`
	}
	if err := json.Unmarshal([]byte(t), &wrapped); err == nil {
		for k, v := range wrapped.Faults {
			n, err := strconv.Atoi(strings.TrimSpace(k))
			if err != nil || n <= 0 {
				continue
			}
			if c := canonicalFault(v); c != "" {
				out[n] = c
			}
		}
		for _, r := range wrapped.Rules {
			n := r.At
			if n == 0 {
				n = r.Nth
			}
			if n <= 0 {
				continue
			}
			if c := canonicalFault(r.Fault); c != "" {
				out[n] = c
			}
		}
		return out, nil
	}
	var arr []struct {
		At    int    `json:"at"`
		Nth   int    `json:"nth"`
		Fault string `json:"fault"`
	}
	if err := json.Unmarshal([]byte(t), &arr); err == nil {
		for _, r := range arr {
			n := r.At
			if n == 0 {
				n = r.Nth
			}
			if n <= 0 {
				continue
			}
			if c := canonicalFault(r.Fault); c != "" {
				out[n] = c
			}
		}
		return out, nil
	}
	return nil, fmt.Errorf("faults: expected {\"3\":\"http500\"} or rules array")
}

// selectFault resolves the fault for one request. Directive faults win,
// then the by-counter rule for this request number, then the seeded
// fault-rate roll. Returns "" when the request must proceed normally.
func (s *Server) selectFault(d Directives, seq uint64) string {
	if d.Fault != "" {
		return d.Fault
	}
	if d.Hang {
		return faultHang
	}
	if d.Crash {
		return faultCrash
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if f, ok := s.faultsByN[int(seq)]; ok {
		return f
	}
	if s.faultRate > 0 && s.rng != nil {
		if s.rng.Float64() < s.faultRate {
			return randomFaultClasses[s.rng.Intn(len(randomFaultClasses))]
		}
	}
	return ""
}

// writeHTTPFault writes a status-code fault in the shape each protocol
// family expects. Anthropic (claude) endpoints use the Anthropic error
// envelope (notably 529 overloaded_error); the rest use an OpenAI-style
// {"error": {...}} body. 429 always carries Retry-After.
func writeHTTPFault(w http.ResponseWriter, protocol, fault, model string) {
	status := 500
	msg := "internal server error"
	typ := "server_error"
	switch fault {
	case faultHTTP429:
		status, msg, typ = 429, "rate limit exceeded", "rate_limit_error"
	case faultHTTP500:
		status, msg, typ = 500, "internal server error", "server_error"
	case faultHTTP502:
		status, msg, typ = 502, "bad gateway", "server_error"
	case faultHTTP503:
		status, msg, typ = 503, "service unavailable", "server_error"
	case faultHTTP529:
		status, msg, typ = 529, "Overloaded", "overloaded_error"
	case faultHTTP401:
		status, msg, typ = 401, "invalid api key", "authentication_error"
	}
	if fault == faultHTTP429 {
		w.Header().Set("Retry-After", "1")
	}
	if protocol == "anthropic" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		enc, _ := json.Marshal(map[string]any{
			"type": "error",
			"error": map[string]any{
				"type":    typ,
				"message": msg,
			},
		})
		w.Write(enc)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc, _ := json.Marshal(map[string]any{
		"error": map[string]any{
			"message": msg,
			"type":    typ,
			"code":    status,
		},
	})
	w.Write(enc)
}

// streamFaultTTFB sleeps before the first byte, honouring cancel.
func streamFaultTTFB(r *http.Request, ms int) bool {
	if ms <= 0 {
		ms = 300
	}
	select {
	case <-r.Context().Done():
		return false
	case <-time.After(time.Duration(ms) * time.Millisecond):
		return true
	}
}

// stallPoint runs the stall fault: emit nothing for D ms, then resume.
// Caller emits the first N pieces, calls this, then emits the rest.
func stallPoint(r *http.Request, d Directives) bool {
	ms := d.StallMs
	if ms <= 0 {
		ms = 500
	}
	select {
	case <-r.Context().Done():
		return false
	case <-time.After(time.Duration(ms) * time.Millisecond):
		return true
	}
}
