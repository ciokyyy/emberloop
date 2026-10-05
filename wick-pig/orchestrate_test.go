package wick

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeAgents answers `herdr agent list` with one worker in each named state.
// The pane is p1, the pane the store below puts the worker in.
func fakeAgents(state string) Result {
	return Result{Stdout: `{"result":{"agents":[{"pane_id":"p1","agent_status":"` + state +
		`","name":"w01x","terminal_title_stripped":"working"}]}}`}
}

func withWorker(t *testing.T, id string) {
	t.Helper()
	old := statePath
	statePath = filepath.Join(t.TempDir(), "workers.json")
	t.Cleanup(func() { statePath = old })
	if err := save(map[string]Worker{id: {ID: id, Pane: "p1", Branch: "implement/" + id}}); err != nil {
		t.Fatal(err)
	}
}

// The harness can cut a tool call at MCP_TOOL_TIMEOUT, which is shorter than a
// long wait: a request of an hour must return inside the cap and say that it
// was clamped, so the model knows to call again rather than to treat the wait
// as broken.
func TestWaitToolClampsTimeoutToTheCap(t *testing.T) {
	withWorker(t, "01-x")

	oldCap, oldExec := waitCap, waitExec
	waitCap = 50 * time.Millisecond
	waitExec = (&recorder{reply: func(string, []string) Result { return fakeAgents("working") }}).run
	t.Cleanup(func() { waitCap, waitExec = oldCap, oldExec })

	args, _ := json.Marshal(map[string]any{"timeout_ms": float64(3600000)})
	start := time.Now()
	out, err := waitTool.Execute(context.Background(), "", args)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("wait_workers: %v", err)
	}

	if elapsed > 5*time.Second {
		t.Errorf("an hour-long request blocked for %s: it must return within the cap", elapsed)
	}
	if elapsed < 20*time.Millisecond {
		t.Errorf("returned in %s: the clamp was not applied, the request was ignored", elapsed)
	}
	if !strings.Contains(out, "clamped") {
		t.Errorf("a clamped wait must say so, got %q", out)
	}
	if !strings.Contains(out, "wait_workers") {
		t.Errorf("a clamped wait must tell the caller what to do next, got %q", out)
	}
}

func TestWaitToolUnderTheCapIsNotClamped(t *testing.T) {
	withWorker(t, "01-x")

	oldExec := waitExec
	waitExec = (&recorder{reply: func(string, []string) Result { return fakeAgents("working") }}).run
	t.Cleanup(func() { waitExec = oldExec })

	// timeout_ms 0 returns now, and a request below the cap is honoured as-is,
	// so neither may claim to have been clamped.
	args, _ := json.Marshal(map[string]any{"timeout_ms": float64(0)})
	out, err := waitTool.Execute(context.Background(), "", args)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "clamped") {
		t.Errorf("a wait under the cap must not claim a clamp, got %q", out)
	}
	if !strings.Contains(out, "still running") {
		t.Errorf("the worker is still working, got %q", out)
	}
}

func TestWaitToolDescriptionStatesTheCap(t *testing.T) {
	if WaitCap != 5*time.Minute {
		t.Errorf("WaitCap = %s, want 5m", WaitCap)
	}
	if waitCap != WaitCap {
		t.Errorf("waitCap = %s, want the default %s", waitCap, WaitCap)
	}
	if !strings.Contains(waitTool.desc, "300000") {
		t.Errorf("the description must state the cap the tool enforces, got %q", waitTool.desc)
	}
}
