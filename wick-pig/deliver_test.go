package wick

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// deliverTestState points the submit and agent-state stores at a temp dir and
// captures the exec the notice path drives.
func deliverTestState(t *testing.T) {
	t.Helper()
	oldState, oldExec := statePath, deliverExec
	statePath = filepath.Join(t.TempDir(), "workers.json")
	t.Cleanup(func() { statePath, deliverExec = oldState, oldExec })
}

// workerType is the built-in worker as the submit tool sees it.
func workerType() AgentType {
	return AgentType{Name: "worker", Blocking: false, Statuses: []string{"done", "blocked"}}
}

// submitOnce runs one submit through the real tool and returns its result.
func submitOnce(t *testing.T, typ AgentType, id, status, field, body string) (string, error) {
	t.Helper()
	tool := submitToolFor(typ, id)
	raw, _ := json.Marshal(map[string]any{"status": status, field: body})
	return tool.Execute(context.Background(), "", raw)
}

// A non-blocking agent's submit is delivered into its parent pane as a system
// notice through `herdr agent prompt`, and succeeds.
func TestNonBlockingSubmitPromptsTheParentPane(t *testing.T) {
	deliverTestState(t)
	if err := WriteAgentState(AgentState{ID: "w1", Type: "worker", Parent: "wP", ParentID: "p", Deliver: true}); err != nil {
		t.Fatal(err)
	}
	rec := &recorder{reply: func(string, []string) Result { return Result{} }}
	deliverExec = rec.run

	if _, err := submitOnce(t, workerType(), "w1", "done", "summary", "all good"); err != nil {
		t.Fatalf("submit: %v", err)
	}
	if len(rec.calls) != 1 {
		t.Fatalf("want exactly one herdr call, got %v", rec.calls)
	}
	call := strings.Join(rec.calls[0], " ")
	if !strings.HasPrefix(call, herdrBin()+" agent prompt wP ") {
		t.Fatalf("the notice must be prompted into the parent pane, got %q", call)
	}
	for _, want := range []string{
		"<system-notice> worker w1 submitted (done).",
		"Resume your work using the result below.",
		"── w1 ── all good",
		"</system-notice>",
	} {
		if !strings.Contains(call, want) {
			t.Errorf("notice missing %q:\n%s", want, call)
		}
	}
}

// When herdr refuses `agent prompt`, the notice is typed as one flattened line
// and Entered, the same fallback the nudge path uses for a pig pane.
func TestNoticeFallsBackToTypingOneLine(t *testing.T) {
	deliverTestState(t)
	if err := WriteAgentState(AgentState{ID: "a1", Type: "scout", Parent: "wP", Deliver: true}); err != nil {
		t.Fatal(err)
	}
	rec := &recorder{reply: func(_ string, args []string) Result {
		if args[0] == "agent" && args[1] == "prompt" {
			return Result{Code: 1, Stdout: `{"error":{"code":"agent_not_ready"}}`}
		}
		return Result{}
	}}
	deliverExec = rec.run

	scout := AgentType{Name: "scout", Blocking: true, Statuses: []string{"answered"}}
	if _, err := submitOnce(t, scout, "a1", "answered", "report", "one\ntwo"); err != nil {
		t.Fatalf("submit: %v", err)
	}
	var typed, entered bool
	for _, c := range rec.calls {
		line := strings.Join(c[1:], " ")
		switch {
		case strings.HasPrefix(line, "pane send-text wP "):
			typed = true
			if strings.Contains(line, "\n") || !strings.Contains(line, "── a1 ── one two") {
				t.Errorf("typed notice must be one flattened line, got %q", line)
			}
		case line == "pane send-keys wP Enter":
			entered = typed
		}
	}
	if !typed || !entered {
		t.Errorf("want send-text then Enter, calls: %v", rec.calls)
	}
}

// A blocking agent whose submit is returned to a waiting spawn is never typed
// into any pane.
func TestBlockingSubmitIsNeverTyped(t *testing.T) {
	deliverTestState(t)
	rec := &recorder{reply: func(string, []string) Result { return Result{} }}
	deliverExec = rec.run

	scout := AgentType{Name: "scout", Blocking: true, Statuses: []string{"answered"}}
	if _, err := submitOnce(t, scout, "a1", "answered", "report", "facts"); err != nil {
		t.Fatalf("submit: %v", err)
	}
	if len(rec.calls) != 0 {
		t.Errorf("a returned blocking submit must not touch a pane, got %v", rec.calls)
	}
}

