package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}

func initRunRepo(t *testing.T, tickets map[string]string) (repo, spec string) {
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
	for name, content := range tickets {
		writeFile(t, repo, ".scratch/demo/issues/"+name, content)
	}
	testGit(t, repo, "add", "-A")
	testGit(t, repo, "commit", "-m", "spec")
	return repo, filepath.Join(repo, ".scratch", "demo", "spec.md")
}

func demoTickets() map[string]string {
	return map[string]string{
		"01-first.md":  "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
		"02-second.md": "# 02: Second\n\n**Blocked by:** 01\n\n**Status:** open\n\n- [ ] beta\n",
	}
}

type reviewVerdictJSON struct {
	Verdict string `json:"verdict"`
	At      string `json:"at"`
	Fresh   bool   `json:"fresh"`
}

type runTicketJSON struct {
	ID               string                       `json:"id"`
	Num              int                          `json:"num"`
	Title            string                       `json:"title"`
	Blockers         []string                     `json:"blockers"`
	State            string                       `json:"state"`
	WorkerSession    string                       `json:"workerSession"`
	ReviewerSessions map[string]string            `json:"reviewerSessions"`
	Verdicts         map[string]reviewVerdictJSON `json:"verdicts"`
	Tip              string                       `json:"tip"`
	LastRefusal      string                       `json:"lastRefusal"`
}

type runJSON struct {
	ID      string          `json:"id"`
	Spec    string          `json:"spec"`
	Status  string          `json:"status"`
	Harness string          `json:"harness"`
	Model   string          `json:"model"`
	Effort  string          `json:"effort"`
	Tickets []runTicketJSON `json:"tickets"`
}

func startRun(t *testing.T, base, spec, repo, harness, model string) runJSON {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"spec": spec, "repo": repo, "harness": harness, "model": model})
	resp, err := http.Post(base+"/api/runs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("start status = %d, want 201", resp.StatusCode)
	}
	var out struct {
		Run runJSON `json:"run"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.Run
}

func getRun(t *testing.T, base, id string) runJSON {
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
		Run runJSON `json:"run"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.Run
}

