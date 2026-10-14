package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunTicketSessionIndexHTTPAndRestart(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n- [ ] acceptance\n",
	})
	stateDir := tempDir(t)
	d := NewDaemon(stateDir, NewFakeBackend())
	d.hostClaudeDir = filepath.Join(tempDir(t), "host-claude")
	d.hostPigDir = filepath.Join(tempDir(t), "host-pig")
	for _, dir := range []string{d.hostClaudeDir, d.hostPigDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	const runID, ticketID = "run-session-index", "01-first"
	run := &runState{
		ID: runID, Spec: spec, Repo: repo, Status: runStatusDone, Owner: "owner-1",
		Harness: "fake", Model: "fake-model", SpecBranch: "main",
		CreatedAt: time.Now().UTC(),
		Tickets:   []staticTicket{{ID: ticketID, Num: 1, Title: "First", Path: filepath.Join(repo, ".scratch", "demo", "issues", "01-first.md")}},
		Sessions:  map[string]string{}, Reviewers: map[string]map[string]string{},
		Rows:     map[string]savedRow{ticketID: {State: runStatusDone}},
		Branches: map[string]string{},
	}
	worker := SessionMeta{ID: "worker-session", Harness: "fake", Model: "fake-model", WorkDir: repo, Status: "idle", CreatedAt: time.Now().UTC(), Owner: run.Owner, RunWorker: true}
	specReviewer := SessionMeta{ID: "spec-reviewer-session", Harness: "fake", Model: "fake-model", WorkDir: repo, Status: "idle", CreatedAt: time.Now().UTC(), Owner: run.Owner, Reviewer: reviewerSpecType}
	standardsReviewer := SessionMeta{ID: "standards-reviewer-session", Harness: "fake", Model: "fake-model", WorkDir: repo, Status: "idle", CreatedAt: time.Now().UTC(), Owner: run.Owner, Reviewer: reviewerStandardsType}
	oldWorker := SessionMeta{ID: "old-worker-session", Harness: "fake", Model: "fake-model", WorkDir: repo, Status: "idle", CreatedAt: time.Now().UTC(), Owner: run.Owner, ParentID: runID, RunWorker: true}

	d.mu.Lock()
	d.runs[runID] = run
	for _, meta := range []SessionMeta{worker, specReviewer, standardsReviewer} {
		d.sessions[meta.ID] = &session{meta: meta, notify: make(chan struct{})}
		d.persistMeta(meta)
	}
	run.Sessions[ticketID] = worker.ID
	run.Reviewers[ticketID] = map[string]string{
		reviewerSpecType: specReviewer.ID, reviewerStandardsType: standardsReviewer.ID,
	}
	d.sessions[oldWorker.ID] = &session{meta: oldWorker, notify: make(chan struct{})}
	d.persistMeta(oldWorker)
	if !d.indexRunTicketSessionLocked(runID, ticketID, oldWorker.ID) {
		t.Fatalf("could not index historical Worker Session %q", oldWorker.ID)
	}
	for _, meta := range []SessionMeta{worker, specReviewer, standardsReviewer} {
		if !d.indexRunTicketSessionLocked(runID, ticketID, meta.ID) {
			t.Fatalf("could not index attached Session %q", meta.ID)
		}
	}
	d.persistRun(run)
	d.mu.Unlock()

	// A real child is indexed as soon as startSession persists its ParentID.
	_, startErr := d.startSession(SessionMeta{
		ID: "indexed-scout", Harness: "fake", Model: "fake-model", WorkDir: repo,
		Status: "idle", CreatedAt: time.Now().UTC(), Owner: run.Owner,
		ParentID: worker.ID, Scout: true,
	})
	if startErr != nil {
		t.Fatalf("start indexed scout: %v", startErr)
	}
	// A matching ParentID without an index entry must not be found by scanning
	// every Session while rendering this Ticket.
	d.mu.Lock()
	d.sessions["unindexed-scout"] = &session{meta: SessionMeta{
		ID: "unindexed-scout", Harness: "fake", Status: "idle", ParentID: worker.ID, Scout: true,
	}}
	d.mu.Unlock()

	assertRunAgents := func(handler http.Handler, wantIDs ...string) {
		t.Helper()
		client := httptest.NewServer(handler)
		defer client.Close()
		resp, err := client.Client().Get(client.URL + "/api/runs/" + runID)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			raw, _ := io.ReadAll(resp.Body)
			t.Fatalf("Run detail status = %d: %s", resp.StatusCode, raw)
		}
		var out struct {
			Run struct {
				Tickets []struct {
					Agents []ticketAgent `json:"agents"`
				} `json:"tickets"`
			} `json:"run"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		if len(out.Run.Tickets) != 1 {
			t.Fatalf("tickets = %d, want 1", len(out.Run.Tickets))
		}
		got := make([]string, 0, len(out.Run.Tickets[0].Agents))
		for _, agent := range out.Run.Tickets[0].Agents {
			got = append(got, agent.SessionID)
		}
		if len(got) != len(wantIDs) {
			t.Fatalf("ticket agents = %v, want %v", got, wantIDs)
		}
		for i, id := range wantIDs {
			if got[i] != id {
				t.Fatalf("ticket agents = %v, want %v", got, wantIDs)
			}
		}
	}

	want := []string{worker.ID, standardsReviewer.ID, specReviewer.ID, "indexed-scout"}
	assertRunAgents(authedHandler(t, d), want...)

	listServer := httptest.NewServer(authedHandler(t, d))
	resp, err := listServer.Client().Get(listServer.URL + "/api/runs")
	if err != nil {
		listServer.Close()
		t.Fatal(err)
	}
	listBody, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	listServer.Close()
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || strings.Contains(string(listBody), worker.ID) || strings.Contains(string(listBody), `"tickets"`) {
		t.Fatalf("Runs list must remain a session-free summary, got status %d: %s", resp.StatusCode, listBody)
	}

	// The index is stored in the Run file and rebuilt when the Daemon reloads.
	restarted := NewDaemon(stateDir, NewFakeBackend())
	restarted.hostClaudeDir = d.hostClaudeDir
	restarted.hostPigDir = d.hostPigDir
	assertRunAgents(authedHandler(t, restarted), want...)
	restarted.mu.Lock()
	indexed := runSessionIDsLocked(restarted, restarted.runs[runID], ticketID)
	persistedIndex := append([]string(nil), restarted.runs[runID].TicketSessions[ticketID]...)
	restarted.mu.Unlock()
	if len(persistedIndex) != len(want)+1 || !containsSessionID(persistedIndex, oldWorker.ID) {
		t.Fatalf("persisted Ticket index = %v, want current and historical Sessions", persistedIndex)
	}
	if len(indexed) != len(want) {
		t.Fatalf("restarted indexed Sessions = %v, want %v", indexed, want)
	}
}
