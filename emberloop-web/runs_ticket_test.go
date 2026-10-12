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

// Table tests for ticket checkbox parsing: the same marker rule the driver
// ticks by.
func TestParseTicketBoxesTable(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []ticketBox
	}{
		{name: "empty", content: "# T\nno boxes\n", want: []ticketBox{}},
		{
			name:    "mixed markers",
			content: "- [ ] alpha\n- [x] beta\n* [ ] gamma\n* [X] delta\n",
			want: []ticketBox{
				{Text: "alpha", Checked: false},
				{Text: "beta", Checked: true},
				{Text: "gamma", Checked: false},
				{Text: "delta", Checked: true},
			},
		},
		{
			name:    "prose brackets ignored",
			content: "mention [ ] in prose\n- [ ] real\n",
			want:    []ticketBox{{Text: "real", Checked: false}},
		},
		{
			name:    "text trimmed",
			content: "- [ ]   spaced out   \n",
			want:    []ticketBox{{Text: "spaced out", Checked: false}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseTicketBoxes(tc.content)
			if len(got) != len(tc.want) {
				t.Fatalf("boxes = %+v, want %+v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("box %d = %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// Table tests for worker follow-ups parsing: text after Follow-ups:.
func TestParseFollowupsTable(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "none when no marker", text: "done", want: ""},
		{name: "note", text: "done. Follow-ups: fix the linter", want: "fix the linter"},
		{name: "none means none", text: "done. Follow-ups: none", want: ""},
		{name: "nothing means none", text: "Follow-ups: nothing", want: ""},
		{name: "empty after marker", text: "Follow-ups:", want: ""},
		{name: "multiline kept", text: "Follow-ups: a\nb", want: "a\nb"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseFollowups(tc.text); got != tc.want {
				t.Fatalf("followups = %q, want %q", got, tc.want)
			}
		})
	}
}

// extTicket decodes the ticket page fields of GET /api/runs/{id}.
type extTicket struct {
	ID            string                       `json:"id"`
	State         string                       `json:"state"`
	Branch        string                       `json:"branch"`
	Tip           string                       `json:"tip"`
	Title         string                       `json:"title"`
	Body          string                       `json:"body"`
	Boxes         []ticketBox                  `json:"boxes"`
	LastRefusal   string                       `json:"lastRefusal"`
	NeedsYou      string                       `json:"needsYou"`
	Followups     string                       `json:"followups"`
	Agents        []ticketAgent                `json:"agents"`
	Rounds        []ticketRound                `json:"rounds"`
	WorkerSession string                       `json:"workerSession"`
	Verdicts      map[string]reviewVerdictJSON `json:"verdicts"`
	Gate          struct {
		Required []string `json:"required"`
		Verdicts map[string]struct {
			Verdict string `json:"verdict"`
			At      string `json:"at"`
			Fresh   bool   `json:"fresh"`
		} `json:"verdicts"`
	} `json:"gate"`
}

type extRun struct {
	ID      string      `json:"id"`
	Status  string      `json:"status"`
	Tickets []extTicket `json:"tickets"`
}

func getExtRun(t *testing.T, base, id string) extRun {
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
		Run extRun `json:"run"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.Run
}

func extTicketByID(r extRun, id string) extTicket {
	for _, tk := range r.Tickets {
		if tk.ID == id {
			return tk
		}
	}
	return extTicket{}
}

func submitWorker(t *testing.T, base, id, status string, completed []string, summary string) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"status": status, "completed": completed, "summary": summary})
	resp, err := http.Post(base+"/api/sessions/"+id+"/submit", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("worker submit status = %d, want 202", resp.StatusCode)
	}
}

// A reviewer submit without its tip is refused as a tool error: reviewers
// report through the submit tool with verdict, tip, ran and findings.
func TestReviewerSubmitWithoutTipRefused(t *testing.T) {
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	commitTicketWork(t, repo, "01-first")
	submitSession(t, srv.URL, ticketByID(r, "01-first").WorkerSession, "done", []string{"alpha"})
	revs := waitReviewers(t, srv.URL, run.ID, "01-first")

	body, _ := json.Marshal(map[string]any{"verdict": "pass"})
	resp, err := http.Post(srv.URL+"/api/sessions/"+revs["reviewer-spec"]+"/submit", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("reviewer submit status = %d, want 400", resp.StatusCode)
	}
	var out struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(out.Error), "tip") {
		t.Fatalf("reviewer submit error = %q, want it to name the tip", out.Error)
	}
}

// Messages to reviewer sessions are refused: prompt, steer, follow-up and
// spawn are read-only for reviewers.
func TestReviewerMessagesRefused409(t *testing.T) {
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	commitTicketWork(t, repo, "01-first")
	submitSession(t, srv.URL, ticketByID(r, "01-first").WorkerSession, "done", []string{"alpha"})
	revs := waitReviewers(t, srv.URL, run.ID, "01-first")
	rev := revs["reviewer-spec"]

	for _, path := range []string{"prompt", "steer", "followup"} {
		var body []byte
		if path == "prompt" {
			body, _ = json.Marshal(map[string]any{"text": "hello"})
		} else {
			body, _ = json.Marshal(map[string]any{"text": "hello"})
		}
		resp, err := http.Post(srv.URL+"/api/sessions/"+rev+"/"+path, "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusConflict {
			t.Fatalf("%s to reviewer status = %d, want 409", path, resp.StatusCode)
		}
	}
	spawnBody, _ := json.Marshal(map[string]any{"agents": []any{map[string]any{"prompt": "x"}}})
	resp, err := http.Post(srv.URL+"/api/sessions/"+rev+"/spawn", "application/json", bytes.NewReader(spawnBody))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict && resp.StatusCode != http.StatusForbidden {
		t.Fatalf("spawn on reviewer status = %d, want 409/403", resp.StatusCode)
	}
}

// An unparseable reviewer reply re-prompts once for the format; a second
// failure marks the ticket needs-you for the user.
func TestReviewerUnparseableRepromptsOnceThenNeedsYou(t *testing.T) {
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	fb := NewFakeBackend()
	fb.ReplyFunc = func(id, text string) (string, bool) {
		if strings.Contains(text, "Review ticket") || strings.Contains(text, "RESPOND IN FORMAT") {
			return "just some prose with no verdict", true
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
	commitTicketWork(t, repo, "01-first")
	submitSession(t, srv.URL, ticketByID(r, "01-first").WorkerSession, "done", []string{"alpha"})
	revs := waitReviewers(t, srv.URL, run.ID, "01-first")

	deadline := time.Now().Add(15 * time.Second)
	for {
		sess := getSession(t, srv.URL, revs["reviewer-spec"])
		if strings.Count(sessTranscript(sess), "RESPOND IN FORMAT") == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("reviewer transcript misses the single format re-prompt:\n%s", sessTranscript(sess))
		}
		time.Sleep(100 * time.Millisecond)
	}
	r2 := getExtRun(t, srv.URL, run.ID)
	tk := extTicketByID(r2, "01-first")
	_ = tk

	deadline = time.Now().Add(15 * time.Second)
	for {
		tk = extTicketByID(getExtRun(t, srv.URL, run.ID), "01-first")
		if tk.State == "needs-you" && tk.NeedsYou != "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("ticket state = %q needsYou = %q, want needs-you with a reason", tk.State, tk.NeedsYou)
		}
		time.Sleep(100 * time.Millisecond)
	}
	sess := getSession(t, srv.URL, revs["reviewer-spec"])
	if n := strings.Count(sessTranscript(sess), "RESPOND IN FORMAT"); n != 1 {
		t.Fatalf("format re-prompts = %d, want exactly one", n)
	}
}

// Rounds persist per ticket and reload without loss; old run files without
// rounds still load.
func TestRoundsPersistAndReload(t *testing.T) {
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	stateDir := tempDir(t)
	d := NewDaemon(stateDir, NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	commitTicketWork(t, repo, "01-first")
	submitWorker(t, srv.URL, ticketByID(r, "01-first").WorkerSession, "done", []string{"alpha"}, "done. Follow-ups: check lint")
	passReviewers(t, srv.URL, run.ID, "01-first")
	waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return r.Status == "done"
	}, "run done")
	er := getExtRun(t, srv.URL, run.ID)
	tk := extTicketByID(er, "01-first")
	if len(tk.Rounds) != 1 {
		t.Fatalf("rounds = %+v, want one round", tk.Rounds)
	}
	rd := tk.Rounds[0]
	if rd.N != 1 || rd.Tip == "" {
		t.Fatalf("round = %+v, want n=1 with the reviewed tip", rd)
	}
	if rd.StartedAt.IsZero() || rd.EndedAt == nil || rd.EndedAt.IsZero() {
		t.Fatalf("round times = %+v, want started and ended", rd)
	}
	for _, typ := range []string{"reviewer-spec", "reviewer-standards"} {
		rv, ok := rd.Reviews[typ]
		if !ok || rv.Verdict != "pass" || rv.At != rd.Tip || rv.HeldFromRound != nil {
			t.Fatalf("round review %s = %+v, want fresh pass at the tip", typ, rv)
		}
		if rv.DurationMs < 0 {
			t.Fatalf("round review %s duration = %d, want >= 0", typ, rv.DurationMs)
		}
	}
	if tk.Followups != "check lint" {
		t.Fatalf("followups = %q, want the worker summary note", tk.Followups)
	}
	raw, err := os.ReadFile(filepath.Join(stateDir, "enderbloom", "runs", run.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"rounds"`) {
		t.Fatalf("run file misses rounds")
	}
	srv.Close()

	d2 := NewDaemon(stateDir, NewFakeBackend())
	srv2 := httptest.NewServer(authedHandler(t, d2))
	defer srv2.Close()
	er2 := getExtRun(t, srv2.URL, run.ID)
	tk2 := extTicketByID(er2, "01-first")
	if len(tk2.Rounds) != 1 || tk2.Rounds[0].Tip != rd.Tip {
		t.Fatalf("reloaded rounds = %+v, want the persisted round", tk2.Rounds)
	}
	if tk2.Rounds[0].Reviews["reviewer-spec"].Verdict != "pass" {
		t.Fatalf("reloaded review = %+v, want the pass", tk2.Rounds[0].Reviews)
	}
	if tk2.Followups != "check lint" {
		t.Fatalf("reloaded followups = %q, want the note", tk2.Followups)
	}

	legacyDir := tempDir(t)
	legacy := map[string]any{
		"id": "run-legacy", "spec": spec, "repo": repo, "status": "paused",
		"harness": "fake", "model": "m", "tickets": []any{}, "owner": "owner-1",
	}
	lraw, _ := json.Marshal(legacy)
	if err := os.MkdirAll(filepath.Join(legacyDir, "enderbloom", "runs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "enderbloom", "runs", "run-legacy.json"), lraw, 0o644); err != nil {
		t.Fatal(err)
	}
	d3 := NewDaemon(legacyDir, NewFakeBackend())
	srv3 := httptest.NewServer(authedHandler(t, d3))
	defer srv3.Close()
	resp, err := http.Get(srv3.URL + "/api/runs/run-legacy")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("legacy get status = %d, want 200", resp.StatusCode)
	}
}

// A same-tip second done holds the passes instead of rerunning the
// reviewers: the new round carries heldFromRound and no new gate prompt.
func TestHeldPassNoRerun(t *testing.T) {
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	d := NewDaemon(tempDir(t), NewFakeBackend())
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
	// Dirty the branch before the verdicts arrive so the finish cannot merge
	// in the race between a pass and this setup.
	worktree := filepath.Join(repo, ".el-worktrees", "01-first")
	writeFile(t, worktree, "dirty.txt", "uncommitted\n")
	passReviewers(t, srv.URL, run.ID, "01-first")
	deadline := time.Now().Add(15 * time.Second)
	for {
		tk := extTicketByID(getExtRun(t, srv.URL, run.ID), "01-first")
		if tk.LastRefusal != "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no dirty refusal; state=%q", tk.State)
		}
		time.Sleep(100 * time.Millisecond)
	}
	tip := strings.TrimSpace(testGit(t, repo, "rev-parse", "implement/01-first"))
	// The dirty file was never committed, so removing it cleans the tree
	// without a new commit: the resubmit tests the same-tip hold.
	if err := os.Remove(filepath.Join(worktree, "dirty.txt")); err != nil {
		t.Fatal(err)
	}
	submitSession(t, srv.URL, worker, "done", []string{"alpha"})

	deadline = time.Now().Add(15 * time.Second)
	for {
		tk := extTicketByID(getExtRun(t, srv.URL, run.ID), "01-first")
		if len(tk.Rounds) >= 2 {
			rd := tk.Rounds[1]
			if rd.Tip == tip {
				for _, typ := range []string{"reviewer-spec", "reviewer-standards"} {
					rv, ok := rd.Reviews[typ]
					if !ok || rv.Verdict != "pass" || rv.HeldFromRound == nil || *rv.HeldFromRound != 1 || rv.At != tip {
						t.Fatalf("round 2 review %s = %+v, want pass held from round 1 at %q", typ, rv, tip)
					}
				}
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("no held second round: %+v", extTicketByID(getExtRun(t, srv.URL, run.ID), "01-first").Rounds)
		}
		time.Sleep(100 * time.Millisecond)
	}
	for _, sess := range revs {
		if n := strings.Count(sessTranscript(getSession(t, srv.URL, sess)), "Review ticket"); n != 1 {
			t.Fatalf("reviewer got %d gate prompts, want one (held, not rerun)", n)
		}
	}
}

