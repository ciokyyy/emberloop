package wick

import (
	"encoding/json"
	"fmt"
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
// a startup dialog swallowed stays at the head and is retried. Later tickets
// own the row, macros, raw mode and restart survival; their channel messages
// (macro run, pause/resume) do not exist yet.

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

// inputQueue is the wrapper's FIFO of prompts: user messages go first only
// from ticket 02, the restart handoff pins ahead only from ticket 05, so
// here enqueue always appends in arrival order.
type inputQueue struct {
	mu    sync.Mutex
	items []queueItem
}

func newInputQueue() *inputQueue { return &inputQueue{} }

// enqueue appends one item and reports the pending count.
func (q *inputQueue) enqueue(kind QueueKind, text, source string) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = append(q.items, queueItem{Kind: kind, Text: text, Source: source})
	return len(q.items)
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
// beside the agent records. lockName keeps the file name safe.
func ctrlSocketPath(agentID string) string {
	return filepath.Join(filepath.Dir(statePath), "agents", "ctrl-"+lockName(agentID)+".sock")
}

// ctrlRequest is one channel message. Macro-run and pause/resume arrive in
// later tickets; the server rejects what it does not know.
type ctrlRequest struct {
	Op     string `json:"op"`
	Kind   string `json:"kind,omitempty"`
	Text   string `json:"text,omitempty"`
	Source string `json:"source,omitempty"`
}

// ctrlResponse answers one request with the pending count after it.
type ctrlResponse struct {
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
	Pending int    `json:"pending"`
}

// serveCtrl listens on the agent's socket and enqueues into q until stop
// runs. A stale socket file from a dead wrapper is removed first; stop
// closes the listener and removes the file.
func serveCtrl(agentID string, q *inputQueue) (stop func(), err error) {
	path := ctrlSocketPath(agentID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
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
					continue
				}
			}
			go serveOne(conn, q)
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
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(ctrlTimeout))
	var req ctrlRequest
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		_ = json.NewEncoder(conn).Encode(ctrlResponse{Error: "bad request: " + err.Error()})
		return
	}
	_ = json.NewEncoder(conn).Encode(handleCtrl(q, req))
}

// handleCtrl runs one request against the queue.
func handleCtrl(q *inputQueue, req ctrlRequest) ctrlResponse {
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

// queueAcceptWait is how long a written item waits for the transcript to
// show its turn before the pump writes it again: a startup dialog can
// swallow the first attempt, and the item stays at the head until accepted.
const queueAcceptWait = 10 * time.Second

// deliveryBytes is exactly what the child PTY receives for one item: the
// whole message followed by Enter. Bracketed paste per harness arrives with
// the ticket-04 harness table; the default writes plain text.
func deliveryBytes(text string) []byte { return []byte(text + "\r") }

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
}

// queuePump delivers one queue to one child: the head item goes out when
// its gate passes, and leaves only once the transcript shows its turn.
type queuePump struct {
	mu       sync.Mutex
	q        *inputQueue
	gates    pumpGates
	written  bool
	baseline int
	lastTry  time.Time
}

func newQueuePump(q *inputQueue, gates pumpGates) *queuePump {
	return &queuePump{q: q, gates: gates}
}

func (p *queuePump) enqueue(kind QueueKind, text, source string) int {
	return p.q.enqueue(kind, text, source)
}

func (p *queuePump) len() int { return p.q.len() }

// step moves the head item once: write it when its gate passes, pop it when
// the transcript shows its turn, rewrite it when acceptance runs out. At
// most one child write per step.
func (p *queuePump) step() {
	p.mu.Lock()
	defer p.mu.Unlock()
	head, ok := p.q.head()
	if !ok {
		p.written = false
		return
	}
	if !p.written {
		if !p.gates.gate(head.Kind) {
			return
		}
		if err := p.gates.write(deliveryBytes(head.Text)); err != nil {
			return
		}
		p.written = true
		p.lastTry = p.gates.now()
		if p.gates.countTurns == nil {
			p.q.pop()
			p.written = false
			return
		}
		p.baseline = p.gates.countTurns(head.Text)
		return
	}
	if p.gates.countTurns(head.Text) > p.baseline {
		p.q.pop()
		p.written = false
		return
	}
	if !p.gates.now().Before(p.lastTry.Add(queueAcceptWait)) {
		if err := p.gates.write(deliveryBytes(head.Text)); err != nil {
			return
		}
		p.lastTry = p.gates.now()
		p.baseline = p.gates.countTurns(head.Text)
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
func newWrapperPump(src HeaderSource, q *inputQueue, write func([]byte) error) *queuePump {
	return newQueuePump(q, pumpGates{
		gate:       func(kind QueueKind) bool { return queueGate(src, kind) },
		countTurns: wrapperTurns(src),
		write:      write,
		now:        time.Now,
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
	h := Registered(src.Harness)
	if h == nil {
		return true
	}
	if kind == QueueKindQueue {
		state, _ := h.TurnState(path)
		return state == TurnDone
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
		return countUserTurns(resolveTranscript(src), src.Harness, text)
	}
}
