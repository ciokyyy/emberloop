package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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
		time.Sleep(20 * time.Millisecond)
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
	t.Parallel()
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
	d := runTestDaemon(t, tempDir(t), fb)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	worker := ticketByID(r, "01-first").WorkerSession
	commitTicketWork(t, repo, "01-first")
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
		// Fresh needs the driver's row tip to catch up with the branch tip,
		// which lands on a later pass than the verdict itself.
		return tk.Verdicts["reviewer-spec"].Verdict == "pass" && tk.Verdicts["reviewer-spec"].Fresh && tk.State != "merged"
	}, "spec pass fresh without merging")
	tk := ticketByID(r, "01-first")
	if tk.Tip == "" {
		t.Fatalf("tip empty, want the worker branch tip")
	}
	if tk.LastRefusal != "" {
		t.Fatalf("lastRefusal = %q, want empty while a pass is still missing", tk.LastRefusal)
	}
}

// A changes verdict is delivered to the same Worker unchanged; the rework
// loop keeps the Reviewer sessions and merges once both pass at the new tip.
// The spec reviewer reports a blocker; the standards reviewer passes with a
// minor so the Worker receives both blocking and non-blocking findings.
func TestReviewFindingsReachWorkerAndReworkMerges(t *testing.T) {
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
		if strings.Contains(text, "reviewer-standards") {
			return "VERDICT: pass\nstyle.go:2 minor: align", true
		}
		return "VERDICT: pass", true
	}
	// Slow driver: this reviewer rework sequence races a fast driver the
	// way pause/switch tests do.
	d := NewDaemon(tempDir(t), fb)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	worker := ticketByID(r, "01-first").WorkerSession
	commitTicketWork(t, repo, "01-first")
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
	if transcript := sessTranscript(sess); !strings.Contains(transcript, "handle nil") || !strings.Contains(transcript, "r1-reviewer-spec-1") || !strings.Contains(transcript, `"blocking": true`) || !strings.Contains(transcript, "r1-reviewer-standards-1") || !strings.Contains(transcript, `"blocking": false`) {
		t.Fatalf("worker transcript misses the full typed finding list:\n%s", transcript)
	}

	worktree := filepath.Join(repo, ".el-worktrees", "01-first")
	writeFile(t, worktree, "work.go", "package main\n")
	testGit(t, worktree, "add", "work.go")
	testGit(t, worktree, "commit", "-m", "fix nil")
	if code, out := submitWorkerWithDispositions(t, srv.URL, worker, nil); code != http.StatusConflict || !strings.Contains(strings.ToLower(fmt.Sprint(out["error"])), "missing disposition") {
		t.Fatalf("done without the blocker disposition = %d (%v), want 409 with the missing disposition", code, out)
	}
	if code, out := submitWorkerWithDispositions(t, srv.URL, worker, []ReviewDisposition{{FindingID: "r1-reviewer-spec-1", Disposition: "declined"}}); code != http.StatusConflict || !strings.Contains(strings.ToLower(fmt.Sprint(out["error"])), "reason") {
		t.Fatalf("declined without a reason = %d (%v), want 409 requiring a reason", code, out)
	}
	if code, out := submitWorkerWithDispositions(t, srv.URL, worker, []ReviewDisposition{{FindingID: "r1-reviewer-spec-1", Disposition: "deferred"}}); code != http.StatusConflict || !strings.Contains(strings.ToLower(fmt.Sprint(out["error"])), "ticket") {
		t.Fatalf("deferred without a Ticket = %d (%v), want 409 requiring a Ticket number", code, out)
	}
	if code, out := submitWorkerWithDispositions(t, srv.URL, worker, []ReviewDisposition{{FindingID: "r1-reviewer-spec-1", Disposition: "deferred", Ticket: "11"}}); code != http.StatusAccepted {
		t.Fatalf("done with the blocker deferred to Ticket 11 = %d (%v), want 202", code, out)
	}
	storedRound := extTicketByID(getExtRun(t, srv.URL, run.ID), "01-first").Rounds[0]
	if len(storedRound.SentFindings) != 2 {
		t.Fatalf("stored findings = %+v, want blocker and minor", storedRound.SentFindings)
	}
	var blocker, minor *workerReviewFinding
	for i := range storedRound.SentFindings {
		finding := &storedRound.SentFindings[i]
		if finding.FindingID == "r1-reviewer-spec-1" {
			blocker = finding
		}
		if finding.FindingID == "r1-reviewer-standards-1" {
			minor = finding
		}
	}
	if blocker == nil || blocker.Disposition == nil || blocker.Disposition.Ticket != "11" || minor == nil || minor.Blocking || minor.Disposition != nil {
		t.Fatalf("stored findings = %+v, want deferred blocker and optional undisposed minor", storedRound.SentFindings)
	}
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
	t.Parallel()
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
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	d.SetSettingsPath(settings)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "request-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	commitTicketWork(t, repo, "01-first")
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
	t.Parallel()
	repo, _ := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	branch := "implement/01-first"
	worktree := filepath.Join(repo, ".el-worktrees", "01-first")
	testGit(t, repo, "worktree", "add", "-b", branch, worktree, "HEAD")
	t.Cleanup(func() { testGit(t, repo, "worktree", "remove", "--force", worktree) })
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
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
	t.Parallel()
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
	d := runTestDaemon(t, tempDir(t), fb)
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
	t.Parallel()
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
	d := runTestDaemon(t, tempDir(t), fb)
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
		time.Sleep(20 * time.Millisecond)
	}
}

