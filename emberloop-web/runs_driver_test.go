package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// commitTicketWorkSeq numbers the committed files so every call has something
// new to commit.
var commitTicketWorkSeq atomic.Int64

// commitTicketWork commits one uniquely-named file in the Ticket's worktree,
// so a following done submit passes the uncommitted-work gate (a done submit
// with no commits ahead of the base, or with uncommitted changes, is
// refused). Repeated calls commit distinct files.
func commitTicketWork(t *testing.T, repo, ticketID string) {
	t.Helper()
	n := commitTicketWorkSeq.Add(1)
	name := fmt.Sprintf("ticket-work-%d.go", n)
	commitTicketFile(t, filepath.Join(repo, ".el-worktrees", ticketID), name, fmt.Sprintf("package main\n// ticket work %d\n", n), "ticket work")
}

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

// runTestDaemon starts a Daemon for Run tests: the driver polls every
// 100ms instead of the production 500ms, so multi-pass Runs settle fast.
// Tests with pause/switch/resume sequences that race a fast driver keep
// NewDaemon (see runs_pause_switch_test.go).
func runTestDaemon(t *testing.T, stateDir string, backend Backend) *Daemon {
	t.Helper()
	d := NewDaemon(stateDir, backend)
	d.SetPollInterval(testPollInterval())
	return d
}

// runTestSlots caps how many Run tests (driver goroutines, git subprocesses,
// fast polls) run at once. Dozens in parallel on 4 cores stretch every
// timing window and make order-sensitive assertions flake; a small cap keeps
// the load predictable while the rest of the suite still runs in parallel.
// A test takes one slot on its first initRunRepo and frees it at cleanup.
var (
	runTestSlots = make(chan struct{}, runTestParallelism())
	runTestHeld  sync.Map
)

func runTestParallelism() int {
	if v, err := strconv.Atoi(os.Getenv("EL_TEST_RUN_PAR")); err == nil && v > 0 {
		return v
	}
	return 2
}

func holdRunTestSlot(t *testing.T) {
	t.Helper()
	if _, loaded := runTestHeld.LoadOrStore(t.Name(), true); loaded {
		return
	}
	runTestSlots <- struct{}{}
	t.Cleanup(func() {
		runTestHeld.Delete(t.Name())
		<-runTestSlots
	})
}

func initRunRepo(t *testing.T, tickets map[string]string) (repo, spec string) {
	t.Helper()
	holdRunTestSlot(t)
	// Fast path: copy the cached template for this ticket set (no git
	// spawn). The template holds byte-identical files and history to the
	// init/add/commit below; git identity and config isolation come from
	// TestMain's process env, never per-test Setenv, so this stays
	// parallel-safe.
	if dir, ok := runRepoTemplate(t, tickets); ok {
		repo = tempDir(t)
		copyDir(t, repo, dir)
		return repo, filepath.Join(repo, ".scratch", "demo", "spec.md")
	}
	repo = tempDir(t)
	testGit(t, repo, "init", "-b", "main", ".")
	writeFile(t, repo, ".scratch/demo/spec.md", "# Demo spec\n")
	for name, content := range tickets {
		writeFile(t, repo, ".scratch/demo/issues/"+name, content)
	}
	// Run-launched worktrees seed a root AGENTS.md when the checkout lacks
	// one (ticket 27); the fixture commits one so the seed stays a no-op
	// here and fake-harness workers (which never commit) keep a clean tree.
	writeFile(t, repo, "AGENTS.md", "# AGENTS.md — demo\n\n## Child DOX Index\n\n(none)\n")
	testGit(t, repo, "add", "-A")
	testGit(t, repo, "commit", "-m", "spec")
	return repo, filepath.Join(repo, ".scratch", "demo", "spec.md")
}

// runRepoTemplates caches one committed template repo per ticket set for the
// process: the first test needing a set pays init/add/commit once, the rest
// copy it. Template dirs are registered for TestMain removal after the run.
var (
	runTemplateMu sync.Mutex
	runTemplates  = map[string]string{}
)