// The ticket page fields ride GET: boxes, body, title, branch, tip, state,
// stateSince, needsYou, lastRefusal, followups, agents, gate and rounds.
func TestRunTicketJSONExposesPageFields(t *testing.T) {
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n- [ ] beta\n",
	})
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	worker := ticketByID(r, "01-first").WorkerSession
	// A scout on the worker shows in agents.
	spawnBody, _ := json.Marshal(map[string]any{"agents": []any{map[string]any{"prompt": "look around", "type": "scout"}}})
	resp, err := http.Post(srv.URL+"/api/sessions/"+worker+"/spawn", "application/json", bytes.NewReader(spawnBody))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	commitTicketWork(t, repo, "01-first")
	submitWorker(t, srv.URL, worker, "done", []string{"alpha"}, "done. Follow-ups: check lint")
	passReviewers(t, srv.URL, run.ID, "01-first")
	waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return r.Status == "done"
	}, "run done")

	tk := extTicketByID(getExtRun(t, srv.URL, run.ID), "01-first")
	if tk.Title != "First" {
		t.Fatalf("title = %q, want First", tk.Title)
	}
	// The merge completes the ticket, ticking every box the way the CLI
	// completion does; the boxes re-read from the file show it.
	if !strings.Contains(tk.Body, "- [x] alpha") || !strings.Contains(tk.Body, "- [x] beta") {
		t.Fatalf("body misses completed boxes:\n%s", tk.Body)
	}
	if len(tk.Boxes) != 2 || tk.Boxes[0] != (ticketBox{Text: "alpha", Checked: true}) || tk.Boxes[1] != (ticketBox{Text: "beta", Checked: true}) {
		t.Fatalf("boxes = %+v, want both ticked after the merge", tk.Boxes)
	}
	if tk.Branch != "implement/01-first" {
		t.Fatalf("branch = %q", tk.Branch)
	}
	if tk.State != "merged" {
		t.Fatalf("state = %q, want merged", tk.State)
	}
	if tk.Followups != "check lint" {
		t.Fatalf("followups = %q", tk.Followups)
	}
	roles := map[string]bool{}
	for _, a := range tk.Agents {
		roles[a.Role] = true
		if a.SessionID == "" || a.Harness == "" || a.Status == "" {
			t.Fatalf("agent = %+v, want session, harness and status", a)
		}
	}
	for _, want := range []string{"worker", "reviewer-spec", "reviewer-standards", "scout"} {
		if !roles[want] {
			t.Fatalf("agents = %+v, want role %s", tk.Agents, want)
		}
	}
	if len(tk.Gate.Required) != 2 {
		t.Fatalf("gate.required = %+v, want both reviewers", tk.Gate.Required)
	}
	for _, typ := range []string{"reviewer-spec", "reviewer-standards"} {
		v, ok := tk.Gate.Verdicts[typ]
		if !ok || v.Verdict != "pass" {
			t.Fatalf("gate verdict %s = %+v, want pass", typ, v)
		}
	}
	if len(tk.Rounds) != 1 {
		t.Fatalf("rounds = %+v, want one", tk.Rounds)
	}
}

