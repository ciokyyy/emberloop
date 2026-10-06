package tui

import (
	"errors"
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