// runRepoTemplate returns the cached template for tickets, building it on
// first use. False means the build failed; the caller falls back to a fresh
// init. The build runs outside the cache lock: t.Fatal exits the goroutine,
// which must never happen while holding it, and a rare double build of one
// key is harmless (both are identical; the loser is still removed).
func runRepoTemplate(t *testing.T, tickets map[string]string) (string, bool) {
	t.Helper()
	names := make([]string, 0, len(tickets))
	for name := range tickets {
		names = append(names, name)
	}
	sort.Strings(names)
	var key strings.Builder
	for _, name := range names {
		key.WriteString(name)
		key.WriteString("\x00")
		key.WriteString(tickets[name])
		key.WriteString("\x00")
	}
	runTemplateMu.Lock()
	dir, ok := runTemplates[key.String()]
	runTemplateMu.Unlock()
	if ok {
		return dir, true
	}
	build, err := os.MkdirTemp("", "el-web-runrepo-")
	if err != nil {
		return "", false
	}
	registerSharedTemp(build)
	testGit(t, build, "init", "-b", "main", ".")
	writeFile(t, build, ".scratch/demo/spec.md", "# Demo spec\n")
	for _, name := range names {
		writeFile(t, build, ".scratch/demo/issues/"+name, tickets[name])
	}
	writeFile(t, build, "AGENTS.md", "# AGENTS.md — demo\n\n## Child DOX Index\n\n(none)\n")
	testGit(t, build, "add", "-A")
	testGit(t, build, "commit", "-m", "spec")
	runTemplateMu.Lock()
	runTemplates[key.String()] = build
	runTemplateMu.Unlock()
	return build, true
}

// copyDir copies the template tree src into dst (which must exist): files
// keep their modes, symlinks are recreated. It never spawns git.
func copyDir(t *testing.T, dst, src string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			return os.MkdirAll(target, info.Mode())
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			_ = os.Remove(target)
			return os.Symlink(link, target)
		default:
			in, err := os.Open(path)
			if err != nil {
				return err
			}
			defer in.Close()
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode())
			if err != nil {
				return err
			}
			_, err = io.Copy(out, in)
			cerr := out.Close()
			if err != nil {
				return err
			}
			return cerr
		}
	})
	if err != nil {
		t.Fatalf("copy template: %v", err)
	}
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
	Branch           string                       `json:"branch"`
	NeedsYou         string                       `json:"needsYou"`
	Chain            map[string]ticketChainEntry  `json:"chain"`
}

type runJSON struct {
	ID              string          `json:"id"`
	Spec            string          `json:"spec"`
	Status          string          `json:"status"`
	Harness         string          `json:"harness"`
	Model           string          `json:"model"`
	Effort          string          `json:"effort"`
	Error           string          `json:"error"`
	FinalMergeFiles []string        `json:"finalMergeFiles"`
	SpecBranch      string          `json:"specBranch"`
	SpecBase        string          `json:"specBase"`
	SpecBehind      int             `json:"specBehind"`
	Tickets         []runTicketJSON `json:"tickets"`
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
		time.Sleep(20 * time.Millisecond)
	}
}

// Slice 1: Start creates a Run and returns it; a second Start on the same
// spec while active is refused.
func TestStartRunCreatesRunAndRefusesSecond(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, demoTickets())
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
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
			t.Fatalf("timed out waiting for %s: %v (run %s error %q files %v)", what, states, r.Status, r.Error, r.FinalMergeFiles)
		}
		time.Sleep(20 * time.Millisecond)
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
	t.Parallel()
	repo, spec := initRunRepo(t, chainTickets())
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
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
	commitTicketWork(t, repo, "01-first")
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
	commitTicketWork(t, repo, "02-second")
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
			commitTicketWork(t, repo, "03-third")
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

// Slice 3: independent Tickets still start one at a time under a ticket cap
// of one.
func TestRunRespectsMaxTickets(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md":  "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
		"02-second.md": "# 02: Second\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] beta\n",
	})
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	d.SetMaxTickets(1)
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
	commitTicketWork(t, repo, busy)
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