// A reviewer pass after the Worker moved the tip under review binds to the
// tip of the prompt it answers, not the latest prompted tip: with T1
// prompted, then T2 prompted, the first reply still lands at T1 stale.
func TestReviewPassAfterRepromptBindsAnsweredTip(t *testing.T) {
	t.Parallel()
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
	d := runTestDaemon(t, tempDir(t), fb)
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

// tipFromReviewPrompt extracts the tip SHA from a reviewer gate prompt ("on
// branch <branch> at tip <tip>"), so scripted tool submits judge the prompted
// tip. Empty when the prompt carries none.
func tipFromReviewPrompt(text string) string {
	idx := strings.Index(text, "at tip ")
	if idx < 0 {
		return ""
	}
	rest := text[idx+len("at tip "):]
	if end := strings.IndexAny(rest, " \n.,"); end >= 0 {
		return rest[:end]
	}
	return strings.TrimSpace(rest)
}

// submitReview posts one reviewer `submit` tool call and returns the status
// and decoded body, so tool-error cases assert the refusal directly.
func submitReview(t *testing.T, base, sessionID string, payload map[string]any) (int, map[string]any) {
	t.Helper()
	body, _ := json.Marshal(payload)
	resp, err := http.Post(base+"/api/sessions/"+sessionID+"/submit", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out == nil {
		out = map[string]any{}
	}
	return resp.StatusCode, out
}

// submitWorkerWithDispositions posts a Run Worker's done payload and returns
// the gate response so tests can assert typed Disposition refusals.
func submitWorkerWithDispositions(t *testing.T, base, sessionID string, dispositions []ReviewDisposition) (int, map[string]any) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"status": "done", "completed": []string{"alpha"}, "dispositions": dispositions})
	resp, err := http.Post(base+"/api/sessions/"+sessionID+"/submit", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// A scripted reviewer `submit` through the fake Backend records both verdicts
// and merges: the tool path needs no VERDICT text. A pass may carry minors
// without blocking.
func TestReviewerSubmitToolScriptMergesWithMinors(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	fb := NewFakeBackend()
	fb.ReviewSubmit = func(sessionID, prompt string) (ReviewerSubmit, bool) {
		tip := tipFromReviewPrompt(prompt)
		if tip == "" {
			return ReviewerSubmit{}, false
		}
		return ReviewerSubmit{
			Verdict: "pass",
			Tip:     tip,
			Ran:     []ReviewRan{{Command: "go vet ./...", ExitCode: 0}},
			Findings: []ReviewFinding{
				{File: "work.go", Line: 1, Severity: "minor", Problem: "rename var"},
			},
		}, true
	}
	d := runTestDaemon(t, tempDir(t), fb)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	commitTicketWork(t, repo, "01-first")
	submitSession(t, srv.URL, ticketByID(r, "01-first").WorkerSession, "done", []string{"alpha"})
	waitReviewers(t, srv.URL, run.ID, "01-first")
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return r.Status == "done"
	}, "run done after tool passes with minors")
	tk := ticketByID(r, "01-first")
	if tk.Verdicts["reviewer-spec"].Verdict != "pass" || tk.Verdicts["reviewer-standards"].Verdict != "pass" {
		t.Fatalf("verdicts = %+v, want both pass via the tool", tk.Verdicts)
	}
}

