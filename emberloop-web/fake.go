package web

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// containsReviewPrompt reports a reviewer gate prompt or format re-prompt:
// the only prompts reviewer sessions ever get.
func containsReviewPrompt(text string) bool {
	return strings.Contains(text, "Review ticket") || strings.Contains(text, "RESPOND IN FORMAT")
}

// FakeBackend replays scripted Events for tests and the walking skeleton.
// Delay holds a turn open: queue tests set it so a Steer or Follow-up can
// arrive while the Session still works. Turns are interruptible, so cancel
// tests can stop a working Session the way the interrupt route does.
// DelayFunc, when non-nil, picks the hold per prompt text (still
// interruptible): tests hold one slow turn open while the rest settle fast.
type FakeBackend struct {
	Delay time.Duration
	// DelayFunc overrides Delay per prompt; nil keeps Delay.
	DelayFunc func(text string) time.Duration
	// ReplyFunc, when non-nil, scripts the assistant reply per prompt:
	// returning (text, true) uses text as the whole reply, ("", false)
	// keeps the default "fake reply to: ...". Run tests script reviewer
	// VERDICT replies through it for the legacy fallback; the submit-tool
	// path uses ReviewSubmit instead. The seam stays fake Backend over HTTP.
	ReplyFunc func(sessionID, text string) (string, bool)

	// ReviewSubmit, when non-nil, scripts a reviewer `submit` tool call: for
	// reviewer prompts it returns the tool payload to submit instead of a
	// VERDICT text reply. Returning (ReviewerSubmit{}, false) falls back to
	// ReplyFunc and the default pass. The scripted submit is logged as a
	// submit Event carrying the payload JSON, the way the submit tool would
	// log it, so handleReviewerTurnEnd records the tool verdict. Tests drive
	// the Run seam through it without a model.
	ReviewSubmit func(sessionID, prompt string) (ReviewerSubmit, bool)
	// TurnFailure, when non-nil, scripts a failed provider turn. Returning
	// (error text, true) emits an error Event and status=failed; false uses
	// the ordinary scripted reply. Run tests use it to exercise Chain advance.
	TurnFailure func(sessionID, prompt string) (string, bool)
	// CreateFailure, when non-nil, can refuse a CreateSession by metadata.
	// Run tests use it to target a start failure at one role or harness.
	CreateFailure func(meta SessionMeta) error
	// CreateErrs, when set, fail the next CreateSession attempts in order.
	// Run tests use it to script provider failures before a Session exists.
	CreateErrs []error
	// UsageSize, when positive, makes each turn emit a usage Event sized
	// from the characters the current handle has seen, so tests can watch
	// the context meter fall after a Handoff.
	UsageSize int
	// StartErr, when set, makes StartHandle fail.
	StartErr error
	// StartErrs, when set, fail the next StartHandle calls in order.
	StartErrs []error
	// ResumeNote, when set, makes ResumeOK refuse a released Session with it.
	ResumeNote string
	// HandoffTurnErr, when set, makes a Handoff prompt report an error
	// Event before it ever works.
	HandoffTurnErr string

	mu         sync.Mutex
	stop       map[string]chan struct{}
	gen        map[string]int
	seen       map[string]int
	released   map[string]int
	starts     map[string][]handleStart
	interrupts map[string]int
}

func NewFakeBackend() *FakeBackend { return &FakeBackend{} }

// getDelay reads the scripted turn Delay under the lock, so tests can retune
// it between turns without racing a starting turn.
func (f *FakeBackend) getDelay() time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Delay
}

// SetDelay retunes the scripted turn Delay. Turns already running keep the
// Delay they started with.
func (f *FakeBackend) SetDelay(delay time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Delay = delay
}

