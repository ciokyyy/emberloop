package web

import (
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// The repo check reuses one worktree path per repo, so Go's caches hit, and
// cleans it between tips so leftovers never leak into the next check.
func TestRepoCheckReusesCleanWorktree(t *testing.T) {
	t.Parallel()
	repo, _ := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	// The check leaves a stray file and fails if it finds one already there.
	writeFile(t, repo, ".emberloop/check", "#!/bin/sh\ntest -e stray && exit 1\npwd > ../check-pwd\ntouch stray\n")
	testGit(t, repo, "add", ".emberloop/check")
	testGit(t, repo, "commit", "-m", "add check")
	first := strings.TrimSpace(testGit(t, repo, "rev-parse", "HEAD"))
	testGit(t, repo, "commit", "--allow-empty", "-m", "second tip")
	second := strings.TrimSpace(testGit(t, repo, "rev-parse", "HEAD"))
	out := filepath.Join(repo, ".el-worktrees", "check-pwd")

	var dirs []string
	for _, tip := range []string{first, second, first} {
		if green, failing := runRepoCheckAtTip(repo, tip); !green {
			t.Fatalf("check at %s = red %q, want green on a clean worktree", tip, failing)
		}
		raw, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		dirs = append(dirs, strings.TrimSpace(string(raw)))
	}
	if dirs[0] != dirs[1] || dirs[1] != dirs[2] {
		t.Fatalf("check dirs = %q, want one reused path", dirs)
	}
	if got := strings.TrimSpace(testGit(t, dirs[2], "rev-parse", "HEAD")); got != first {
		t.Fatalf("check worktree HEAD = %s, want %s", got, first)
	}
}

// A spec-branch tip that a Ticket tip check already passed needs no base
// check: a fast-forward merge lands that same commit.
func TestBaseCheckReusesGreenTipCheck(t *testing.T) {
	t.Parallel()
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	var checks atomic.Int32
	d.baseCheck = func(_, _, _ string) (bool, []string) {
		checks.Add(1)
		return false, []string{"TestShouldNotRun"}
	}
	d.mu.Lock()
	d.runs["r1"] = &runState{ID: "r1", TipCheckCache: map[string]baseCheckResult{"abc": {OK: true}}}
	d.mu.Unlock()
	if !d.baseCheckReady("r1", "", "spec/x", "abc") {
		t.Fatal("base check not ready for a tip its tip check passed")
	}
	if green, failing := d.cachedBaseCheck("r1", "", "spec/x", "abc"); !green {
		t.Fatalf("base check = red %q, want the green tip check reused", failing)
	}
	if got := checks.Load(); got != 0 {
		t.Fatalf("base checks run = %d, want none", got)
	}
}

// A missing base check starts once in the background; callers for the same
// tip share it instead of running their own.
func TestBaseCheckRunsOnceInBackground(t *testing.T) {
	t.Parallel()
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	release := make(chan struct{})
	var checks atomic.Int32
	d.baseCheck = func(_, _, _ string) (bool, []string) {
		checks.Add(1)
		<-release
		return true, nil
	}
	d.mu.Lock()
	d.runs["r1"] = &runState{ID: "r1"}
	d.mu.Unlock()
	if d.baseCheckReady("r1", "", "spec/x", "abc") {
		t.Fatal("base check ready before it ran")
	}
	waitUntil(t, "background check to start", func() bool { return checks.Load() == 1 })
	if d.baseCheckReady("r1", "", "spec/x", "abc") {
		t.Fatal("base check ready while it still runs")
	}
	shared := make(chan bool)
	go func() {
		green, _ := d.cachedBaseCheck("r1", "", "spec/x", "abc")
		shared <- green
	}()
	time.Sleep(20 * time.Millisecond)
	close(release)
	if !<-shared {
		t.Fatal("waiting caller read red, want the shared green result")
	}
	if !d.baseCheckReady("r1", "", "spec/x", "abc") {
		t.Fatal("base check not ready after it finished")
	}
	if got := checks.Load(); got != 1 {
		t.Fatalf("base checks run = %d, want one shared run", got)
	}
}