// A completed list is report-only until the Ticket merges into the spec
// branch. The final green gate then fast-forwards that checked branch to master.
func TestRunMarksTicketDoneAfterSpecAndMasterMerge(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n- [ ] beta\n",
	})
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
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
	commitTicketWork(t, repo, "01-first")
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
	if !strings.Contains(string(raw), "- [x] alpha") || !strings.Contains(string(raw), "- [x] beta") || !strings.Contains(string(raw), "**Status:** done") {
		t.Fatalf("master ticket = %q, want all boxes ticked and Status done after the gate", raw)
	}
	// The checked Ticket file is committed on the spec branch before the
	// green final gate advances master.
	specRaw := strings.TrimSpace(testGit(t, repo, "show", "spec/demo:.scratch/demo/issues/01-first.md"))
	if !strings.Contains(specRaw, "- [x] alpha") || !strings.Contains(specRaw, "- [x] beta") {
		t.Fatalf("spec ticket file = %q, want every box ticked", specRaw)
	}
	if !strings.Contains(specRaw, "Status:") || !strings.Contains(specRaw, "done") {
		t.Fatalf("spec ticket file = %q, want Status done", specRaw)
	}
}

// Slice 5a: a dirty worktree keeps the Worker alive with a submit-time
// refusal naming uncommitted changes. Committing and resubmitting merges.
// The finish keeps its own dirty check for the submit-to-finish race; the
// submit gate is what Workers normally hit.
func TestRunDirtyWorktreeKeepsWorkerAlive(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
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
	code, msg := submitGateRaw(t, srv.URL, sess, "done", []string{"alpha"}, "all done", "")
	if code != http.StatusConflict {
		t.Fatalf("dirty done status = %d, want 409", code)
	}
	if !strings.Contains(strings.ToLower(msg), "uncommitted changes") {
		t.Fatalf("dirty done error = %q, want uncommitted changes", msg)
	}
	// The Worker stays alive on the same Session and no Reviewer starts.
	r = getRun(t, srv.URL, run.ID)
	for _, tk := range r.Tickets {
		if tk.ID == "01-first" {
			if tk.State == "merged" {
				t.Fatalf("dirty ticket merged, want it kept alive")
			}
			if tk.WorkerSession != sess {
				t.Fatalf("worker changed, want the same Worker alive")
			}
			if len(tk.ReviewerSessions) != 0 {
				t.Fatalf("reviewers = %v, want none after a refused submit", tk.ReviewerSessions)
			}
		}
	}
	if err := os.Remove(filepath.Join(worktree, "dirty.txt")); err != nil {
		t.Fatal(err)
	}
	commitTicketWork(t, repo, "01-first")
	submitSession(t, srv.URL, sess, "done", []string{"alpha"})
	passReviewers(t, srv.URL, run.ID, "01-first")
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return r.Status == "done"
	}, "run done after clean resubmit")
	for _, tk := range r.Tickets {
		if tk.State != "merged" {
			t.Fatalf("ticket %s state = %q, want merged", tk.ID, tk.State)
		}
	}
	// The merged Worker's Session ended before its worktree went: a live agent
	// process must never lose its cwd (its next tool call would fail getwd).
	d.mu.Lock()
	status := d.sessions[sess].meta.Status
	d.mu.Unlock()
	if status != "ended" {
		t.Fatalf("merged worker status = %q, want ended before its worktree is removed", status)
	}
	if _, err := os.Stat(worktree); !os.IsNotExist(err) {
		t.Fatalf("merged worktree still exists: %v", err)
	}
}

