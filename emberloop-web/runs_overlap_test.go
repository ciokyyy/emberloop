package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type overlapWarningJSON struct {
	OtherRun    string   `json:"otherRun"`
	OtherBranch string   `json:"otherBranch"`
	Files       []string `json:"files"`
	Message     string   `json:"message"`
}

// initTwoSpecRepo builds one repo holding two independent specs with
// distinct ticket ids, so their ticket branches and worktrees never
// collide while their spec branches share the repo. Each spec chains a
// second ticket behind its first, so both Runs stay live while the first
// tickets merge and the overlap check sees two running Runs.
func initTwoSpecRepo(t *testing.T) (repo, demoSpec, otherSpec string) {
	t.Helper()
	t.Setenv("GIT_AUTHOR_NAME", "web-test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "web-test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("HOME", tempDir(t))
	repo = tempDir(t)
	testGit(t, repo, "init", "-b", "main", ".")
	writeFile(t, repo, ".scratch/demo/spec.md", "# Demo spec\n")
	writeFile(t, repo, ".scratch/demo/issues/01-first.md", "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n")
	writeFile(t, repo, ".scratch/demo/issues/02-second.md", "# 02: Second\n\n**Blocked by:** 01\n\n**Status:** open\n\n- [ ] beta\n")
	writeFile(t, repo, ".scratch/other/spec.md", "# Other spec\n")
	writeFile(t, repo, ".scratch/other/issues/01-other.md", "# 01: Other\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n")
	writeFile(t, repo, ".scratch/other/issues/02-followup.md", "# 02: Followup\n\n**Blocked by:** 01\n\n**Status:** open\n\n- [ ] beta\n")
	writeFile(t, repo, "AGENTS.md", "# AGENTS.md — demo\n\n## Child DOX Index\n\n(none)\n")
	testGit(t, repo, "add", "-A")
	testGit(t, repo, "commit", "-m", "spec")
	return repo, filepath.Join(repo, ".scratch", "demo", "spec.md"), filepath.Join(repo, ".scratch", "other", "spec.md")
}

func getWarnings(t *testing.T, base, id string) []overlapWarningJSON {
	t.Helper()
	resp, err := http.Get(base + "/api/runs/" + id)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get status = %d, want 200", resp.StatusCode)
	}
	var out struct {
		Run struct {
			Warnings []overlapWarningJSON `json:"warnings"`
		} `json:"run"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.Run.Warnings
}

func waitWarnings(t *testing.T, base, id string, want bool, what string) []overlapWarningJSON {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		w := getWarnings(t, base, id)
		if (len(w) > 0) == want {
			return w
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for warnings=%v on %s: got %v", want, what, w)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func commitWorktreeFile(t *testing.T, worktree, name, content, msg string) {
	t.Helper()
	writeFile(t, worktree, name, content)
	testGit(t, worktree, "add", name)
	testGit(t, worktree, "commit", "-m", msg)
}

func warningFiles(w []overlapWarningJSON) []string {
	var out []string
	for _, e := range w {
		out = append(out, e.Files...)
	}
	return out
}

// Two live specs that dirty the same file warn on both Runs: after each
// spec branch carries the shared file, both Runs hold a warning Event
// naming it and the other Run. The chained second tickets keep both Runs
// running while the first tickets merge, so the check sees live Runs.
func TestSpecOverlapWarnsBothRuns(t *testing.T) {
	repo, demoSpec, otherSpec := initTwoSpecRepo(t)
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	runA := startRun(t, srv.URL, demoSpec, repo, "fake", "fake-model")
	runB := startRun(t, srv.URL, otherSpec, repo, "fake", "fake-model")
	if runA.SpecBranch == runB.SpecBranch {
		t.Fatalf("spec branches = %q and %q, want two distinct branches", runA.SpecBranch, runB.SpecBranch)
	}

	rA := waitRunCond(t, srv.URL, runA.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "demo worker started")
	rB := waitRunCond(t, srv.URL, runB.ID, func(r runJSON) bool {
		return ticketByID(r, "01-other").WorkerSession != ""
	}, "other worker started")

	commitWorktreeFile(t, filepath.Join(repo, ".el-worktrees", "01-first"), "shared.txt", "demo\n", "demo change")
	submitSession(t, srv.URL, ticketByID(rA, "01-first").WorkerSession, "done", []string{"alpha"})
	passReviewers(t, srv.URL, runA.ID, "01-first")
	waitRunCond(t, srv.URL, runA.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").State == "merged"
	}, "demo merged")
	// Only one side merged: no overlap yet.
	if w := getWarnings(t, srv.URL, runA.ID); len(w) != 0 {
		t.Fatalf("demo warnings = %v, want none before the other side merges", w)
	}

	commitWorktreeFile(t, filepath.Join(repo, ".el-worktrees", "01-other"), "shared.txt", "other\n", "other change")
	submitSession(t, srv.URL, ticketByID(rB, "01-other").WorkerSession, "done", []string{"alpha"})
	passReviewers(t, srv.URL, runB.ID, "01-other")
	waitRunCond(t, srv.URL, runB.ID, func(r runJSON) bool {
		return ticketByID(r, "01-other").State == "merged"
	}, "other merged")

	wA := waitWarnings(t, srv.URL, runA.ID, true, "demo run")
	wB := waitWarnings(t, srv.URL, runB.ID, true, "other run")
	if got := warningFiles(wA); !containsFile(got, "shared.txt") {
		t.Fatalf("demo warning files = %v, want shared.txt", got)
	}
	if got := warningFiles(wB); !containsFile(got, "shared.txt") {
		t.Fatalf("other warning files = %v, want shared.txt", got)
	}
	if wA[0].OtherRun != runB.ID {
		t.Fatalf("demo warning otherRun = %q, want %q", wA[0].OtherRun, runB.ID)
	}
	if wB[0].OtherRun != runA.ID {
		t.Fatalf("other warning otherRun = %q, want %q", wB[0].OtherRun, runA.ID)
	}
	if !strings.Contains(wA[0].Message, "shared.txt") || !strings.Contains(wB[0].Message, "shared.txt") {
		t.Fatalf("warning messages = %q and %q, want both to name shared.txt", wA[0].Message, wB[0].Message)
	}
}

func containsFile(files []string, want string) bool {
	for _, f := range files {
		if f == want {
			return true
		}
	}
	return false
}

// Two live specs that dirty disjoint files never warn: both Runs merge
// clean and both warning lists stay empty.
func TestSpecOverlapDisjointNoWarning(t *testing.T) {
	repo, demoSpec, otherSpec := initTwoSpecRepo(t)
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	runA := startRun(t, srv.URL, demoSpec, repo, "fake", "fake-model")
	runB := startRun(t, srv.URL, otherSpec, repo, "fake", "fake-model")

	rA := waitRunCond(t, srv.URL, runA.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "demo worker started")
	rB := waitRunCond(t, srv.URL, runB.ID, func(r runJSON) bool {
		return ticketByID(r, "01-other").WorkerSession != ""
	}, "other worker started")

	commitWorktreeFile(t, filepath.Join(repo, ".el-worktrees", "01-first"), "demo-only.txt", "demo\n", "demo change")
	submitSession(t, srv.URL, ticketByID(rA, "01-first").WorkerSession, "done", []string{"alpha"})
	passReviewers(t, srv.URL, runA.ID, "01-first")
	waitRunCond(t, srv.URL, runA.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").State == "merged"
	}, "demo merged")

	commitWorktreeFile(t, filepath.Join(repo, ".el-worktrees", "01-other"), "other-only.txt", "other\n", "other change")
	submitSession(t, srv.URL, ticketByID(rB, "01-other").WorkerSession, "done", []string{"alpha"})
	passReviewers(t, srv.URL, runB.ID, "01-other")
	waitRunCond(t, srv.URL, runB.ID, func(r runJSON) bool {
		return ticketByID(r, "01-other").State == "merged"
	}, "other merged")

	if w := getWarnings(t, srv.URL, runA.ID); len(w) != 0 {
		t.Fatalf("demo warnings = %v, want none for disjoint specs", w)
	}
	if w := getWarnings(t, srv.URL, runB.ID); len(w) != 0 {
		t.Fatalf("other warnings = %v, want none for disjoint specs", w)
	}
}

// A finished Run is not live: when one spec is already done, the other
// merging the same file warns on neither Run.
func TestSpecOverlapDoneRunExcluded(t *testing.T) {
	repo, demoSpec, otherSpec := initTwoSpecRepo(t)
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	runA := startRun(t, srv.URL, demoSpec, repo, "fake", "fake-model")
	runB := startRun(t, srv.URL, otherSpec, repo, "fake", "fake-model")

	rA := waitRunCond(t, srv.URL, runA.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "demo worker started")
	rB := waitRunCond(t, srv.URL, runB.ID, func(r runJSON) bool {
		return ticketByID(r, "01-other").WorkerSession != ""
	}, "other worker started")

	// Finish the whole demo Run first: both tickets merge, so its status
	// is done before the other spec merges.
	commitWorktreeFile(t, filepath.Join(repo, ".el-worktrees", "01-first"), "shared.txt", "demo\n", "demo change")
	submitSession(t, srv.URL, ticketByID(rA, "01-first").WorkerSession, "done", []string{"alpha"})
	passReviewers(t, srv.URL, runA.ID, "01-first")
	rA = waitRunCond(t, srv.URL, runA.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").State == "merged" &&
			ticketByID(r, "02-second").WorkerSession != ""
	}, "demo first merged and second started")
	submitSession(t, srv.URL, ticketByID(rA, "02-second").WorkerSession, "done", []string{"beta"})
	passReviewers(t, srv.URL, runA.ID, "02-second")
	waitRunCond(t, srv.URL, runA.ID, func(r runJSON) bool {
		return r.Status == "done"
	}, "demo run done")

	commitWorktreeFile(t, filepath.Join(repo, ".el-worktrees", "01-other"), "shared.txt", "other\n", "other change")
	submitSession(t, srv.URL, ticketByID(rB, "01-other").WorkerSession, "done", []string{"alpha"})
	passReviewers(t, srv.URL, runB.ID, "01-other")
	waitRunCond(t, srv.URL, runB.ID, func(r runJSON) bool {
		return ticketByID(r, "01-other").State == "merged"
	}, "other merged")

	if w := getWarnings(t, srv.URL, runA.ID); len(w) != 0 {
		t.Fatalf("demo warnings = %v, want none: the run is done, not live", w)
	}
	if w := getWarnings(t, srv.URL, runB.ID); len(w) != 0 {
		t.Fatalf("other warnings = %v, want none: the other run is done, not live", w)
	}
}