// sseRunReader reads Run SSE snapshots until cond holds or the deadline
// passes. It returns the last snapshot seen.
func sseRunReader(t *testing.T, base, runID string, deadline time.Duration, cond func(extRun) bool, what string) extRun {
	t.Helper()
	client := &http.Client{Timeout: deadline + 5*time.Second}
	req, err := http.NewRequest("GET", base+"/api/runs/"+runID+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("events status = %d, want 200", resp.StatusCode)
	}
	buf := make([]byte, 0, 64*1024)
	chunk := make([]byte, 4096)
	end := time.Now().Add(deadline)
	var last extRun
	// The stream stays open; heartbeats keep it alive while nothing
	// changes. Reads block until the next frame or close.
	for time.Now().Before(end) {
		n, err := resp.Body.Read(chunk)
		if n > 0 {
			buf = append(buf, chunk[:n]...)
			for {
				idx := bytes.Index(buf, []byte("\n\n"))
				if idx < 0 {
					break
				}
				frame := string(buf[:idx])
				buf = buf[idx+2:]
				for _, line := range strings.Split(frame, "\n") {
					if !strings.HasPrefix(line, "data: ") {
						continue
					}
					var body extRun
					if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &body); err != nil {
						continue
					}
					last = body
					if cond(body) {
						return body
					}
				}
			}
		}
		if err != nil {
			break
		}
	}
	t.Fatalf("timed out waiting for %s (last run %+v)", what, last.Status)
	return last
}