// A `pass` with empty `ran` is refused with a tool error and records no
// verdict: the ticket stays under review without a pass.
func TestReviewerSubmitPassEmptyRanRefused(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	fb := NewFakeBackend()
	fb.Delay = 30 * time.Second
	d := runTestDaemon(t, tempDir(t), fb)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	commitTicketWork(t, repo, "01-first")
	submitSession(t, srv.URL, ticketByID(r, "01-first").WorkerSession, "done", []string{"alpha"})
	revs := waitReviewers(t, srv.URL, run.ID, "01-first")
	r = getRun(t, srv.URL, run.ID)
	// The branch tip straight from git: the driver's row tip can lag it, and
	// a verdict submitted at a lagged tip would record stale.
	tip := strings.TrimSpace(testGit(t, repo, "rev-parse", "implement/01-first"))
	if tip == "" {
		t.Fatalf("tip empty, want the worker branch tip")
	}
	code, out := submitReview(t, srv.URL, revs["reviewer-spec"], map[string]any{
		"verdict": "pass", "tip": tip, "ran": []any{}, "findings": []any{},
	})
	if code != http.StatusBadRequest {
		t.Fatalf("pass without ran status = %d (%v), want 400", code, out)
	}
	if msg, _ := out["error"].(string); !strings.Contains(strings.ToLower(msg), "ran") {
		t.Fatalf("pass without ran error = %q, want it to name ran", msg)
	}
	r = getRun(t, srv.URL, run.ID)
	if v := ticketByID(r, "01-first").Verdicts["reviewer-spec"]; v.Verdict != "" {
		t.Fatalf("verdict = %+v, want none after the refused submit", v)
	}
}

// A `changes` with only minors is refused with a tool error and records no
// verdict: nits never block through the tool.
func TestReviewerSubmitChangesMinorsOnlyRefused(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	fb := NewFakeBackend()
	fb.Delay = 30 * time.Second
	d := runTestDaemon(t, tempDir(t), fb)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	commitTicketWork(t, repo, "01-first")
	submitSession(t, srv.URL, ticketByID(r, "01-first").WorkerSession, "done", []string{"alpha"})
	revs := waitReviewers(t, srv.URL, run.ID, "01-first")
	r = getRun(t, srv.URL, run.ID)
	// The branch tip straight from git: the driver's row tip can lag it, and
	// a verdict submitted at a lagged tip would record stale.
	tip := strings.TrimSpace(testGit(t, repo, "rev-parse", "implement/01-first"))
	if tip == "" {
		t.Fatalf("tip empty, want the worker branch tip")
	}
	code, out := submitReview(t, srv.URL, revs["reviewer-spec"], map[string]any{
		"verdict": "changes", "tip": tip,
		"ran":      []any{map[string]any{"command": "go vet ./...", "exitCode": 0}},
		"findings": []any{map[string]any{"file": "work.go", "line": 1, "severity": "minor", "problem": "rename var"}},
	})
	if code != http.StatusBadRequest {
		t.Fatalf("changes with minors status = %d (%v), want 400", code, out)
	}
	if msg, _ := out["error"].(string); !strings.Contains(strings.ToLower(msg), "blocker") {
		t.Fatalf("changes with minors error = %q, want it to name blocker or major", msg)
	}
	r = getRun(t, srv.URL, run.ID)
	if v := ticketByID(r, "01-first").Verdicts["reviewer-spec"]; v.Verdict != "" {
		t.Fatalf("verdict = %+v, want none after the refused submit", v)
	}
}