// Slice 5b: a rebase conflict keeps the Worker alive and shows the refusal.
// The conflict is against the spec branch, the merge target: a main move
// alone never blocks a ticket.
func TestRunRebaseConflictKeepsWorkerAlive(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
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
	// Move the spec branch under the ticket through a throwaway worktree,
	// so the main checkout never moves: the finish rebases onto spec/demo.
	specWT := filepath.Join(repo, ".el-worktrees", "spec-move")
	testGit(t, repo, "worktree", "add", specWT, "spec/demo")
	t.Cleanup(func() { testGit(t, repo, "worktree", "remove", "--force", specWT) })
	writeFile(t, specWT, "conflict.txt", "spec\n")
	testGit(t, specWT, "add", "conflict.txt")
	testGit(t, specWT, "commit", "-m", "spec moves")
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
			if !strings.Contains(tk.LastRefusal, "spec/demo") {
				t.Fatalf("lastRefusal = %q, want the spec branch named", tk.LastRefusal)
			}
		}
	}
}

// Slice 6: roles choose harness, model and effort; a missing role falls
// back to the Session default from the Start request.
func TestRunRolesChooseWorkerConfig(t *testing.T) {
	t.Parallel()
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
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	d.SetSettingsPath(settings)
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
	t.Parallel()
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
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	d.SetSettingsPath(settings)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "request-model")
	if run.Model != "impl-model" || run.Effort != "max" {
		t.Fatalf("run config = %s/%s, want impl-model/max", run.Model, run.Effort)
	}
}

func TestRunFallsBackToRequestWithoutRole(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	missing := filepath.Join(tempDir(t), "missing.json")
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	d.SetSettingsPath(missing)
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
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
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
	commitTicketWork(t, repo, "01-first")
	submitSession(t, srv.URL, sess, "done", []string{"alpha"})
	passReviewers(t, srv.URL, run.ID, "01-first")
	waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return r.Status == "done"
	}, "run done after done submit")
}

// A detached user checkout does not block promotion: the gate checks and
// fast-forwards master in a temporary clean worktree, leaving the detached
// checkout untouched.
func TestRunDetachedPromotesThroughTemporaryMasterCheckout(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md":  "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
		"02-second.md": "# 02: Second\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] beta\n",
	})
	initialMaster := strings.TrimSpace(testGit(t, repo, "rev-parse", "main"))
	testGit(t, repo, "checkout", "--detach")
	initialHEAD := strings.TrimSpace(testGit(t, repo, "rev-parse", "HEAD"))
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
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
	commitTicketWork(t, repo, "01-first")
	submitSession(t, srv.URL, sess["01-first"], "done", []string{"alpha"})
	commitTicketWork(t, repo, "02-second")
	submitSession(t, srv.URL, sess["02-second"], "done", []string{"beta"})
	passReviewers(t, srv.URL, run.ID, "01-first")
	passReviewers(t, srv.URL, run.ID, "02-second")
	waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return r.Status == runStatusDone
	}, "run done from detached checkout")
	if got := strings.TrimSpace(testGit(t, repo, "rev-parse", "main")); got != strings.TrimSpace(testGit(t, repo, "rev-parse", "spec/demo")) {
		t.Fatalf("main = %s, want final spec tip", got)
	}
	if got := strings.TrimSpace(testGit(t, repo, "rev-parse", "HEAD")); got != initialHEAD || got != initialMaster {
		t.Fatalf("detached HEAD moved from %s to %s during promotion", initialHEAD, got)
	}
}

