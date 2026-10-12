package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func postRunJSON(t *testing.T, base, path string, body any) (int, runJSON, string) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(base+path, "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Run   runJSON `json:"run"`
		Error string  `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out.Run, out.Error
}

func TestTicketPauseSwitchResumeKeepsBranchAndWIP(t *testing.T) {
	repo, spec := initRunRepo(t, demoTickets())
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != ""
	}, "Ticket Worker started")
	worker := ticketByID(r, "01-first").WorkerSession
	branch := ticketByID(r, "01-first").Branch
	worktree := getSession(t, srv.URL, worker).Meta.WorkDir

	if code, _, _ := postRunJSON(t, srv.URL, "/api/runs/"+run.ID+"/tickets/01-first/switch", map[string]string{"backend": "fake", "model": "switched-model"}); code != http.StatusConflict {
		t.Fatalf("Switch while running = %d, want 409", code)
	}
	writeFile(t, worktree, "pause-checkpoint.txt", "preserve this uncommitted work\n")
	if code, paused, errText := postRunJSON(t, srv.URL, "/api/runs/"+run.ID+"/tickets/01-first/pause", map[string]string{}); code != http.StatusOK {
		t.Fatalf("Ticket pause = %d (%s), want 200", code, errText)
	} else if ticketByID(paused, "01-first").State != runStatusPaused {
		t.Fatalf("paused Ticket state = %q, want paused", ticketByID(paused, "01-first").State)
	}
	if got := getSession(t, srv.URL, worker).Meta.Status; got != "ended" {
		t.Fatalf("paused Worker status = %q, want ended", got)
	}
	if subject := testGit(t, worktree, "log", "-1", "--format=%s"); !bytes.Contains([]byte(subject), []byte("WIP: paused Run "+run.ID+" Ticket 01-first")) {
		t.Fatalf("checkpoint commit subject = %q, want labelled WIP", subject)
	}
	if got := testGit(t, worktree, "show", "HEAD:pause-checkpoint.txt"); got != "preserve this uncommitted work\n" {
		t.Fatalf("checkpoint file = %q, want committed content", got)
	}

	code, switched, errText := postRunJSON(t, srv.URL, "/api/runs/"+run.ID+"/tickets/01-first/switch", map[string]string{"backend": "fake", "model": "switched-model"})
	if code != http.StatusOK {
		t.Fatalf("Ticket Switch = %d (%s), want 200", code, errText)
	}
	if got := ticketByID(switched, "01-first").Chain["worker"].Model; got != "switched-model" {
		t.Fatalf("active Worker model = %q, want switched-model", got)
	}
	if got := ticketByID(getRun(t, srv.URL, run.ID), "01-first").WorkerSession; got != worker {
		t.Fatalf("Switch restarted Worker automatically: got %q, want ended %q", got, worker)
	}

	code, resumed, errText := postRunJSON(t, srv.URL, "/api/runs/"+run.ID+"/tickets/01-first/resume", map[string]string{})
	if code != http.StatusOK {
		t.Fatalf("Ticket Resume = %d (%s), want 200", code, errText)
	}
	next := ticketByID(resumed, "01-first")
	if next.WorkerSession == "" || next.WorkerSession == worker {
		t.Fatalf("resumed Worker = %q, want a new Session replacing %q", next.WorkerSession, worker)
	}
	if next.Branch != branch {
		t.Fatalf("resumed branch = %q, want same branch %q", next.Branch, branch)
	}
	waitSessionCond(t, srv.URL, next.WorkerSession, func(s sessionBody) bool {
		_, _, ok := firstHandoffAndTask(s.Events)
		return ok
	}, "Ticket Resume Handoff and task")
	if code, _, _ := postRunJSON(t, srv.URL, "/api/runs/"+run.ID+"/tickets/01-first/switch", map[string]string{"backend": "fake", "model": "late"}); code != http.StatusConflict {
		t.Fatalf("Switch after Resume = %d, want 409", code)
	}
}

func TestRunPauseSwitchResumeReplacesWorkersManually(t *testing.T) {
	tickets := map[string]string{
		"01-first.md":  "# 01: First\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] alpha\n",
		"02-second.md": "# 02: Second\n\n**Blocked by:** None\n\n**Status:** open\n\n- [ ] beta\n",
	}
	repo, spec := initRunRepo(t, tickets)
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	r := waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != "" && ticketByID(r, "02-second").WorkerSession != ""
	}, "both Workers started")
	old := map[string]string{}
	branches := map[string]string{}
	for _, id := range []string{"01-first", "02-second"} {
		tk := ticketByID(r, id)
		old[id], branches[id] = tk.WorkerSession, tk.Branch
		writeFile(t, getSession(t, srv.URL, tk.WorkerSession).Meta.WorkDir, "pause.txt", id+" work\n")
	}

	_, code := postRunAction(t, srv.URL, run.ID, "pause")
	if code != http.StatusOK {
		t.Fatalf("Run pause = %d, want 200", code)
	}
	for _, id := range []string{"01-first", "02-second"} {
		if got := getSession(t, srv.URL, old[id]).Meta.Status; got != "ended" {
			t.Fatalf("%s Worker status = %q, want ended", id, got)
		}
		worktree := filepath.Join(repo, ".el-worktrees", id)
		if got := testGit(t, worktree, "show", "HEAD:pause.txt"); got != id+" work\n" {
			t.Fatalf("%s WIP content = %q", id, got)
		}
	}
	if code, _, _ := postRunJSON(t, srv.URL, "/api/runs/"+run.ID+"/switch", map[string]string{"backend": "fake", "model": "run-switched"}); code != http.StatusOK {
		t.Fatalf("Run Switch = %d, want 200", code)
	}
	if got := getRun(t, srv.URL, run.ID).Model; got != "run-switched" {
		t.Fatalf("Run model = %q, want run-switched", got)
	}
	if code, _, _ := postRunJSON(t, srv.URL, "/api/runs/"+run.ID+"/switch", map[string]string{"backend": "fake", "model": "late"}); code != http.StatusOK {
		t.Fatalf("repeated paused Run Switch = %d, want 200", code)
	}
	for _, id := range []string{"01-first", "02-second"} {
		if got := ticketByID(getRun(t, srv.URL, run.ID), id).WorkerSession; got != old[id] {
			t.Fatalf("Switch restarted %s automatically: got %q, want %q", id, got, old[id])
		}
	}

	if _, code := postRunAction(t, srv.URL, run.ID, "resume"); code != http.StatusOK {
		t.Fatalf("Run Resume = %d, want 200", code)
	}
	r = waitRunCond(t, srv.URL, run.ID, func(r runJSON) bool {
		return ticketByID(r, "01-first").WorkerSession != old["01-first"] && ticketByID(r, "02-second").WorkerSession != old["02-second"]
	}, "replacement Workers after manual Run Resume")
	for _, id := range []string{"01-first", "02-second"} {
		tk := ticketByID(r, id)
		if tk.Branch != branches[id] {
			t.Fatalf("%s branch changed from %q to %q", id, branches[id], tk.Branch)
		}
		waitSessionCond(t, srv.URL, tk.WorkerSession, func(s sessionBody) bool {
			_, _, ok := firstHandoffAndTask(s.Events)
			return ok
		}, id+" Resume Handoff and task")
	}
}

func TestRunSwitchRequiresPause(t *testing.T) {
	repo, spec := initRunRepo(t, demoTickets())
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	if code, _, _ := postRunJSON(t, srv.URL, "/api/runs/"+run.ID+"/switch", map[string]string{"backend": "fake", "model": "new-model"}); code != http.StatusConflict {
		t.Fatalf("unpaused Run Switch = %d, want 409", code)
	}
}
