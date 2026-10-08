package web

import "time"

// FakeBackend replays scripted Events for tests and the walking skeleton.
// Delay holds a turn open: queue tests set it so a Steer or Follow-up can
// arrive while the Session still works.
type FakeBackend struct {
	Delay time.Duration
}

func NewFakeBackend() *FakeBackend { return &FakeBackend{} }

func (f *FakeBackend) OnPrompt(sessionID, text string, appendEvent func(kind, text, status string)) {
	appendEvent("status", "", "working")
	delay := f.Delay
	if delay <= 0 {
		delay = 5 * time.Millisecond
	}
	time.Sleep(delay)
	appendEvent("assistant_chunk", "fake reply to: "+text, "")
	appendEvent("status", "", "idle")
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