// A verdict whose tip is behind the branch tip records stale and does not
// count: the verdict lands at its tip, reads not fresh, and the ticket does
// not merge on it.
func TestReviewerSubmitStaleTipRecordedNotCounted(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	fb := NewFakeBackend()
	fb.Delay = 30 * time.Second
	d := runTestDaemon(t, tempDir(t), fb)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	commitTicketWork(t, repo, "01-first")
	submitSession(t, srv.URL, ticketByID(r, "01-first").WorkerSession, "done", []string{"alpha"})
	revs := waitReviewers(t, srv.URL, run.ID, "01-first")
	r = getRun(t, srv.URL, run.ID)
	// The branch tip straight from git: the driver's row tip can lag it, and
	// the stale check compares against the live branch tip.
	tip := strings.TrimSpace(testGit(t, repo, "rev-parse", "implement/01-first"))
	if tip == "" {
		t.Fatalf("tip empty, want the worker branch tip")
	}
	stale := "1111111111111111111111111111111111111111"
	if stale == tip {
		t.Fatalf("stale tip %q equals the branch tip, want them apart", stale)
	}
	code, out := submitReview(t, srv.URL, revs["reviewer-spec"], map[string]any{
		"verdict": "pass", "tip": stale,
		"ran":      []any{map[string]any{"command": "go vet ./...", "exitCode": 0}},
		"findings": []any{},
	})
	if code != http.StatusAccepted {
		t.Fatalf("stale submit status = %d (%v), want 202", code, out)
	}
	if staleOut, _ := out["stale"].(bool); !staleOut {
		t.Fatalf("stale submit response = %v, want stale true", out)
	}
	if cur, _ := out["currentTip"].(string); cur != tip {
		t.Fatalf("stale submit currentTip = %q, want the branch tip %q", cur, tip)
	}
	r = getRun(t, srv.URL, run.ID)
	got := ticketByID(r, "01-first").Verdicts["reviewer-spec"]
	if got.Verdict != "pass" || got.At != stale {
		t.Fatalf("verdict = %+v, want pass recorded at the stale tip %q", got, stale)
	}
	if got.Fresh {
		t.Fatalf("verdict = %+v, want stale against the branch tip %q", got, tip)
	}
	if tk := ticketByID(r, "01-first"); tk.State == "merged" {
		t.Fatalf("ticket merged on a stale verdict, want it held")
	}
}

// A `pass` with minors through the tool does not block: both reviewers pass
// with minors and `ran`, and the ticket merges.
func TestReviewerSubmitPassWithMinorsMerges(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	fb := NewFakeBackend()
	fb.ReplyFunc = func(string, string) (string, bool) { return "", true }
	d := runTestDaemon(t, tempDir(t), fb)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	worker := ticketByID(r, "01-first").WorkerSession
	commitTicketWork(t, repo, "01-first")
	submitSession(t, srv.URL, worker, "done", []string{"alpha"})
	revs := waitReviewers(t, srv.URL, run.ID, "01-first")
	r = getRun(t, srv.URL, run.ID)
	// The branch tip straight from git: the driver's row tip can lag it, and
	// a verdict submitted at a lagged tip would record stale.
	tip := strings.TrimSpace(testGit(t, repo, "rev-parse", "implement/01-first"))
	if tip == "" {
		t.Fatalf("tip empty, want the worker branch tip")
	}
	for _, typ := range []string{"reviewer-spec", "reviewer-standards"} {
		code, out := submitReview(t, srv.URL, revs[typ], map[string]any{
			"verdict": "pass", "tip": tip,
			"ran":      []any{map[string]any{"command": "go test ./...", "exitCode": 0}},
			"findings": []any{map[string]any{"file": "work.go", "line": 2, "severity": "minor", "problem": "nit"}},
		})
		if code != http.StatusAccepted {
			t.Fatalf("%s pass with minors status = %d (%v), want 202", typ, code, out)
		}
	}
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return r.Status == "done"
	}, "run done after tool passes with minors")
	if tk := ticketByID(r, "01-first"); tk.State != "merged" {
		t.Fatalf("ticket state = %q, want merged", tk.State)
	}
	transcript := sessTranscript(getSession(t, srv.URL, worker))
	if !strings.Contains(transcript, `"blocking": false`) || !strings.Contains(transcript, `"problem": "nit"`) {
		t.Fatalf("non-blocking pass findings did not reach the worker:\n%s", transcript)
	}
	round := extTicketByID(getExtRun(t, srv.URL, run.ID), "01-first").Rounds[0]
	if len(round.SentFindings) != 2 || round.SentToWorkerAt == nil {
		t.Fatalf("sent findings = %+v, want both minor findings recorded before the non-blocking merge", round)
	}
	for _, finding := range round.SentFindings {
		if finding.Blocking || finding.Disposition != nil {
			t.Fatalf("minor finding = %+v, want non-blocking with optional disposition absent", finding)
		}
	}
	if code, out := submitWorkerWithDispositions(t, srv.URL, worker, []ReviewDisposition{
		{FindingID: "r1-reviewer-standards-1", Disposition: "declined", Reason: "style is consistent"},
		{FindingID: "r1-reviewer-spec-1", Disposition: "deferred", Ticket: "12"},
	}); code != http.StatusAccepted {
		t.Fatalf("optional minor dispositions after merge = %d (%v), want 202", code, out)
	}
	round = extTicketByID(getExtRun(t, srv.URL, run.ID), "01-first").Rounds[0]
	if round.SentFindings[0].Disposition == nil || round.SentFindings[0].Disposition.Reason != "style is consistent" || round.SentFindings[1].Disposition == nil || round.SentFindings[1].Disposition.Ticket != "12" {
		t.Fatalf("minor dispositions = %+v, want declined reason and deferred Ticket on the recorded findings", round.SentFindings)
	}
}

