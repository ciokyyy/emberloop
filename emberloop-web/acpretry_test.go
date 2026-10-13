package web

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// A dead adapter resends the same prompt on a respawned handle: one retry
// after one death, no error Event, the turn ends idle.
func TestPromptRespawnsOnceAfterAdapterDeath(t *testing.T) {
	oldBackoff := acpRespawnBackoff
	acpRespawnBackoff = []time.Duration{time.Millisecond, time.Millisecond}
	t.Cleanup(func() { acpRespawnBackoff = oldBackoff })
	oldRetry := claudeLoginRetry
	claudeLoginRetry = time.Millisecond
	t.Cleanup(func() { claudeLoginRetry = oldRetry })

	var conns int64
	stub := &stubConn{promptErrs: []error{fmt.Errorf("adapter closed"), nil}}
	be := newStubClaudeBackend(func() (acpConn, error) { atomic.AddInt64(&conns, 1); return stub, nil })
	meta := SessionMeta{ID: "r1", Harness: "claude", Model: "haiku", WorkDir: tempDir(t)}
	if err := be.StartSession(meta); err != nil {
		t.Fatal(err)
	}
	type ev struct{ kind, text, status string }
	var got []ev
	be.OnPrompt("r1", "hi", func(kind, text, status string) { got = append(got, ev{kind, text, status}) })
	stub.mu.Lock()
	prompts := len(stub.prompts)
	stub.mu.Unlock()
	if prompts != 2 {
		t.Fatalf("prompts after one adapter death = %d, want 2 (one respawned retry)", prompts)
	}
	if atomic.LoadInt64(&conns) != 2 {
		t.Fatalf("adapter starts = %d, want 2 (the retry respawns through the resume path)", conns)
	}
	for _, e := range got {
		if e.kind == "error" {
			t.Fatalf("error Event after a recovered death: %v", got)
		}
	}
	if len(got) == 0 || got[len(got)-1].kind != "status" || got[len(got)-1].status != "idle" {
		t.Fatalf("last Event = %v, want terminal idle", got)
	}
	reconnecting := false
	for _, e := range got {
		if e.kind == "status" && e.status == "working" && strings.Contains(e.text, "reconnecting") {
			reconnecting = true
		}
	}
	if !reconnecting {
		t.Fatalf("no reconnecting status among %v, want one", got)
	}
}

// Three dead adapters fail the turn with one actionable error naming the
// attempts, and status failed.
func TestPromptGivesUpAfterThreeAdapterDeaths(t *testing.T) {
	oldBackoff := acpRespawnBackoff
	acpRespawnBackoff = []time.Duration{time.Millisecond, time.Millisecond}
	t.Cleanup(func() { acpRespawnBackoff = oldBackoff })
	oldRetry := claudeLoginRetry
	claudeLoginRetry = time.Millisecond
	t.Cleanup(func() { claudeLoginRetry = oldRetry })

	stub := &stubConn{promptErr: fmt.Errorf("adapter closed")}
	be := newStubClaudeBackend(func() (acpConn, error) { return stub, nil })
	meta := SessionMeta{ID: "r2", Harness: "claude", Model: "haiku", WorkDir: tempDir(t)}
	if err := be.StartSession(meta); err != nil {
		t.Fatal(err)
	}
	type ev struct{ kind, text, status string }
	var got []ev
	be.OnPrompt("r2", "hi", func(kind, text, status string) { got = append(got, ev{kind, text, status}) })
	stub.mu.Lock()
	prompts := len(stub.prompts)
	stub.mu.Unlock()
	if prompts != 3 {
		t.Fatalf("prompts after three adapter deaths = %d, want 3", prompts)
	}
	var errs []string
	var lastStatus string
	for _, e := range got {
		if e.kind == "error" {
			errs = append(errs, e.text)
		}
		if e.kind == "status" {
			lastStatus = e.status
		}
	}
	if len(errs) != 1 || !strings.Contains(errs[0], "3 attempts") {
		t.Fatalf("error Events = %v, want one naming 3 attempts", errs)
	}
	if lastStatus != "failed" {
		t.Fatalf("last status = %q, want failed", lastStatus)
	}
}

// A deliberate close mid-prompt drops the orphaned result quietly: no error
// Event, no failed status, no respawn.
func TestPromptClosedMidTurnIsQuiet(t *testing.T) {
	oldBackoff := acpRespawnBackoff
	acpRespawnBackoff = []time.Duration{time.Millisecond, time.Millisecond}
	t.Cleanup(func() { acpRespawnBackoff = oldBackoff })

	var conns int64
	stub := &stubConn{promptErr: fmt.Errorf("adapter closed"), blockPrompt: make(chan struct{})}
	be := newStubClaudeBackend(func() (acpConn, error) { atomic.AddInt64(&conns, 1); return stub, nil })
	meta := SessionMeta{ID: "r3", Harness: "claude", Model: "haiku", WorkDir: tempDir(t)}
	if err := be.StartSession(meta); err != nil {
		t.Fatal(err)
	}
	type ev struct{ kind, text, status string }
	var got []ev
	done := make(chan struct{})
	go func() {
		defer close(done)
		be.OnPrompt("r3", "hi", func(kind, text, status string) { got = append(got, ev{kind, text, status}) })
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		stub.mu.Lock()
		n := len(stub.prompts)
		stub.mu.Unlock()
		if n == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("prompt never reached the adapter")
		}
		time.Sleep(time.Millisecond)
	}
	be.DropSession("r3")
	close(stub.blockPrompt)
	<-done
	for _, e := range got {
		if e.kind == "error" {
			t.Fatalf("error Event after a deliberate close: %v", got)
		}
		if e.kind == "status" && e.status == "failed" {
			t.Fatalf("failed status after a deliberate close: %v", got)
		}
	}
	if atomic.LoadInt64(&conns) != 1 {
		t.Fatalf("adapter starts = %d, want 1 (no respawn after a deliberate close)", conns)
	}
}
