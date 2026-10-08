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
// scripted Events; real Backends (pig in ticket 03, claude in ticket 04)
// drive the harness SDK. Backends are registered per harness; the default
// Backend serves any harness without one.
type Backend interface {
	// OnPrompt runs after the prompt Event is stored. It appends follow-up
	// Events through appendEvent.
	OnPrompt(sessionID, text string, appendEvent func(kind, text, status string))
	// CreateSession allocates the harness Session; emit streams its Events.
	// An error rejects the create with a 400.
	CreateSession(meta SessionMeta, emit func(kind, text, status string)) error
	// CloseSession releases the harness Session without deleting history.
	CloseSession(sessionID string)
	// Models lists the harness's runnable models.
	Models() []string
	// ReleaseSession releases the agent process without deleting history
	// and returns the token ResumeSession needs ("" when none).
	ReleaseSession(sessionID string) string
	// ResumeSession reallocates the agent process for a released Session
	// using meta.ResumeToken.
	ResumeSession(meta SessionMeta, emit func(kind, text, status string)) error
	// ResumeToken reports the current live handle for persistence.
	ResumeToken(sessionID string) string
	// ResumeOK reports whether a released Session can take a new prompt,
	// with the why-not when it cannot.
	ResumeOK(meta SessionMeta) (bool, string)
}

// Daemon owns every Session, writes Event logs and serves the API.
type Daemon struct {
	stateDir string
	backend  Backend
	claude   *ClaudeBackend
	// maxRunning overrides the max_workers setting when positive.
	maxRunning int
	// starting counts Sessions reserving a running slot while their
	// backend handle builds before publish.
	starting int
	mu       sync.Mutex
	sessions map[string]*session
	backends map[string]Backend
}
type session struct {
	meta   SessionMeta
	events []Event
	notify chan struct{}
	last   time.Time
	// live tells whether the backend holds an agent process. Released
	// and never-started Sessions keep history only.
	live bool
	// active counts launched turns still running. A release waits for
	// zero, so it never drops a process a new prompt just took.
	active int
}

// NewDaemon loads persisted Sessions from stateDir/enderbloom/sessions.
// Loaded Sessions hold history only; a new prompt resumes them.
func NewDaemon(stateDir string, backend Backend) *Daemon {
	d := &Daemon{
		stateDir: stateDir,
		backend:  backend,
		sessions: map[string]*session{},
		backends: map[string]Backend{},
	}
	d.load()
	return d
}

// RegisterBackend routes one harness to its Backend. The binary registers
// the pig Backend; tests register their own.
func (d *Daemon) RegisterBackend(harness string, be Backend) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.backends[harness] = be
}

// backendForLocked picks the harness Backend; callers hold d.mu.
func (d *Daemon) backendForLocked(harness string) Backend {
	if be, ok := d.backends[harness]; ok {
		return be
	}
	return d.backend
}

// Handler returns the full HTTP handler with the loopback guard applied.
func (d *Daemon) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", d.serveUI)
	mux.HandleFunc("/api/models", d.handleModels)
	mux.HandleFunc("/api/sessions", d.handleSessions)
	mux.HandleFunc("/api/sessions/", d.handleSessionSub)
	return guard(mux)
}

// WithClaudeBackend routes harness "claude" Sessions to the ACP backend.
// It also registers the backend so the generic harness lookup finds it.
// Without it claude Sessions have nowhere to run.
func (d *Daemon) WithClaudeBackend(b *ClaudeBackend) *Daemon {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.claude = b
	d.backends["claude"] = b
	return d
}

// handleModels lists the harness's runnable models for the new-Session form.
func (d *Daemon) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	harness := r.URL.Query().Get("harness")
	if harness == "" {
		http.Error(w, "harness is required", http.StatusBadRequest)
		return
	}
	if harness == "claude" {
		writeJSON(w, http.StatusOK, map[string]any{"models": ClaudeModels()})
		return
	}
	if harness != "pig" && harness != "fake" {
		http.Error(w, "unknown harness (want claude, pig or fake)", http.StatusBadRequest)
		return
	}
	d.mu.Lock()
	be := d.backendForLocked(harness)
	d.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"models": be.Models()})
}

