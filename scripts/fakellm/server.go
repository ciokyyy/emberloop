// Server: shared mux, JSONL logging, stats, and streaming helpers.
package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Server is the deterministic fake LLM server.
type Server struct {
	mu          sync.Mutex
	models      []Model
	counts      map[string]int
	modelCounts map[string]int
	faultCounts map[string]int
	lastReq     string
	catalogGets int
	latency     time.Duration

	// Process-unique tool-call sequence. Never reset: every emitted
	// call_<n>/toolu_<n>/fc_<n> id is unique for the process lifetime.
	toolSeq uint64
	// Global LLM request sequence (1-based) for --faults by-counter rules.
	reqSeq uint64

	cooperate bool
	faultsByN map[int]string
	faultRate float64
	rng       *rand.Rand

	logMu   sync.Mutex
	logFile *os.File
}

// NewServer builds a Server. logPath may be "" to disable JSONL logging.
func NewServer(models []Model, logPath string, latency time.Duration) (*Server, error) {
	s := &Server{
		models:      append([]Model(nil), models...),
		counts:      map[string]int{},
		modelCounts: map[string]int{},
		faultCounts: map[string]int{},
		faultsByN:   map[int]string{},
		rng:         rand.New(rand.NewSource(time.Now().UnixNano())),
		latency:     latency,
	}
	if logPath != "" {
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, err
		}
		s.logFile = f
	}
	return s, nil
}

// SetCooperate enables cooperative tool-calling for all requests.
func (s *Server) SetCooperate(on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cooperate = on
}

// SetFaults installs by-counter fault rules (1-based request seq -> class).
func (s *Server) SetFaults(m map[int]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.faultsByN = m
}

// SetFaultRate enables seeded random faults: each request fails with
// probability p with a random fault class.
func (s *Server) SetFaultRate(p float64, seed int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.faultRate = p
	s.rng = rand.New(rand.NewSource(seed))
}

// nextToolID returns a process-unique tool-call id with the given prefix.
func (s *Server) nextToolID(prefix string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.toolSeq++
	return fmt.Sprintf("%s%d", prefix, s.toolSeq)
}

// cooperateEnabled reports whether COOPERATE mode applies to this request:
// global --cooperate flag, X-Fake-Cooperate header, or FAKELLM_COOPERATE /
// FAKE_COOPERATE env. Header/env values "0", "false", "no", "off" disable.
func (s *Server) cooperateEnabled(r *http.Request) bool {
	if v := r.Header.Get("X-Fake-Cooperate"); v != "" {
		return truthy(v)
	}
	s.mu.Lock()
	on := s.cooperate
	s.mu.Unlock()
	if on {
		return true
	}
	if v, ok := os.LookupEnv("FAKELLM_COOPERATE"); ok && v != "" {
		return truthy(v)
	}
	if v, ok := os.LookupEnv("FAKE_COOPERATE"); ok && v != "" {
		return truthy(v)
	}
	return false
}

func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "0", "false", "no", "off", "disabled":
		return false
	}
	return true
}

// nextReqSeq assigns the next global LLM request number (1-based).
func (s *Server) nextReqSeq() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reqSeq++
	return s.reqSeq
}

// Close releases the log file, if any.
func (s *Server) Close() error {
	s.logMu.Lock()
	defer s.logMu.Unlock()
	if s.logFile != nil {
		return s.logFile.Close()
	}
	return nil
}

// Handler wires every protocol route onto one mux.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", s.handleChat)
	mux.HandleFunc("/v1/responses", s.handleResponses)
	mux.HandleFunc("/v1/messages", s.handleAnthropic)
	mux.HandleFunc("/v1beta/models/", s.handleGemini)
	mux.HandleFunc("/v1/models/", s.handleGemini)
	mux.HandleFunc("/v1/models", s.handleOpenAIModels)
	mux.HandleFunc("/api/v1/models", s.handleOpenRouterModels)
	mux.HandleFunc("/api.json", s.handleModelsDev)
	mux.HandleFunc("/api/models/providers/", s.handlePiProvider)
	mux.HandleFunc("/__catalog", s.handleCatalog)
	mux.HandleFunc("/__stats", s.handleStats)
	return mux
}

