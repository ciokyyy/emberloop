package web

import (
	"path/filepath"
	"strings"
	"testing"
)

func checkRepo(t *testing.T, script string) (repo, tip string) {
	t.Helper()
	repo, _ = initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	writeFile(t, repo, ".emberloop/check", script)
	testGit(t, repo, "add", ".emberloop/check")
	testGit(t, repo, "commit", "-m", "add check")
	return repo, strings.TrimSpace(testGit(t, repo, "rev-parse", "HEAD"))
}

// A tip without a check file is green even when the file exists untracked
// on disk: git words that case "exists on disk, but not in".
func TestRepoCheckMissingAtTipIsGreenEvenWhenUntrackedOnDisk(t *testing.T) {
	t.Parallel()
	repo, _ := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	tip := strings.TrimSpace(testGit(t, repo, "rev-parse", "HEAD"))
	writeFile(t, repo, ".emberloop/check", "#!/bin/sh\nexit 1\n")
	if green, failing := runRepoCheckAtTip(repo, tip); !green {
		t.Fatalf("check = red %q, want green: the tip has no check file", failing)
	}
	for _, msg := range []string{
		"fatal: path '.emberloop/check' does not exist in 'abc'",
		"fatal: path '.emberloop/check' exists on disk, but not in 'abc'",
	} {
		if !checkFileAbsentAtTip(msg) {
			t.Errorf("checkFileAbsentAtTip(%q) = false, want true", msg)
		}
	}
	if checkFileAbsentAtTip("fatal: bad object abc") {
		t.Error("a bad object is not an absent check file")
	}
}

// A red check reruns: a flake that passes the second time is green.
func TestRepoCheckRerunsAndPassesAFlake(t *testing.T) {
	t.Parallel()
	repo, tip := checkRepo(t, "#!/bin/sh\nif [ -e ../flake-seen ]; then exit 0; fi\ntouch ../flake-seen\necho 'FAIL example.com/pkg 600.1s'\nexit 1\n")
	if green, failing := runRepoCheckAtTip(repo, tip); !green {
		t.Fatalf("check = red %q, want the flake to pass on rerun", failing)
	}
}

// A check that fails on different tests every run is a flaky machine: the
// tip gate reads it as unverifiable, not as a Worker fault.
func TestRepoCheckFlakingOnDifferentTestsIsInfra(t *testing.T) {
	t.Parallel()
	repo, tip := checkRepo(t, "#!/bin/sh\nn=$(cat ../n 2>/dev/null || echo 0)\nn=$((n+1))\necho $n > ../n\necho \"--- FAIL: TestFlake$n (0.1s)\"\necho 'FAIL'\nexit 1\n")
	green, failing := runRepoCheckAtTip(repo, tip)
	if green {
		t.Fatal("check = green, want red")
	}
	if len(failing) != 1 || !strings.HasPrefix(failing[0], checkFlakedPrefix) {
		t.Fatalf("failing = %q, want one flaked entry", failing)
	}
	if !isCheckInfraError(failing) {
		t.Fatalf("failing = %q, want an infra error so the tip gate fails open", failing)
	}
}

// A test that fails in every run is a real failure and the one a Worker is
// told about; tests that failed in only some runs are dropped.
func TestRepoCheckKeepsTestsFailingInEveryRun(t *testing.T) {
	t.Parallel()
	repo, tip := checkRepo(t, "#!/bin/sh\nn=$(cat ../n 2>/dev/null || echo 0)\nn=$((n+1))\necho $n > ../n\necho '--- FAIL: TestReal (0.1s)'\necho \"--- FAIL: TestFlake$n (0.1s)\"\nexit 1\n")
	green, failing := runRepoCheckAtTip(repo, tip)
	if green || len(failing) != 1 || failing[0] != "TestReal" {
		t.Fatalf("check = green %v failing %q, want only TestReal", green, failing)
	}
}

