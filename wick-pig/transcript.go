package wick

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// newSessionID is a random UUID. claude requires one; pig takes any string.
func newSessionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// FindSession returns the transcript under root whose file name carries id.
// It is the lookup the context command resolves a session through.
func FindSession(root, id string) string { return findSession(root, id) }

// findSession returns the transcript under root whose file name carries id.
func findSession(root, id string) string {
	var found string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".jsonl") && strings.Contains(d.Name(), id) {
			found = p
			return fs.SkipAll
		}
		return nil
	})
	return found
}

// lastRecord decodes the final complete JSON line that satisfies pick.
// ponytail: rereads the whole file each poll; fine for review-sized sessions.
func lastRecord(path string, pick func(map[string]any) (string, bool)) (text string, ok bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
	for sc.Scan() {
		var rec map[string]any
		if json.Unmarshal(sc.Bytes(), &rec) != nil {
			continue
		}
		if t, hit := pick(rec); hit {
			text, ok = t, true
		}
	}
	return text, ok
}

// textOf joins the text blocks of a message content array.
func textOf(content any) string {
	blocks, _ := content.([]any)
	var parts []string
	for _, b := range blocks {
		if m, _ := b.(map[string]any); m["type"] == "text" {
			if s, _ := m["text"].(string); s != "" {
				parts = append(parts, s)
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

// A turn is what the transcript says the agent is doing right now.
const (
	TurnDone    = "done"
	TurnFailed  = "failed"
	TurnRunning = "running"
)

// retryBackoff is the wait before each nudge of a failed turn. The failures
// seen so far are the model provider returning an empty body in bursts that
// clear after minutes, so the waits are long. A var so tests can zero it.
var retryBackoff = []time.Duration{10 * time.Second, 45 * time.Second, 120 * time.Second}

// nudgeSettle gives the agent time to write the nudge into its transcript, so
// the same failure is not nudged twice.
var nudgeSettle = 3 * time.Second

// lastTurn is the state of the last record classify recognises. Unknown
// shapes are skipped, so a transcript this code cannot read is "running",
// never "failed".
func lastTurn(path string, classify func(rec map[string]any) (state, detail string, ok bool)) (string, string) {
	state, detail := TurnRunning, ""
	_, _ = lastRecord(path, func(rec map[string]any) (string, bool) {
		if st, d, ok := classify(rec); ok {
			state, detail = st, d
		}
		return "", false
	})
	return state, detail
}

// agentForPane finds the agent that runs in pane from its record, without
// asking herdr: the socket's presence below says whether its wrapper runs.
func agentForPane(pane string) (string, bool) {
	entries, err := os.ReadDir(agentStateDir())
	if err != nil {
		return "", false
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		if st, ok, err := ReadAgentState(id); err == nil && ok && st.Pane == pane {
			return id, true
		}
	}
	return "", false
}

// deliverViaWrapper tries the agent's wrapper channel: a wrapped agent's
// prompt arrives through its queue, never through herdr send-text. A
// missing agent record or a failed channel round trip falls back to herdr.
func deliverViaWrapper(pane, text string) bool {
	id, ok := agentForPane(pane)
	if !ok {
		return false
	}
	_, err := ctrlEnqueue(id, QueueKindSteer, text, "deliver")
	return err == nil
}

// DeliverPrompt sends text to the agent in pane as the message itself, so the
// agent spends no turn reading a file. It is the one shared prompt-delivery
// path: steering, reviewer follow-ups, late first messages and (later)
// compress handoffs all send through it. herdr only prompts an agent it
// detects itself; pig only reports its state, so herdr refuses to prompt it
// ("not an active named agent") and the text is typed into the pane instead.
// Typed text is one line: a newline would submit the message half-written.
func DeliverPrompt(run Exec, pane, text string) error {
	if deliverViaWrapper(pane, text) {
		return nil
	}
	if res, err := run(herdrBin(), []string{"agent", "prompt", pane, text}, ExecOpts{Timeout: 30 * time.Second}); err == nil && res.Code == 0 {
		return nil
	}
	line := strings.Join(strings.Fields(text), " ")
	for _, args := range [][]string{{"pane", "send-text", pane, line}, {"pane", "send-keys", pane, "Enter"}} {
		res, err := run(herdrBin(), args, ExecOpts{Timeout: 15 * time.Second})
		if err != nil {
			return err
		}
		if res.Code != 0 {
			return fmt.Errorf("herdr %s %s failed: %s", args[0], args[1], firstLine(res.Stderr, res.Stdout))
		}
	}
	return nil
}

// Nudge sends text to the agent in pane through the shared DeliverPrompt path,
// kept under its old name for its callers.
func Nudge(run Exec, pane, text string) error {
	return DeliverPrompt(run, pane, text)
}

// WaitReply polls the session transcript until the agent ends a turn, and
// returns that last message. It is the same text a human sees last in the pane.
// after is the transcript's length when a follow-up was sent: the reply must
// be written past it, so the previous turn's reply is not mistaken for the
// answer. 0 means the session's first turn.
// A turn that ends in a provider error is nudged through nudge (nil to turn
// that off) after each backoff, then reported as an error.
func WaitReply(h Harness, s Session, after int64, timeout time.Duration, nudge func() error) (string, error) {
	deadline := time.Now().Add(timeout)
	nudges := 0
	for time.Now().Before(deadline) {
		if p := findSession(h.SessionRoot(s), s.ID); p != "" && grownPast(p, after) {
			// A follow-up is answered once the turn it started is done; until
			// then the last finished reply is still the previous one.
			state, detail := h.TurnState(p)
			if after == 0 || state == TurnDone {
				if text, ok := h.LastReply(p); ok && text != "" {
					return text, nil
				}
			}
			if state == TurnFailed && nudge != nil {
				if nudges >= len(retryBackoff) {
					return "", fmt.Errorf("%s turn failed after %d retries: %s", h.Name(), nudges, detail)
				}
				time.Sleep(retryBackoff[nudges])
				nudges++
				if err := nudge(); err != nil {
					return "", err
				}
				time.Sleep(nudgeSettle)
				continue
			}
		}
		time.Sleep(pollEvery)
	}
	return "", fmt.Errorf("%s did not finish a turn within %s", h.Name(), timeout)
}

// grownPast reports whether the file at p is longer than after bytes.
func grownPast(p string, after int64) bool {
	if after == 0 {
		return true
	}
	info, err := os.Stat(p)
	return err == nil && info.Size() > after
}

// NewSessionID is newSessionID for the launcher.
func NewSessionID() string { return newSessionID() }

// Retry nudges the agent behind a worker id or, failing that, a pane id.
func Retry(run Exec, target string) error {
	pane := target
	if w, err := recall(target); err == nil {
		pane = w.Pane
	}
	return Nudge(run, pane, "continue")
}

// Supervise nudges the agent in pane whenever its turn ends in a provider
// error, with the same backoff WaitReply uses, until stop closes. A merger has
// no parent to notice, so its own launcher watches it.
func Supervise(run Exec, h Harness, s Session, pane string, stop <-chan struct{}) {
	nudges := 0
	for {
		select {
		case <-stop:
			return
		case <-time.After(pollEvery):
		}
		p := findSession(h.SessionRoot(s), s.ID)
		if p == "" {
			continue
		}
		state, _ := h.TurnState(p)
		if state != TurnFailed {
			nudges = 0
			continue
		}
		if nudges >= len(retryBackoff) {
			return
		}
		select {
		case <-stop:
			return
		case <-time.After(retryBackoff[nudges]):
		}
		nudges++
		if Nudge(run, pane, "continue") != nil {
			return
		}
		time.Sleep(nudgeSettle)
	}
}