// The submit route checks the Session before validating the payload: an
// unknown Session answers 404 even with an invalid reviewer payload. Bad
// JSON and a worker submit without a status keep their own messages.
func TestSessionSubmitRoutesAndMessages(t *testing.T) {
	t.Parallel()
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	// Unknown session with an invalid reviewer payload still 404s: the
	// reviewer check runs before validation (which would 400 on the empty tip).
	if code, _ := submitReview(t, srv.URL, "missing-session", map[string]any{
		"verdict": "pass", "tip": "", "ran": []any{}, "findings": []any{},
	}); code != http.StatusNotFound {
		t.Fatalf("unknown reviewer submit status = %d, want 404", code)
	}

	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	post := func(body string) (int, string) {
		t.Helper()
		resp, err := http.Post(srv.URL+"/api/sessions/"+id+"/submit", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(raw)
	}
	if code, body := post(`{bad`); code != http.StatusBadRequest || !strings.Contains(body, "bad json") {
		t.Fatalf("bad json submit = %d %q, want 400 with the decode message", code, body)
	}
	if code, body := post(`{"report":"x"}`); code != http.StatusBadRequest || !strings.Contains(body, "status is required") {
		t.Fatalf("worker submit without status = %d %q, want 400 with the status message", code, body)
	}
}

// A patch-identical rebase reuses a `changes` verdict with no Reviewer turn:
// the amended tip carries new SHAs but the same patch id, so the changes and
// the pass move to the new tip, the refusal is told again, and the ticket
// stays unmerged. (A second done at the very same tip never reaches the
// reviewers: the worker submit gate refuses it as an empty resubmit.)
func TestReviewPatchIdenticalRebaseReusesChanges(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	fb := NewFakeBackend()
	fb.ReviewSubmit = func(sessionID, prompt string) (ReviewerSubmit, bool) {
		tip := tipFromReviewPrompt(prompt)
		if tip == "" {
			return ReviewerSubmit{}, false
		}
		ran := []ReviewRan{{Command: "go vet ./...", ExitCode: 0}}
		if strings.Contains(prompt, "reviewer-spec") {
			return ReviewerSubmit{
				Verdict: "changes", Tip: tip, Ran: ran,
				Findings: []ReviewFinding{{File: "work.go", Line: 1, Severity: "major", Problem: "handle nil"}},
			}, true
		}
		return ReviewerSubmit{Verdict: "pass", Tip: tip, Ran: ran}, true
	}
	d := runTestDaemon(t, tempDir(t), fb)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	worker := ticketByID(r, "01-first").WorkerSession
	worktree := filepath.Join(repo, ".el-worktrees", "01-first")
	writeFile(t, worktree, "work.go", "package main\n")
	testGit(t, worktree, "add", "work.go")
	testGit(t, worktree, "commit", "-m", "work")
	submitSession(t, srv.URL, worker, "done", []string{"alpha"})
	revs := waitReviewers(t, srv.URL, run.ID, "01-first")

	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		tk := ticketByID(r, "01-first")
		return tk.LastRefusal != "" && strings.Contains(tk.LastRefusal, "handle nil")
	}, "changes refusal told")
	t1 := ticketByID(r, "01-first").Tip
	if t1 == "" {
		t.Fatalf("tip empty, want the worker branch tip")
	}

	// Amend the message: new SHAs, identical diff. The new tip keeps the
	// done submit past the empty-resubmit gate, and the unchanged patch id
	// reuses both verdicts with no Reviewer turn.
	testGit(t, worktree, "commit", "--amend", "-m", "work rebased")
	t2 := strings.TrimSpace(testGit(t, repo, "rev-parse", "implement/01-first"))
	if t2 == "" || t1 == t2 {
		t.Fatalf("tips = %q and %q, want two different SHAs", t1, t2)
	}
	if code, out := submitWorkerWithDispositions(t, srv.URL, worker, []ReviewDisposition{{FindingID: "r1-reviewer-spec-1", Disposition: "fixed"}}); code != http.StatusAccepted {
		t.Fatalf("done with blocker disposition after identical rebase = %d (%v), want 202", code, out)
	}

	deadline := time.Now().Add(15 * time.Second)
	for {
		tk := extTicketByID(getExtRun(t, srv.URL, run.ID), "01-first")
		if len(tk.Rounds) >= 2 {
			rd := tk.Rounds[1]
			if rd.Tip == t2 && len(rd.Reviews) == 2 {
				for _, typ := range []string{"reviewer-spec", "reviewer-standards"} {
					rv, ok := rd.Reviews[typ]
					if !ok || rv.HeldFromRound == nil || *rv.HeldFromRound != 1 || rv.At != t2 {
						t.Fatalf("round 2 review %s = %+v, want verdict held from round 1 at %q", typ, rv, t2)
					}
				}
				if got := rd.Reviews["reviewer-spec"].Verdict; got != "changes" {
					t.Fatalf("round 2 spec verdict = %q, want the reused changes", got)
				}
				if got := rd.Reviews["reviewer-standards"].Verdict; got != "pass" {
					t.Fatalf("round 2 standards verdict = %q, want the reused pass", got)
				}
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("no reused second round: %+v", extTicketByID(getExtRun(t, srv.URL, run.ID), "01-first").Rounds)
		}
		time.Sleep(20 * time.Millisecond)
	}
	// No second Reviewer turn went out: one gate prompt per reviewer, and
	// the same persistent Sessions. The reused changes refusal still stands.
	for _, sess := range revs {
		if n := strings.Count(sessTranscript(getSession(t, srv.URL, sess)), "Review ticket"); n != 1 {
			t.Fatalf("reviewer got %d gate prompts, want one (reused, not rerun)", n)
		}
	}
	for typ, sess := range waitReviewers(t, srv.URL, run.ID, "01-first") {
		if sess != revs[typ] {
			t.Fatalf("reviewer %s session changed, want the persistent one", typ)
		}
	}
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		tk := ticketByID(r, "01-first")
		return tk.LastRefusal != "" && strings.Contains(tk.LastRefusal, "handle nil")
	}, "reused changes refusal told again")
	if tk := ticketByID(r, "01-first"); tk.State == "merged" {
		t.Fatalf("ticket merged on reused changes, want it kept")
	}
}