// A check that stays red is red, and the banner shows why: the failing
// test, or the output tail when no test line parses.
func TestRepoCheckStaysRedAndNamesTheCause(t *testing.T) {
	t.Parallel()
	repo, tip := checkRepo(t, "#!/bin/sh\necho 'sh: 1: bun: not found'\nexit 127\n")
	green, failing := runRepoCheckAtTip(repo, tip)
	if green {
		t.Fatal("check = green, want red")
	}
	if len(failing) != 1 || !strings.Contains(failing[0], "bun: not found") {
		t.Fatalf("failing = %q, want the output tail naming bun", failing)
	}
	if isCheckInfraError(failing) {
		t.Fatalf("failing = %q read as an infra error", failing)
	}
}

func TestCheckOutputTailKeepsOnlyTheLastLines(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	for i := 0; i < 20; i++ {
		b.WriteString("line\n")
	}
	b.WriteString("last\n")
	got := checkOutputTail(b.String())
	if !strings.HasSuffix(got, "last") || strings.Count(got, "|") != 7 {
		t.Fatalf("tail = %q, want the last 8 lines", got)
	}
	if checkOutputTail("  \n\n") != "" {
		t.Fatal("blank output must give an empty tail")
	}
}

// Only the runner's own "could not run" entries are infra errors; a check
// whose output merely says so is red.
func TestIsCheckInfraErrorReadsOnlyRunnerEntries(t *testing.T) {
	t.Parallel()
	if !isCheckInfraError([]string{"base check could not run: worktree: x"}) {
		t.Error("runner entry must read as infra")
	}
	if isCheckInfraError([]string{"check failed: playwright could not run | exit 1"}) {
		t.Error("output text must not read as infra")
	}
}

func TestDiffBasePromptNamesTheTicketsOwnDiff(t *testing.T) {
	t.Parallel()
	base := strings.Repeat("a", 40)
	tip := strings.Repeat("b", 40)
	got := diffBasePrompt(base, tip)
	for _, want := range []string{"git diff " + shortSHA(base) + " " + shortSHA(tip), "earlier merged tickets", "never flag"} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt = %q, want %q", got, want)
		}
	}
	if diffBasePrompt("", tip) != "" || diffBasePrompt(base, "") != "" {
		t.Error("unknown base or tip must give no note")
	}
}

// The Worker idles while the Daemon checks its tip; that check counts as
// activity, so the ten-minute idle alert does not fire on a healthy ticket.
func TestTipCheckInFlightCountsAsTicketActivity(t *testing.T) {
	t.Parallel()
	holdRunTestSlot(t)
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	started := make(chan struct{})
	release := make(chan struct{})
	d.tipCheck = func(_, _, _ string) (bool, []string) {
		close(started)
		<-release
		return true, nil
	}
	d.mu.Lock()
	rs := &runState{ID: "r1", Repo: filepath.Join(tempDir(t), "repo"), Branches: map[string]string{"01-a": "implement/01-a"}, notify: make(chan struct{})}
	d.runs["r1"] = rs
	d.mu.Unlock()
	done := make(chan bool)
	go func() { done <- d.checkTipBeforeReview("r1", "01-a", "tip1") }()
	<-started
	d.mu.Lock()
	during := rs.tipChecking["01-a"]
	d.mu.Unlock()
	close(release)
	if skip := <-done; skip {
		t.Fatal("green tip check must not skip the reviewers")
	}
	d.mu.Lock()
	after := rs.tipChecking["01-a"]
	d.mu.Unlock()
	if during != 1 || after != 0 {
		t.Fatalf("tipChecking during/after = %d/%d, want 1/0", during, after)
	}
}

// Merges into one spec branch share a lock, so two finishing tickets never
// race the fast-forward; other branches and repos do not wait on it.
func TestSpecMergeLockIsPerRepoAndBranch(t *testing.T) {
	t.Parallel()
	a := specMergeLock("/r", "spec/a")
	if a != specMergeLock("/r/", "spec/a") {
		t.Error("same repo and branch must share one lock")
	}
	if a == specMergeLock("/r", "spec/b") || a == specMergeLock("/other", "spec/a") {
		t.Error("another branch or repo must not share the lock")
	}
}