// A parent pane herdr no longer knows drops the notice: the submit succeeds and
// nothing more is sent.
func TestNoticeForAMissingParentPaneIsDropped(t *testing.T) {
	deliverTestState(t)
	if err := WriteAgentState(AgentState{ID: "w1", Type: "worker", Parent: "gone", Deliver: true}); err != nil {
		t.Fatal(err)
	}
	rec := &recorder{reply: func(_ string, args []string) Result {
		if args[0] == "agent" && args[1] == "prompt" {
			return Result{Code: 1, Stdout: `{"error":{"code":"pane_not_found","message":"pane gone not found"}}`}
		}
		return Result{}
	}}
	deliverExec = rec.run

	if _, err := submitOnce(t, workerType(), "w1", "done", "summary", "all good"); err != nil {
		t.Fatalf("a missing parent pane must not fail the submit: %v", err)
	}
	if len(rec.calls) != 1 {
		t.Errorf("nothing may be sent after a missing pane, got %v", rec.calls)
	}
}

// concurrentExec tracks the most `agent prompt` calls in flight at once.
type concurrentExec struct {
	active, max int32
	hold        time.Duration
}

func (c *concurrentExec) run(cmd string, args []string, _ ExecOpts) (Result, error) {
	if cmd == herdrBin() && len(args) > 1 && args[0] == "agent" && args[1] == "prompt" {
		n := atomic.AddInt32(&c.active, 1)
		for {
			m := atomic.LoadInt32(&c.max)
			if n <= m || atomic.CompareAndSwapInt32(&c.max, m, n) {
				break
			}
		}
		time.Sleep(c.hold)
		atomic.AddInt32(&c.active, -1)
	}
	return Result{}, nil
}

// Submits to one parent pane are serialized; submits to different panes are
// not.
func TestSubmitsToAPaneSerialize(t *testing.T) {
	deliverTestState(t)
	c := &concurrentExec{hold: 40 * time.Millisecond}
	deliverExec = c.run
	for _, id := range []string{"w1", "w2"} {
		if err := WriteAgentState(AgentState{ID: id, Type: "worker", Parent: "same", Deliver: true}); err != nil {
			t.Fatal(err)
		}
	}

	var wg sync.WaitGroup
	for _, id := range []string{"w1", "w2"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			if _, err := submitOnce(t, workerType(), id, "done", "summary", "x"); err != nil {
				t.Errorf("submit %s: %v", id, err)
			}
		}(id)
	}
	wg.Wait()
	if got := atomic.LoadInt32(&c.max); got != 1 {
		t.Errorf("deliveries to one pane overlapped: max concurrent = %d, want 1", got)
	}

	// A fresh run to two different panes must overlap.
	c2 := &concurrentExec{hold: 60 * time.Millisecond}
	deliverExec = c2.run
	for _, id := range []string{"x1", "x2"} {
		if err := WriteAgentState(AgentState{ID: id, Type: "worker", Parent: "pane-" + id, Deliver: true}); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"x1", "x2"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			if _, err := submitOnce(t, workerType(), id, "done", "summary", "x"); err != nil {
				t.Errorf("submit %s: %v", id, err)
			}
		}(id)
	}
	wg.Wait()
	if got := atomic.LoadInt32(&c2.max); got < 2 {
		t.Errorf("deliveries to different panes must overlap: max concurrent = %d, want 2", got)
	}
}

// backgroundAgent and a submit are serialized by the child's lock: one of them
// owns the result. A child that already submitted is left alone; one that has
// not is switched to the notice path.
func TestBackgroundAgentLeavesASubmittedChildAlone(t *testing.T) {
	deliverTestState(t)
	if err := WriteAgentState(AgentState{ID: "a1", Type: "scout", ParentID: "p", Parent: "wP"}); err != nil {
		t.Fatal(err)
	}
	if err := WriteAgentSubmit(AgentSubmit{AgentID: "a1", Type: "scout", Status: "answered", Report: "facts"}); err != nil {
		t.Fatal(err)
	}
	bg, err := backgroundAgent("a1")
	if err != nil {
		t.Fatal(err)
	}
	if bg {
		t.Error("an agent that already submitted must not be backgrounded")
	}
	if st, ok, _ := ReadAgentState("a1"); ok && st.Deliver {
		t.Error("a submitted agent must not be switched to the notice path")
	}
	if err := WriteAgentState(AgentState{ID: "a2", Type: "scout", ParentID: "p", Parent: "wP"}); err != nil {
		t.Fatal(err)
	}
	bg, err = backgroundAgent("a2")
	if err != nil {
		t.Fatal(err)
	}
	if !bg {
		t.Error("an agent with no submit must be backgrounded")
	}
	if st, ok, _ := ReadAgentState("a2"); !ok || !st.Deliver {
		t.Errorf("a backgrounded agent must have Deliver set: %+v ok=%v", st, ok)
	}
}