// record logs one request as JSONL and bumps the counters.
// fault is the injected fault class or "" for none.
func (s *Server) record(r *http.Request, model string, tools []string, stream bool, fault string) {
	if tools == nil {
		tools = []string{}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	s.mu.Lock()
	s.counts[r.URL.Path]++
	if model != "" {
		s.modelCounts[model]++
	}
	if fault != "" {
		s.faultCounts[fault]++
	}
	s.lastReq = now
	s.mu.Unlock()
	if s.logFile == nil {
		return
	}
	_, hasAuth := r.Header["Authorization"]
	entry := map[string]any{
		"ts":     now,
		"method": r.Method,
		"path":   r.URL.Path,
		"model":  model,
		"tools":  tools,
		"auth":   hasAuth,
		"ua":     r.Header.Get("User-Agent"),
		"stream": stream,
		"fault":  fault,
	}
	enc, _ := json.Marshal(entry)
	s.logMu.Lock()
	defer s.logMu.Unlock()
	s.logFile.Write(append(enc, '\n'))
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	byPath := map[string]int{}
	for k, v := range s.counts {
		byPath[k] = v
	}
	byModel := map[string]int{}
	for k, v := range s.modelCounts {
		byModel[k] = v
	}
	byFault := map[string]int{}
	for k, v := range s.faultCounts {
		byFault[k] = v
	}
	last := s.lastReq
	catGets := s.catalogGets
	s.mu.Unlock()
	total := 0
	for _, v := range byPath {
		total += v
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total":           total,
		"by_path":         byPath,
		"by_model":        byModel,
		"by_fault":        byFault,
		"catalog_fetches": catGets,
		"last_request":    last,
	})
}

// applyLatency sleeps the configured base latency, honouring cancel.
func (s *Server) applyLatency(r *http.Request) bool {
	if s.latency <= 0 {
		return true
	}
	select {
	case <-r.Context().Done():
		return false
	case <-time.After(s.latency):
		return true
	}
}

// sleepChunks sleeps ms between streamed chunks, honouring cancel.
func sleepChunks(r *http.Request, ms int) bool {
	if ms <= 0 {
		return true
	}
	select {
	case <-r.Context().Done():
		return false
	case <-time.After(time.Duration(ms) * time.Millisecond):
		return true
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc, _ := json.Marshal(v)
	w.Write(enc)
}

func writeSSEHeader(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
}

func flush(w http.ResponseWriter) {
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// sseData writes one `data: <json>` SSE frame.
func sseData(w http.ResponseWriter, v any) {
	enc, _ := json.Marshal(v)
	fmt.Fprintf(w, "data: %s\n\n", enc)
	flush(w)
}

// sseEvent writes one `event: <name>` + `data:` SSE frame.
func sseEvent(w http.ResponseWriter, event string, v any) {
	enc, _ := json.Marshal(v)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, enc)
	flush(w)
}

// hangOpen holds a streaming connection open after one chunk until the
// client goes away. Used for the HANG directive.
func hangOpen(w http.ResponseWriter, r *http.Request, first string) {
	writeSSEHeader(w)
	fmt.Fprint(w, first)
	flush(w)
	<-r.Context().Done()
}

// hangNonStream holds a non-streaming connection open with headers sent.
func hangNonStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	flush(w)
	<-r.Context().Done()
}

// crashConn writes partial bytes then closes the TCP connection with a real
// close on the live socket via hijack. Under httptest (no hijack support)
// it degrades to a truncated body.
func crashConn(w http.ResponseWriter, r *http.Request, partial string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, partial)
	flush(w)
	if hj, ok := w.(http.Hijacker); ok {
		if conn, _, err := hj.Hijack(); err == nil {
			// Best-effort RST: linger(0) forces a real TCP RST instead of
			// a graceful FIN close, then close the socket.
			if tl, ok := conn.(interface{ SetLinger(int) error }); ok {
				_ = tl.SetLinger(0)
			}
			conn.Close()
			return
		}
	}
}

// countTokens estimates realistic token counts deterministically.
func countTokens(s string) int {
	n := len([]rune(s)) / 4
	if strings.TrimSpace(s) != "" && n < 1 {
		n = 1
	}
	return n
}

// splitChunks splits reply text into deterministic stream pieces.
func splitChunks(text string) []string {
	if text == "" {
		return nil
	}
	words := strings.SplitAfter(text, " ")
	var out []string
	cur := ""
	for _, wd := range words {
		cur += wd
		if len(cur) >= 12 {
			out = append(out, cur)
			cur = ""
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	if len(out) == 0 {
		return []string{text}
	}
	return out
}