// Every ticket change publishes a snapshot over the Run SSE: box ticks,
// verdicts, round appends and state changes arrive without polling.
func TestRunSSEPublishesTicketChanges(t *testing.T) {
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	worker := ticketByID(r, "01-first").WorkerSession

	done := make(chan extRun, 1)
	go func() {
		got := sseRunReader(t, srv.URL, run.ID, 20*time.Second, func(er extRun) bool {
			if len(er.Tickets) == 0 {
				return false
			}
			tk := er.Tickets[0]
			return len(tk.Boxes) == 1 && tk.Boxes[0].Checked &&
				tk.Gate.Verdicts["reviewer-spec"].Verdict == "pass" &&
				tk.Gate.Verdicts["reviewer-standards"].Verdict == "pass" &&
				len(tk.Rounds) >= 1 && tk.Followups == "check lint"
		}, "boxes ticked, both verdicts, a round and followups over SSE")
		done <- got
	}()
	commitTicketWork(t, repo, "01-first")
	submitWorker(t, srv.URL, worker, "done", []string{"alpha"}, "done. Follow-ups: check lint")
	select {
	case got := <-done:
		if got.Tickets[0].State == "" {
			t.Fatalf("SSE ticket misses state")
		}
	case <-time.After(25 * time.Second):
		t.Fatalf("SSE never delivered the ticket changes")
	}
}

