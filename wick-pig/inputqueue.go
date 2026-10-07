package wick

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Input queue core (input-queue ticket 01): the wrapper that owns an agent
// child also owns one queue, and is the only writer to the child PTY.
// Every sender — tell, compress handoffs, reviewer follow-ups, steering,
// late prompts — enqueues through the wrapper's control channel; the shared
// DeliverPrompt helper tries the channel first and keeps the herdr path as
// the fallback when no wrapper runs.
//
// Each item is a steer (delivered at the next tool boundary: no outstanding
// tool call) or a queued message (delivered after the turn ends). Delivery
// counts as accepted only when the transcript shows the user turn — an item
// a startup dialog swallowed stays at the head and is retried. A compress
// restart arrives as a restart request (ticket 05): the handoff pins ahead
// of the surviving queue. Later tickets own macros and the rest of raw mode;
// their channel messages (macro run, pause/resume) do not exist yet.

// QueueKind is when one queued item goes out: a steer at the next tool
// boundary, a queued message after the turn ends.
type QueueKind string

const (
	QueueKindSteer QueueKind = "steer"
	QueueKindQueue QueueKind = "queue"
)

// queueItem is one prompt waiting in the wrapper: what to send, when, and
// who asked.
type queueItem struct {
	Kind   QueueKind
	Text   string
	Source string
}

// QueueSourceHandoff marks the compress-restart handoff: the one item that
// pins ahead of everything, user messages included.
const QueueSourceHandoff = "handoff"

// inputQueue is the wrapper's FIFO of prompts: user messages go first before
// pending injections (ticket 02), preserving arrival order among themselves.
// The restart handoff pins ahead of all (ticket 05).
type inputQueue struct {
	mu       sync.Mutex
	items    []queueItem
	inFlight bool
}

func newInputQueue() *inputQueue { return &inputQueue{} }

// enqueue appends or inserts one item and reports the pending count. Text is
// collapsed to a single line on the way in, so delivery and acceptance agree
// on what the child receives. User messages (Source == "user") go behind the
// restart handoff and before pending injections.
// enqueueHandoff pins one restart handoff ahead of everything waiting, in
// arrival order among handoffs. The restart resets the pump first, so no
// item is mid-flight to the new child when this lands.
func (q *inputQueue) enqueueHandoff(text string) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	item := queueItem{Kind: QueueKindSteer, Text: collapseLine(text), Source: QueueSourceHandoff}
	idx := 0
	for idx < len(q.items) && q.items[idx].Source == QueueSourceHandoff {
		idx++
	}
	q.items = append(q.items[:idx], append([]queueItem{item}, q.items[idx:]...)...)
	return len(q.items)
}

func (q *inputQueue) enqueue(kind QueueKind, text, source string) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	item := queueItem{Kind: kind, Text: collapseLine(text), Source: source}
	if source == "user" {
		start := 0
		if q.inFlight && len(q.items) > 0 {
			start = 1
		}
		idx := start
		for idx < len(q.items) && q.items[idx].Source == QueueSourceHandoff {
			idx++
		}
		for idx < len(q.items) && q.items[idx].Source == "user" {
			idx++
		}
		q.items = append(q.items[:idx], append([]queueItem{item}, q.items[idx:]...)...)
	} else {
		q.items = append(q.items, item)
	}
	return len(q.items)
}

func (q *inputQueue) setInFlight(v bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.inFlight = v
}

// len reports how many items wait.
func (q *inputQueue) len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

// ctrlTimeout bounds one channel round trip: the socket is local, so a slow
// answer means no wrapper listens. A var so tests can shrink it.
var ctrlTimeout = 3 * time.Second

// ctrlSocketPath names one wrapper's control channel by agent identity,
// beside the agent records. lockName keeps the file name safe; a path past
// 100 bytes falls back to a hash, since unix socket paths stop near 108.
func ctrlSocketPath(agentID string) string {
	dir := filepath.Join(filepath.Dir(statePath), "agents")
	name := "ctrl-" + lockName(agentID) + ".sock"
	if len(filepath.Join(dir, name)) > 100 {
		h := fnv.New64a()
		_, _ = h.Write([]byte(agentID))
		name = fmt.Sprintf("ctrl-%x.sock", h.Sum64())
	}
	return filepath.Join(dir, name)
}

// ctrlRequest is one channel message. Macro-run and pause/resume arrive in
// later tickets; the server rejects what it does not know. A restart
// carries the compress handoff plus the new child's launch (ticket 05).
type ctrlRequest struct {
	Op      string   `json:"op"`
	Kind    string   `json:"kind,omitempty"`
	Text    string   `json:"text,omitempty"`
	Source  string   `json:"source,omitempty"`
	Session string   `json:"session,omitempty"`
	Count   int      `json:"count,omitempty"`
	Bin     string   `json:"bin,omitempty"`
	Args    []string `json:"args,omitempty"`
	Env     []string `json:"env,omitempty"`
}

