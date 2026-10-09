package web

import (
	"sync"
	"time"
)

// FakeBackend replays scripted Events for tests and the walking skeleton.
// Delay holds a turn open: queue tests set it so a Steer or Follow-up can
// arrive while the Session still works. Turns are interruptible, so cancel
// tests can stop a working Session the way the interrupt route does.
type FakeBackend struct {
	Delay time.Duration

	mu   sync.Mutex
	stop map[string]chan struct{}
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
	appendEvent("status", "", "working")
	delay := f.Delay
	if delay <= 0 {
		delay = 5 * time.Millisecond
	}
	select {
	case <-time.After(delay):
		appendEvent("assistant_chunk", "fake reply to: "+text, "")
		appendEvent("status", "", "idle")
	case <-ch:
		appendEvent("status", "", "idle")
	}
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
func (f *FakeBackend) ReleaseSession(sessionID string) string { return "fake-resume" }

// ResumeSession rebuilds the fake handle; the log is the whole state.
func (f *FakeBackend) ResumeSession(meta SessionMeta, emit func(kind, text, status string)) error {
	return nil
}

// ResumeToken reports the fixed fake marker.
func (f *FakeBackend) ResumeToken(sessionID string) string { return "fake-resume" }

// ResumeOK always holds: the fake resumes from the log alone.
func (f *FakeBackend) ResumeOK(meta SessionMeta) (bool, string) { return true, "" }