// A rebase that keeps the diff identical reuses both verdicts with no
// Reviewer turn: the amended tip carries new SHAs but the same patch id, so
// the passes move to the new tip and the ticket merges without re-review.
func TestReviewPatchIdenticalRebaseReusesVerdict(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	fb := NewFakeBackend()
	fb.ReviewSubmit = func(sessionID, prompt string) (ReviewerSubmit, bool) {
		tip := tipFromReviewPrompt(prompt)
		if tip == "" {
			return ReviewerSubmit{}, false
		}
		return ReviewerSubmit{
			Verdict: "pass", Tip: tip,
			Ran: []ReviewRan{{Command: "go vet ./...", ExitCode: 0}},
		}, true
	}
	d := runTestDaemon(t, tempDir(t), fb)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	worker := ticketByID(r, "01-first").WorkerSession
	worktree := filepath.Join(repo, ".el-worktrees", "01-first")
	writeFile(t, worktree, "work.go", "package main\n")
	testGit(t, worktree, "add", "work.go")
	testGit(t, worktree, "commit", "-m", "work v1")
	submitSession(t, srv.URL, worker, "done", []string{"alpha"})
	revs := waitReviewers(t, srv.URL, run.ID, "01-first")
	// Block the merge with a dirty worktree before the verdicts land, so the
	// passed tip stays to be rebased.
	writeFile(t, worktree, "dirty.txt", "uncommitted\n")
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		tk := ticketByID(r, "01-first")
		return tk.Verdicts["reviewer-spec"].Verdict == "pass" &&
			tk.Verdicts["reviewer-standards"].Verdict == "pass" &&
			tk.LastRefusal != ""
	}, "both pass at the tip with the merge blocked")
	t1 := ticketByID(r, "01-first").Tip
	// Clean the tree and amend the message: new SHAs, identical diff.
	if err := os.Remove(filepath.Join(worktree, "dirty.txt")); err != nil {
		t.Fatal(err)
	}
	testGit(t, worktree, "commit", "--amend", "-m", "work v1 rebased")
	t2 := strings.TrimSpace(testGit(t, repo, "rev-parse", "implement/01-first"))
	if t1 == "" || t2 == "" || t1 == t2 {
		t.Fatalf("tips = %q and %q, want two different SHAs", t1, t2)
	}
	submitSession(t, srv.URL, worker, "done", []string{"alpha"})

	// The passes carry to the rebased tip with no new Reviewer turn, and the
	// ticket merges on them.
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return r.Status == "done"
	}, "run done after the patch-identical rebase")
	tk := ticketByID(r, "01-first")
	if tk.State != "merged" {
		t.Fatalf("ticket state = %q, want merged on the reused passes", tk.State)
	}
	// The gate merged on the carried passes, so they read fresh at finish
	// time; the merge's own rebase moves the branch after, so freshness is
	// asserted on the round instead of the post-merge snapshot.
	for _, v := range []reviewVerdictJSON{tk.Verdicts["reviewer-spec"], tk.Verdicts["reviewer-standards"]} {
		if v.Verdict != "pass" || v.At != t2 {
			t.Fatalf("verdict = %+v, want pass carried to %q", v, t2)
		}
	}
	ext := extTicketByID(getExtRun(t, srv.URL, run.ID), "01-first")
	if len(ext.Rounds) < 2 {
		t.Fatalf("rounds = %+v, want a second round carrying the reuse", ext.Rounds)
	}
	rd := ext.Rounds[1]
	if rd.Tip != t2 {
		t.Fatalf("round 2 tip = %q, want the rebased tip %q", rd.Tip, t2)
	}
	for _, typ := range []string{"reviewer-spec", "reviewer-standards"} {
		rv, ok := rd.Reviews[typ]
		if !ok || rv.Verdict != "pass" || rv.HeldFromRound == nil || *rv.HeldFromRound != 1 || rv.At != t2 {
			t.Fatalf("round 2 review %s = %+v, want pass held from round 1 at %q", typ, rv, t2)
		}
	}
	for _, sess := range revs {
		if n := strings.Count(sessTranscript(getSession(t, srv.URL, sess)), "Review ticket"); n != 1 {
			t.Fatalf("reviewer got %d gate prompts, want one (reused, not rerun)", n)
		}
	}
	for typ, sess := range waitReviewers(t, srv.URL, run.ID, "01-first") {
		if sess != revs[typ] {
			t.Fatalf("reviewer %s session changed, want the persistent one", typ)
		}
	}
}

