package web

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func prepareFinalGateRun(t *testing.T, tickets map[string]string) (string, string, string, *Daemon, string) {
	t.Helper()
	repo, _ := initRunRepo(t, tickets)
	master := strings.TrimSpace(testGit(t, repo, "rev-parse", "main"))
	testGit(t, repo, "branch", "spec/demo", master)
	d := NewDaemon(tempDir(t), NewFakeBackend())
	runID := "final-gate-test"
	d.mu.Lock()
	d.runs[runID] = &runState{
		ID:         runID,
		Repo:       repo,
		SpecBranch: "spec/demo",
		Status:     runStatusRunning,
		Merged:     map[string]string{},
		Discarded:  map[string]bool{},
		unpause:    make(chan struct{}),
	}
	d.mu.Unlock()
	return repo, runID, "refs/heads/main", d, master
}

func TestFinalMergeBuildFailureNeedsYouAndKeepsMaster(t *testing.T) {
	repo, runID, masterRef, d, master := prepareFinalGateRun(t, nil)
	var checks atomic.Int32
	d.runFinalBuild = func(worktree string) (string, error) {
		if err := writeGeneratedTestFiles(worktree); err != nil {
			return "", err
		}
		return "npm install output", errors.New("npm install failed")
	}
	d.runRepoCheck = func(string) (string, error) {
		checks.Add(1)
		return "green", nil
	}

	out, err := d.finalizeSpecRun(runID)
	if err != nil || !out.NeedsYou {
		t.Fatalf("final merge = %+v, %v; want recoverable Needs-you", out, err)
	}
	if !strings.Contains(out.Reason, "npm install output") || !containsString(out.Files, "emberloop-web/web/dist/index.html") {
		t.Fatalf("build failure diagnostics = %q files=%v", out.Reason, out.Files)
	}
	if checks.Load() != 0 {
		t.Fatalf("repo check calls = %d, want none after failed build", checks.Load())
	}
	if got := strings.TrimSpace(testGit(t, repo, "rev-parse", masterRef)); got != master {
		t.Fatalf("master = %s after build failure, want unchanged %s", got, master)
	}
	if got := strings.TrimSpace(testGit(t, repo, "rev-parse", "spec/demo")); got != master {
		t.Fatalf("spec branch = %s after failed build, want unchanged %s", got, master)
	}
}

func TestFinalMergeNeedsYouWhenSpecBranchMovesDuringBuild(t *testing.T) {
	repo, runID, masterRef, d, master := prepareFinalGateRun(t, nil)
	initialSpec := strings.TrimSpace(testGit(t, repo, "rev-parse", "spec/demo"))
	var checks atomic.Int32
	var movedSpec string
	d.runFinalBuild = func(worktree string) (string, error) {
		if err := writeGeneratedTestFiles(worktree); err != nil {
			return "", err
		}
		tree := strings.TrimSpace(testGit(t, repo, "rev-parse", initialSpec+"^{tree}"))
		movedSpec = strings.TrimSpace(testGit(t, repo,
			"-c", "user.name=web-test", "-c", "user.email=test@example.com",
			"commit-tree", tree, "-p", initialSpec, "-m", "concurrent spec movement"))
		testGit(t, repo, "update-ref", "refs/heads/spec/demo", movedSpec, initialSpec)
		return "generated", nil
	}
	d.runRepoCheck = func(string) (string, error) {
		checks.Add(1)
		return "green", nil
	}

	out, err := d.finalizeSpecRun(runID)
	if err != nil || !out.NeedsYou || !strings.Contains(out.Reason, "spec branch moved") {
		t.Fatalf("final merge = %+v, %v; want spec-ref Needs-you", out, err)
	}
	if checks.Load() != 0 {
		t.Fatalf("repo check calls = %d, want none after spec-ref CAS failure", checks.Load())
	}
	if got := strings.TrimSpace(testGit(t, repo, "rev-parse", masterRef)); got != master {
		t.Fatalf("master = %s after spec-ref CAS failure, want unchanged %s", got, master)
	}
	if got := strings.TrimSpace(testGit(t, repo, "rev-parse", "spec/demo")); got != movedSpec {
		t.Fatalf("spec/demo = %s, want concurrent tip %s", got, movedSpec)
	}
}

