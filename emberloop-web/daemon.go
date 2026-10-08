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
	// groups tracks one spawn call each: its parent and members, so the
	// last member to finish delivers one grouped result as a Steer.
	groups map[string]*spawnGroup
	// runs tracks one deterministic Run each: its Tickets and their Worker
	// Sessions. submits holds every Session's Worker submits by Session id,
	// which the driver loop reads for merges and checkbox ticks.
	runs    map[string]*runState
	submits map[string]*submitRec
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
	// busy marks a turn in flight. Messages arriving while busy queue
	// in pending (fake and claude) or in the harness native queue (pig)
	// and deliver after the turn. Only the turn runner mutates these
	// under d.mu.
	busy    bool
	pending []queuedMsg
	// resuming marks a resume in flight. A second send on a released
	// Session gets 409 instead of double-resuming past the cap.
	// releasing marks a release in flight. Sends answer 409 while it
	// runs, so a stale release never kills a freshly resumed process.
	resuming  bool
	releasing bool
}

// queuedMsg is one not-yet-delivered message. Mode is prompt, steer or
// followup; degraded marks a claude Steer stored as a Follow-up.
type queuedMsg struct {
	mode     string
	text     string
	degraded bool
}

// steerBackend is the harness native queue seam. Pig implements it; the
// Daemon owns the queue for every other harness.
type steerBackend interface {
	Steer(sessionID, text string)
	FollowUp(sessionID, text string)
	QueuedMessages(sessionID string) []QueuedMessage
	ClearQueued(sessionID string) int
}

// claudeDegradeNote labels a Steer the claude adapter cannot inject
// mid-turn: the spike showed prompts queue until the turn ends, so the
// Daemon stores it as a Follow-up and says so in the transcript.
const claudeDegradeNote = "[Steer degraded to follow-up: claude delivers mid-turn prompts after the turn] "