func (f *FakeBackend) OnPrompt(sessionID, text string, appendEvent func(kind, text, status string)) {
	ch := make(chan struct{})
	f.mu.Lock()
	if f.stop == nil {
		f.stop = map[string]chan struct{}{}
	}
	f.stop[sessionID] = ch
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		if cur, ok := f.stop[sessionID]; ok && cur == ch {
			delete(f.stop, sessionID)
		}
		f.mu.Unlock()
	}()
	if f.HandoffTurnErr != "" && strings.Contains(text, "# Handoff") {
		appendEvent("error", f.HandoffTurnErr, "")
		appendEvent("status", "", "idle")
		return
	}
	if f.TurnFailure != nil {
		if detail, ok := f.TurnFailure(sessionID, text); ok {
			appendEvent("status", "", "working")
			appendEvent("error", detail, "")
			appendEvent("status", "", "failed")
			return
		}
	}
	appendEvent("status", "", "working")
	delay := f.getDelay()
	if f.DelayFunc != nil {
		delay = f.DelayFunc(text)
	}
	if delay <= 0 {
		delay = 5 * time.Millisecond
	}
	// A scripted reviewer submit wins over VERDICT text: it logs a submit
	// Event carrying the tool payload, so the Daemon records the tool verdict
	// the way a real submit tool call would.
	if containsReviewPrompt(text) && f.ReviewSubmit != nil {
		if sub, ok := f.ReviewSubmit(sessionID, text); ok {
			payload, _ := json.Marshal(sub)
			select {
			case <-time.After(delay):
				appendEvent("assistant_chunk", "submitting "+sub.Verdict+" at "+sub.Tip, "")
				appendEvent("submit", string(payload), "")
				appendEvent("status", "", "idle")
			case <-ch:
				appendEvent("status", "", "idle")
			}
			return
		}
	}
	reply, ok := "", false
	if f.ReplyFunc != nil {
		reply, ok = f.ReplyFunc(sessionID, text)
	}
	if !ok {
		if containsReviewPrompt(text) {
			// Reviewer gate prompts and format re-prompts default to a
			// pass, so runs without scripted reviews still merge; tests
			// needing changes script ReplyFunc per reviewer type. This is the
			// legacy fallback for a Backend without the submit tool.
			reply = "VERDICT: pass"
		} else {
			reply = "fake reply to: " + text
		}
	}
	select {
	case <-time.After(delay):
		appendEvent("assistant_chunk", reply, "")
		if f.UsageSize > 0 {
			f.mu.Lock()
			if f.seen == nil {
				f.seen = map[string]int{}
			}
			f.seen[sessionID] += len(text) + len(reply)
			used := f.seen[sessionID] / 4
			f.mu.Unlock()
			appendEvent("usage", usageText(used, f.UsageSize), "")
		}
		appendEvent("status", "", "idle")
	case <-ch:
		appendEvent("status", "", "idle")
	}
}

// OnPromptImages takes image turns like the real Backends: the fake echoes
// the prompt text (with its resolved file paths), so stored-image prompts
// behave the same at this seam as everywhere else.
func (f *FakeBackend) OnPromptImages(sessionID, text string, images []Image, appendEvent func(kind, text, status string)) {
	f.OnPrompt(sessionID, text, appendEvent)
}

// Interrupt stops a running fake turn: the sleeper wakes and settles idle
// with no reply. It reports whether a turn was running.
func (f *FakeBackend) Interrupt(sessionID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	ch, ok := f.stop[sessionID]
	if !ok {
		return false
	}
	delete(f.stop, sessionID)
	close(ch)
	if f.interrupts == nil {
		f.interrupts = map[string]int{}
	}
	f.interrupts[sessionID]++
	return true
}

// InterruptCount reports how many times Interrupt stopped one Session's
// turn. Tests assert a submit stops its turn exactly once.
func (f *FakeBackend) InterruptCount(sessionID string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.interrupts[sessionID]
}

func (f *FakeBackend) CreateSession(meta SessionMeta, emit func(kind, text, status string)) error {
	f.recordStart(meta, handleCreate)
	if f.CreateFailure != nil {
		if err := f.CreateFailure(meta); err != nil {
			return err
		}
	}
	f.mu.Lock()
	if len(f.CreateErrs) > 0 {
		err := f.CreateErrs[0]
		f.CreateErrs = f.CreateErrs[1:]
		f.mu.Unlock()
		return err
	}
	f.mu.Unlock()
	return nil
}

