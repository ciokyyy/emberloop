package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

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

// passReviewers waits for both reviewers to pass via their VERDICT
// replies (the fake Backend defaults reviewer prompts to pass). It sends
// nothing: reviewers never submit.
func passReviewers(t *testing.T, base, runID, ticketID string) {
	t.Helper()
	waitReviewers(t, base, runID, ticketID)
	waitRunCond(t, base, runID, func(r runJSON) bool {
		tk := ticketByID(r, ticketID)
		return tk.Verdicts["reviewer-spec"].Verdict == "pass" &&
			tk.Verdicts["reviewer-standards"].Verdict == "pass"
	}, "both reviewers pass")
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
// Reviewers reply VERDICT in their turn: the spec reviewer passes at once
// while the standards reviewer works, so the gate holds on one pass.
func TestReviewersSpawnOnDoneAndGateRefusesWithoutPass(t *testing.T) {
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	fb := NewFakeBackend()
	fb.ReplyFunc = func(id, text string) (string, bool) {
		if strings.Contains(text, "reviewer-standards") {
			time.Sleep(4 * time.Second)
			return "VERDICT: pass", true
		}
		if strings.Contains(text, "Review ticket") || strings.Contains(text, "RESPOND IN FORMAT") {
			return "VERDICT: pass", true
		}
		return "", false
	}
	d := NewDaemon(tempDir(t), fb)
	srv := httptest.NewServer(authedHandler(t, d))
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
// The spec reviewer reports changes on its first turn and passes after the
// rework; the standards reviewer always passes.
func TestReviewFindingsReachWorkerAndReworkMerges(t *testing.T) {
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
	d := NewDaemon(tempDir(t), fb)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	worker := ticketByID(r, "01-first").WorkerSession
	submitSession(t, srv.URL, worker, "done", []string{"alpha"})
	revs := waitReviewers(t, srv.URL, run.ID, "01-first")

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
	settings := filepath.Join(tempDir(t), "settings.json")
	raw, _ := json.Marshal(map[string]any{
		"roles": map[string]any{
			"reviewer-spec": map[string]any{"harness": "fake", "model": "spec-model"},
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
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.SetMaxRunning(1)
	d.mu.Lock()
	d.sessions["holder"] = &session{meta: SessionMeta{ID: "holder", Harness: "fake", Model: "m", Status: "idle", Owner: "owner-1"}, live: true, notify: make(chan struct{})}
	d.runs["r-cap"] = &runState{
		ID: "r-cap", Spec: "spec", Owner: "owner-1", Repo: repo, Status: "running",
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
// it stands when the turn ends: a Worker commit mid-review reads stale,
// never fresh. The reply arrives as the turn's final assistant message;
// the Daemon stamps the oldest owed prompt.
func TestReviewVerdictStampsPromptedTip(t *testing.T) {
	repo, _ := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	branch := "implement/01-first"
	worktree := filepath.Join(repo, ".el-worktrees", "01-first")
	testGit(t, repo, "worktree", "add", "-b", branch, worktree, "HEAD")
	t.Cleanup(func() { testGit(t, repo, "worktree", "remove", "--force", worktree) })
	prompted := "1111111111111111111111111111111111111111"
	cur := strings.TrimSpace(testGit(t, repo, "rev-parse", branch))
	fb := NewFakeBackend()
	fb.Delay = 30 * time.Second
	d := NewDaemon(tempDir(t), fb)
	prompt := reviewerPrompt("01-first", "spec", "reviewer-spec", branch, prompted)
	d.mu.Lock()
	d.sessions["rev-spec"] = &session{
		meta: SessionMeta{ID: "rev-spec", Harness: "fake", Model: "m", Status: "idle", Owner: "owner-1"},
		live: true, notify: make(chan struct{}),
		events: []Event{
			{Seq: 1, SessionID: "rev-spec", Kind: "prompt", Text: prompt},
			{Seq: 2, SessionID: "rev-spec", Kind: "assistant_chunk", Text: "VERDICT: pass"},
		},
	}
	d.runs["r-tip"] = &runState{
		ID: "r-tip", Spec: "spec", Owner: "owner-1", Repo: repo, Status: "running",
		Harness: "fake", Model: "fake-model",
		notify:        make(chan struct{}),
		unpause:       make(chan struct{}),
		Tickets:       []staticTicket{{ID: "01-first", Num: 1, Title: "First"}},
		Branches:      map[string]string{"01-first": branch},
		Worktrees:     map[string]string{"01-first": worktree},
		Reviewers:     map[string]map[string]string{"01-first": {"reviewer-spec": "rev-spec"}},
		ReviewTips:    map[string]map[string]string{"01-first": {"reviewer-spec": prompted}},
		ReviewPending: map[string]map[string][]string{"01-first": {"reviewer-spec": {prompted}}},
		Reviews:       map[string]map[string]storedReview{},
		Rows:          map[string]savedRow{"01-first": {State: "in-review", Tip: cur}},
	}
	d.mu.Unlock()
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	d.handleReviewerTurnEnd("rev-spec")
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
	cur := strings.TrimSpace(testGit(t, repo, "rev-parse", branch))
	fb := NewFakeBackend()
	fb.Delay = 30 * time.Second
	d := NewDaemon(tempDir(t), fb)
	prompt := reviewerPrompt("01-first", "spec", "reviewer-spec", branch, prompted)
	d.mu.Lock()
	d.sessions["rev-spec"] = &session{
		meta: SessionMeta{ID: "rev-spec", Harness: "fake", Model: "m", Status: "idle", Owner: "owner-1"},
		live: true, notify: make(chan struct{}),
		events: []Event{
			{Seq: 1, SessionID: "rev-spec", Kind: "prompt", Text: prompt},
			{Seq: 2, SessionID: "rev-spec", Kind: "assistant_chunk", Text: "VERDICT: pass"},
		},
	}
	d.runs["r-tip"] = &runState{
		ID: "r-tip", Spec: "spec", Owner: "owner-1", Repo: repo, Status: "running",
		Harness: "fake", Model: "fake-model",
		notify:        make(chan struct{}),
		unpause:       make(chan struct{}),
		Tickets:       []staticTicket{{ID: "01-first", Num: 1, Title: "First"}},
		Branches:      map[string]string{"01-first": branch},
		Worktrees:     map[string]string{"01-first": worktree},
		Reviewers:     map[string]map[string]string{"01-first": {"reviewer-spec": "rev-spec"}},
		ReviewTips:    map[string]map[string]string{"01-first": {"reviewer-spec": prompted}},
		ReviewPending: map[string]map[string][]string{"01-first": {"reviewer-spec": {prompted}}},
		Reviews:       map[string]map[string]storedReview{},
		Rows:          map[string]savedRow{"01-first": {State: "in-review", Tip: cur}},
	}
	d.mu.Unlock()
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	d.handleReviewerTurnEnd("rev-spec")
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
		if strings.Contains(sessTranscript(getSession(t, srv.URL, "rev-spec")), cur) {
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
// prompted, then T2 prompted, the first reply still lands at T1 stale.
func TestReviewPassAfterRepromptBindsAnsweredTip(t *testing.T) {
	repo, _ := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	branch := "implement/01-first"
	worktree := filepath.Join(repo, ".el-worktrees", "01-first")
	testGit(t, repo, "worktree", "add", "-b", branch, worktree, "HEAD")
	t.Cleanup(func() { testGit(t, repo, "worktree", "remove", "--force", worktree) })
	t1 := "1111111111111111111111111111111111111111"
	t2 := "2222222222222222222222222222222222222222"
	cur := strings.TrimSpace(testGit(t, repo, "rev-parse", branch))
	fb := NewFakeBackend()
	fb.Delay = 30 * time.Second
	d := NewDaemon(tempDir(t), fb)
	prompt := reviewerPrompt("01-first", "spec", "reviewer-spec", branch, t2)
	d.mu.Lock()
	d.sessions["rev-spec"] = &session{
		meta: SessionMeta{ID: "rev-spec", Harness: "fake", Model: "m", Status: "idle", Owner: "owner-1"},
		live: true, notify: make(chan struct{}),
		events: []Event{
			{Seq: 1, SessionID: "rev-spec", Kind: "prompt", Text: prompt},
			{Seq: 2, SessionID: "rev-spec", Kind: "assistant_chunk", Text: "VERDICT: pass"},
		},
	}
	d.runs["r-tip"] = &runState{
		ID: "r-tip", Spec: "spec", Owner: "owner-1", Repo: repo, Status: "running",
		Harness: "fake", Model: "fake-model",
		notify:        make(chan struct{}),
		unpause:       make(chan struct{}),
		Tickets:       []staticTicket{{ID: "01-first", Num: 1, Title: "First"}},
		Branches:      map[string]string{"01-first": branch},
		Worktrees:     map[string]string{"01-first": worktree},
		Reviewers:     map[string]map[string]string{"01-first": {"reviewer-spec": "rev-spec"}},
		ReviewTips:    map[string]map[string]string{"01-first": {"reviewer-spec": cur}},
		ReviewPending: map[string]map[string][]string{"01-first": {"reviewer-spec": {t1, t2}}},
		Reviews:       map[string]map[string]storedReview{},
		Rows:          map[string]savedRow{"01-first": {State: "in-review", Tip: cur}},
	}
	d.mu.Unlock()
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	d.handleReviewerTurnEnd("rev-spec")
	r := getRun(t, srv.URL, "r-tip")
	got := ticketByID(r, "01-first").Verdicts["reviewer-spec"]
	if got.Verdict != "pass" || got.At != t1 {
		t.Fatalf("verdict = %+v, want pass at the answered prompt %q (not the latest %q)", got, t1, t2)
	}
	if got.Fresh {
		t.Fatalf("verdict = %+v, want stale", got)
	}
}
