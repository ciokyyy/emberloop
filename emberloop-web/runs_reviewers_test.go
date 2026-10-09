package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func submitReview(t *testing.T, base, id, status, report string) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"status": status, "report": report})
	resp, err := http.Post(base+"/api/sessions/"+id+"/submit", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("review submit status = %d, want 202", resp.StatusCode)
	}
}

func waitReviewers(t *testing.T, base, runID, ticketID string) map[string]string {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		r := getRun(t, base, runID)
		for _, tk := range r.Tickets {
			if tk.ID == ticketID && len(tk.ReviewerSessions) == 2 {
				return tk.ReviewerSessions
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for reviewers of %s", ticketID)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

type sessionBody struct {
	Meta   SessionMeta `json:"session"`
	Events []Event     `json:"events"`
}

func getSession(t *testing.T, base, id string) sessionBody {
	t.Helper()
	resp, err := http.Get(base + "/api/sessions/" + id)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("session get status = %d, want 200", resp.StatusCode)
	}
	var body sessionBody
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body
}

func sessTranscript(s sessionBody) string {
	var b strings.Builder
	for _, e := range s.Events {
		b.WriteString(e.Text)
		b.WriteString("\n")
	}
	return b.String()
}

func passReviewers(t *testing.T, base, runID, ticketID string) {
	t.Helper()
	for _, sess := range waitReviewers(t, base, runID, ticketID) {
		submitReview(t, base, sess, "pass", "")
	}
}

func ticketByID(r runJSON, id string) runTicketJSON {
	for _, tk := range r.Tickets {
		if tk.ID == id {
			return tk
		}
	}
	return runTicketJSON{}
}

// A submitted done spawns both Reviewers once; without passes the merge is
// refused, and the row carries verdicts, tip freshness and no refusal yet.
func TestReviewersSpawnOnDoneAndGateRefusesWithoutPass(t *testing.T) {
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	d := NewDaemon(t.TempDir(), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	worker := ticketByID(r, "01-first").WorkerSession
	submitSession(t, srv.URL, worker, "done", []string{"alpha"})

	revs := waitReviewers(t, srv.URL, run.ID, "01-first")
	if revs["reviewer-spec"] == "" || revs["reviewer-standards"] == "" {
		t.Fatalf("reviewer sessions = %v, want both types", revs)
	}
	if revs["reviewer-spec"] == revs["reviewer-standards"] {
		t.Fatalf("reviewer sessions are the same, want two")
	}
	first := getRun(t, srv.URL, run.ID)
	if got := ticketByID(first, "01-first"); got.State == "merged" {
		t.Fatalf("ticket merged without passes, want the gate to refuse")
	}
	submitReview(t, srv.URL, revs["reviewer-spec"], "pass", "")
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		tk := ticketByID(r, "01-first")
		return tk.Verdicts["reviewer-spec"].Verdict == "pass" && tk.State != "merged"
	}, "spec pass shown without merging")
	tk := ticketByID(r, "01-first")
	if tk.Tip == "" {
		t.Fatalf("tip empty, want the worker branch tip")
	}
	if !tk.Verdicts["reviewer-spec"].Fresh {
		t.Fatalf("spec verdict = %+v, want fresh at the tip", tk.Verdicts["reviewer-spec"])
	}
	if tk.LastRefusal != "" {
		t.Fatalf("lastRefusal = %q, want empty while a pass is still missing", tk.LastRefusal)
	}
}

// A changes verdict is delivered to the same Worker unchanged; the rework
// loop keeps the Reviewer sessions and merges once both pass at the new tip.
func TestReviewFindingsReachWorkerAndReworkMerges(t *testing.T) {
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	d := NewDaemon(t.TempDir(), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	worker := ticketByID(r, "01-first").WorkerSession
	submitSession(t, srv.URL, worker, "done", []string{"alpha"})
	revs := waitReviewers(t, srv.URL, run.ID, "01-first")

	submitReview(t, srv.URL, revs["reviewer-spec"], "changes", "work.go:1 blocker: handle nil")
	submitReview(t, srv.URL, revs["reviewer-standards"], "pass", "")
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		tk := ticketByID(r, "01-first")
		return tk.LastRefusal != "" && strings.Contains(tk.LastRefusal, "handle nil")
	}, "refusal carries the findings")
	if got := ticketByID(r, "01-first"); got.State == "merged" {
		t.Fatalf("ticket merged with a changes verdict, want it kept")
	}
	sess := getSession(t, srv.URL, worker)
	if !strings.Contains(sessTranscript(sess), "handle nil") {
		t.Fatalf("worker transcript misses the findings")
	}

	worktree := filepath.Join(repo, ".el-worktrees", "01-first")
	writeFile(t, worktree, "work.go", "package main\n")
	testGit(t, worktree, "add", "work.go")
	testGit(t, worktree, "commit", "-m", "fix nil")
	submitSession(t, srv.URL, worker, "done", []string{"alpha"})
	// A new commit leaves older verdicts behind; while the re-prompt for
	// the new tip is out the Ticket is under review, not refused, so no
	// new Tell reaches the Worker.
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").LastRefusal == ""
	}, "under review after the new commit")
	if got := ticketByID(r, "01-first"); got.Verdicts["reviewer-standards"].Fresh {
		t.Fatalf("standards verdict = %+v, want stale after the new commit", got.Verdicts["reviewer-standards"])
	}
	sess = getSession(t, srv.URL, worker)
	if strings.Contains(sessTranscript(sess), "not at the tip") {
		t.Fatalf("worker transcript carries a stale refusal, want no second Tell while under review")
	}
	for typ, sess := range waitReviewers(t, srv.URL, run.ID, "01-first") {
		if sess != revs[typ] {
			t.Fatalf("reviewer %s session changed, want the persistent one", typ)
		}
		submitReview(t, srv.URL, sess, "pass", "")
	}
	waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return r.Status == "done"
	}, "run done after both pass at the new tip")
}