func (f *FakeBackend) CloseSession(sessionID string) {}

func (f *FakeBackend) Models() []string { return []string{"fake-model"} }

// ReleaseSession drops the fake handle; the token is a fixed marker.
func (f *FakeBackend) ReleaseSession(sessionID string) string { return f.ResumeToken(sessionID) }

// ResumeSession rebuilds the fake handle; the log is the whole state.
func (f *FakeBackend) ResumeSession(meta SessionMeta, emit func(kind, text, status string)) error {
	f.recordStart(meta, handleResume)
	return nil
}

// ResumeToken reports the fixed fake marker.
func (f *FakeBackend) ResumeToken(sessionID string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if g := f.gen[sessionID]; g > 0 {
		return fmt.Sprintf("fake-resume-%d", g)
	}
	return "fake-resume"
}

// StartHandle starts a fresh fake handle beside the old one: it rotates the
// resume token and clears the context the handle has seen. The returned
// release drops the old handle and counts it.
func (f *FakeBackend) StartHandle(meta SessionMeta, emit func(kind, text, status string)) (string, func(), error) {
	f.mu.Lock()
	if len(f.StartErrs) > 0 {
		err := f.StartErrs[0]
		f.StartErrs = f.StartErrs[1:]
		f.mu.Unlock()
		return "", nil, err
	}
	f.mu.Unlock()
	if f.StartErr != nil {
		return "", nil, f.StartErr
	}
	f.recordStart(meta, handleCompress)
	f.mu.Lock()
	if f.gen == nil {
		f.gen = map[string]int{}
	}
	f.gen[meta.ID]++
	delete(f.seen, meta.ID)
	tok := fmt.Sprintf("fake-resume-%d", f.gen[meta.ID])
	f.mu.Unlock()
	return tok, func() {
		f.mu.Lock()
		if f.released == nil {
			f.released = map[string]int{}
		}
		f.released[meta.ID]++
		f.mu.Unlock()
	}, nil
}

// ReleasedHandles counts old handles released by a Handoff swap.
func (f *FakeBackend) ReleasedHandles(sessionID string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.released[sessionID]
}

// ResumeOK holds unless ResumeNote is set: the fake resumes from the log alone.
func (f *FakeBackend) ResumeOK(meta SessionMeta) (bool, string) {
	if f.ResumeNote != "" {
		return false, f.ResumeNote
	}
	return true, ""
}

// handleStart is one start the fake Backend received: the operation and the
// handle startHandle derived from the record it was handed, plus the owner
// env the Daemon handed the harness start with.
type handleStart struct {
	op     handleOp
	handle handle
	owner  string
	env    []string
}

// Kind is the harness-start kind owner-dir tests read: create, resume or
// handoff (a compress handle swap).
func (s handleStart) Kind() string {
	switch s.op {
	case handleCreate:
		return "create"
	case handleResume:
		return "resume"
	default:
		return "handoff"
	}
}

// Owner is the User who owns the started Session.
func (s handleStart) Owner() string { return s.owner }

// Env is the owner-dirs env the Daemon handed the harness start with
// (CLAUDE_CONFIG_DIR and PIG_CODING_AGENT_DIR per owner).
func (s handleStart) Env() []string { return s.env }

// recordStart keeps what one start received, in order per Session.
func (f *FakeBackend) recordStart(meta SessionMeta, op handleOp) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.starts == nil {
		f.starts = map[string][]handleStart{}
	}
	f.starts[meta.ID] = append(f.starts[meta.ID], handleStart{op: op, handle: startHandle(meta, op), owner: meta.Owner, env: meta.Dirs.env()})
}

// Starts lists the starts one Session got, in order: create, then each
// resume and compress.
func (f *FakeBackend) Starts(sessionID string) []handleStart {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]handleStart(nil), f.starts[sessionID]...)
}