// A scout spawned on the worker publishes its agent over the Run SSE.
func TestRunSSEPublishesScoutAgent(t *testing.T) {
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	worker := ticketByID(r, "01-first").WorkerSession

	done := make(chan extRun, 1)
	go func() {
		got := sseRunReader(t, srv.URL, run.ID, 15*time.Second, func(er extRun) bool {
			for _, a := range er.Tickets[0].Agents {
				if a.Role == "scout" {
					return true
				}
			}
			return false
		}, "scout agent over SSE")
		done <- got
	}()
	spawnBody, _ := json.Marshal(map[string]any{"agents": []any{map[string]any{"prompt": "look around", "type": "scout"}}})
	resp, err := http.Post(srv.URL+"/api/sessions/"+worker+"/spawn", "application/json", bytes.NewReader(spawnBody))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatalf("SSE never delivered the scout agent")
	}
}

// The same refusal three times marks the ticket needs-you with its reason.
func TestRunNeedsYouReasonSameRefusal(t *testing.T) {
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
	})
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "worker started")
	worker := ticketByID(r, "01-first").WorkerSession
	// Space the submits so the driver finishes between them: rapid submits
	// collapse into one finish and one streak entry.
	for i := 0; i < 3; i++ {
		submitSession(t, srv.URL, worker, "blocked", nil)
		time.Sleep(1500 * time.Millisecond)
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		tk := extTicketByID(getExtRun(t, srv.URL, run.ID), "01-first")
		if tk.State == "needs-you" && strings.Contains(tk.NeedsYou, "same refusal 3 times") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("state = %q needsYou = %q, want needs-you with the refusal reason", tk.State, tk.NeedsYou)
		}
		time.Sleep(200 * time.Millisecond)
	}
}