func TestFinalMergeNeedsYouWhenMasterMovesDuringCheck(t *testing.T) {
	repo, runID, masterRef, d, master := prepareFinalGateRun(t, nil)
	d.runFinalBuild = func(worktree string) (string, error) {
		return "generated", writeGeneratedTestFiles(worktree)
	}
	var movedMaster string
	d.runRepoCheck = func(string) (string, error) {
		writeFile(t, repo, "master-during-check.txt", "concurrent commit\n")
		testGit(t, repo, "add", "master-during-check.txt")
		testGit(t, repo, "commit", "-m", "master moves during check")
		movedMaster = strings.TrimSpace(testGit(t, repo, "rev-parse", masterRef))
		return "green", nil
	}

	out, err := d.finalizeSpecRun(runID)
	if err != nil || !out.NeedsYou || !strings.Contains(out.Reason, "master moved") {
		t.Fatalf("final merge = %+v, %v; want master-move Needs-you", out, err)
	}
	if movedMaster == "" || movedMaster == master {
		t.Fatalf("master did not move in the check seam: %q", movedMaster)
	}
	if !containsString(out.Files, "master-during-check.txt") {
		t.Fatalf("final files = %v, want concurrent master file", out.Files)
	}
	if got := strings.TrimSpace(testGit(t, repo, "rev-parse", "HEAD")); got != movedMaster {
		t.Fatalf("checked-out HEAD = %s, want external master tip %s", got, movedMaster)
	}
}

func TestFinalMergeNeedsCleanCheckoutOfMaster(t *testing.T) {
	for _, test := range []struct {
		name      string
		prepare   func(t *testing.T, repo string)
		wantFile  string
		wantError string
	}{
		{
			name: "dirty checkout",
			prepare: func(t *testing.T, repo string) {
				writeFile(t, repo, "local-edit.txt", "keep me\n")
			},
			wantFile:  "local-edit.txt",
			wantError: "clean master checkout",
		},
		{
			name: "checkout on another branch",
			prepare: func(t *testing.T, repo string) {
				testGit(t, repo, "checkout", "-b", "feature")
			},
			wantFile:  "HEAD",
			wantError: "checkout on master",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo, runID, masterRef, d, master := prepareFinalGateRun(t, nil)
			test.prepare(t, repo)
			d.runFinalBuild = func(worktree string) (string, error) {
				return "generated", writeGeneratedTestFiles(worktree)
			}
			d.runRepoCheck = func(string) (string, error) { return "green", nil }

			out, err := d.finalizeSpecRun(runID)
			if err != nil || !out.NeedsYou || !strings.Contains(out.Reason, test.wantError) {
				t.Fatalf("final merge = %+v, %v; want Needs-you for %s", out, err, test.name)
			}
			if !containsString(out.Files, test.wantFile) {
				t.Fatalf("final files = %v, want %q", out.Files, test.wantFile)
			}
			if got := strings.TrimSpace(testGit(t, repo, "rev-parse", masterRef)); got != master {
				t.Fatalf("master = %s, want unchanged %s", got, master)
			}
		})
	}
}

func TestGenericRepoSkipsWebBuildAndNeedsYouWithoutRepoCheck(t *testing.T) {
	repo, runID, masterRef, d, master := prepareFinalGateRun(t, nil)
	out, err := d.finalizeSpecRun(runID)
	if err != nil || !out.NeedsYou {
		t.Fatalf("final merge = %+v, %v; want Needs-you for missing check", out, err)
	}
	if !strings.Contains(out.Reason, "required repository check .emberloop/check is missing") || !containsString(out.Files, ".emberloop/check") {
		t.Fatalf("missing-check diagnostics = %q files=%v", out.Reason, out.Files)
	}
	if got := strings.TrimSpace(testGit(t, repo, "rev-parse", masterRef)); got != master {
		t.Fatalf("master = %s without a repo check, want unchanged %s", got, master)
	}
	if got := strings.TrimSpace(testGit(t, repo, "rev-parse", "spec/demo")); got != master {
		t.Fatalf("spec branch = %s without a repo check, want unchanged %s", got, master)
	}
}

