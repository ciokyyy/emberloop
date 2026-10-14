package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Stop-after-submit ends a Run Worker or Reviewer turn as soon as its
// terminal submit is accepted. These tests drive the seam through the fake
// Backend: an accepted terminal submit interrupts the running turn exactly
// once and the Session settles idle with the verdict recorded, while refused
// submits, opted-out Runs and non-Run Sessions never interrupt.

func waitWorkerSession(t *testing.T, base, runID, ticketID string) string {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		r := getRun(t, base, runID)
		if w := ticketByID(r, ticketID).WorkerSession; w != "" {
			return w
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for the worker of %s", ticketID)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func postWorkerDoneRaw(t *testing.T, base, id string, payload map[string]any) (int, map[string]any) {
	t.Helper()
	body, _ := json.Marshal(payload)
	resp, err := http.Post(base+"/api/sessions/"+id+"/submit", "application/json", bytes.NewReader(body))
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

func countPrompts(t *testing.T, base, id string) int {
	t.Helper()
	n := 0
	for _, e := range getSession(t, base, id).Events {
		if e.Kind == "prompt" {
			n++
		}
	}
	return n
}

func countSessionKind(t *testing.T, base, id, kind string) int {
	t.Helper()
	n := 0
	for _, e := range getSession(t, base, id).Events {
		if e.Kind == kind {
			n++
		}
	}
	return n
}

func startNoStopRun(t *testing.T, base, spec, repo string) runJSON {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"spec": spec, "repo": repo, "harness": "fake", "model": "fake-model",
		"noStopAfterSubmit": true,
	})
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

// An accepted Worker done submit stops the running turn at once: the
// Session goes idle long before the Backend's Delay runs out, the interrupt
// fires exactly once, the verdict is recorded, and no reminder, Chain or
// retry follows. A next prompt to the same Session still runs.
func TestStopAfterSubmitEndsWorkerTurn(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	fb := NewFakeBackend()
	fb.SetDelay(30 * time.Second)
	d := runTestDaemon(t, tempDir(t), fb)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	worker := waitWorkerSession(t, srv.URL, run.ID, "01-first")
	waitStatus(t, srv.URL, worker, "working", 15*time.Second)
	commitTicketWork(t, repo, "01-first")
	submitSession(t, srv.URL, worker, "done", []string{"alpha"})
	// Without the stop the turn would sleep the whole 30s Delay.
	waitStatus(t, srv.URL, worker, "idle", 15*time.Second)
	if n := fb.InterruptCount(worker); n != 1 {
		t.Fatalf("interrupts = %d, want exactly 1", n)
	}
	d.mu.Lock()
	rec := d.submits[worker]
	pendingSubmit := d.stopSubmit[worker]
	chainPending := len(d.runs[run.ID].ChainPendingSubmit)
	sess := d.runs[run.ID].Sessions["01-first"]
	d.mu.Unlock()
	if rec == nil || rec.Status != "done" {
		t.Fatalf("submit rec = %+v, want a recorded done", rec)
	}
	if pendingSubmit {
		t.Fatalf("stop still armed after the turn settled")
	}
	if chainPending != 0 {
		t.Fatalf("Chain pending submits = %d, want none", chainPending)
	}
	if sess != worker {
		t.Fatalf("worker session = %q, want no Chain fallback", sess)
	}
	if got := ticketByID(getRun(t, srv.URL, run.ID), "01-first").NeedsYou; got != "" {
		t.Fatalf("needs-you = %q, want none", got)
	}
	if n := countPrompts(t, srv.URL, worker); n != 1 {
		t.Fatalf("prompt events = %d, want only the task prompt (no reminder)", n)
	}
	if n := countSessionKind(t, srv.URL, worker, "error"); n != 0 {
		t.Fatalf("error events = %d, want none (idle, not failed)", n)
	}
	if st := getSession(t, srv.URL, worker).Meta.Status; st != "idle" {
		t.Fatalf("status = %q, want idle", st)
	}
	// A next prompt to the same Session still runs a full turn.
	fb.SetDelay(50 * time.Millisecond)
	sendPrompt(t, srv.URL, worker, "keep going")
	waitStatus(t, srv.URL, worker, "idle", 10*time.Second)
	if !strings.Contains(sessTranscript(getSession(t, srv.URL, worker)), "fake reply to: keep going") {
		t.Fatalf("worker transcript misses the follow-up reply")
	}
}

// An accepted Reviewer pass submit stops the running reviewer turn at once
// with the verdict recorded and no re-prompt: the tip is fresh, so no retell
// follows, and the reviewer stays usable for the next prompt.
func TestStopAfterSubmitReviewerPass(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	fb := NewFakeBackend()
	fb.SetDelay(30 * time.Second)
	d := runTestDaemon(t, tempDir(t), fb)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	worker := waitWorkerSession(t, srv.URL, run.ID, "01-first")
	waitStatus(t, srv.URL, worker, "working", 15*time.Second)
	commitTicketWork(t, repo, "01-first")
	submitSession(t, srv.URL, worker, "done", []string{"alpha"})
	waitStatus(t, srv.URL, worker, "idle", 15*time.Second)
	revs := waitReviewers(t, srv.URL, run.ID, "01-first")
	rev := revs["reviewer-spec"]
	waitStatus(t, srv.URL, rev, "working", 15*time.Second)
	branch := ticketByID(getRun(t, srv.URL, run.ID), "01-first").Branch
	tip := strings.TrimSpace(testGit(t, repo, "rev-parse", branch))
	code, _ := submitReview(t, srv.URL, rev, map[string]any{
		"verdict": "pass", "tip": tip,
		"ran": []map[string]any{{"command": "go vet ./...", "exitCode": 0}},
	})
	if code != http.StatusAccepted {
		t.Fatalf("submit status = %d, want 202", code)
	}
	waitStatus(t, srv.URL, rev, "idle", 15*time.Second)
	if n := fb.InterruptCount(rev); n != 1 {
		t.Fatalf("interrupts = %d, want exactly 1", n)
	}
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").Verdicts["reviewer-spec"].Verdict == "pass"
	}, "reviewer-spec pass recorded")
	if got := ticketByID(r, "01-first").ReviewerSessions["reviewer-spec"]; got != rev {
		t.Fatalf("reviewer session = %q, want no Chain fallback", got)
	}
	if got := ticketByID(r, "01-first").NeedsYou; got != "" {
		t.Fatalf("needs-you = %q, want none", got)
	}
	if n := countPrompts(t, srv.URL, rev); n != 1 {
		t.Fatalf("prompt events = %d, want only the gate prompt (no re-prompt)", n)
	}
	if st := getSession(t, srv.URL, rev).Meta.Status; st != "idle" {
		t.Fatalf("status = %q, want idle", st)
	}
	fb.SetDelay(50 * time.Millisecond)
	// Reviewer Sessions refuse user messages over HTTP; the Daemon drives
	// the follow-up the way a retell would.
	if err := d.runTellSession(rev, "one more look"); err != nil {
		t.Fatal(err)
	}
	followup := firstPromptWhere(t, srv.URL, rev, func(e Event) bool {
		return strings.Contains(e.Text, "one more look")
	})
	waitTurnEnd(t, srv.URL, rev, followup.Seq)
	if !strings.Contains(sessTranscript(getSession(t, srv.URL, rev)), "fake reply to: one more look") {
		t.Fatalf("reviewer transcript misses the follow-up reply")
	}
}

