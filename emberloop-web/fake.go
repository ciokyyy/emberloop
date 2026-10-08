package web

import "time"

// FakeBackend replays scripted Events for tests and the walking skeleton.
type FakeBackend struct{}

func NewFakeBackend() *FakeBackend { return &FakeBackend{} }

func (f *FakeBackend) OnPrompt(sessionID, text string, appendEvent func(kind, text, status string)) {
	appendEvent("status", "", "working")
	time.Sleep(5 * time.Millisecond)
	appendEvent("assistant_chunk", "fake reply to: "+text, "")
	appendEvent("status", "", "idle")
}