// A re-review with a changed diff is a new message in the same persistent
// Reviewer Session. It carries the prior declined finding and reason, while
// reminding the Reviewer not to re-raise it unless its cited lines changed.
func TestReviewRereviewUsesSameSession(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	fb := NewFakeBackend()
	var mu sync.Mutex
	specCalls := 0
	fb.ReviewSubmit = func(sessionID, prompt string) (ReviewerSubmit, bool) {
		tip := tipFromReviewPrompt(prompt)
		if tip == "" {
			return ReviewerSubmit{}, false
		}
		ran := []ReviewRan{{Command: "go vet ./...", ExitCode: 0}}
		if strings.Contains(prompt, "reviewer-spec") {
			mu.Lock()
			specCalls++
			n := specCalls
			mu.Unlock()
			if n == 1 {
				return ReviewerSubmit{
					Verdict: "changes", Tip: tip, Ran: ran,
					Findings: []ReviewFinding{{File: "work.go", Line: 1, Severity: "major", Problem: "handle nil"}},
				}, true
			}
		}
		return ReviewerSubmit{Verdict: "pass", Tip: tip, Ran: ran}, true
	}
	// Slow driver: this reviewer rereview sequence races a fast driver the
	// way pause/switch tests do.
	d := NewDaemon(tempDir(t), fb)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	worker := ticketByID(r, "01-first").WorkerSession
	worktree := filepath.Join(repo, ".el-worktrees", "01-first")
	writeFile(t, worktree, "work.go", "package main\n")
	testGit(t, worktree, "add", "work.go")
	testGit(t, worktree, "commit", "-m", "work v1")
	submitSession(t, srv.URL, worker, "done", []string{"alpha"})
	revs := waitReviewers(t, srv.URL, run.ID, "01-first")
	waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		tk := ticketByID(r, "01-first")
		return tk.LastRefusal != "" && strings.Contains(tk.LastRefusal, "handle nil")
	}, "changes refusal told")

	// Rework changes the diff, so the re-review must judge the new tip.
	writeFile(t, worktree, "work.go", "package main\n\nfunc f() {}\n")
	testGit(t, worktree, "add", "work.go")
	testGit(t, worktree, "commit", "-m", "work v2")
	if code, out := submitWorkerWithDispositions(t, srv.URL, worker, []ReviewDisposition{{FindingID: "r1-reviewer-spec-1", Disposition: "declined", Reason: "The cited nil path is unreachable from this caller."}}); code != http.StatusAccepted {
		t.Fatalf("done with blocker disposition after rework = %d (%v), want 202", code, out)
	}
	waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return r.Status == "done"
	}, "run done after the re-review passes")

	for typ, sess := range waitReviewers(t, srv.URL, run.ID, "01-first") {
		if sess != revs[typ] {
			t.Fatalf("reviewer %s session changed to %q, want the persistent %q", typ, sess, revs[typ])
		}
		transcript := sessTranscript(getSession(t, srv.URL, sess))
		if n := strings.Count(transcript, "Review ticket"); n != 2 {
			t.Fatalf("reviewer %s got %d gate prompts, want two in the same session", typ, n)
		}
		for _, want := range []string{
			"Previously declined findings and the Worker's reasons:",
			"Do not re-raise any declined finding unless its cited lines have changed since the finding was declined.",
			`"findingId": "r1-reviewer-spec-1"`,
			`"file": "work.go"`,
			`"line": 1`,
			`"problem": "handle nil"`,
			`"reason": "The cited nil path is unreachable from this caller."`,
		} {
			if !strings.Contains(transcript, want) {
				t.Errorf("reviewer %s transcript lacks declined-feedback text %q:\n%s", typ, want, transcript)
			}
		}
	}
}

