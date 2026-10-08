package web

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Backend drives one harness behind the Daemon. The fake Backend replays
// scripted Events; real Backends arrive in later tickets.
type Backend interface {
	// OnPrompt runs after the prompt Event is stored. It appends scripted
	// follow-up Events through appendEvent.
	OnPrompt(sessionID, text string, appendEvent func(kind, text, status string))
}

// Daemon owns every Session, writes Event logs and serves the API.
type Daemon struct {
	stateDir string
	backend  Backend

	mu       sync.Mutex
	sessions map[string]*session
}

type session struct {
	meta   SessionMeta
	events []Event
	notify chan struct{}
	last   time.Time
}

// NewDaemon loads persisted Sessions from stateDir/enderbloom/sessions.
func NewDaemon(stateDir string, backend Backend) *Daemon {
	d := &Daemon{
		stateDir: stateDir,
		backend:  backend,
		sessions: map[string]*session{},
	}
	d.load()
	return d
}

// Handler returns the full HTTP handler with the loopback guard applied.
func (d *Daemon) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", d.serveUI)
	mux.HandleFunc("/api/sessions", d.handleSessions)
	mux.HandleFunc("/api/sessions/", d.handleSessionSub)
	return guard(mux)
}

func (d *Daemon) appendEvent(id, kind, text, status string) Event {
	d.mu.Lock()
	s, ok := d.sessions[id]
	if !ok {
		d.mu.Unlock()
		return Event{}
	}
	e := Event{
		Version:   1,
		Seq:       int64(len(s.events) + 1),
		SessionID: id,
		Kind:      kind,
		Text:      text,
		Status:    status,
		CreatedAt: time.Now().UTC(),
	}
	s.events = append(s.events, e)
	s.last = time.Now()
	if kind == "status" && status != "" {
		s.meta.Status = status
	}
	notify := s.notify
	s.notify = make(chan struct{})
	meta := s.meta
	d.mu.Unlock()

	close(notify)
	d.persistEvent(meta, e)
	d.persistMeta(meta)
	return e
}

