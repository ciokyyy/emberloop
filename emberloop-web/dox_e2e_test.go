package web

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A seeded Worker that commits the seed as its task instructs merges cleanly
// in a real git repo that lacks a root AGENTS.md: the seed rides the branch
// onto the spec branch instead of tripping the dirty-worktree refusal on
// every finish. Master never moves here; the final merge carries it later.
func TestSeededWorkerCommitsAndMerges(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	testGit(t, repo, "rm", "-q", "AGENTS.md")
	testGit(t, repo, "commit", "-qm", "drop root")
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return len(r.Tickets) > 0 && r.Tickets[0].WorkerSession != ""
	}, "worker for the first ticket")
	worker := r.Tickets[0].WorkerSession

	var task string
	for _, e := range transcriptOf(t, srv.URL, worker) {
		// The first prompt is the ticket task; later prompts are submit-gate
		// reminders while the fake worker idles without submitting.
		if e.Kind == "prompt" {
			task = e.Text
			break
		}
	}
	if !strings.Contains(task, "commit it with your work") {
		t.Fatalf("worker task lacks the seed line: %q", task)
	}

	wt, err := os.ReadDir(filepath.Join(repo, ".el-worktrees"))
	if err != nil || len(wt) != 1 {
		t.Fatalf("worktrees = %+v, %v; want the one worker checkout", wt, err)
	}
	worktree := filepath.Join(repo, ".el-worktrees", wt[0].Name())
	testGit(t, worktree, "add", "AGENTS.md")
	testGit(t, worktree, "commit", "-qm", "seed root AGENTS.md")

	submitSession(t, srv.URL, worker, "done", []string{"alpha"})
	passReviewers(t, srv.URL, run.ID, "01-first")
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return r.Status == "done"
	}, "run done")
	for _, tk := range r.Tickets {
		if tk.State != "merged" {
			t.Fatalf("ticket %s state = %q, want merged", tk.ID, tk.State)
		}
	}
	raw := strings.TrimSpace(testGit(t, repo, "show", "spec/demo:AGENTS.md"))
	if !strings.Contains(raw, "Child DOX Index") {
		t.Fatalf("merged file lacks the DOX shape: %q", raw)
	}
}
