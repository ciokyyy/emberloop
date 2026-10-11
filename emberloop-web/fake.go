package web

import (
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
type FakeBackend struct {
	Delay time.Duration
	// ReplyFunc, when non-nil, scripts the assistant reply per prompt:
	// returning (text, true) uses text as the whole reply, ("", false)
	// keeps the default "fake reply to: ...". Run tests script reviewer
	// VERDICT replies through it; the seam stays fake Backend over HTTP.
	ReplyFunc func(sessionID, text string) (string, bool)

	// UsageSize, when positive, makes each turn emit a usage Event sized
	// from the characters the current handle has seen, so tests can watch
	// the context meter fall after a Handoff.
	UsageSize int
	// StartErr, when set, makes StartHandle fail.
	StartErr error
	// StartErrs, when set, fail the next StartHandle calls in order.
	StartErrs []error
	// HandoffTurnErr, when set, makes a Handoff prompt report an error
	// Event before it ever works.
	HandoffTurnErr string

	mu       sync.Mutex
	stop     map[string]chan struct{}
	gen      map[string]int
	seen     map[string]int
	released map[string]int
}

func NewFakeBackend() *FakeBackend { return &FakeBackend{} }

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
	if f.HandoffTurnErr != "" && strings.HasPrefix(text, "# Handoff") {
		appendEvent("error", f.HandoffTurnErr, "")
		appendEvent("status", "", "idle")
		return
	}
	appendEvent("status", "", "working")
	delay := f.Delay
	if delay <= 0 {
		delay = 5 * time.Millisecond
	}
	reply, ok := "", false
	if f.ReplyFunc != nil {
		reply, ok = f.ReplyFunc(sessionID, text)
	}
	if !ok {
		if containsReviewPrompt(text) {
			// Reviewer gate prompts and format re-prompts default to a
			// pass, so runs without scripted reviews still merge; tests
			// needing changes script ReplyFunc per reviewer type.
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
	return true
}

func (f *FakeBackend) CreateSession(meta SessionMeta, emit func(kind, text, status string)) error {
	return nil
}

func (f *FakeBackend) CloseSession(sessionID string) {}

func (f *FakeBackend) Models() []string { return []string{"fake-model"} }

// ReleaseSession drops the fake handle; the token is a fixed marker.
func (f *FakeBackend) ReleaseSession(sessionID string) string { return f.ResumeToken(sessionID) }

// ResumeSession rebuilds the fake handle; the log is the whole state.
func (f *FakeBackend) ResumeSession(meta SessionMeta, emit func(kind, text, status string)) error {
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

// ResumeOK always holds: the fake resumes from the log alone.
func (f *FakeBackend) ResumeOK(meta SessionMeta) (bool, string) { return true, "" }