// Reviewer sessions use their settings roles, falling back to the Run config.
func TestReviewerSessionsUseRolesWithFallback(t *testing.T) {
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	settings := filepath.Join(t.TempDir(), "settings.json")
	raw, _ := json.Marshal(map[string]any{
		"roles": map[string]any{
			"reviewer-spec": map[string]any{"harness": "fake", "model": "spec-model"},
		},
	})
	if err := os.WriteFile(settings, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EL_SETTINGS", settings)
	d := NewDaemon(t.TempDir(), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "request-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	submitSession(t, srv.URL, ticketByID(r, "01-first").WorkerSession, "done", nil)
	revs := waitReviewers(t, srv.URL, run.ID, "01-first")
	if got := getSession(t, srv.URL, revs["reviewer-spec"]).Meta.Model; got != "spec-model" {
		t.Fatalf("spec reviewer model = %q, want spec-model", got)
	}
	if got := getSession(t, srv.URL, revs["reviewer-standards"]).Meta.Model; got != "request-model" {
		t.Fatalf("standards reviewer model = %q, want the run fallback request-model", got)
	}
}

// A Reviewer Session starts through the shared live-Session cap: a full cap
// refuses it, and freeing the cap lets the retry in.
func TestReviewersCountAgainstLiveSessionCap(t *testing.T) {
	repo, _ := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	branch := "implement/01-first"
	worktree := filepath.Join(repo, ".el-worktrees", "01-first")
	testGit(t, repo, "worktree", "add", "-b", branch, worktree, "HEAD")
	t.Cleanup(func() { testGit(t, repo, "worktree", "remove", "--force", worktree) })
	d := NewDaemon(t.TempDir(), NewFakeBackend())
	d.SetMaxRunning(1)
	d.mu.Lock()
	d.sessions["holder"] = &session{meta: SessionMeta{ID: "holder", Harness: "fake", Model: "m", Status: "idle"}, live: true, notify: make(chan struct{})}
	d.runs["r-cap"] = &runState{
		ID: "r-cap", Spec: "spec", Repo: repo, Status: "running",
		Harness: "fake", Model: "fake-model",
		notify:   make(chan struct{}),
		Tickets:  []staticTicket{{ID: "01-first", Num: 1, Title: "First"}},
		Sessions: map[string]string{}, Branches: map[string]string{"01-first": branch},
		Worktrees: map[string]string{"01-first": worktree}, Targets: map[string]string{},
		Bases: map[string]string{}, Merged: map[string]string{},
		Reviewers: map[string]map[string]string{}, Reviews: map[string]map[string]storedReview{},
		ReviewTips: map[string]map[string]string{}, Reprompted: map[string]int{},
	}
	d.mu.Unlock()
	if err := d.runStartReviewer("r-cap", "01-first", "reviewer-spec", "deadbeef"); err == nil || !strings.Contains(err.Error(), "cap") {
		t.Fatalf("runStartReviewer err = %v, want the cap refusal", err)
	}
	d.mu.Lock()
	delete(d.sessions, "holder")
	if sess := d.runs["r-cap"].Reviewers["01-first"]["reviewer-spec"]; sess != "" {
		d.mu.Unlock()
		t.Fatalf("reviewer session = %q, want none while capped", sess)
	}
	d.mu.Unlock()
	if err := d.runStartReviewer("r-cap", "01-first", "reviewer-spec", "deadbeef"); err != nil {
		t.Fatalf("runStartReviewer retry = %v, want the reviewer in", err)
	}
	d.mu.Lock()
	sess := d.runs["r-cap"].Reviewers["01-first"]["reviewer-spec"]
	d.mu.Unlock()
	if sess == "" {
		t.Fatalf("no reviewer session after the cap freed, want one")
	}
}

// A verdict lands at the tip the reviewer was prompted on, not the tip as
// it stands at submit: a Worker commit mid-review reads stale, never fresh.
func TestReviewVerdictStampsPromptedTip(t *testing.T) {
	repo, _ := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	branch := "implement/01-first"
	worktree := filepath.Join(repo, ".el-worktrees", "01-first")
	testGit(t, repo, "worktree", "add", "-b", branch, worktree, "HEAD")
	t.Cleanup(func() { testGit(t, repo, "worktree", "remove", "--force", worktree) })
	prompted := "1111111111111111111111111111111111111111"
	d := NewDaemon(t.TempDir(), NewFakeBackend())
	d.mu.Lock()
	d.sessions["rev-spec"] = &session{meta: SessionMeta{ID: "rev-spec", Harness: "fake", Model: "m", Status: "idle"}, live: true, notify: make(chan struct{})}
	d.runs["r-tip"] = &runState{
		ID: "r-tip", Spec: "spec", Repo: repo, Status: "running",
		Harness: "fake", Model: "fake-model",
		notify:     make(chan struct{}),
		Tickets:    []staticTicket{{ID: "01-first", Num: 1, Title: "First"}},
		Branches:   map[string]string{"01-first": branch},
		Worktrees:  map[string]string{"01-first": worktree},
		Reviewers:  map[string]map[string]string{"01-first": {"reviewer-spec": "rev-spec"}},
		ReviewTips: map[string]map[string]string{"01-first": {"reviewer-spec": prompted}},
		Reviews:    map[string]map[string]storedReview{},
	}
	d.mu.Unlock()
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()
	submitReview(t, srv.URL, "rev-spec", "pass", "")
	r := getRun(t, srv.URL, "r-tip")
	got := ticketByID(r, "01-first").Verdicts["reviewer-spec"]
	if got.Verdict != "pass" || got.At != prompted {
		t.Fatalf("verdict = %+v, want pass at the prompted tip %q", got, prompted)
	}
	if got.Fresh {
		t.Fatalf("verdict = %+v, want stale against the moved branch tip", got)
	}
}

// A verdict already stale at birth re-prompts its reviewer once at the
// current tip: the stale verdict still lands at its prompted tip, but the
// reviewer is asked to judge the tip that moved under review.
func TestReviewStaleVerdictRepromptsCurrentTip(t *testing.T) {
	repo, _ := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	branch := "implement/01-first"
	worktree := filepath.Join(repo, ".el-worktrees", "01-first")
	testGit(t, repo, "worktree", "add", "-b", branch, worktree, "HEAD")
	t.Cleanup(func() { testGit(t, repo, "worktree", "remove", "--force", worktree) })
	prompted := "1111111111111111111111111111111111111111"
	d := NewDaemon(t.TempDir(), NewFakeBackend())
	d.mu.Lock()
	d.runs["r-tip"] = &runState{
		ID: "r-tip", Spec: "spec", Repo: repo, Status: "running",
		Harness: "fake", Model: "fake-model",
		notify:     make(chan struct{}),
		Tickets:    []staticTicket{{ID: "01-first", Num: 1, Title: "First"}},
		Branches:   map[string]string{"01-first": branch},
		Worktrees:  map[string]string{"01-first": worktree},
		Reviewers:  map[string]map[string]string{},
		ReviewTips: map[string]map[string]string{},
		Reviews:    map[string]map[string]storedReview{},
	}
	d.mu.Unlock()
	// Start the reviewer judging an older tip through the real start path,
	// so its Session can take the re-prompt.
	if err := d.runStartReviewer("r-tip", "01-first", "reviewer-spec", prompted); err != nil {
		t.Fatal(err)
	}
	d.mu.Lock()
	sess := d.runs["r-tip"].Reviewers["01-first"]["reviewer-spec"]
	d.mu.Unlock()
	if sess == "" {
		t.Fatalf("no reviewer session, want one")
	}
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()
	submitReview(t, srv.URL, sess, "pass", "")
	cur := strings.TrimSpace(testGit(t, repo, "rev-parse", branch))
	r := getRun(t, srv.URL, "r-tip")
	got := ticketByID(r, "01-first").Verdicts["reviewer-spec"]
	if got.Verdict != "pass" || got.At != prompted {
		t.Fatalf("verdict = %+v, want pass at the prompted tip %q", got, prompted)
	}
	d.mu.Lock()
	retipped := d.runs["r-tip"].ReviewTips["01-first"]["reviewer-spec"]
	d.mu.Unlock()
	if retipped != cur {
		t.Fatalf("prompted tip = %q, want the re-prompt at the current tip %q", retipped, cur)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if strings.Contains(sessTranscript(getSession(t, srv.URL, sess)), cur) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("reviewer transcript misses the re-prompt at %q", cur)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// A reviewer pass after the Worker moved the tip under review binds to the
// tip of the prompt it answers, not the latest prompted tip: with T1
// prompted, then T2 prompted, the first submit still lands at T1 stale.
func TestReviewPassAfterRepromptBindsAnsweredTip(t *testing.T) {
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	d := NewDaemon(t.TempDir(), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	worker := ticketByID(r, "01-first").WorkerSession
	submitSession(t, srv.URL, worker, "done", []string{"alpha"})
	revs := waitReviewers(t, srv.URL, run.ID, "01-first")
	t1 := strings.TrimSpace(testGit(t, repo, "rev-parse", "implement/01-first"))
	worktree := filepath.Join(repo, ".el-worktrees", "01-first")
	writeFile(t, worktree, "work.go", "package main\n")
	testGit(t, worktree, "add", "work.go")
	testGit(t, worktree, "commit", "-m", "second")
	t2 := strings.TrimSpace(testGit(t, repo, "rev-parse", "implement/01-first"))
	if t1 == t2 {
		t.Fatalf("tips equal %q, want a new commit to move the tip", t1)
	}
	submitSession(t, srv.URL, worker, "done", []string{"alpha"})
	// Wait for the driver to publish the new tip before judging freshness:
	// verdict Fresh compares against the published row tip.
	waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").Tip == t2
	}, "driver publishes the new tip")
	specSess := revs["reviewer-spec"]
	deadline := time.Now().Add(15 * time.Second)
	for {
		if strings.Contains(sessTranscript(getSession(t, srv.URL, specSess)), t2) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("reviewer transcript misses the re-prompt at %q", t2)
		}
		time.Sleep(100 * time.Millisecond)
	}
	submitReview(t, srv.URL, specSess, "pass", "")
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		tk := ticketByID(r, "01-first")
		return tk.Verdicts["reviewer-spec"].Verdict == "pass" && tk.Tip == t2
	}, "spec verdict shown at the new tip")
	got := ticketByID(r, "01-first").Verdicts["reviewer-spec"]
	if got.At != t1 {
		t.Fatalf("verdict at = %q, want the answered prompt %q (not the latest %q)", got.At, t1, t2)
	}
	if got.Fresh {
		t.Fatalf("verdict = %+v, want stale at %q against the tip %q", got, t1, t2)
	}
	if ticketByID(r, "01-first").State == "merged" {
		t.Fatalf("ticket merged on a stale pass, want the gate to refuse")
	}
}
