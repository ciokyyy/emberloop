package tui

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// The live program's herdr seam: `herdr pane list` through a command runner,
// so tests drive Snapshot's liveness without a multiplexer.

func cannedPanes() []byte {
	return []byte(`{"result":{"panes":[{"pane_id":"w1:alive","agent_status":"working"},{"pane_id":"w1:busy","agent_status":"idle"}]}}`)
}

func TestHerdrPanesListsLivePanes(t *testing.T) {
	t.Setenv("HERDR_BIN_PATH", "")
	herdr := HerdrPanes(func(name string, args ...string) ([]byte, error) {
		if name != "herdr" {
			t.Fatalf("binary = %q, want herdr", name)
		}
		return cannedPanes(), nil
	})
	live, err := herdr()
	if err != nil {
		t.Fatalf("HerdrPanes: %v", err)
	}
	if live["w1:alive"] != "working" || live["w1:busy"] != "idle" {
		t.Fatalf("live = %v, want working and idle statuses", live)
	}
	if _, ok := live["w1:dead"]; ok {
		t.Fatalf("live = %v, want alive and busy only", live)
	}
}

func TestHerdrPanesFailsOnBadOutput(t *testing.T) {
	herdr := HerdrPanes(func(name string, args ...string) ([]byte, error) {
		return []byte("not json"), nil
	})
	if _, err := herdr(); err == nil {
		t.Error("non-JSON pane list must fail")
	}
	herdr = HerdrPanes(func(name string, args ...string) ([]byte, error) {
		return nil, errors.New("herdr down")
	})
	if _, err := herdr(); err == nil {
		t.Error("a failing herdr must fail")
	}
}

func TestCurrentRepoPrefersTheGitTopLevel(t *testing.T) {
	repo := CurrentRepo(func(name string, args ...string) ([]byte, error) {
		return []byte("/repo/root\n"), nil
	}, "/repo/root/sub")
	if repo != "/repo/root" {
		t.Errorf("repo = %q, want the top level", repo)
	}
	repo = CurrentRepo(func(name string, args ...string) ([]byte, error) {
		return nil, errors.New("not a repo")
	}, "/cwd")
	if repo != "/cwd" {
		t.Errorf("outside git the cwd is the repo, got %q", repo)
	}
}

// The jump primitive: focusing an agent's herdr tab and pane through the
// CommandRunner seam, so tests drive the focus without a multiplexer.

// jumpPaneListReply is a canned `herdr pane list`: two panes sharing one
// tab, plus the orchestrator alone in another.
func jumpPaneListReply() []byte {
	return []byte(`{"result":{"panes":[{"pane_id":"pane-w1","tab_id":"tab-a","agent_status":"working"},{"pane_id":"pane-rs","tab_id":"tab-a","agent_status":"idle"},{"pane_id":"pane-orch","tab_id":"tab-o","agent_status":"working"}]}}`)
}

// recordJumpCalls returns a runner that records every invocation as one
// string and serves the canned pane list for `pane list`.
func recordJumpCalls(calls *[]string) CommandRunner {
	return func(name string, args ...string) ([]byte, error) {
		*calls = append(*calls, strings.Join(append([]string{name}, args...), " "))
		if len(args) >= 2 && args[0] == "pane" && args[1] == "list" {
			return jumpPaneListReply(), nil
		}
		return []byte(`{}`), nil
	}
}

// noPaneChanges fails the test when a jump invocation closes, moves or
// splits panes: jumping only ever focuses.
func noPaneChanges(t *testing.T, calls []string) {
	t.Helper()
	for _, c := range calls {
		for _, field := range strings.Fields(c) {
			switch field {
			case "close", "move", "split":
				t.Errorf("jump ran %q, want focus only", c)
			}
		}
	}
}

func TestFocusAgentPaneFocusesItsTabThenItsPane(t *testing.T) {
	t.Setenv("HERDR_BIN_PATH", "")
	var calls []string
	if err := focusAgentPane(recordJumpCalls(&calls), "pane-w1"); err != nil {
		t.Fatalf("focusAgentPane: %v", err)
	}
	want := []string{"herdr pane list", "herdr tab focus tab-a", "herdr agent focus pane-w1"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %q, want %q", calls, want)
	}
	noPaneChanges(t, calls)
}

func TestFocusAgentPaneGoneMakesNoFocusCalls(t *testing.T) {
	t.Setenv("HERDR_BIN_PATH", "")
	var calls []string
	err := focusAgentPane(recordJumpCalls(&calls), "pane-dead")
	if !errors.Is(err, errPaneGone) {
		t.Fatalf("err = %v, want the pane-gone error", err)
	}
	if !reflect.DeepEqual(calls, []string{"herdr pane list"}) {
		t.Fatalf("calls = %q, want only the pane list lookup", calls)
	}
}

func TestFocusAgentEmptyPaneMakesNoCalls(t *testing.T) {
	var calls []string
	if err := focusAgentPane(recordJumpCalls(&calls), ""); !errors.Is(err, errPaneGone) {
		t.Fatalf("err = %v, want the pane-gone error", err)
	}
	if len(calls) != 0 {
		t.Fatalf("calls = %q, want none without a pane", calls)
	}
}