// ctrlResponse answers one request with the pending count after it.
type ctrlResponse struct {
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
	Pending int    `json:"pending"`
}

// serveCtrl listens on the agent's socket and enqueues into q until stop
// runs. A socket that still answers belongs to a live wrapper, so a second
// one for the same agent refuses to start; a stale file from a dead wrapper
// is removed first; stop closes the listener and removes the file. A
// restart request without a restart handler only pins the handoff; the
// wrapper serves with its child-swapping handler instead.
func serveCtrl(agentID string, q *inputQueue) (stop func(), err error) {
	return serveCtrlRestart(agentID, q, nil)
}

// ctrlRestart is one compress restart for a wrapped agent: the handoff to
// pin ahead of the surviving queue, the new harness session it starts, and
// the new child's launch.
type ctrlRestart struct {
	Handoff       string
	SessionID     string
	CompressCount int
	Bin           string
	Args          []string
	Env           []string
}

// serveCtrlRestart listens like serveCtrl, but a restart request also runs
// onRestart, which swaps the wrapper's child and rotates its session before
// the handoff pins. A nil handler keeps the queue-only behavior.
func serveCtrlRestart(agentID string, q *inputQueue, onRestart func(ctrlRestart) (int, error)) (stop func(), err error) {
	path := ctrlSocketPath(agentID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if conn, err := net.DialTimeout("unix", path, time.Second); err == nil {
		_ = conn.Close()
		return nil, fmt.Errorf("control channel %s already served", path)
	}
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	done := make(chan struct{})
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				select {
				case <-done:
					return
				default:
				}
				if errors.Is(err, net.ErrClosed) {
					return
				}
				time.Sleep(50 * time.Millisecond)
				continue
			}
			go serveOneRestart(conn, q, onRestart)
		}
	}()
	return func() {
		close(done)
		_ = ln.Close()
		_ = os.Remove(path)
	}, nil
}

// serveOne answers one connection: one JSON request in, one JSON reply out.
func serveOne(conn net.Conn, q *inputQueue) {
	serveOneRestart(conn, q, nil)
}

// serveOneRestart answers one connection with a restart handler: the
// wrapper's child swap, or nil for the queue only.
func serveOneRestart(conn net.Conn, q *inputQueue, onRestart func(ctrlRestart) (int, error)) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(ctrlTimeout))
	var req ctrlRequest
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		_ = json.NewEncoder(conn).Encode(ctrlResponse{Error: "bad request: " + err.Error()})
		return
	}
	_ = json.NewEncoder(conn).Encode(handleCtrlRestart(q, req, onRestart))
}

// handleCtrl runs one request against the queue.
func handleCtrl(q *inputQueue, req ctrlRequest) ctrlResponse {
	return handleCtrlRestart(q, req, nil)
}

// handleCtrlRestart runs one request against the queue, restarting the
// wrapper's child when the request asks and a handler is set. Without one
// a restart only pins the handoff, so the supervisor seam stays drivable
// on a bare queue.
func handleCtrlRestart(q *inputQueue, req ctrlRequest, onRestart func(ctrlRestart) (int, error)) ctrlResponse {
	switch req.Op {
	case "pending":
		return ctrlResponse{OK: true, Pending: q.len()}
	case "enqueue":
		if strings.TrimSpace(req.Text) == "" {
			return ctrlResponse{Error: "empty prompt: nothing to queue"}
		}
		switch QueueKind(req.Kind) {
		case QueueKindSteer, QueueKindQueue:
			return ctrlResponse{OK: true, Pending: q.enqueue(QueueKind(req.Kind), req.Text, req.Source)}
		default:
			return ctrlResponse{Error: "unknown kind " + strconv.Quote(req.Kind) + ": want steer or queue"}
		}
	case "restart":
		if strings.TrimSpace(req.Text) == "" {
			return ctrlResponse{Error: "empty handoff: nothing to restart"}
		}
		if onRestart == nil {
			return ctrlResponse{OK: true, Pending: q.enqueueHandoff(req.Text)}
		}
		n, err := onRestart(ctrlRestart{
			Handoff: req.Text, SessionID: req.Session, CompressCount: req.Count,
			Bin: req.Bin, Args: req.Args, Env: req.Env,
		})
		if err != nil {
			return ctrlResponse{Error: err.Error()}
		}
		return ctrlResponse{OK: true, Pending: n}
	default:
		return ctrlResponse{Error: "unknown op " + strconv.Quote(req.Op)}
	}
}

