package el

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The web driver reuses the CLI merge checks through the web exports:
// box ticks, the done mark and the rebase plus fast-forward-only merge.
func TestTickDriverTicketFileTicksNamedBoxes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "01-first.md")
	raw := "# 01: First\n\n**Blocked by:** None\n\n- [ ] alpha\n- [ ] beta\n"
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	unknown, err := TickDriverTicketFile(path, []string{"alpha", "gamma"})
	if err != nil {
		t.Fatalf("TickDriverTicketFile: %v", err)
	}
	if len(unknown) != 1 || unknown[0] != "gamma" {
		t.Fatalf("unknown = %v, want [gamma]", unknown)
	}
	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "- [x] alpha") || !strings.Contains(string(out), "- [ ] beta") {
		t.Fatalf("ticked file = %q, want alpha ticked and beta open", out)
	}
}

// The done mark sets Status done and ticks every box, leaving the commit
// to the caller.
func TestMarkDriverTicketDoneMarksStatusAndBoxes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "01-first.md")
	raw := "# 01: First\n\nStatus: open\n\n**Blocked by:** None\n\n- [x] alpha\n- [ ] beta\n"
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := MarkDriverTicketDone(path)
	if err != nil {
		t.Fatalf("MarkDriverTicketDone: %v", err)
	}
	if !changed {
		t.Fatalf("changed = false, want true")
	}
	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "Status: done") {
		t.Fatalf("done file = %q, want Status done", out)
	}
	if !strings.Contains(string(out), "- [x] beta") {
		t.Fatalf("done file = %q, want every box ticked", out)
	}
	if _, err := MarkDriverTicketDone(filepath.Join(t.TempDir(), "missing.md")); err == nil {
		t.Fatalf("missing file err = nil, want an error")
	}
}

func gitTestEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_AUTHOR_NAME", "el-test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "el-test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("HOME", t.TempDir())
}

func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	res, err := OSExec("git", args, ExecOpts{Cwd: dir})
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	if res.Code != 0 {
		t.Fatalf("git %v: code %d: %s%s", args, res.Code, res.Stderr, res.Stdout)
	}
	return strings.TrimSpace(res.Stdout)
}

func initTestRepo(t *testing.T) string {
	t.Helper()
	gitTestEnv(t)
	return templatedAtxtRepo(t)
}

// The shared merge rebases the branch onto the target and fast-forwards it.
func TestMergeCheckedBranchMergesRebasedBranch(t *testing.T) {
	repo := initTestRepo(t)
	tree := filepath.Join(t.TempDir(), "wt")
	gitRun(t, repo, "worktree", "add", "-b", "implement/w", tree, "HEAD")
	if err := os.WriteFile(filepath.Join(tree, "b.txt"), []byte("b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, tree, "add", "b.txt")
	gitRun(t, tree, "commit", "-m", "worker change")
	if err := os.WriteFile(filepath.Join(repo, "c.txt"), []byte("c\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, repo, "add", "c.txt")
	gitRun(t, repo, "commit", "-m", "target moves on")

	sha, kept, err := MergeCheckedBranch(OSExec, repo, tree, "implement/w", "main", "main", "have it commit them and submit done again")
	if err != nil {
		t.Fatalf("MergeCheckedBranch: %v", err)
	}
	if kept != "" {
		t.Fatalf("kept = %q, want a merge", kept)
	}
	if want := gitRun(t, repo, "rev-parse", "implement/w"); sha != want {
		t.Fatalf("sha = %q, want %q", sha, want)
	}
	if got := gitRun(t, repo, "rev-parse", "main"); got != sha {
		t.Fatalf("main = %q, want the merged %q", got, sha)
	}
}

// A dirty worktree is kept with the caller's advice, never merged.
func TestMergeCheckedBranchKeepsDirtyWorktree(t *testing.T) {
	repo := initTestRepo(t)
	tree := filepath.Join(t.TempDir(), "wt")
	gitRun(t, repo, "worktree", "add", "-b", "implement/w", tree, "HEAD")
	if err := os.WriteFile(filepath.Join(tree, "dirty.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, kept, err := MergeCheckedBranch(OSExec, repo, tree, "implement/w", "main", "main", "have it commit them and submit done again")
	if err != nil {
		t.Fatalf("MergeCheckedBranch: %v", err)
	}
	want := "the worktree has uncommitted changes, which would be lost; have it commit them and submit done again"
	if kept != want {
		t.Fatalf("kept = %q, want %q", kept, want)
	}
}

// A rebase conflict aborts and keeps the branch unchanged.
func TestMergeCheckedBranchKeepsRebaseConflict(t *testing.T) {
	repo := initTestRepo(t)
	tree := filepath.Join(t.TempDir(), "wt")
	gitRun(t, repo, "worktree", "add", "-b", "implement/w", tree, "HEAD")
	if err := os.WriteFile(filepath.Join(tree, "a.txt"), []byte("worker\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, tree, "add", "a.txt")
	gitRun(t, tree, "commit", "-m", "worker edits a")
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("target\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, repo, "add", "a.txt")
	gitRun(t, repo, "commit", "-m", "target edits a")

	before := gitRun(t, repo, "rev-parse", "implement/w")
	_, kept, err := MergeCheckedBranch(OSExec, repo, tree, "implement/w", "main", "main", "have it commit them and submit done again")
	if err != nil {
		t.Fatalf("MergeCheckedBranch: %v", err)
	}
	if !strings.Contains(kept, "rebase onto main conflicted in a.txt") {
		t.Fatalf("kept = %q, want the rebase conflict naming a.txt", kept)
	}
	// The refusal tells the Worker to redo the rebase and resolve it itself;
	// a bare "branch unchanged" read as a status and the Worker resubmitted
	// done unchanged until the same-refusal alert fired.
	for _, want := range []string{"`git rebase main`", "`git rebase --continue`", "submit done again"} {
		if !strings.Contains(kept, want) {
			t.Fatalf("kept = %q, want the resolve instruction %q", kept, want)
		}
	}
	if got := gitRun(t, repo, "rev-parse", "implement/w"); got != before {
		t.Fatalf("branch moved to %q, want it unchanged at %q", got, before)
	}
}