func waitRunTickets(t *testing.T, base, id string, want map[string]string) runJSON {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		r := getRun(t, base, id)
		got := map[string]string{}
		for _, tk := range r.Tickets {
			got[tk.ID] = tk.State
		}
		match := len(got) == len(want)
		for k, v := range want {
			if got[k] != v {
				match = false
			}
		}
		if match {
			return r
		}
		if time.Now().After(deadline) {
			for _, tk := range r.Tickets {
				t.Logf("ticket %s state=%s refusal=%q reviewers=%v verdicts=%+v tip=%s", tk.ID, tk.State, tk.LastRefusal, tk.ReviewerSessions, tk.Verdicts, tk.Tip)
			}
			for _, tk := range r.Tickets {
				for typ, sid := range tk.ReviewerSessions {
					sess := getSession(t, base, sid)
					for _, e := range sess.Events {
						t.Logf("reviewer %s %s #%d %s %.80q %s", typ, sid[:6], e.Seq, e.Kind, e.Text, e.Status)
					}
				}
			}
			t.Fatalf("tickets = %v, want %v", got, want)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// Slice 1: Start creates a Run and returns it; a second Start on the same
// spec while active is refused.
func TestStartRunCreatesRunAndRefusesSecond(t *testing.T) {
	repo, spec := initRunRepo(t, demoTickets())
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	if run.ID == "" {
		t.Fatalf("run id empty in %+v", run)
	}
	if run.Status != "running" {
		t.Fatalf("run status = %q, want running", run.Status)
	}
	if len(run.Tickets) != 2 {
		t.Fatalf("tickets = %+v, want two", run.Tickets)
	}

	body, _ := json.Marshal(map[string]string{"spec": spec, "repo": repo, "harness": "fake", "model": "fake-model"})
	resp, err := http.Post(srv.URL+"/api/runs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("second start status = %d, want 409", resp.StatusCode)
	}

	resp2, err := http.Get(srv.URL + "/api/runs")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	var out struct {
		Runs []runJSON `json:"runs"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.Runs) != 1 || out.Runs[0].ID != run.ID {
		t.Fatalf("runs = %+v, want the one started run", out.Runs)
	}
}

func submitSession(t *testing.T, base, id, status string, completed []string) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"status": status, "completed": completed})
	resp, err := http.Post(base+"/api/sessions/"+id+"/submit", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("submit status = %d, want 202", resp.StatusCode)
	}
}

func waitRunCond(t *testing.T, base, id string, cond func(runJSON) bool, what string) runJSON {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		r := getRun(t, base, id)
		if cond(r) {
			return r
		}
		if time.Now().After(deadline) {
			states := map[string]string{}
			for _, tk := range r.Tickets {
				states[tk.ID] = tk.State + "/" + tk.WorkerSession
			}
			t.Fatalf("timed out waiting for %s: %v (run %s)", what, states, r.Status)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func chainTickets() map[string]string {
	return map[string]string{
		"01-first.md":  "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
		"02-second.md": "# 02: Second\n\n**Blocked by:** 01\n\n**Status:** open\n\n- [ ] beta\n",
		"03-third.md":  "# 03: Third\n\n**Blocked by:** 02\n\n**Status:** open\n\n- [ ] gamma\n",
	}
}

// Slice 2: Tickets start in number order once their blockers merged, and a
// Worker that submits done merges.
func TestRunStartsTicketsInBlockerOrder(t *testing.T) {
	repo, spec := initRunRepo(t, chainTickets())
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		byID := map[string]runTicketJSON{}
		for _, tk := range r.Tickets {
			byID[tk.ID] = tk
		}
		return byID["01-first"].WorkerSession != "" &&
			byID["02-second"].WorkerSession == "" &&
			byID["03-third"].WorkerSession == ""
	}, "only 01-first started")
	first := ""
	for _, tk := range r.Tickets {
		if tk.ID == "01-first" {
			first = tk.WorkerSession
		}
	}
	submitSession(t, srv.URL, first, "done", []string{"alpha"})
	passReviewers(t, srv.URL, run.ID, "01-first")
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		byID := map[string]runTicketJSON{}
		for _, tk := range r.Tickets {
			byID[tk.ID] = tk
		}
		return byID["01-first"].State == "merged" &&
			byID["02-second"].WorkerSession != "" &&
			byID["03-third"].WorkerSession == ""
	}, "01-first merged and 02-second started")
	second := ""
	for _, tk := range r.Tickets {
		if tk.ID == "02-second" {
			second = tk.WorkerSession
		}
	}
	submitSession(t, srv.URL, second, "done", []string{"beta"})
	passReviewers(t, srv.URL, run.ID, "02-second")
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		byID := map[string]runTicketJSON{}
		for _, tk := range r.Tickets {
			byID[tk.ID] = tk
		}
		return byID["02-second"].State == "merged" &&
			byID["03-third"].WorkerSession != ""
	}, "02-second merged and 03-third started")
	for _, tk := range r.Tickets {
		if tk.ID == "03-third" {
			submitSession(t, srv.URL, tk.WorkerSession, "done", []string{"gamma"})
		}
	}
	passReviewers(t, srv.URL, run.ID, "03-third")
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return r.Status == "done"
	}, "run done")
	for _, tk := range r.Tickets {
		if tk.State != "merged" {
			t.Fatalf("ticket %s state = %q, want merged", tk.ID, tk.State)
		}
	}
}

// Slice 3: independent Tickets still start one at a time under a cap of one.
func TestRunRespectsLiveSessionCap(t *testing.T) {
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md":  "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
		"02-second.md": "# 02: Second\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] beta\n",
	})
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.SetMaxRunning(1)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		started := 0
		for _, tk := range r.Tickets {
			if tk.WorkerSession != "" {
				started++
			}
		}
		return started == 1
	}, "exactly one worker started")
	busy := ""
	idle := ""
	for _, tk := range r.Tickets {
		if tk.WorkerSession != "" {
			busy = tk.ID
		} else {
			idle = tk.ID
		}
	}
	var busySession string
	for _, tk := range r.Tickets {
		if tk.ID == busy {
			busySession = tk.WorkerSession
		}
	}
	submitSession(t, srv.URL, busySession, "done", nil)
	passReviewers(t, srv.URL, run.ID, busy)
	_ = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		for _, tk := range r.Tickets {
			if tk.ID == busy && tk.State == "merged" {
				return true
			}
		}
		return false
	}, "busy ticket merged")
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		for _, tk := range r.Tickets {
			if tk.ID == idle && tk.WorkerSession != "" {
				return true
			}
		}
		return false
	}, "second ticket started after the merge")
	_ = r
}

// Slice 4: checkboxes tick from the Worker completed list, written only by
// the driver. The ticket file gains [x] and Status done after the merge.
func TestRunTicksBoxesFromCompletedList(t *testing.T) {
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n- [ ] beta\n",
	})
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		for _, tk := range r.Tickets {
			if tk.ID == "01-first" && tk.WorkerSession != "" {
				return true
			}
		}
		return false
	}, "worker started")
	var sess string
	for _, tk := range r.Tickets {
		if tk.ID == "01-first" {
			sess = tk.WorkerSession
		}
	}
	submitSession(t, srv.URL, sess, "done", []string{"alpha"})
	passReviewers(t, srv.URL, run.ID, "01-first")
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return r.Status == "done"
	}, "run done")
	for _, tk := range r.Tickets {
		if tk.State != "merged" {
			t.Fatalf("ticket %s state = %q, want merged", tk.ID, tk.State)
		}
	}
	raw, err := os.ReadFile(filepath.Join(repo, ".scratch", "demo", "issues", "01-first.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "- [x] alpha") {
		t.Fatalf("ticket file = %q, want alpha ticked", raw)
	}
	if !strings.Contains(string(raw), "Status:") || !strings.Contains(string(raw), "done") {
		t.Fatalf("ticket file = %q, want Status done", raw)
	}
}

// Slice 5a: a dirty worktree keeps the Worker alive and shows the refusal
// on the Ticket row. Cleaning and resubmitting merges.
func TestRunDirtyWorktreeKeepsWorkerAlive(t *testing.T) {
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		for _, tk := range r.Tickets {
			if tk.ID == "01-first" && tk.WorkerSession != "" {
				return true
			}
		}
		return false
	}, "worker started")
	var sess string
	for _, tk := range r.Tickets {
		if tk.ID == "01-first" {
			sess = tk.WorkerSession
		}
	}
	worktree := filepath.Join(repo, ".el-worktrees", "01-first")
	writeFile(t, worktree, "dirty.txt", "dirty\n")
	submitSession(t, srv.URL, sess, "done", []string{"alpha"})
	passReviewers(t, srv.URL, run.ID, "01-first")
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		for _, tk := range r.Tickets {
			if tk.ID == "01-first" && tk.LastRefusal != "" {
				return true
			}
		}
		return false
	}, "refusal shown")
	for _, tk := range r.Tickets {
		if tk.ID == "01-first" {
			if tk.State == "merged" {
				t.Fatalf("dirty ticket merged, want it kept alive")
			}
			if !strings.Contains(tk.LastRefusal, "uncommitted changes") {
				t.Fatalf("lastRefusal = %q, want uncommitted changes", tk.LastRefusal)
			}
			if tk.WorkerSession != sess {
				t.Fatalf("worker changed, want the same Worker alive")
			}
		}
	}
	if err := os.Remove(filepath.Join(worktree, "dirty.txt")); err != nil {
		t.Fatal(err)
	}
	submitSession(t, srv.URL, sess, "done", []string{"alpha"})
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return r.Status == "done"
	}, "run done after clean resubmit")
	for _, tk := range r.Tickets {
		if tk.State != "merged" {
			t.Fatalf("ticket %s state = %q, want merged", tk.ID, tk.State)
		}
	}
}

// Slice 5b: a rebase conflict keeps the Worker alive and shows the refusal.
func TestRunRebaseConflictKeepsWorkerAlive(t *testing.T) {
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		for _, tk := range r.Tickets {
			if tk.ID == "01-first" && tk.WorkerSession != "" {
				return true
			}
		}
		return false
	}, "worker started")
	var sess string
	for _, tk := range r.Tickets {
		if tk.ID == "01-first" {
			sess = tk.WorkerSession
		}
	}
	worktree := filepath.Join(repo, ".el-worktrees", "01-first")
	writeFile(t, worktree, "conflict.txt", "worker\n")
	testGit(t, worktree, "add", "conflict.txt")
	testGit(t, worktree, "commit", "-m", "worker change")
	writeFile(t, repo, "conflict.txt", "main\n")
	testGit(t, repo, "add", "conflict.txt")
	testGit(t, repo, "commit", "-m", "main moves")
	submitSession(t, srv.URL, sess, "done", []string{"alpha"})
	passReviewers(t, srv.URL, run.ID, "01-first")
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		for _, tk := range r.Tickets {
			if tk.ID == "01-first" && tk.LastRefusal != "" {
				return true
			}
		}
		return false
	}, "rebase refusal shown")
	for _, tk := range r.Tickets {
		if tk.ID == "01-first" {
			if tk.State == "merged" {
				t.Fatalf("conflicting ticket merged, want it kept alive")
			}
			if !strings.Contains(tk.LastRefusal, "rebase onto") || !strings.Contains(tk.LastRefusal, "conflicted") {
				t.Fatalf("lastRefusal = %q, want the rebase conflict", tk.LastRefusal)
			}
		}
	}
}

// Slice 6: roles choose harness, model and effort; a missing role falls
// back to the Session default from the Start request.
func TestRunRolesChooseWorkerConfig(t *testing.T) {
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	settings := filepath.Join(tempDir(t), "settings.json")
	raw, _ := json.Marshal(map[string]any{
		"roles": map[string]any{
			"worker": map[string]any{"harness": "fake", "model": "role-model", "effort": "high"},
		},
	})
	if err := os.WriteFile(settings, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EL_SETTINGS", settings)
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "request-model")
	if run.Harness != "fake" || run.Model != "role-model" || run.Effort != "high" {
		t.Fatalf("run config = %s/%s/%s, want fake/role-model/high", run.Harness, run.Model, run.Effort)
	}
}

// The settings page's implementer slot is the Run worker role and beats the
// older worker slot.
func TestRunImplementerRoleBeatsWorkerRole(t *testing.T) {
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	settings := filepath.Join(tempDir(t), "settings.json")
	raw, _ := json.Marshal(map[string]any{
		"roles": map[string]any{
			"worker":      map[string]any{"harness": "fake", "model": "worker-model"},
			"implementer": map[string]any{"harness": "fake", "model": "impl-model", "effort": "max"},
		},
	})
	if err := os.WriteFile(settings, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EL_SETTINGS", settings)
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "request-model")
	if run.Model != "impl-model" || run.Effort != "max" {
		t.Fatalf("run config = %s/%s, want impl-model/max", run.Model, run.Effort)
	}
}

func TestRunFallsBackToRequestWithoutRole(t *testing.T) {
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	t.Setenv("EL_SETTINGS", filepath.Join(tempDir(t), "missing.json"))
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "request-model")
	if run.Harness != "fake" || run.Model != "request-model" {
		t.Fatalf("run config = %s/%s, want fake/request-model", run.Harness, run.Model)
	}
}

// Review fixes: a Worker that submits blocked is not merged; its refusal
// waits for done, and the next done submit merges.
func TestRunBlockedSubmitWaitsForDone(t *testing.T) {
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		for _, tk := range r.Tickets {
			if tk.ID == "01-first" && tk.WorkerSession != "" {
				return true
			}
		}
		return false
	}, "worker started")
	var sess string
	for _, tk := range r.Tickets {
		if tk.ID == "01-first" {
			sess = tk.WorkerSession
		}
	}
	submitSession(t, srv.URL, sess, "blocked", nil)
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		for _, tk := range r.Tickets {
			if tk.ID == "01-first" && tk.LastRefusal != "" {
				return true
			}
		}
		return false
	}, "blocked refusal shown")
	for _, tk := range r.Tickets {
		if tk.ID == "01-first" {
			if tk.State == "merged" {
				t.Fatalf("blocked ticket merged, want it kept alive")
			}
			if !strings.Contains(tk.LastRefusal, "waiting for done") {
				t.Fatalf("lastRefusal = %q, want the wait for done", tk.LastRefusal)
			}
		}
	}
	submitSession(t, srv.URL, sess, "done", []string{"alpha"})
	passReviewers(t, srv.URL, run.ID, "01-first")
	waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return r.Status == "done"
	}, "run done after done submit")
}

// Review fixes: on a detached repo (no target branch) the second of two
// parallel Tickets rebases onto the repo HEAD as it stands at finish, not
// the stale recorded base, so both merge.
func TestRunDetachedMergesTwoTickets(t *testing.T) {
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md":  "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
		"02-second.md": "# 02: Second\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] beta\n",
	})
	testGit(t, repo, "checkout", "--detach")
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		n := 0
		for _, tk := range r.Tickets {
			if tk.WorkerSession != "" {
				n++
			}
		}
		return n == 2
	}, "both workers started")
	sess := map[string]string{}
	for _, tk := range r.Tickets {
		sess[tk.ID] = tk.WorkerSession
	}
	submitSession(t, srv.URL, sess["01-first"], "done", []string{"alpha"})
	submitSession(t, srv.URL, sess["02-second"], "done", []string{"beta"})
	passReviewers(t, srv.URL, run.ID, "01-first")
	passReviewers(t, srv.URL, run.ID, "02-second")
	waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return r.Status == "done"
	}, "run done on detached repo")
}

// Review fixes: on a detached repo the finish rebases onto the repo HEAD as
// it stands at finish, not the stale recorded base. The worker branch below
// diverged from the recorded base after another merge moved HEAD: the old
// base rebase is a no-op and the fast-forward fails, the current HEAD
// rebase lands it.
func TestRunDetachedFinishRebasesOntoCurrentHead(t *testing.T) {
	repo, _ := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	testGit(t, repo, "checkout", "--detach")
	base := testGit(t, repo, "rev-parse", "HEAD")
	base = strings.TrimSpace(base)
	branch := "implement/01-first"
	worktree := filepath.Join(repo, ".el-worktrees", "01-first")
	testGit(t, repo, "worktree", "add", "-b", branch, worktree, base)
	writeFile(t, worktree, "worker.txt", "worker\n")
	testGit(t, worktree, "add", "worker.txt")
	testGit(t, worktree, "commit", "-m", "worker change")
	writeFile(t, repo, "main.txt", "main\n")
	testGit(t, repo, "add", "main.txt")
	testGit(t, repo, "commit", "-m", "main moves")

	d := NewDaemon(tempDir(t), NewFakeBackend())
	ticketPath := filepath.Join(repo, ".scratch", "demo", "issues", "01-first.md")
	d.runs["r-detached"] = &runState{
		ID: "r-detached", Spec: "spec", Repo: repo, Status: "running",
		notify:    make(chan struct{}),
		Tickets:   []staticTicket{{ID: "01-first", Num: 1, Title: "First", Path: ticketPath}},
		Sessions:  map[string]string{"01-first": "sess1"},
		Branches:  map[string]string{"01-first": branch},
		Worktrees: map[string]string{"01-first": worktree},
		Targets:   map[string]string{},
		Bases:     map[string]string{"01-first": base},
		Merged:    map[string]string{},
		Reviewers: map[string]map[string]string{},
		Reviews:   map[string]map[string]storedReview{},
	}
	tip := strings.TrimSpace(testGit(t, worktree, "rev-parse", "HEAD"))
	d.runs["r-detached"].Reviewers["01-first"] = map[string]string{"reviewer-standards": "sess-std", "reviewer-spec": "sess-spec"}
	d.runs["r-detached"].Reviews["01-first"] = map[string]storedReview{"reviewer-standards": {Verdict: "pass", At: tip}, "reviewer-spec": {Verdict: "pass", At: tip}}
	fin, err := d.runFinishWorker("r-detached", "01-first")
	if err != nil {
		t.Fatalf("finish = %v, want merge", err)
	}
	if !fin.Merged {
		t.Fatalf("finish merged = false (%q), want the current-HEAD rebase to land", fin.Detail)
	}
}

// Review fixes: Start refuses a spec outside the picked repo, so ticket
// paths never escape it.
func TestStartRunRejectsSpecOutsideRepo(t *testing.T) {
	_, spec := initRunRepo(t, demoTickets())
	other := tempDir(t)
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{"spec": spec, "repo": other, "harness": "fake", "model": "fake-model"})
	resp, err := http.Post(srv.URL+"/api/runs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("start status = %d, want 400", resp.StatusCode)
	}
}

// Review fixes: a Run saved as running reloads as paused, spending nothing
// after a Daemon restart.
func TestLoadRunsRestoresPaused(t *testing.T) {
	stateDir := tempDir(t)
	dir := filepath.Join(stateDir, "enderbloom", "runs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{
		"id": "run-paused", "spec": "/repo/.scratch/demo/spec.md", "repo": "/repo",
		"status": "running", "harness": "fake", "model": "m", "tickets": []any{}, "owner": "owner-1",
	})
	if err := os.WriteFile(filepath.Join(dir, "run-paused.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	d := NewDaemon(stateDir, NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/runs/run-paused")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get status = %d, want 200", resp.StatusCode)
	}
	var out struct {
		Run runJSON `json:"run"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Run.Status != "paused" {
		t.Fatalf("run status = %q, want paused", out.Run.Status)
	}
}

// Review fixes: the events stream serves the live Ticket table at once with
// wait=0.
func TestServeRunEventsStreamsTable(t *testing.T) {
	repo, spec := initRunRepo(t, demoTickets())
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	resp, err := http.Get(srv.URL + "/api/runs/" + run.ID + "/events?wait=0")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		t.Fatal(err)
	}
	line := ""
	for _, l := range strings.Split(buf.String(), "\n") {
		if strings.HasPrefix(l, "data: ") {
			line = strings.TrimPrefix(l, "data: ")
			break
		}
	}
	if line == "" {
		t.Fatalf("no data line in %q", buf.String())
	}
	var body runJSON
	if err := json.Unmarshal([]byte(line), &body); err != nil {
		t.Fatal(err)
	}
	if body.ID != run.ID {
		t.Fatalf("events run = %q, want %q", body.ID, run.ID)
	}
	if len(body.Tickets) != 2 {
		t.Fatalf("events tickets = %d, want 2", len(body.Tickets))
	}
}

// A round still open at the tip is under review, not a refusal: the gate
// must not tell the worker a stale `changes` verdict while a reviewer is
// still judging the new tip, or the driver parks on the worker's next
// submit while the worker waits for the verdict. Once the round closes
// with fresh changes, finish refuses with them.
func TestRunFinishOpenRoundAtTipIsUnderReview(t *testing.T) {
	repo, _ := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	base := strings.TrimSpace(testGit(t, repo, "rev-parse", "HEAD"))
	branch := "implement/01-first"
	worktree := filepath.Join(repo, ".el-worktrees", "01-first")
	testGit(t, repo, "worktree", "add", "-b", branch, worktree, base)
	writeFile(t, worktree, "worker.txt", "worker\n")
	testGit(t, worktree, "add", "worker.txt")
	testGit(t, worktree, "commit", "-m", "worker change")
	tip := strings.TrimSpace(testGit(t, worktree, "rev-parse", "HEAD"))
	old := base

	d := NewDaemon(tempDir(t), NewFakeBackend())
	rs := &runState{
		ID: "r-open", Spec: "spec", Repo: repo, Status: "running",
		notify:    make(chan struct{}),
		Tickets:   []staticTicket{{ID: "01-first", Num: 1, Title: "First"}},
		Sessions:  map[string]string{"01-first": "sess1"},
		Branches:  map[string]string{"01-first": branch},
		Worktrees: map[string]string{"01-first": worktree},
		Targets:   map[string]string{},
		Bases:     map[string]string{"01-first": base},
		Merged:    map[string]string{},
		Reviewers: map[string]map[string]string{"01-first": {"reviewer-standards": "sess-std", "reviewer-spec": "sess-spec"}},
		Reviews: map[string]map[string]storedReview{"01-first": {
			"reviewer-standards": {Verdict: "changes", At: old},
			"reviewer-spec":      {Verdict: "pass", At: tip},
		}},
		Rounds: map[string][]ticketRound{"01-first": {{N: 2, Tip: tip, StartedAt: time.Now()}}},
	}
	d.runs["r-open"] = rs

	fin, err := d.runFinishWorker("r-open", "01-first")
	if err != nil {
		t.Fatal(err)
	}
	if fin.Merged || fin.Detail != "" {
		t.Fatalf("finish = %+v, want empty under-review result while the round is open", fin)
	}
	if got := rs.Rounds["01-first"][0].SentToWorkerAt; got != nil {
		t.Fatalf("sentToWorkerAt = %v, want nothing told while the round is open", got)
	}

	end := time.Now()
	rs.Rounds["01-first"][0].EndedAt = &end
	rs.Reviews["01-first"]["reviewer-standards"] = storedReview{Verdict: "changes", At: tip, Report: "fix it"}
	fin, err = d.runFinishWorker("r-open", "01-first")
	if err != nil {
		t.Fatal(err)
	}
	if fin.Merged || !strings.Contains(fin.Detail, "reviewer-standards: changes at") {
		t.Fatalf("finish = %+v, want the fresh changes told once the round closed", fin)
	}
	if rs.Rounds["01-first"][0].SentToWorkerAt == nil {
		t.Fatal("sentToWorkerAt unset, want the closed round's findings recorded as told")
	}
}

// A Daemon restart must not reset a Worker's submit count: the saved Run
// carries it, so the count stays above Reprompted and the next done submit
// re-prompts the reviewers instead of being read as already handled.
func TestWorkerSubmitSurvivesDaemonRestart(t *testing.T) {
	state := tempDir(t)
	d := NewDaemon(state, NewFakeBackend())
	d.mu.Lock()
	d.sessions["w1"] = &session{meta: SessionMeta{ID: "w1", Harness: "fake", Model: "m", Status: "idle", Owner: "owner-1"}, live: true, notify: make(chan struct{})}
	d.runs["r-sub"] = &runState{
		ID: "r-sub", Spec: "spec", Owner: "owner-1", Repo: tempDir(t), Status: "running",
		notify:   make(chan struct{}),
		Tickets:  []staticTicket{{ID: "01-first", Num: 1, Title: "First"}},
		Sessions: map[string]string{"01-first": "w1"},
	}
	d.mu.Unlock()
	srv := httptest.NewServer(authedHandler(t, d))
	body := `{"status":"done","completed":["alpha"]}`
	for i := 0; i < 2; i++ {
		resp, err := http.Post(srv.URL+"/api/sessions/w1/submit", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	srv.Close()

	d2 := NewDaemon(state, NewFakeBackend())
	d2.mu.Lock()
	rec := d2.submits["w1"]
	d2.mu.Unlock()
	if rec == nil || rec.Count != 2 || rec.Status != "done" {
		t.Fatalf("restored submit = %+v, want count 2 status done", rec)
	}
}

// A submit count reset to 1 while Reprompted is 2 is lifted past it, so the
// reviewers are re-prompted for the new tip.
func TestWorkerSubmitCountOutrunsReprompted(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.mu.Lock()
	d.sessions["w1"] = &session{meta: SessionMeta{ID: "w1", Harness: "fake", Model: "m", Status: "idle", Owner: "owner-1"}, live: true, notify: make(chan struct{})}
	d.runs["r-cnt"] = &runState{
		ID: "r-cnt", Spec: "spec", Owner: "owner-1", Repo: tempDir(t), Status: "running",
		notify:     make(chan struct{}),
		Tickets:    []staticTicket{{ID: "01-first", Num: 1, Title: "First"}},
		Sessions:   map[string]string{"01-first": "w1"},
		Reprompted: map[string]int{"01-first": 2},
	}
	d.mu.Unlock()
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/api/sessions/w1/submit", "application/json", strings.NewReader(`{"status":"done"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	d.mu.Lock()
	n := d.submits["w1"].Count
	d.mu.Unlock()
	if n != 3 {
		t.Fatalf("count = %d, want 3 (above Reprompted 2)", n)
	}
}