// A refused submit never stops its turn: the agent keeps working and a
// later accepted submit still stops it exactly once. Covers the Worker gate
// refusal and the Reviewer tool-error refusal in one run.
func TestStopAfterSubmitRefusedKeepsWorking(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	fb := NewFakeBackend()
	fb.SetDelay(30 * time.Second)
	d := runTestDaemon(t, tempDir(t), fb)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	worker := waitWorkerSession(t, srv.URL, run.ID, "01-first")
	waitStatus(t, srv.URL, worker, "working", 15*time.Second)
	code, body := postWorkerDoneRaw(t, srv.URL, worker, map[string]any{
		"status": "done", "completed": []string{"alpha"}, "report": "the work is not done yet",
	})
	if code != http.StatusConflict {
		t.Fatalf("refused submit status = %d, want 409", code)
	}
	if errText, _ := body["error"].(string); !strings.Contains(errText, "not done") {
		t.Fatalf("refused submit error = %v, want the not-done refusal", body)
	}
	if n := fb.InterruptCount(worker); n != 0 {
		t.Fatalf("interrupts after refusal = %d, want 0", n)
	}
	if st := getSession(t, srv.URL, worker).Meta.Status; st != "working" {
		t.Fatalf("status after refusal = %q, want working", st)
	}
	commitTicketWork(t, repo, "01-first")
	submitSession(t, srv.URL, worker, "done", []string{"alpha"})
	waitStatus(t, srv.URL, worker, "idle", 15*time.Second)
	if n := fb.InterruptCount(worker); n != 1 {
		t.Fatalf("interrupts after the accepted submit = %d, want 1", n)
	}

	revs := waitReviewers(t, srv.URL, run.ID, "01-first")
	rev := revs["reviewer-spec"]
	waitStatus(t, srv.URL, rev, "working", 15*time.Second)
	branch := ticketByID(getRun(t, srv.URL, run.ID), "01-first").Branch
	tip := strings.TrimSpace(testGit(t, repo, "rev-parse", branch))
	code, _ = submitReview(t, srv.URL, rev, map[string]any{"verdict": "pass", "tip": tip})
	if code != http.StatusBadRequest {
		t.Fatalf("empty-ran submit status = %d, want 400", code)
	}
	if n := fb.InterruptCount(rev); n != 0 {
		t.Fatalf("reviewer interrupts after refusal = %d, want 0", n)
	}
	if st := getSession(t, srv.URL, rev).Meta.Status; st != "working" {
		t.Fatalf("reviewer status after refusal = %q, want working", st)
	}
	code, _ = submitReview(t, srv.URL, rev, map[string]any{
		"verdict": "pass", "tip": tip,
		"ran": []map[string]any{{"command": "go vet ./...", "exitCode": 0}},
	})
	if code != http.StatusAccepted {
		t.Fatalf("submit status = %d, want 202", code)
	}
	waitStatus(t, srv.URL, rev, "idle", 15*time.Second)
	if n := fb.InterruptCount(rev); n != 1 {
		t.Fatalf("reviewer interrupts after the accepted submit = %d, want 1", n)
	}
}