func validClaudeModel(model string) bool {
	for _, m := range ClaudeModels() {
		if m == model {
			return true
		}
	}
	return false
}

// removeSession drops a Session that failed to start, with its log and any
// backend state, so a failed claude start leaks no adapter process.
func (d *Daemon) removeSession(id string) {
	d.mu.Lock()
	delete(d.sessions, id)
	d.mu.Unlock()
	if d.claude != nil {
		d.claude.DropSession(id)
	}
	_ = os.RemoveAll(d.sessionDir(id))
}

// capMessage refuses a start over the concurrency cap. It names the setting
// the cap came from so the message tells the user where to raise it.
func capMessage(running, max int) string {
	return fmt.Sprintf("session cap reached: %d running (max %d from max_workers); end a Session or wait for one to go idle", running, max)
}

// persistResumeToken stores the backend's live handle so a restart or a
// later prompt can resume from it.
func (d *Daemon) persistResumeToken(be Backend, id string) {
	if be == nil {
		return
	}
	tok := be.ResumeToken(id)
	if tok == "" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if s, ok := d.sessions[id]; ok {
		s.meta.ResumeToken = tok
		d.persistMeta(s.meta)
	}
}

// resumeBackendLocked picks the Backend a prompt resumes through. Claude
// Sessions need the wired claude Backend; without it only history reads.
// Callers hold d.mu.
func (d *Daemon) resumeBackendLocked(meta SessionMeta) (Backend, string) {
	if meta.Harness == "claude" {
		if d.claude == nil {
			return nil, "claude backend not configured; history is read-only"
		}
		return d.claude, ""
	}
	return d.backendForLocked(meta.Harness), ""
}

// resumeViewLocked reports whether a new prompt would be accepted now: live
// Sessions take one, ended Sessions never do, and released Sessions defer
// to the Backend. Callers hold d.mu.
func (d *Daemon) resumeViewLocked(s *session) (bool, string) {
	if s.meta.Status == "ended" {
		return false, "session ended; history is read-only"
	}
	if s.live {
		return true, ""
	}
	be, unwired := d.resumeBackendLocked(s.meta)
	if unwired != "" {
		return false, unwired
	}
	return be.ResumeOK(s.meta)
}

// finishTurn releases the agent process once the Session settles at idle or
// failed with no turn left running, so settled Sessions hold history only
// and a retry resumes from the log. The prompt path counts each launched
// turn in active under the lock; a release that finds a turn still running
// leaves the process alone, which closes the race where a new prompt lands
// while the old turn's status still reads idle.
func (d *Daemon) finishTurn(id string) {
	d.mu.Lock()
	s, ok := d.sessions[id]
	if ok && s.active > 0 {
		s.active--
	}
	if !ok || s.active > 0 || !s.live || (s.meta.Status != "idle" && s.meta.Status != "failed") {
		d.mu.Unlock()
		return
	}
	s.live = false
	be := d.backendForLocked(s.meta.Harness)
	if s.meta.Harness == "claude" && d.claude != nil {
		be = d.claude
	}
	d.mu.Unlock()
	if be == nil {
		return
	}
	tok := be.ReleaseSession(id)
	d.mu.Lock()
	defer d.mu.Unlock()
	s, ok = d.sessions[id]
	if !ok {
		return
	}
	// A prompt that resumed meanwhile already flipped live back on; its
	// ResumeSession owns the token now, so only store when still released.
	if !s.live && tok != "" {
		s.meta.ResumeToken = tok
		d.persistMeta(s.meta)
	}
}