// Review fixes: on a detached repo the finish rebases onto the repo HEAD as
// it stands at finish, not the stale recorded base. The worker branch below
// diverged from the recorded base after another merge moved HEAD: the old
// base rebase is a no-op and the fast-forward fails, the current HEAD
// rebase lands it.
func TestRunDetachedFinishRebasesOntoCurrentHead(t *testing.T) {
	t.Parallel()
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

	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
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
	t.Parallel()
	_, spec := initRunRepo(t, demoTickets())
	other := tempDir(t)
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
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
	t.Parallel()
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
	d := runTestDaemon(t, stateDir, NewFakeBackend())
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
	t.Parallel()
	repo, spec := initRunRepo(t, demoTickets())
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
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
	t.Parallel()
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

	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
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
	t.Parallel()
	state := tempDir(t)
	d := runTestDaemon(t, state, NewFakeBackend())
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

	d2 := runTestDaemon(t, state, NewFakeBackend())
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
	t.Parallel()
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
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

// submitGateRaw posts one Worker submit with report and summary and returns
// the status code and error text, so gate refusals can assert 409.
func submitGateRaw(t *testing.T, base, id, status string, completed []string, report, summary string, dispositions ...ReviewDisposition) (int, string) {
	t.Helper()
	payload := map[string]any{"status": status, "completed": completed, "report": report, "summary": summary}
	if len(dispositions) > 0 {
		payload["dispositions"] = dispositions
	}
	body, _ := json.Marshal(payload)
	resp, err := http.Post(base+"/api/sessions/"+id+"/submit", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Error   string `json:"error"`
		Submits int    `json:"submits"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out.Error
}

// A done submit with no new commit since the last changes verdict is refused
// with a reason; a new commit lets the next done through.
func TestWorkerDoneWithoutNewCommitAfterChangesRefused(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	fb := NewFakeBackend()
	var mu sync.Mutex
	specCalls := 0
	fb.ReplyFunc = func(id, text string) (string, bool) {
		if !strings.Contains(text, "Review ticket") && !strings.Contains(text, "RESPOND IN FORMAT") {
			return "", false
		}
		if strings.Contains(text, "reviewer-spec") {
			mu.Lock()
			specCalls++
			n := specCalls
			mu.Unlock()
			if n == 1 {
				return "VERDICT: changes\nwork.go:1 blocker: handle nil", true
			}
		}
		return "VERDICT: pass", true
	}
	d := runTestDaemon(t, tempDir(t), fb)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	worker := ticketByID(r, "01-first").WorkerSession
	commitTicketWork(t, repo, "01-first")
	if code, _ := submitGateRaw(t, srv.URL, worker, "done", []string{"alpha"}, "all done", ""); code != http.StatusAccepted {
		t.Fatalf("first done status = %d, want 202", code)
	}
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		tk := ticketByID(r, "01-first")
		return tk.LastRefusal != "" && strings.Contains(tk.LastRefusal, "handle nil")
	}, "changes refusal shown")
	code, msg := submitGateRaw(t, srv.URL, worker, "done", []string{"alpha"}, "all done", "")
	if code != http.StatusConflict {
		t.Fatalf("empty resubmit status = %d, want 409", code)
	}
	if !strings.Contains(strings.ToLower(msg), "no new commit") {
		t.Fatalf("empty resubmit error = %q, want it to name no new commit", msg)
	}
	worktree := filepath.Join(repo, ".el-worktrees", "01-first")
	writeFile(t, worktree, "work.go", "package main\n")
	testGit(t, worktree, "add", "work.go")
	testGit(t, worktree, "commit", "-m", "fix nil")
	if code, _ := submitGateRaw(t, srv.URL, worker, "done", []string{"alpha"}, "all done", "", ReviewDisposition{FindingID: "r1-reviewer-spec-1", Disposition: "fixed"}); code != http.StatusAccepted {
		t.Fatalf("done after a new commit status = %d, want 202", code)
	}
}

// A done submit whose text says the work is not done is refused; finished
// text, Follow-ups notes and reports about handling bad input go through.
func TestWorkerDoneNotDoneTextRefused(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	worker := ticketByID(r, "01-first").WorkerSession
	commitTicketWork(t, repo, "01-first")
	code, msg := submitGateRaw(t, srv.URL, worker, "done", []string{"alpha"}, "Not done: auth handler missing", "")
	if code != http.StatusConflict {
		t.Fatalf("not-done done status = %d, want 409", code)
	}
	if !strings.Contains(strings.ToLower(msg), "not done") {
		t.Fatalf("not-done error = %q, want it to name not done", msg)
	}
	code, msg = submitGateRaw(t, srv.URL, worker, "done", []string{"alpha"}, "all done", "work is not complete")
	if code != http.StatusConflict {
		t.Fatalf("summary not-done status = %d, want 409", code)
	}
	if !strings.Contains(strings.ToLower(msg), "not done") {
		t.Fatalf("summary not-done error = %q, want it to name not done", msg)
	}
	// The Follow-ups section is excluded: follow-up notes never refuse.
	if code, _ := submitGateRaw(t, srv.URL, worker, "done", []string{"alpha"}, "All done, boxes ticked", "Follow-ups: still needs docs"); code != http.StatusAccepted {
		t.Fatalf("follow-ups done status = %d, want 202", code)
	}
	// Reports about handling unfinished or invalid input are not refusals.
	for _, report := range []string{"handles incomplete input", "rejects not yet valid tokens", "reports unfinished jobs as skipped"} {
		if msg := refuseNotDoneText(report, ""); msg != "" {
			t.Fatalf("report %q refused: %q, want accepted", report, msg)
		}
	}
	if code, _ := submitGateRaw(t, srv.URL, worker, "done", []string{"alpha"}, "All done, boxes ticked", "Follow-ups: none"); code != http.StatusAccepted {
		t.Fatalf("finished done status = %d, want 202", code)
	}
}

// A second blocked submit with the same question becomes needs-you; a
// different question does not.
func TestWorkerSecondBlockedSameQuestionNeedsYou(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	worker := ticketByID(r, "01-first").WorkerSession
	if code, _ := submitGateRaw(t, srv.URL, worker, "blocked", nil, "Need the API key for X", ""); code != http.StatusAccepted {
		t.Fatalf("first blocked status = %d, want 202", code)
	}
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").LastRefusal != ""
	}, "blocked refusal shown")
	if got := ticketByID(r, "01-first"); got.State == "needs-you" {
		t.Fatalf("first blocked state = needs-you, want it to wait for done")
	}
	if code, _ := submitGateRaw(t, srv.URL, worker, "blocked", nil, "Need a different token for Y", ""); code != http.StatusAccepted {
		t.Fatalf("different blocked status = %d, want 202", code)
	}
	if got := getRun(t, srv.URL, run.ID); ticketByID(got, "01-first").State == "needs-you" {
		t.Fatalf("different question state = needs-you, want it to wait for done")
	}
	if code, _ := submitGateRaw(t, srv.URL, worker, "blocked", nil, "Need  a different   token for Y", ""); code != http.StatusAccepted {
		t.Fatalf("repeat blocked status = %d, want 202", code)
	}
	waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").State == "needs-you"
	}, "repeat blocked becomes needs-you")
	// A done submit clears the blocked question track, while needs-you stays
	// until a retry: the next identical blocked starts a fresh count.
	commitTicketWork(t, repo, "01-first")
	if code, _ := submitGateRaw(t, srv.URL, worker, "done", []string{"alpha"}, "All done", ""); code != http.StatusAccepted {
		t.Fatalf("done after blocked status = %d, want 202", code)
	}
	d.mu.Lock()
	if _, ok := d.runs[run.ID].Blocked["01-first"]; ok {
		d.mu.Unlock()
		t.Fatalf("blocked track survives done, want it cleared")
	}
	d.mu.Unlock()
	if code, _ := postTicketAction(t, srv.URL, run.ID, "01-first", "retry", ""); code != http.StatusOK {
		t.Fatalf("retry status = %d, want 200", code)
	}
	if code, _ := submitGateRaw(t, srv.URL, worker, "blocked", nil, "Need a different token for Y", ""); code != http.StatusAccepted {
		t.Fatalf("blocked after retry status = %d, want 202", code)
	}
	// The retry's row can still read needs-you until the next driver pass
	// publishes; a blocked submit that wrongly counted as the repeat would
	// keep it there, so wait for the state to leave needs-you.
	waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").State != "needs-you"
	}, "blocked after retry starts a fresh count")
}

// A retry clears the blocked question, and the blocked track survives a
// Daemon restart through the persisted Run file.
func TestWorkerBlockedResetOnRetryAndRestart(t *testing.T) {
	t.Parallel()
	state := tempDir(t)
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	d := runTestDaemon(t, state, NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	worker := ticketByID(r, "01-first").WorkerSession
	if code, _ := submitGateRaw(t, srv.URL, worker, "blocked", nil, "Need the API key for X", ""); code != http.StatusAccepted {
		t.Fatalf("first blocked status = %d, want 202", code)
	}
	// Retry clears the track while running.
	if code, _ := postTicketAction(t, srv.URL, run.ID, "01-first", "retry", ""); code != http.StatusOK {
		t.Fatalf("retry status = %d, want 200", code)
	}
	d.mu.Lock()
	if _, ok := d.runs[run.ID].Blocked["01-first"]; ok {
		d.mu.Unlock()
		t.Fatalf("blocked track survives retry, want it cleared")
	}
	d.mu.Unlock()
	// A fresh blocked starts over after the retry.
	if code, _ := submitGateRaw(t, srv.URL, worker, "blocked", nil, "Need the API key for X", ""); code != http.StatusAccepted {
		t.Fatalf("blocked after retry status = %d, want 202", code)
	}
	if got := getRun(t, srv.URL, run.ID); ticketByID(got, "01-first").State == "needs-you" {
		t.Fatalf("blocked after retry state = needs-you, want a fresh count")
	}
	// Pause first so the reloaded Daemon is the only driver afterwards.
	if _, code := postRunAction(t, srv.URL, run.ID, "pause"); code != http.StatusOK {
		t.Fatalf("pause status = %d, want 200", code)
	}
	srv.Close()
	d2 := runTestDaemon(t, state, NewFakeBackend())
	srv2 := httptest.NewServer(authedHandler(t, d2))
	defer srv2.Close()
	d2.mu.Lock()
	rs, ok := d2.runs[run.ID]
	if !ok {
		d2.mu.Unlock()
		t.Fatalf("run %s missing after restart", run.ID)
	}
	track, ok := rs.Blocked["01-first"]
	d2.mu.Unlock()
	if !ok || track.Count != 1 {
		t.Fatalf("blocked track after restart = %+v, want count 1", track)
	}
	if _, code := postRunAction(t, srv2.URL, run.ID, "resume"); code != http.StatusOK {
		t.Fatalf("resume status = %d, want 200", code)
	}
	r = getRun(t, srv2.URL, run.ID)
	worker = ticketByID(r, "01-first").WorkerSession
	if code, _ := submitGateRaw(t, srv2.URL, worker, "blocked", nil, "Need the API key for X", ""); code != http.StatusAccepted {
		t.Fatalf("repeat blocked status = %d, want 202", code)
	}
	waitRunCond(t, srv2.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").State == "needs-you"
	}, "repeat blocked becomes needs-you after restart")
}

// A provider session-limit failure is visible on the Session, skips submit and
// stall reminders, and still reaches the Run's normal idle needs-you alert.
func TestCappedWorkerGetsNoSubmitNudges(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	fb := NewFakeBackend()
	fb.TurnFailure = func(_ string, prompt string) (string, bool) {
		if strings.Contains(prompt, "Implement ticket 01-first") {
			return "session limit reached for this account", true
		}
		return "", false
	}
	clock := newFakeClock()
	d := runTestDaemon(t, tempDir(t), fb)
	d.clock = clock.get
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	worker := ticketByID(r, "01-first").WorkerSession
	deadline := time.Now().Add(15 * time.Second)
	var sess sessionBody
	for {
		sess = getSession(t, srv.URL, worker)
		if sess.Meta.Status == "failed" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("worker did not expose the scripted capped failure: status=%q", sess.Meta.Status)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !sess.Meta.Capped {
		t.Fatalf("failed Session capped = false, want true; meta=%+v", sess.Meta)
	}
	foundCap := false
	for _, e := range sess.Events {
		if e.Kind == "error" && strings.Contains(strings.ToLower(e.Text), "session limit") {
			foundCap = true
			break
		}
	}
	if !foundCap {
		t.Fatalf("capped Session has no visible session-limit error: %+v", sess.Events)
	}
	if prompts := workerPromptTexts(t, srv.URL, worker); len(prompts) != 1 {
		t.Fatalf("capped Worker received %d prompts before timeout, want only its task", len(prompts))
	}

	// A capped failure is not treated as busy: after ten idle minutes the
	// driver still raises its normal needs-you alert instead of hanging silently.
	clock.advance(11 * time.Minute)
	deadline = time.Now().Add(15 * time.Second)
	for {
		got := actionTicketByID(getActionRun(t, srv.URL, run.ID), "01-first")
		if got.State == "needs-you" && strings.Contains(got.NeedsYou, "no worker activity for 10 minutes") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("capped Worker did not reach idle needs-you: state=%q reason=%q", got.State, got.NeedsYou)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if prompts := workerPromptTexts(t, srv.URL, worker); len(prompts) != 1 {
		t.Fatalf("capped Worker received %d prompts after idle timeout, want only its task: %q", len(prompts), prompts)
	}
}

// testPollInterval is the driver poll for Run tests. EL_TEST_POLL_MS tunes
// it for experiments; the default is 100ms.
func testPollInterval() time.Duration {
	if v, err := strconv.Atoi(os.Getenv("EL_TEST_POLL_MS")); err == nil && v > 0 {
		return time.Duration(v) * time.Millisecond
	}
	return 100 * time.Millisecond
}

// A spec file's max_tickets caps concurrent tickets for a web-started Run:
// with three ready tickets and max_tickets: 2 under a larger global, at most
// two workers ever start at once, and all three still merge.
func TestRunHonorsSpecMaxTickets(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md":  "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
		"02-second.md": "# 02: Second\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] beta\n",
		"03-third.md":  "# 03: Third\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] gamma\n",
	})
	raw, err := os.ReadFile(spec)
	if err != nil {
		t.Fatal(err)
	}
	capped := "---\nmax_tickets: 2\n---\n\n" + string(raw)
	if err := os.WriteFile(spec, []byte(capped), 0o644); err != nil {
		t.Fatal(err)
	}
	testGit(t, repo, "add", "-A")
	testGit(t, repo, "commit", "-m", "spec cap")
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	d.SetMaxTickets(5)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	// WorkerSession stays set after a merge, so only unmerged tickets with
	// a session count toward concurrency.
	maxStarted := 0
	countStarted := func(r runJSON) int {
		started := 0
		for _, tk := range r.Tickets {
			if tk.WorkerSession != "" && tk.State != "merged" {
				started++
			}
		}
		if started > maxStarted {
			maxStarted = started
		}
		return started
	}
	complete := func(r runJSON, id string) {
		for _, tk := range r.Tickets {
			if tk.ID == id {
				commitTicketWork(t, repo, id)
				submitSession(t, srv.URL, tk.WorkerSession, "done", nil)
			}
		}
		passReviewers(t, srv.URL, r.ID, id)
	}

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return countStarted(r) == 2
	}, "two workers started under the spec cap")
	// The third ticket waits while two run.
	first := ""
	for _, tk := range r.Tickets {
		if tk.WorkerSession != "" && first == "" {
			first = tk.ID
		}
	}
	complete(r, first)
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		countStarted(r)
		for _, tk := range r.Tickets {
			if tk.ID == first && tk.State == "merged" {
				return true
			}
		}
		return false
	}, "first ticket merged")
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		started := 0
		for _, tk := range r.Tickets {
			if tk.WorkerSession != "" {
				started++
			}
		}
		return started == 3
	}, "third ticket started after the merge")
	for _, tk := range r.Tickets {
		if tk.State != "merged" {
			complete(r, tk.ID)
		}
	}
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		countStarted(r)
		return r.Status == "done"
	}, "run done")
	for _, tk := range r.Tickets {
		if tk.State != "merged" {
			t.Fatalf("ticket %s state = %q, want merged", tk.ID, tk.State)
		}
	}
	if maxStarted > 2 {
		t.Errorf("max concurrent tickets = %d, want at most the spec's 2", maxStarted)
	}
}
