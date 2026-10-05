package wick

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// stubAgent stands in for a real harness. It is the important half of this test:
// it self-reports through herdr exactly the way pig's integration will, so the
// pane-matching in Wait is exercised against the real herdr, not a mock.
const stubAgent = `#!/usr/bin/env bash
herdr pane report-agent "$HERDR_PANE_ID" --source wick --agent stub --state working --seq 1
sleep 2
herdr pane report-agent "$HERDR_PANE_ID" --source wick --agent stub --state idle --seq 2
while true; do sleep 60; done
`

func TestExecSeamReachesHerdr(t *testing.T) {
	if _, err := exec.LookPath("herdr"); err != nil {
		t.Skip("herdr not on PATH")
	}
	res, err := osExec("herdr", []string{"--version"}, ExecOpts{})
	t.Logf("code=%d err=%v stdout=%q stderr=%q", res.Code, err, res.Stdout, res.Stderr)
	if err != nil {
		t.Fatalf("osExec errored: %v", err)
	}
	if res.Stdout == "" {
		t.Fatal("herdr --version produced no stdout through the exec seam")
	}
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestSpawnWaitFinish(t *testing.T) {
	if _, err := exec.LookPath("herdr"); err != nil {
		t.Skip("herdr not on PATH")
	}

	// Isolated state so the test never touches a real worker store.
	statePath = filepath.Join(t.TempDir(), "workers.json")

	repo := t.TempDir()
	gitOut(t, repo, "init", "-q", "-b", "main")
	gitOut(t, repo, "config", "user.email", "wick@test.local")
	gitOut(t, repo, "config", "user.name", "wick")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitOut(t, repo, "add", ".")
	gitOut(t, repo, "commit", "-qm", "base")

	ticket := filepath.Join(repo, "07-example.md")
	if err := os.WriteFile(ticket, []byte("# Example\n\nBlocked by: \n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stub := filepath.Join(t.TempDir(), "stub-agent")
	if err := os.WriteFile(stub, []byte(stubAgent), 0o755); err != nil {
		t.Fatal(err)
	}

	run := osExec

	// --- spawn: worktree isolation is the whole point --------------------
	worker, err := Spawn(run, SpawnInput{
		Ticket:  ticket,
		Repo:    repo,
		Base:    "main",
		Harness: "stub",
		Launch:  []string{stub},
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	// Teardown is registered before the first assertion on purpose: a failed
	// assertion must never be the reason a workspace is left behind in the
	// user's herdr. herdr opens a workspace for the worktree *and* one for the
	// source checkout, so sweep every workspace whose pane sits under this
	// test's repo rather than closing one by id.
	t.Cleanup(func() {
		_, _ = Finish(run, worker.ID, false)
		reply, err := herdrJSON(run, []string{"pane", "list"}, 30*time.Second)
		if err != nil {
			return
		}
		for _, pane := range reply.Result.Panes {
			if strings.HasPrefix(pane.Cwd, repo) && pane.Workspace != "" {
				_, _ = herdrJSON(run, []string{"workspace", "close", pane.Workspace}, 30*time.Second)
			}
		}
	})

	if worker.ID != "07-example" {
		t.Errorf("id = %q, want 07-example", worker.ID)
	}
	if worker.Branch != "implement/07-example" {
		t.Errorf("branch = %q, want implement/07-example", worker.Branch)
	}
	if _, err := os.Stat(worker.Worktree); err != nil {
		t.Errorf("worktree missing: %v", err)
	}
	if got := gitOut(t, repo, "branch", "--list", worker.Branch); got == "" {
		t.Errorf("branch %s not created", worker.Branch)
	}
	if got := gitOut(t, worker.Worktree, "rev-parse", "--abbrev-ref", "HEAD"); got != worker.Branch {
		t.Errorf("worktree HEAD = %q, want %q", got, worker.Branch)
	}

	// --- wait: the worker settles into idle ------------------------------
	statuses, pending, err := Wait(run, nil, 30*time.Second)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("got %d statuses, want 1", len(statuses))
	}
	if !contains(settled, statuses[0].State) {
		t.Errorf("state = %q, want a settled state; herdr must match the worker by pane, not by its reported name", statuses[0].State)
	}
	t.Logf("worker settled as %q (detail %q)", statuses[0].State, statuses[0].Detail)
	if len(pending) != 0 {
		t.Errorf("pending = %v, want none", pending)
	}

	// --- finish: rebase, fast-forward, drop the worktree -----------------
	if err := os.WriteFile(filepath.Join(worker.Worktree, "feature.txt"), []byte("done\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitOut(t, worker.Worktree, "add", ".")
	gitOut(t, worker.Worktree, "commit", "-qm", "worker work")

	result, err := Finish(run, worker.ID, true)
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if !result.Merged {
		t.Fatalf("merged = false, detail = %q", result.Detail)
	}
	if result.SHA == "" {
		t.Error("finish returned no sha")
	}
	if _, err := os.Stat(filepath.Join(repo, "feature.txt")); err != nil {
		t.Errorf("work not present on main: %v", err)
	}
	if _, err := os.Stat(worker.Worktree); !os.IsNotExist(err) {
		t.Errorf("worktree still on disk after finish")
	}

	if _, _, err := Wait(run, nil, 0); err != nil {
		t.Errorf("store should be empty and readable: %v", err)
	}
}

func TestHerdrNameIsLegalForHerdr(t *testing.T) {
	// herdr rejects a name that does not start with a lowercase letter, and
	// to-tickets numbers its tickets, so 01-probe is not a legal name.
	for _, in := range []string{"01-probe", "probe", "07-a_b", "Fix The Thing", "", "../etc/passwd",
		"this-is-a-very-long-ticket-name-that-overflows-the-limit"} {
		got := herdrName(in)
		if got == "" {
			t.Fatalf("herdrName(%q) returned nothing", in)
		}
		if got[0] < 'a' || got[0] > 'z' {
			t.Errorf("herdrName(%q) = %q must start with a lowercase letter", in, got)
		}
		if len(got) > 32 {
			t.Errorf("herdrName(%q) = %q is %d chars; herdr allows 32", in, got, len(got))
		}
		for _, r := range got {
			legal := r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_'
			if !legal {
				t.Errorf("herdrName(%q) = %q contains %q, which herdr rejects", in, got, r)
			}
		}
	}
	if got := herdrName("01-probe"); got != "w01-probe" {
		t.Errorf("a numbered ticket needs a letter prefix, got %q", got)
	}
	if got := herdrName("probe"); got != "probe" {
		t.Errorf("an already-legal name must be left alone, got %q", got)
	}
}