func (d *Daemon) appendEvent(id, kind, text, status string) Event {
	d.mu.Lock()
	defer d.mu.Unlock()
	s, ok := d.sessions[id]
	if !ok {
		return Event{}
	}
	// Ended is terminal: a late turn edge (failed after a DELETE closed the
	// SDK session) must never reopen the Session.
	if kind == "status" && s.meta.Status == "ended" {
		status = "ended"
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
	// Persist under the lock so concurrent appends land in the log in
	// sequence order and the metadata never goes stale.
	d.persistEvent(meta, e)
	d.persistMeta(meta)
	close(notify)
	return e
}

func (d *Daemon) handleSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		d.mu.Lock()
		metas := make([]SessionMeta, 0, len(d.sessions))
		lasts := map[string]time.Time{}
		for _, s := range d.sessions {
			metas = append(metas, s.meta)
			lasts[s.meta.ID] = s.last
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
		if req.Harness == "claude" && !validClaudeModel(req.Model) {
			http.Error(w, "unknown claude model (want opus, sonnet or haiku)", http.StatusBadRequest)
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
		// The cap counts Sessions holding an agent process. Over it the
		// create is refused with a clear message.
		d.mu.Lock()
		max := d.maxRunningLocked()
		if d.liveCountLocked() >= max {
			running := d.liveCountLocked()
			d.mu.Unlock()
			writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": capMessage(running, max)})
			return
		}
		if req.Harness != "claude" {
			d.starting++
		}
		d.mu.Unlock()
		// Allocate the harness Session before publishing: a concurrent GET
		// or prompt must never see a Session with no harness handle.
		// Claude skips the generic path here: its blocking StartSession runs
		// below after publishing, so MCP tools are connected before the
		// first prompt can go out.
		if req.Harness != "claude" {
			d.mu.Lock()
			be := d.backendForLocked(req.Harness)
			d.mu.Unlock()
			if be == nil {
				d.mu.Lock()
				d.starting--
				d.mu.Unlock()
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "backend not configured"})
				return
			}
			id := meta.ID
			if err := be.CreateSession(meta, func(kind, text, status string) {
				d.appendEvent(id, kind, text, status)
			}); err != nil {
				d.mu.Lock()
				d.starting--
				d.mu.Unlock()
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			d.mu.Lock()
			d.sessions[meta.ID] = &session{meta: meta, live: true, notify: make(chan struct{}), last: time.Now()}
			d.persistMeta(meta)
			d.starting--
			d.mu.Unlock()
			d.persistResumeToken(be, meta.ID)
		} else {
			// Claude creation blocks until session/new answers, so MCP tools
			// are connected before the first prompt can go out. The Session
			// publishes live to hold its cap slot while it starts.
			d.mu.Lock()
			d.sessions[meta.ID] = &session{meta: meta, live: true, notify: make(chan struct{}), last: time.Now()}
			d.persistMeta(meta)
			cb := d.claude
			d.mu.Unlock()
			if cb == nil {
				d.removeSession(meta.ID)
				http.Error(w, "claude backend not configured", http.StatusInternalServerError)
				return
			}
			id := meta.ID
			if err := cb.CreateSession(meta, func(kind, text, status string) {
				d.appendEvent(id, kind, text, status)
			}); err != nil {
				d.removeSession(meta.ID)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			d.persistResumeToken(cb, meta.ID)
		}
		d.appendEvent(meta.ID, "status", "", "idle")
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
		live := s.live
		resumable, note := d.resumeViewLocked(s)
		d.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{
			"session": meta, "events": events,
			"live": live, "resumable": resumable, "resumeNote": note,
		})
	case sub == "prompt" && r.Method == "POST":
		var req struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Text == "" {
			http.Error(w, "text is required", http.StatusBadRequest)
			return
		}
		d.mu.Lock()
		s, ok := d.sessions[id]
		if !ok {
			d.mu.Unlock()
			http.NotFound(w, r)
			return
		}
		if s.meta.Status == "ended" {
			d.mu.Unlock()
			http.Error(w, "session ended", http.StatusGone)
			return
		}
		meta := s.meta
		needResume := !s.live
		be, unwired := d.resumeBackendLocked(meta)
		d.mu.Unlock()
		if needResume {
			// A released Session resumes from the log before the prompt
			// runs. Where the harness cannot resume, history stays
			// readable and the prompt is refused with the why.
			if unwired != "" {
				writeJSON(w, http.StatusConflict, map[string]any{"error": unwired})
				return
			}
			if be == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "backend not configured"})
				return
			}
			if okResume, reason := be.ResumeOK(meta); !okResume {
				writeJSON(w, http.StatusConflict, map[string]any{"error": reason})
				return
			}
			d.mu.Lock()
			cur, ok := d.sessions[id]
			resume := ok && !cur.live && cur.meta.Status != "ended"
			if resume {
				if d.liveCountLocked() >= d.maxRunningLocked() {
					running, max := d.liveCountLocked(), d.maxRunningLocked()
					d.mu.Unlock()
					writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": capMessage(running, max)})
					return
				}
				d.starting++
				meta = cur.meta
			}
			d.mu.Unlock()
			if resume {
				if err := be.ResumeSession(meta, func(kind, text, status string) {
					d.appendEvent(id, kind, text, status)
				}); err != nil {
					d.mu.Lock()
					d.starting--
					d.mu.Unlock()
					writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
					return
				}
				d.mu.Lock()
				d.starting--
				if cur, ok := d.sessions[id]; ok && !cur.live {
					cur.live = true
					if tok := be.ResumeToken(id); tok != "" {
						cur.meta.ResumeToken = tok
					}
					d.persistMeta(cur.meta)
				}
				d.mu.Unlock()
			}
		}
		// Final gate under the lock: a concurrent end or a resume that
		// lost its race must not launch a turn on a dead process. Live
		// turns count in active so finishTurn never drops their process.
		d.mu.Lock()
		s, ok = d.sessions[id]
		if !ok {
			d.mu.Unlock()
			http.NotFound(w, r)
			return
		}
		if s.meta.Status == "ended" {
			d.mu.Unlock()
			http.Error(w, "session ended", http.StatusGone)
			return
		}
		if !s.live {
			d.mu.Unlock()
			writeJSON(w, http.StatusConflict, map[string]any{"error": "session is resuming; send the prompt again"})
			return
		}
		if be == nil {
			d.mu.Unlock()
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "backend not configured"})
			return
		}
		s.active++
		d.mu.Unlock()
		e := d.appendEvent(id, "prompt", req.Text, "")
		if be != nil {
			go func() {
				be.OnPrompt(id, req.Text, func(kind, text, status string) {
					d.appendEvent(id, kind, text, status)
				})
				// The turn is over: release the agent process so idle
				// Sessions hold no memory. The next prompt resumes it.
				d.finishTurn(id)
			}()
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"seq": e.Seq})
	case sub == "" && r.Method == "DELETE":
		// Mark ended under the lock so two concurrent DELETEs emit one
		// ended Event; the slow CloseSession runs outside the lock.
		d.mu.Lock()
		s, ok := d.sessions[id]
		if !ok {
			d.mu.Unlock()
			http.NotFound(w, r)
			return
		}
		if s.meta.Status == "ended" {
			m := s.meta
			d.mu.Unlock()
			writeJSON(w, http.StatusOK, m)
			return
		}
		s.meta.Status = "ended"
		wasLive := s.live
		s.live = false
		d.persistMeta(s.meta)
		be := d.backendForLocked(s.meta.Harness)
		d.mu.Unlock()
		if wasLive && be != nil {
			be.CloseSession(id)
		}
		d.appendEvent(id, "status", "", "ended")
		d.mu.Lock()
		m := d.sessions[id].meta
		d.mu.Unlock()
		writeJSON(w, http.StatusOK, m)
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
	// Atomic write: a crash mid-write never leaves a corrupt meta.json
	// that load would silently drop.
	tmp, err := os.CreateTemp(dir, "meta-*.json")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return
	}
	_ = os.Rename(tmpName, filepath.Join(dir, "meta.json"))
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
		// Order by sequence number so a log written under concurrency
		// still replays in order; malformed lines are skipped above.
		sort.Slice(events, func(i, j int) bool { return events[i].Seq < events[j].Seq })
		// Key by the metadata ID so the map key always matches meta.ID.
		key := meta.ID
		if key == "" {
			key = id
		}
		d.sessions[key] = &session{meta: meta, events: events, notify: make(chan struct{}), last: meta.CreatedAt}
		if len(events) > 0 {
			d.sessions[key].last = events[len(events)-1].CreatedAt
		}
	}
}