// ctrlRoundTrip sends one request to the agent's wrapper and returns the
// pending count after it. Any failure means no wrapper runs there.
func ctrlRoundTrip(agentID string, req ctrlRequest) (int, error) {
	conn, err := net.DialTimeout("unix", ctrlSocketPath(agentID), ctrlTimeout)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(ctrlTimeout))
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return 0, err
	}
	var res ctrlResponse
	if err := json.NewDecoder(conn).Decode(&res); err != nil {
		return 0, err
	}
	if !res.OK {
		return 0, fmt.Errorf("wick control channel: %s", res.Error)
	}
	return res.Pending, nil
}

// ctrlEnqueue sends one prompt to the agent's wrapper: a steer by default
// from the shared delivery path.
func ctrlEnqueue(agentID string, kind QueueKind, text, source string) (int, error) {
	return ctrlRoundTrip(agentID, ctrlRequest{Op: "enqueue", Kind: string(kind), Text: text, Source: source})
}

// ctrlPending asks the agent's wrapper how many prompts wait. Without a
// wrapper it is 0, so the strip hides the count.
func ctrlPending(agentID string) int {
	n, err := ctrlRoundTrip(agentID, ctrlRequest{Op: "pending"})
	if err != nil {
		return 0
	}
	return n
}

// wrapperAlive says whether the agent's wrapper answers its channel: a
// compress restart goes through it instead of a pane run when it does.
func wrapperAlive(agentID string) bool {
	if agentID == "" {
		return false
	}
	_, err := ctrlRoundTrip(agentID, ctrlRequest{Op: "pending"})
	return err == nil
}

// ctrlRestartAgent restarts the agent's wrapper on the new session: the
// wrapper swaps its child, keeps the queue, row, history and raw state, and
// pins the handoff ahead of everything. It reports the pending count after.
func ctrlRestartAgent(agentID string, r ctrlRestart) (int, error) {
	return ctrlRoundTrip(agentID, ctrlRequest{
		Op: "restart", Text: r.Handoff, Session: r.SessionID, Count: r.CompressCount,
		Bin: r.Bin, Args: r.Args, Env: r.Env,
	})
}

// queueAcceptWait is how long a written item waits for the transcript to
// show its turn before the pump writes it again: a startup dialog can
// swallow the first attempt, and the item stays at the head until accepted.
const queueAcceptWait = 10 * time.Second

// collapseLine folds one item to a single line, the way the herdr fallback
// does: a newline in the PTY would submit the message half-written.
func collapseLine(text string) string { return strings.Join(strings.Fields(text), " ") }

// submitSettle is the pause between an item's text and its Enter. A TUI that
// reads a fast burst as one paste (claude does for long input) swallows an
// Enter in the same burst into the pasted text, so the message sits unsent in
// the input box. Text first, a short gap, then Enter alone. A var so tests
// run it fast.
var submitSettle = 250 * time.Millisecond

// deliver writes one item to the child PTY: the single line, a settle pause,
// then Enter in its own write. The bytes the child receives in order are the
// line followed by "\r". Bracketed paste per harness arrives with the
// ticket-04 harness table; the default writes plain text.
func (p *queuePump) deliver(text string) error {
	if err := p.gates.write([]byte(collapseLine(text))); err != nil {
		return err
	}
	if submitSettle > 0 {
		time.Sleep(submitSettle)
	}
	return p.gates.write([]byte("\r"))
}

// head reports the first waiting item without removing it.
func (q *inputQueue) head() (queueItem, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return queueItem{}, false
	}
	return q.items[0], true
}

// pop drops the first waiting item.
func (q *inputQueue) pop() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) > 0 {
		q.items = q.items[1:]
		q.inFlight = false
	}
}

// pumpGates is the pump's injectable world: when one kind may go out, how
// many transcript user turns show a text, where child bytes go, and the
// clock. A nil countTurns means the harness shows no verifiable turn, so a
// write counts as accepted at once.
type pumpGates struct {
	gate       func(QueueKind) bool
	countTurns func(text string) int
	write      func([]byte) error
	now        func() time.Time
	hasDraft   func() bool
}

// queuePump delivers one queue to one child: the head item goes out when
// its gate passes, and leaves only once the transcript shows its turn. A
// restart stops the old pump on the swap, so a captured step can never
// write twice to the new child.
type queuePump struct {
	mu       sync.Mutex
	q        *inputQueue
	gates    pumpGates
	written  bool
	baseline int
	lastTry  time.Time
	tries    int
	stopped  bool
}

func newQueuePump(q *inputQueue, gates pumpGates) *queuePump {
	return &queuePump{q: q, gates: gates}
}

func (p *queuePump) enqueue(kind QueueKind, text, source string) int {
	return p.q.enqueue(kind, text, source)
}

func (p *queuePump) len() int { return p.q.len() }