// A Reviewer turn that ended without a recorded verdict at the current tip
// counts as no verdict: the silent seat records nothing, the ticket never
// merges on it, and after the re-prompt fails too the ticket needs the user.
func TestReviewMissingVerdictBlocksMerge(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	fb := NewFakeBackend()
	fb.ReplyFunc = func(id, text string) (string, bool) {
		if strings.Contains(text, "reviewer-spec") || strings.Contains(text, "RESPOND IN FORMAT") {
			return "still reviewing, no verdict yet", true
		}
		return "", false
	}
	d := runTestDaemon(t, tempDir(t), fb)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	commitTicketWork(t, repo, "01-first")
	submitSession(t, srv.URL, ticketByID(r, "01-first").WorkerSession, "done", []string{"alpha"})
	waitReviewers(t, srv.URL, run.ID, "01-first")

	deadline := time.Now().Add(15 * time.Second)
	for {
		tk := extTicketByID(getExtRun(t, srv.URL, run.ID), "01-first")
		if tk.NeedsYou != "" {
			if !strings.Contains(tk.NeedsYou, "reviewer-spec") {
				t.Fatalf("needs-you = %q, want it to name the silent reviewer", tk.NeedsYou)
			}
			if got := tk.Verdicts["reviewer-spec"]; got.Verdict != "" {
				t.Fatalf("spec verdict = %+v, want none after two verdict-less turns", got)
			}
			if tk.State == "merged" {
				t.Fatalf("ticket merged with a missing verdict, want it blocked")
			}
			if got := tk.Verdicts["reviewer-standards"]; got.Verdict != "pass" {
				t.Fatalf("standards verdict = %+v, want the judging seat to pass", got)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("no needs-you for the silent reviewer: %+v", extTicketByID(getExtRun(t, srv.URL, run.ID), "01-first"))
		}
		time.Sleep(20 * time.Millisecond)
	}
}
