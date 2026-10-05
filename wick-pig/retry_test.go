package wick

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	errRec  = `{"type":"message","message":{"role":"assistant","stopReason":"error","errorMessage":"Provider returned an empty response","content":[]}}` + "\n"
	doneRec = `{"type":"message","message":{"role":"assistant","stopReason":"stop","content":[{"type":"text","text":"ok"}]}}` + "\n"
	runRec  = `{"type":"message","message":{"role":"assistant","stopReason":"toolUse","content":[]}}` + "\n"
)

func fastRetries(t *testing.T) {
	ob, os_ := retryBackoff, nudgeSettle
	retryBackoff, nudgeSettle = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}, time.Millisecond
	t.Cleanup(func() { retryBackoff, nudgeSettle = ob, os_ })
}

func TestTurnState(t *testing.T) {
	d := t.TempDir()
	for rec, want := range map[string]string{errRec: TurnFailed, doneRec: TurnDone, runRec: TurnRunning} {
		p := filepath.Join(d, "t.jsonl")
		os.WriteFile(p, []byte(rec), 0o644)
		if got, _ := (pigHarness{}).TurnState(p); got != want {
			t.Errorf("%q: %s, want %s", rec, got, want)
		}
	}
}

func TestCheckTurnRetriesThenFails(t *testing.T) {
	fastRetries(t)
	statePath = filepath.Join(t.TempDir(), "workers.json")
	root := t.TempDir()
	t.Setenv("HOME", root)
	w := Worker{ID: "w1", Pane: "p1", Harness: "pig", SessionID: "sid", Worktree: root}
	p := filepath.Join(pigHarness{}.SessionRoot(Session{ID: "sid", Cwd: root}), "x-sid.jsonl")
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, []byte(errRec), 0o644)

	nudges := 0
	run := func(cmd string, args []string, _ ExecOpts) (Result, error) {
		if strings.Contains(strings.Join(args, " "), "prompt") {
			nudges++
		}
		return Result{}, nil
	}
	state := ""
	for i := 0; i < 20 && state != "failed"; i++ {
		state, _ = checkTurn(run, w, "idle", "")
		time.Sleep(3 * time.Millisecond)
	}
	if state != "failed" || nudges != 3 {
		t.Errorf("state %s after %d nudges, want failed after 3", state, nudges)
	}
}

func TestCheckTurnLeavesRunningAlone(t *testing.T) {
	statePath = filepath.Join(t.TempDir(), "workers.json")
	root := t.TempDir()
	t.Setenv("HOME", root)
	w := Worker{ID: "w2", Pane: "p2", Harness: "pig", SessionID: "sid2", Worktree: root}
	p := filepath.Join(pigHarness{}.SessionRoot(Session{ID: "sid2", Cwd: root}), "x-sid2.jsonl")
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, []byte(runRec), 0o644)
	run := func(string, []string, ExecOpts) (Result, error) {
		t.Error("nudged a running turn")
		return Result{}, nil
	}
	if s, _ := checkTurn(run, w, "idle", ""); s == "failed" || s == "retrying" {
		t.Errorf("state %s", s)
	}
}

func TestWaitReportsGonePaneAsFailed(t *testing.T) {
	statePath = filepath.Join(t.TempDir(), "workers.json")
	save(map[string]Worker{"g": {ID: "g", Pane: "x:p1", Started: time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)}})
	run := func(cmd string, args []string, _ ExecOpts) (Result, error) {
		if args[0] == "pane" {
			return Result{Stdout: `{"error":{"code":"pane_not_found"}}`, Code: 1}, nil
		}
		return Result{Stdout: `{"result":{"agents":[]}}`}, nil
	}
	got, pending, err := Wait(run, []string{"g"}, 0)
	if err != nil || len(pending) != 0 || got[0].State != "failed" {
		t.Errorf("got %+v pending %v err %v", got, pending, err)
	}
	// still booting: not listed yet, young record
	save(map[string]Worker{"g": {ID: "g", Pane: "x:p1", Started: time.Now().UTC().Format(time.RFC3339)}})
	got, pending, _ = Wait(run, []string{"g"}, 0)
	if got[0].State != "unknown" || len(pending) != 1 {
		t.Errorf("young worker: %+v pending %v", got, pending)
	}
}