// stop retires the pump: a restart calls it on the swapped-out pump, so a
// step captured before the swap stays quiet. An in-flight step finishes
// first: stop waits on the pump lock.
func (p *queuePump) stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopped = true
}

// step moves the head item once: write it when its gate passes, pop it when
// the transcript shows its turn, rewrite it when acceptance runs out. At
// most one child write per step. A stopped pump does nothing.
func (p *queuePump) step() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped {
		return
	}
	head, ok := p.q.head()
	if !ok {
		p.written = false
		p.q.setInFlight(false)
		return
	}
	if !p.written {
		if head.Source != "user" && p.gates.hasDraft != nil && p.gates.hasDraft() {
			return
		}
		if !p.gates.gate(head.Kind) {
			return
		}
		if p.gates.countTurns == nil {
			if err := p.deliver(head.Text); err != nil {
				return
			}
			p.q.pop()
			return
		}
		p.baseline = p.gates.countTurns(head.Text)
		if err := p.deliver(head.Text); err != nil {
			return
		}
		p.written = true
		p.q.setInFlight(true)
		p.lastTry = p.gates.now()
		return
	}
	if p.gates.countTurns(head.Text) > p.baseline {
		p.q.pop()
		p.written = false
		p.q.setInFlight(false)
		p.tries = 0
		return
	}
	wait := queueAcceptWait * time.Duration(1<<min(p.tries, 3))
	if !p.gates.now().Before(p.lastTry.Add(wait)) {
		p.baseline = p.gates.countTurns(head.Text)
		if err := p.deliver(head.Text); err != nil {
			return
		}
		p.tries++
		p.lastTry = p.gates.now()
	}
}

// countUserTurns counts the transcript's user turns showing text: the
// acceptance signal for a delivered item. Only real user turns count —
// assistant lines and tool results never do — and a missing transcript
// counts nothing, so an item written before startup stays queued.
func countUserTurns(path, harness, text string) int {
	want := strings.TrimSpace(text)
	if want == "" {
		return 0
	}
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	n := 0
	scanLines(f, func(rec map[string]any) {
		if strings.Contains(userTurnText(rec, harness), want) {
			n++
		}
	})
	return n
}

// userTurnText reads one record's user text, or "" when it is no user turn:
// pig's user-role messages, claude's user records with string or text-block
// content. Tool results in either shape read as "".
func userTurnText(rec map[string]any, harness string) string {
	m, _ := rec["message"].(map[string]any)
	if m == nil || m["role"] != "user" {
		return ""
	}
	switch harness {
	case "pig":
		if rec["type"] != "message" {
			return ""
		}
		return textBlocks(m["content"])
	case "claude":
		if rec["type"] != "user" {
			return ""
		}
		if s, ok := m["content"].(string); ok {
			return strings.TrimSpace(s)
		}
		return textBlocks(m["content"])
	default:
		return ""
	}
}

// queuePumpEvery steps the wrapper's queue beside the read loop. A var so
// tests can hurry it.
var queuePumpEvery = 500 * time.Millisecond

// newWrapperPump delivers q to one wrapped child on the production gates:
// a steer at the tool boundary, a queued message at turn end, accepted only
// when the transcript shows the turn.
func newWrapperPump(src HeaderSource, q *inputQueue, write func([]byte) error, hasDraft func() bool, now func() time.Time) *queuePump {
	nowFn := time.Now
	if now != nil {
		nowFn = now
	}
	return newQueuePump(q, pumpGates{
		gate:       func(kind QueueKind) bool { return queueGate(src, kind) },
		countTurns: wrapperTurns(src),
		write:      write,
		now:        nowFn,
		hasDraft:   hasDraft,
	})
}

// queueGate says whether one kind may go out now. Without a transcript or a
// known harness the gates open: acceptance still verifies the write, so an
// item sent before startup waits as unaccepted instead of stalling forever.
func queueGate(src HeaderSource, kind QueueKind) bool {
	path := resolveTranscript(src)
	if path == "" {
		return true
	}
	if _, err := os.Stat(path); err != nil {
		return true
	}
	h := Registered(src.Harness)
	if h == nil {
		return true
	}
	if kind == QueueKindQueue {
		state, _ := h.TurnState(path)
		return state == TurnDone || state == TurnFailed
	}
	snap, err := summarizePath(src.Harness, path)
	if err != nil {
		return true
	}
	return snap.Outstanding == 0
}

// wrapperTurns counts the transcript's user turns for acceptance, or nil
// when the harness shows no verifiable turn: those writes count as accepted
// at once.
func wrapperTurns(src HeaderSource) func(string) int {
	if Registered(src.Harness) == nil {
		return nil
	}
	return func(text string) int {
		path := resolveTranscript(src)
		if path == "" {
			return 0
		}
		return countUserTurns(path, src.Harness, text)
	}
}