// NewDaemon loads persisted Sessions from stateDir/enderbloom/sessions.
// Loaded Sessions hold history only; a new prompt resumes them.
func NewDaemon(stateDir string, backend Backend) *Daemon {
	d := &Daemon{
		stateDir: stateDir,
		backend:  backend,
		sessions: map[string]*session{},
		backends: map[string]Backend{},
		groups:   map[string]*spawnGroup{},
		runs:     map[string]*runState{},
		submits:  map[string]*submitRec{},
	}
	d.load()
	d.loadGroups()
	d.loadRuns()
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
	mux.HandleFunc("/api/settings", d.handleSettings)
	mux.HandleFunc("/api/sessions", d.handleSessions)
	mux.HandleFunc("/api/sessions/", d.handleSessionSub)
	mux.HandleFunc("/api/specs", d.handleSpecs)
	mux.HandleFunc("/api/specs/preview", d.handleSpecPreview)
	mux.HandleFunc("/api/runs", d.handleRuns)
	mux.HandleFunc("/api/runs/", d.handleRunSub)
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

// startError is a Session start refusal carrying its HTTP status. asJSON
// marks the {"error"} envelope; otherwise the message goes out as plain text.
type startError struct {
	code   int
	msg    string
	asJSON bool
}

func (e *startError) Error() string { return e.msg }

// startSession allocates the harness Session and publishes it live,
// enforcing the concurrency cap. It returns the owning Backend for the
// caller to run turns on. Both the create route and spawn share it, so a
// Subagent starts exactly like a Session.
func (d *Daemon) startSession(meta SessionMeta) (Backend, *startError) {
	// The cap counts Sessions holding an agent process. Over it the
	// start is refused with a clear message.
	d.mu.Lock()
	max := d.maxRunningLocked()
	if d.liveCountLocked() >= max {
		running := d.liveCountLocked()
		d.mu.Unlock()
		return nil, &startError{code: http.StatusTooManyRequests, msg: capMessage(running, max), asJSON: true}
	}
	if meta.Harness != "claude" {
		d.starting++
	}
	d.mu.Unlock()
	// Allocate the harness Session before publishing: a concurrent GET
	// or prompt must never see a Session with no harness handle.
	// Claude skips the generic path here: its blocking StartSession runs
	// below after publishing, so MCP tools are connected before the
	// first prompt can go out.
	if meta.Harness != "claude" {
		d.mu.Lock()
		be := d.backendForLocked(meta.Harness)
		d.mu.Unlock()
		if be == nil {
			d.mu.Lock()
			d.starting--
			d.mu.Unlock()
			return nil, &startError{code: http.StatusInternalServerError, msg: "backend not configured", asJSON: true}
		}
		id := meta.ID
		if err := be.CreateSession(meta, func(kind, text, status string) {
			d.appendEvent(id, kind, text, status)
		}); err != nil {
			d.mu.Lock()
			d.starting--
			d.mu.Unlock()
			return nil, &startError{code: http.StatusBadRequest, msg: err.Error()}
		}
		d.mu.Lock()
		d.sessions[meta.ID] = &session{meta: meta, live: true, notify: make(chan struct{}), last: time.Now()}
		d.persistMeta(meta)
		d.starting--
		d.mu.Unlock()
		return be, nil
	}
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
		return nil, &startError{code: http.StatusInternalServerError, msg: "claude backend not configured"}
	}
	id := meta.ID
	if err := cb.CreateSession(meta, func(kind, text, status string) {
		d.appendEvent(id, kind, text, status)
	}); err != nil {
		d.removeSession(meta.ID)
		return nil, &startError{code: http.StatusInternalServerError, msg: err.Error()}
	}
	return cb, nil
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

// finishTurn ends the current turn, drains the Daemon pending list in turn
// order, then releases the agent process once the Session settles at idle or
// failed with no turn left running, so settled Sessions hold history only
// and a retry resumes from the log. Send paths count each launched turn in
// active under the lock; a release that finds a turn still running leaves
// the process alone, which closes the race where a new message lands while
// the old turn's status still reads idle. An ended Session drops its pending
// list: ending is terminal too.
func (d *Daemon) finishTurn(id string) {
	for {
		d.mu.Lock()
		s, ok := d.sessions[id]
		if !ok {
			d.mu.Unlock()
			return
		}
		if s.active > 0 {
			s.active--
		}
		if s.meta.Status == "ended" {
			s.pending = nil
			s.busy = false
			d.mu.Unlock()
			return
		}
		if len(s.pending) > 0 {
			next := s.pending[0]
			s.pending = s.pending[1:]
			s.active++
			be := d.backendForLocked(s.meta.Harness)
			d.mu.Unlock()
			d.appendModeEvent(id, "prompt", transcriptText(next.text, next.degraded), "", modeLabel(next.mode))
			if be == nil {
				continue
			}
			be.OnPrompt(id, next.text, func(kind, text, status string) {
				d.appendEvent(id, kind, text, status)
			})
			continue
		}
		s.busy = false
		if !s.live || s.active > 0 || (s.meta.Status != "idle" && s.meta.Status != "failed") {
			d.mu.Unlock()
			return
		}
		// Flip live off and mark the release under one lock hold: a send
		// in the release window answers 409 instead of resuming a process
		// the stale ReleaseSession would then kill by id lookup.
		s.live = false
		s.releasing = true
		be := d.backendForLocked(s.meta.Harness)
		if s.meta.Harness == "claude" && d.claude != nil {
			be = d.claude
		}
		d.mu.Unlock()
		if be == nil {
			d.mu.Lock()
			if rs, ok := d.sessions[id]; ok {
				rs.releasing = false
			}
			d.mu.Unlock()
			return
		}
		tok := be.ReleaseSession(id)
		d.mu.Lock()
		s, ok = d.sessions[id]
		if !ok {
			d.mu.Unlock()
			return
		}
		s.releasing = false
		// A message that resumed meanwhile already flipped live back on; its
		// ResumeSession owns the token now, so only store when still released.
		if !s.live && tok != "" {
			s.meta.ResumeToken = tok
			d.persistMeta(s.meta)
		}
		d.mu.Unlock()
		return
	}
}

func (d *Daemon) appendEvent(id, kind, text, status string) Event {
	return d.appendModeEvent(id, kind, text, status, "")
}

func (d *Daemon) appendModeEvent(id, kind, text, status, mode string) Event {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.appendModeEventLocked(id, kind, text, status, mode)
}

// appendModeEventLocked stores one Event; callers hold d.mu.
func (d *Daemon) appendModeEventLocked(id, kind, text, status, mode string) Event {
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
		Mode:      mode,
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

// sendResult is the outcome of routing one message.
type sendResult struct {
	seq      int64
	queued   bool
	position int
}

// resumeReleased resumes a released Session's agent process before a send.
// It returns 0 when the Session is live and ready; otherwise the HTTP code
// and error text the send route answers with. The flow mirrors the create
// cap: a resume over the cap is 429, a harness that cannot resume is 409
// with the why, and history stays readable either way.
func (d *Daemon) resumeReleased(id string) (int, string) {
	d.mu.Lock()
	s, ok := d.sessions[id]
	if !ok {
		d.mu.Unlock()
		return http.StatusNotFound, "not found"
	}
	if s.meta.Status == "ended" {
		d.mu.Unlock()
		return http.StatusGone, "session ended"
	}
	if s.live {
		d.mu.Unlock()
		return 0, ""
	}
	// Single-flight: a second send on a released Session answers 409
	// instead of double-resuming past the cap or stacking loads. A send
	// in a release window answers 409 too, so the stale release never
	// kills the process the retry resumes.
	if s.resuming || s.releasing {
		d.mu.Unlock()
		return http.StatusConflict, "session is resuming; send the message again"
	}
	s.resuming = true
	meta := s.meta
	be, unwired := d.resumeBackendLocked(meta)
	d.mu.Unlock()
	// The flag clears on every exit below; the Session pointer stays
	// valid even if the entry is deleted meanwhile.
	clear := func() {
		d.mu.Lock()
		s.resuming = false
		d.mu.Unlock()
	}
	// A released Session resumes from the log before the message runs.
	// Where the harness cannot resume, history stays readable and the
	// message is refused with the why.
	if unwired != "" {
		clear()
		return http.StatusConflict, unwired
	}
	if be == nil {
		clear()
		return http.StatusInternalServerError, "backend not configured"
	}
	if okResume, reason := be.ResumeOK(meta); !okResume {
		clear()
		return http.StatusConflict, reason
	}
	d.mu.Lock()
	cur, ok := d.sessions[id]
	resume := ok && !cur.live && cur.meta.Status != "ended"
	if resume {
		if d.liveCountLocked() >= d.maxRunningLocked() {
			running, max := d.liveCountLocked(), d.maxRunningLocked()
			d.mu.Unlock()
			clear()
			return http.StatusTooManyRequests, capMessage(running, max)
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
			clear()
			return http.StatusInternalServerError, err.Error()
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
	clear()
	// Final gate: a concurrent end must not send on a dead process.
	d.mu.Lock()
	s, ok = d.sessions[id]
	if !ok {
		d.mu.Unlock()
		return http.StatusNotFound, "not found"
	}
	if s.meta.Status == "ended" {
		d.mu.Unlock()
		return http.StatusGone, "session ended"
	}
	if !s.live {
		d.mu.Unlock()
		return http.StatusConflict, "session is resuming; send the message again"
	}
	d.mu.Unlock()
	return 0, ""
}

// sendMessage resumes a released Session if needed, then routes one prompt,
// steer or follow-up. It writes the HTTP response itself. Refusals carry
// the why as {"error"} so the UI can show it.
func (d *Daemon) sendMessage(w http.ResponseWriter, r *http.Request, id, mode, text string) {
	// Scout Subagents are read-only: their spawn prompt is their only turn.
	d.mu.Lock()
	if s, ok := d.sessions[id]; ok && s.meta.Scout {
		d.mu.Unlock()
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "scout sessions are read-only"})
		return
	}
	d.mu.Unlock()
	if code, msg := d.resumeReleased(id); code != 0 {
		if code == http.StatusNotFound {
			http.NotFound(w, r)
			return
		}
		if code == http.StatusGone {
			http.Error(w, msg, code)
			return
		}
		writeJSON(w, code, map[string]any{"error": msg})
		return
	}
	res, code := d.routeLive(id, mode, text)
	if code != http.StatusAccepted {
		if code == http.StatusNotFound {
			http.NotFound(w, r)
			return
		}
		if code == http.StatusGone {
			http.Error(w, "session ended", code)
			return
		}
		writeJSON(w, code, map[string]any{"error": "session is resuming; send the message again"})
		return
	}
	writeJSON(w, code, writeSendResult(res))
}

// routeLive stores or queues one prompt, steer or follow-up on a live
// Session. Idle Sessions start a turn at once; messages arriving mid-turn
// queue: in the pig native queue for pig steers and follow-ups, in the
// Daemon pending list otherwise. A claude Steer arriving mid-turn degrades
// to a Follow-up: the adapter queues mid-turn prompts until the turn ends,
// so injection at a tool boundary is impossible. The fake Backend keeps both
// mode labels so the seam tests cover them, though it delivers both after
// the turn. Launched turns count in active so finishTurn never releases
// their process early.
func (d *Daemon) routeLive(id, mode, text string) (sendResult, int) {
	d.mu.Lock()
	s, ok := d.sessions[id]
	if !ok {
		d.mu.Unlock()
		return sendResult{}, http.StatusNotFound
	}
	if s.meta.Status == "ended" {
		d.mu.Unlock()
		return sendResult{}, http.StatusGone
	}
	if !s.live {
		d.mu.Unlock()
		return sendResult{}, http.StatusConflict
	}
	harness := s.meta.Harness
	be := d.backendForLocked(harness)
	degraded := false
	if harness == "claude" && mode == "steer" && s.busy {
		mode = "followup"
		degraded = true
	}
	// A working pig Session steers through the SDK native queue, which
	// delivers at the next tool boundary. The native call and the prompt
	// Event land under d.mu together, so the turn cannot end between
	// them and strand the message until the turn after next. Lock order
	// is d.mu before the backend lock; no Backend path takes them in
	// the reverse order. The Event lands at once so the transcript shows
	// what was queued, even if it is cleared before delivery.
	if harness == "pig" && s.busy && (mode == "steer" || mode == "followup") {
		if qb, ok := be.(steerBackend); ok {
			if mode == "steer" {
				qb.Steer(id, text)
			} else {
				qb.FollowUp(id, text)
			}
			e := d.appendModeEventLocked(id, "prompt", text, "", mode)
			d.mu.Unlock()
			return sendResult{seq: e.Seq, queued: true}, http.StatusAccepted
		}
	}
	if s.busy {
		s.pending = append(s.pending, queuedMsg{mode: mode, text: text, degraded: degraded})
		pos := len(s.pending)
		d.mu.Unlock()
		return sendResult{queued: true, position: pos}, http.StatusAccepted
	}
	s.busy = true
	s.active++
	d.mu.Unlock()
	e := d.appendModeEvent(id, "prompt", transcriptText(text, degraded), "", modeLabel(mode))
	if be != nil {
		go d.runTurn(id, be, text)
	} else {
		d.finishTurn(id)
	}
	return sendResult{seq: e.Seq}, http.StatusAccepted
}

// transcriptText prefixes a degraded claude Steer so the transcript labels
// it as a Follow-up. The backend always receives the original text.
func transcriptText(text string, degraded bool) string {
	if degraded {
		return claudeDegradeNote + text
	}
	return text
}

// modeLabel maps a route mode to the prompt Event mode. Plain prompts keep
// the field empty for log back-compatibility.
func modeLabel(mode string) string {
	if mode == "steer" || mode == "followup" {
		return mode
	}
	return ""
}

// runTurn drives one backend turn, then drains the Daemon pending list in
// turn order. Pig native queues drain inside the SDK Send, never here.
func (d *Daemon) runTurn(id string, be Backend, text string) {
	be.OnPrompt(id, text, func(kind, text, status string) {
		d.appendEvent(id, kind, text, status)
	})
	d.finishTurn(id)
	// A settled Subagent may complete its Group: the last member to finish
	// delivers one grouped result to the parent as a Steer.
	d.maybeDeliverGroup(id)
}

// listQueue reports not-yet-delivered messages.
// It merges the Daemon pending list with pig native queues.
func (d *Daemon) listQueue(id string) ([]QueuedMessage, int) {
	d.mu.Lock()
	s, ok := d.sessions[id]
	if !ok {
		d.mu.Unlock()
		return nil, http.StatusNotFound
	}
	out := make([]QueuedMessage, 0, len(s.pending))
	for _, p := range s.pending {
		out = append(out, QueuedMessage{Mode: p.mode, Text: p.text, Degraded: p.degraded})
	}
	var qb steerBackend
	if s.meta.Harness == "pig" {
		qb, _ = d.backendForLocked(s.meta.Harness).(steerBackend)
	}
	d.mu.Unlock()
	if qb != nil {
		out = append(qb.QueuedMessages(id), out...)
	}
	return out, http.StatusOK
}

// dropQueue clears not-yet-delivered messages before delivery.
// It clears the Daemon pending list and pig native queues.
func (d *Daemon) dropQueue(id string) (int, int) {
	d.mu.Lock()
	s, ok := d.sessions[id]
	if !ok {
		d.mu.Unlock()
		return 0, http.StatusNotFound
	}
	n := len(s.pending)
	s.pending = nil
	var qb steerBackend
	if s.meta.Harness == "pig" {
		qb, _ = d.backendForLocked(s.meta.Harness).(steerBackend)
	}
	d.mu.Unlock()
	if qb != nil {
		n += qb.ClearQueued(id)
	}
	return n, http.StatusOK
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
		req.WorkDir = absWorkDir(req.WorkDir)
		meta := SessionMeta{
			ID:        newID(),
			Harness:   req.Harness,
			Model:     req.Model,
			WorkDir:   req.WorkDir,
			Status:    "idle",
			CreatedAt: time.Now().UTC(),
		}
		be, serr := d.startSession(meta)
		if serr != nil {
			if serr.asJSON {
				writeJSON(w, serr.code, map[string]any{"error": serr.msg})
			} else {
				http.Error(w, serr.msg, serr.code)
			}
			return
		}
		d.persistResumeToken(be, meta.ID)
		d.appendEvent(meta.ID, "status", "", "idle")
		d.mu.Lock()
		m := d.sessions[meta.ID].meta
		d.mu.Unlock()
		writeJSON(w, http.StatusCreated, m)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// writeSendResult renders one routing outcome. Immediate turns carry the
// prompt Event seq; Daemon-queued messages carry their queue position.
func writeSendResult(res sendResult) map[string]any {
	out := map[string]any{"seq": res.seq, "queued": res.queued}
	if res.queued && res.seq == 0 {
		out["position"] = res.position
	}
	return out
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
			Mode string `json:"mode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Text == "" {
			http.Error(w, "text is required", http.StatusBadRequest)
			return
		}
		mode := req.Mode
		if mode == "" {
			mode = "prompt"
		}
		if mode != "prompt" && mode != "steer" && mode != "followup" {
			http.Error(w, "unknown mode (want prompt, steer or followup)", http.StatusBadRequest)
			return
		}
		d.sendMessage(w, r, id, mode, req.Text)
	case (sub == "steer" || sub == "followup") && r.Method == "POST":
		var req struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Text == "" {
			http.Error(w, "text is required", http.StatusBadRequest)
			return
		}
		d.sendMessage(w, r, id, sub, req.Text)
	case sub == "queue" && r.Method == "GET":
		q, code := d.listQueue(id)
		if code != http.StatusOK {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"queue": q})
	case sub == "queue" && r.Method == "DELETE":
		n, code := d.dropQueue(id)
		if code != http.StatusOK {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"cleared": n})
	case sub == "submit" && r.Method == "POST":
		d.handleSessionSubmit(w, r, id)
	case sub == "spawn" && r.Method == "POST":
		d.handleSpawn(w, r, id)
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

// absWorkDir expands a leading ~ and makes the path absolute: the claude
// adapter rejects relative working directories.
func absWorkDir(p string) string {
	p = strings.TrimSpace(p)
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}