func (d *Daemon) handleSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		d.mu.Lock()
		metas := make([]SessionMeta, 0, len(d.sessions))
		lasts := map[string]time.Time{}
		for id, s := range d.sessions {
			metas = append(metas, s.meta)
			lasts[id] = s.last
		}
		d.mu.Unlock()
		sort.Slice(metas, func(i, j int) bool {
			if lasts[metas[i].ID].Equal(lasts[metas[j].ID]) {
				return metas[i].ID > metas[j].ID
			}
			return lasts[metas[i].ID].After(lasts[metas[j].ID])
		})
		writeJSON(w, http.StatusOK, metas)
	case "POST":
		var req struct {
			Harness string `json:"harness"`
			Model   string `json:"model"`
			WorkDir string `json:"workDir"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if req.Harness == "" || req.Model == "" || req.WorkDir == "" {
			http.Error(w, "harness, model and workDir are required", http.StatusBadRequest)
			return
		}
		meta := SessionMeta{
			ID:        newID(),
			Harness:   req.Harness,
			Model:     req.Model,
			WorkDir:   req.WorkDir,
			Status:    "idle",
			CreatedAt: time.Now().UTC(),
		}
		d.mu.Lock()
		d.sessions[meta.ID] = &session{meta: meta, notify: make(chan struct{}), last: time.Now()}
		d.mu.Unlock()
		d.persistMeta(meta)
		e := d.appendEvent(meta.ID, "status", "", "idle")
		_ = e
		d.mu.Lock()
		m := d.sessions[meta.ID].meta
		d.mu.Unlock()
		writeJSON(w, http.StatusCreated, m)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (d *Daemon) handleSessionSub(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	parts := strings.SplitN(rest, "/", 2)
	id := parts[0]
	sub := ""
	if len(parts) == 2 {
		sub = parts[1]
	}
	switch {
	case sub == "" && r.Method == "GET":
		d.mu.Lock()
		s, ok := d.sessions[id]
		if !ok {
			d.mu.Unlock()
			http.NotFound(w, r)
			return
		}
		meta := s.meta
		events := append([]Event(nil), s.events...)
		d.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"session": meta, "events": events})
	case sub == "prompt" && r.Method == "POST":
		var req struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Text == "" {
			http.Error(w, "text is required", http.StatusBadRequest)
			return
		}
		d.mu.Lock()
		if _, ok := d.sessions[id]; !ok {
			d.mu.Unlock()
			http.NotFound(w, r)
			return
		}
		d.mu.Unlock()
		e := d.appendEvent(id, "prompt", req.Text, "")
		if d.backend != nil {
			be := d.backend
			go be.OnPrompt(id, req.Text, func(kind, text, status string) {
				d.appendEvent(id, kind, text, status)
			})
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"seq": e.Seq})
	case sub == "events" && r.Method == "GET":
		d.serveEvents(w, r, id)
	default:
		http.NotFound(w, r)
	}
}

func (d *Daemon) serveEvents(w http.ResponseWriter, r *http.Request, id string) {
	since, _ := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
	if v := r.Header.Get("Last-Event-ID"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			since = n
		}
	}
	d.mu.Lock()
	s, ok := d.sessions[id]
	d.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, _ := w.(http.Flusher)

	pos := since
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		d.mu.Lock()
		s, ok = d.sessions[id]
		var pending []Event
		var ch chan struct{}
		if ok {
			for _, e := range s.events {
				if e.Seq > pos {
					pending = append(pending, e)
				}
			}
			ch = s.notify
		}
		d.mu.Unlock()
		if !ok {
			return
		}
		for _, e := range pending {
			pos = e.Seq
			fmt.Fprintf(w, "id: %d\ndata: %s\n\n", e.Seq, mustJSON(e))
			if flusher != nil {
				flusher.Flush()
			}
		}
		// If the client asked for a finite replay via ?wait=0, stop here.
		if r.URL.Query().Get("wait") == "0" {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			fmt.Fprintf(w, ": heartbeat\n\n")
			if flusher != nil {
				flusher.Flush()
			}
		case <-ch:
		}
	}
}

// guard rejects requests whose Host or Origin is not loopback.
func guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackHost(r.Host) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || !isLoopbackHost(u.Host) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func isLoopbackHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if host == "" {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// --- persistence: stateDir/enderbloom/sessions/{id}/meta.json + events.jsonl ---

func (d *Daemon) sessionDir(id string) string {
	return filepath.Join(d.stateDir, "enderbloom", "sessions", id)
}

func (d *Daemon) persistMeta(meta SessionMeta) {
	dir := d.sessionDir(meta.ID)
	_ = os.MkdirAll(dir, 0o755)
	b, _ := json.Marshal(meta)
	_ = os.WriteFile(filepath.Join(dir, "meta.json"), b, 0o644)
}

func (d *Daemon) persistEvent(meta SessionMeta, e Event) {
	dir := d.sessionDir(meta.ID)
	_ = os.MkdirAll(dir, 0o755)
	f, err := os.OpenFile(filepath.Join(dir, "events.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	b, _ := json.Marshal(e)
	_, _ = f.Write(append(b, '\n'))
}

func (d *Daemon) load() {
	root := filepath.Join(d.stateDir, "enderbloom", "sessions")
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, en := range entries {
		if !en.IsDir() {
			continue
		}
		id := en.Name()
		metaBytes, err := os.ReadFile(filepath.Join(root, id, "meta.json"))
		if err != nil {
			continue
		}
		var meta SessionMeta
		if err := json.Unmarshal(metaBytes, &meta); err != nil {
			continue
		}
		var events []Event
		if raw, err := os.ReadFile(filepath.Join(root, id, "events.jsonl")); err == nil {
			for _, line := range strings.Split(string(raw), "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				var e Event
				if err := json.Unmarshal([]byte(line), &e); err == nil {
					events = append(events, e)
				}
			}
		}
		d.sessions[id] = &session{meta: meta, events: events, notify: make(chan struct{}), last: meta.CreatedAt}
		if len(events) > 0 {
			d.sessions[id].last = events[len(events)-1].CreatedAt
		}
	}
}