func TestFinalMergeWaitsForEveryTicketToMergeOrDiscard(t *testing.T) {
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md":  "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
		"02-second.md": "# 02: Second\n\n**Blocked by:** 01\n\n**Status:** open\n\n- [ ] beta\n",
	})
	initialMaster := strings.TrimSpace(testGit(t, repo, "rev-parse", "main"))
	d := NewDaemon(tempDir(t), NewFakeBackend())
	var builds, checks atomic.Int32
	d.runFinalBuild = func(string) (string, error) { builds.Add(1); return "", nil }
	d.runRepoCheck = func(string) (string, error) { checks.Add(1); return "green", nil }
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "first worker started")
	submitSession(t, srv.URL, ticketByID(r, "01-first").WorkerSession, "done", []string{"alpha"})
	passReviewers(t, srv.URL, run.ID, "01-first")
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").State == "merged" && ticketByID(r, "02-second").WorkerSession != ""
	}, "first merged and second started")
	if code, _ := postTicketAction(t, srv.URL, run.ID, "02-second", "park", ""); code != http.StatusOK {
		t.Fatalf("park second ticket status = %d, want 200", code)
	}
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool { return r.Status == runStatusNeedsYou }, "incomplete spec Needs-you")
	secondPath := filepath.ToSlash(filepath.Join(".scratch", "demo", "issues", "02-second.md"))
	if !containsString(r.FinalMergeFiles, secondPath) || !strings.Contains(r.Error, "02-second") {
		t.Fatalf("incomplete final gate = %q files=%v, want second Ticket listed", r.Error, r.FinalMergeFiles)
	}
	if builds.Load() != 0 || checks.Load() != 0 {
		t.Fatalf("partial spec ran build/check %d/%d times", builds.Load(), checks.Load())
	}
	if got := strings.TrimSpace(testGit(t, repo, "rev-parse", "main")); got != initialMaster {
		t.Fatalf("master = %s with an unresolved Ticket, want unchanged %s", got, initialMaster)
	}

	// A Needs-you Run still accepts Ticket actions: discard the parked Ticket,
	// then Resume and let the completed spec pass its final gate.
	if code, _ := postTicketAction(t, srv.URL, run.ID, "02-second", "discard", ""); code != http.StatusOK {
		t.Fatalf("discard unresolved ticket status = %d, want 200", code)
	}
	if _, code := postRunAction(t, srv.URL, run.ID, "resume"); code != http.StatusOK {
		t.Fatalf("resume after discard status = %d, want 200", code)
	}
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool { return r.Status == runStatusDone }, "discarded Ticket final merge")
	if builds.Load() != 1 || checks.Load() != 1 {
		t.Fatalf("build/check calls after complete spec = %d/%d, want 1/1", builds.Load(), checks.Load())
	}
	if got := strings.TrimSpace(testGit(t, repo, "rev-parse", "main")); got != strings.TrimSpace(testGit(t, repo, "rev-parse", "spec/demo")) {
		t.Fatalf("master = %s after discard-and-resume, want spec tip %s", got, strings.TrimSpace(testGit(t, repo, "rev-parse", "spec/demo")))
	}
}

func TestFinalRepoCheckMissingIsExplicit(t *testing.T) {
	worktree := tempDir(t)
	d := NewDaemon(tempDir(t), NewFakeBackend())
	output, err := d.runFinalRepoCheck(worktree)
	if err == nil || !strings.Contains(err.Error(), ".emberloop/check is missing") || output != "" {
		t.Fatalf("missing check = %q, %v; want explicit missing-check error", output, err)
	}
}
func TestRunGeneratedBuildRefreshesWebAndE2ELockfiles(t *testing.T) {
	root := tempDir(t)
	webDir := filepath.Join(root, "emberloop-web", "web")
	e2eDir := filepath.Join(webDir, "e2e")
	if err := os.MkdirAll(e2eDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{filepath.Join(webDir, "package.json"), filepath.Join(e2eDir, "package.json")} {
		if err := os.WriteFile(file, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	bin := tempDir(t)
	npm := filepath.Join(bin, "npm")
	script := `#!/bin/sh
 set -eu
 printf '%s|%s\n' "$PWD" "$*" >> "$EL_NPM_LOG"
 if [ "$PWD" = "$EL_NPM_WEB" ]; then
   case "$*" in
     install*) printf 'web lock\n' > package-lock.json ;;
     'run build') mkdir -p dist; printf 'bundle\n' > dist/index.html ;;
   esac
 elif [ "$PWD" = "$EL_NPM_WEB/e2e" ]; then
   printf 'e2e lock\n' > package-lock.json
 fi
 `
	if err := os.WriteFile(npm, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(tempDir(t), "npm.log")
	t.Setenv("EL_NPM_LOG", log)
	t.Setenv("EL_NPM_WEB", webDir)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	d := NewDaemon(tempDir(t), NewFakeBackend())
	if output, err := d.runGeneratedBuild(root); err != nil {
		t.Fatalf("runGeneratedBuild = %q, %v", output, err)
	}
	for file, want := range map[string]string{
		filepath.Join(webDir, "dist", "index.html"): "bundle\n",
		filepath.Join(webDir, "package-lock.json"):  "web lock\n",
		filepath.Join(e2eDir, "package-lock.json"):  "e2e lock\n",
	} {
		got, err := os.ReadFile(file)
		if err != nil || string(got) != want {
			t.Fatalf("generated %s = %q, %v; want %q", file, got, err, want)
		}
	}
	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	commands := string(raw)
	for _, want := range []string{
		webDir + "|install --no-audit --no-fund",
		webDir + "|run build",
		e2eDir + "|install --package-lock-only --no-audit --no-fund",
	} {
		if !strings.Contains(commands, want) {
			t.Errorf("npm commands %q do not include %q", commands, want)
		}
	}
}