// A Run started with noStopAfterSubmit keeps the old behavior: the accepted
// submit records and the turn runs to its natural end with no interrupt.
func TestStopAfterSubmitDisabled(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	fb := NewFakeBackend()
	fb.SetDelay(30 * time.Second)
	d := runTestDaemon(t, tempDir(t), fb)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startNoStopRun(t, srv.URL, spec, repo)
	worker := waitWorkerSession(t, srv.URL, run.ID, "01-first")
	waitStatus(t, srv.URL, worker, "working", 15*time.Second)
	commitTicketWork(t, repo, "01-first")
	submitSession(t, srv.URL, worker, "done", []string{"alpha"})
	time.Sleep(1500 * time.Millisecond)
	if n := fb.InterruptCount(worker); n != 0 {
		t.Fatalf("interrupts on the opted-out run = %d, want 0", n)
	}
	if st := getSession(t, srv.URL, worker).Meta.Status; st != "working" {
		t.Fatalf("status = %q, want the turn still working", st)
	}
	d.mu.Lock()
	optOut := d.runs[run.ID].NoStopAfterSubmit
	d.mu.Unlock()
	if !optOut {
		t.Fatalf("run noStopAfterSubmit = false, want the opt-out recorded")
	}
	resp, err := http.Get(srv.URL + "/api/runs/" + run.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Run map[string]any `json:"run"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Run["noStopAfterSubmit"] != true {
		t.Fatalf("run json noStopAfterSubmit = %v, want true", out.Run["noStopAfterSubmit"])
	}
	if code, _ := postInterrupt(t, srv.URL, worker); code != http.StatusOK {
		t.Fatalf("cleanup interrupt status = %d, want 200", code)
	}
	waitStatus(t, srv.URL, worker, "idle", 15*time.Second)
}

// A submit on a Session that is no Run Worker's or Reviewer's never stops
// its turn, even though the submit itself is accepted and recorded.
func TestStopAfterSubmitIgnoresNonRunSession(t *testing.T) {
	t.Parallel()
	fb := NewFakeBackend()
	fb.SetDelay(10 * time.Second)
	d := runTestDaemon(t, tempDir(t), fb)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	sendPrompt(t, srv.URL, id, "slow work please")
	waitStatus(t, srv.URL, id, "working", 10*time.Second)
	submitSession(t, srv.URL, id, "done", []string{"alpha"})
	time.Sleep(500 * time.Millisecond)
	if n := fb.InterruptCount(id); n != 0 {
		t.Fatalf("interrupts on a non-Run session = %d, want 0", n)
	}
	if st := getSession(t, srv.URL, id).Meta.Status; st != "working" {
		t.Fatalf("status = %q, want the turn still working", st)
	}
	if code, _ := postInterrupt(t, srv.URL, id); code != http.StatusOK {
		t.Fatalf("cleanup interrupt status = %d, want 200", code)
	}
	waitStatus(t, srv.URL, id, "idle", 15*time.Second)
}

// On a real harness the stop arms on the submit and fires only on the
// submit tool's terminal update: other tools completing first must not stop
// the turn, and a non-terminal submit must never arm it. The pig harness
// here runs on the fake Backend so the tool updates are scripted by hand.
func TestStopAfterSubmitArmsUntilToolDone(t *testing.T) {
	t.Parallel()
	fb := NewFakeBackend()
	fb.SetDelay(30 * time.Second)
	d := runTestDaemon(t, tempDir(t), fb)
	d.RegisterBackend("pig", fb)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	seedStopRun(t, d, "r-arm", map[string]string{"t1": "w1"})
	w1 := createSession(t, srv.URL, "pig", "fake-model", tempDir(t))
	remapStopRun(t, d, "r-arm", map[string]string{"t1": w1})
	sendPrompt(t, srv.URL, w1, "do the thing")
	waitStatus(t, srv.URL, w1, "working", 10*time.Second)
	submitSession(t, srv.URL, w1, "blocked", []string{"alpha"})
	time.Sleep(500 * time.Millisecond)
	if n := fb.InterruptCount(w1); n != 0 {
		t.Fatalf("interrupts before the tool result = %d, want 0 (armed, not fired)", n)
	}
	if st := getSession(t, srv.URL, w1).Meta.Status; st != "working" {
		t.Fatalf("status = %q, want the turn still working", st)
	}
	toolDone := func(name string) {
		t.Helper()
		d.appendEvent(w1, "tool_call_updated", ToolCall{ID: "call-" + name, Name: name, Status: "completed"}.Text(), "completed")
	}
	toolDone("bash")
	time.Sleep(200 * time.Millisecond)
	if n := fb.InterruptCount(w1); n != 0 {
		t.Fatalf("interrupts on another tool's update = %d, want 0", n)
	}
	toolDone("submit")
	waitStatus(t, srv.URL, w1, "idle", 15*time.Second)
	if n := fb.InterruptCount(w1); n != 1 {
		t.Fatalf("interrupts on the submit tool update = %d, want 1", n)
	}
	d.mu.Lock()
	rec := d.submits[w1]
	d.mu.Unlock()
	if rec == nil || rec.Status != "blocked" {
		t.Fatalf("submit rec = %+v, want the recorded blocked", rec)
	}
	if n := countPrompts(t, srv.URL, w1); n != 1 {
		t.Fatalf("prompt events = %d, want only the task prompt", n)
	}
	// Settling disarms: a later submit tool update is a no-op.
	toolDone("submit")
	if n := fb.InterruptCount(w1); n != 1 {
		t.Fatalf("interrupts after settle = %d, want still 1", n)
	}
	// A next prompt to the same Session still runs a full turn.
	fb.SetDelay(50 * time.Millisecond)
	sendPrompt(t, srv.URL, w1, "continue")
	waitStatus(t, srv.URL, w1, "idle", 10*time.Second)
	if !strings.Contains(sessTranscript(getSession(t, srv.URL, w1)), "fake reply to: continue") {
		t.Fatalf("transcript misses the follow-up reply")
	}
}

// A non-terminal Worker submit never arms the stop, even when a submit tool
// update lands later in the same turn.
func TestStopAfterSubmitIgnoresNonTerminal(t *testing.T) {
	t.Parallel()
	fb := NewFakeBackend()
	fb.SetDelay(30 * time.Second)
	d := runTestDaemon(t, tempDir(t), fb)
	d.RegisterBackend("pig", fb)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	seedStopRun(t, d, "r-nonterm", map[string]string{"t1": "w1"})
	w1 := createSession(t, srv.URL, "pig", "fake-model", tempDir(t))
	remapStopRun(t, d, "r-nonterm", map[string]string{"t1": w1})
	sendPrompt(t, srv.URL, w1, "do the thing")
	waitStatus(t, srv.URL, w1, "working", 10*time.Second)
	body, _ := json.Marshal(map[string]any{"status": "progress", "completed": []string{"alpha"}})
	resp, err := http.Post(srv.URL+"/api/sessions/"+w1+"/submit", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("submit status = %d, want 202", resp.StatusCode)
	}
	if st := getSession(t, srv.URL, w1).Meta.Status; st != "working" {
		t.Fatalf("status = %q, want the turn still working", st)
	}
	if code, _ := postInterrupt(t, srv.URL, w1); code != http.StatusOK {
		t.Fatalf("cleanup interrupt status = %d, want 200", code)
	}
	waitStatus(t, srv.URL, w1, "idle", 15*time.Second)
}

func seedStopRun(t *testing.T, d *Daemon, runID string, workers map[string]string) {
	t.Helper()
	tickets := []staticTicket{}
	for ticket := range workers {
		tickets = append(tickets, staticTicket{ID: ticket, Num: 1, Title: ticket})
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.runs[runID] = &runState{
		ID: runID, Spec: "spec", Owner: "owner-1", Repo: "", Status: "running",
		Harness: "pig", Model: "fake-model",
		Tickets:  tickets,
		Sessions: map[string]string{},
		notify:   make(chan struct{}),
		unpause:  make(chan struct{}),
	}
}

func remapStopRun(t *testing.T, d *Daemon, runID string, workers map[string]string) {
	t.Helper()
	d.mu.Lock()
	defer d.mu.Unlock()
	rs, ok := d.runs[runID]
	if !ok {
		t.Fatalf("unknown run %s", runID)
	}
	for ticket, sess := range workers {
		rs.Sessions[ticket] = sess
	}
}

// A scripted reviewer submit Event carrying a refused payload never stops
// its turn: the agent keeps working and the turn-end re-prompt still runs,
// ending needs-you after the second refusal.
func TestStopAfterSubmitFakeReviewerEventRefused(t *testing.T) {
	t.Parallel()
	repo, _ := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	branch := "implement/01-first"
	worktree := filepath.Join(repo, ".el-worktrees", "01-first")
	testGit(t, repo, "worktree", "add", "-b", branch, worktree, "HEAD")
	t.Cleanup(func() { testGit(t, repo, "worktree", "remove", "--force", worktree) })
	cur := strings.TrimSpace(testGit(t, repo, "rev-parse", branch))
	fb := NewFakeBackend()
	fb.SetDelay(100 * time.Millisecond)
	fb.ReviewSubmit = func(sessionID, prompt string) (ReviewerSubmit, bool) {
		// Pass without ran: refused by the tool rules.
		return ReviewerSubmit{Verdict: "pass", Tip: cur}, true
	}
	d := runTestDaemon(t, tempDir(t), fb)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	rev := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	seedStopReviewer(t, d, "r-ref", "01-first", repo, branch, worktree, cur, rev)
	if err := d.runTellSession(rev, reviewerPrompt("01-first", "spec", "reviewer-spec", branch, cur)); err != nil {
		t.Fatal(err)
	}
	r := waitRunCond(t, srv.URL, "r-ref", func(r runJSON) bool {
		return ticketByID(r, "01-first").NeedsYou != ""
	}, "reviewer needs-you after the refused submits")
	if n := fb.InterruptCount(rev); n != 0 {
		t.Fatalf("interrupts on refused submits = %d, want 0", n)
	}
	if got := ticketByID(r, "01-first").Verdicts["reviewer-spec"].Verdict; got == "pass" {
		t.Fatalf("verdict = pass, want nothing recorded from a refusal")
	}
}

// A scripted reviewer submit Event carrying an accepted payload stops its
// own turn at once, and the turn end still records the verdict.
func TestStopAfterSubmitFakeReviewerEventAccepted(t *testing.T) {
	t.Parallel()
	repo, _ := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	branch := "implement/01-first"
	worktree := filepath.Join(repo, ".el-worktrees", "01-first")
	testGit(t, repo, "worktree", "add", "-b", branch, worktree, "HEAD")
	t.Cleanup(func() { testGit(t, repo, "worktree", "remove", "--force", worktree) })
	cur := strings.TrimSpace(testGit(t, repo, "rev-parse", branch))
	fb := NewFakeBackend()
	// Short Delay: the scripted submit lands fast, and the stop still cuts
	// the turn at the submit Event (the interrupt fires inside the turn).
	fb.SetDelay(200 * time.Millisecond)
	fb.ReviewSubmit = func(sessionID, prompt string) (ReviewerSubmit, bool) {
		return ReviewerSubmit{
			Verdict: "pass", Tip: cur,
			Ran: []ReviewRan{{Command: "go vet ./...", ExitCode: 0}},
		}, true
	}
	d := runTestDaemon(t, tempDir(t), fb)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	rev := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	seedStopReviewer(t, d, "r-acc", "01-first", repo, branch, worktree, cur, rev)
	if err := d.runTellSession(rev, reviewerPrompt("01-first", "spec", "reviewer-spec", branch, cur)); err != nil {
		t.Fatal(err)
	}
	// The scripted submit lands mid-turn; the stop cuts the turn at it.
	r := waitRunCond(t, srv.URL, "r-acc", func(r runJSON) bool {
		return ticketByID(r, "01-first").Verdicts["reviewer-spec"].Verdict == "pass"
	}, "reviewer-spec pass recorded")
	waitStatus(t, srv.URL, rev, "idle", 15*time.Second)
	if n := fb.InterruptCount(rev); n != 1 {
		t.Fatalf("interrupts = %d, want exactly 1", n)
	}
	if got := ticketByID(r, "01-first").NeedsYou; got != "" {
		t.Fatalf("needs-you = %q, want none", got)
	}
}

func seedStopReviewer(t *testing.T, d *Daemon, runID, ticket, repo, branch, worktree, tip, rev string) {
	t.Helper()
	d.mu.Lock()
	defer d.mu.Unlock()
	d.runs[runID] = &runState{
		ID: runID, Spec: "spec", Owner: "owner-1", Repo: repo,
		Status: "running", Harness: "fake", Model: "fake-model",
		Tickets:       []staticTicket{{ID: ticket, Num: 1, Title: "First"}},
		Branches:      map[string]string{ticket: branch},
		Worktrees:     map[string]string{ticket: worktree},
		Reviewers:     map[string]map[string]string{ticket: {"reviewer-spec": rev}},
		ReviewTips:    map[string]map[string]string{ticket: {"reviewer-spec": tip}},
		ReviewPending: map[string]map[string][]string{ticket: {"reviewer-spec": {tip}}},
		Reviews:       map[string]map[string]storedReview{},
		Rows:          map[string]savedRow{ticket: {State: "in-review", Tip: tip}},
		notify:        make(chan struct{}),
		unpause:       make(chan struct{}),
	}
}
