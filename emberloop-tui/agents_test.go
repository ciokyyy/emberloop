package tui

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"example.com/emberloop/statedb"
)

// reviewersScoutsFixture builds a ticket with a worker, two reviewers on
// the worker's worktree, and three scout states: one spawned by the worker,
// one spawned by that scout, and one spawned by someone else. Reviewers
// and the attached scouts carry usage logs.
func reviewersScoutsFixture(t *testing.T) (stateDir, repo string) {
	t.Helper()
	stateDir = t.TempDir()
	repo = t.TempDir()
	writeSpec(t, repo, "alpha", []ticketFile{{"01-one.md", "ready-for-agent"}})
	specPath := filepath.Join(repo, ".scratch", "alpha", "spec.md")
	issues := filepath.Join(repo, ".scratch", "alpha", "issues")
	wt := filepath.Join(repo, ".emberloop-worktrees", "01-one")

	writeJSON(t, filepath.Join(stateDir, "workers.json"), map[string]any{
		"01-one": map[string]any{
			"id": "01-one", "ticket": filepath.Join(issues, "01-one.md"),
			"spec": specPath, "repo": repo, "branch": "implement/01-one",
			"worktree": wt, "pane": "pane-w1",
			"harness": "pig", "model": "opus", "effort": "high",
		},
	})
	writeJSON(t, filepath.Join(stateDir, "runs.json"), map[string]any{
		"alpha": map[string]any{
			"slug": "alpha", "spec": specPath, "repo": repo, "branch": "spec/alpha",
			"agent_id": "orch", "pane": "pane-orch", "harness": "pig",
		},
	})
	writeJSON(t, filepath.Join(stateDir, "reviewers.json"), map[string]any{
		wt: map[string]any{
			"reviewer-standards": map[string]any{
				"pane": "pane-rs", "session_id": "rs1", "harness": "pig",
				"model": "sonnet", "effort": "medium", "verdict": "pass",
			},
			"reviewer-spec": map[string]any{
				"pane": "pane-rp", "session_id": "rp1", "harness": "claude",
				"model": "opus", "effort": "high", "verdict": "",
			},
		},
	})
	writeJSON(t, filepath.Join(stateDir, "agents", "sc1.json"), map[string]any{
		"id": "sc1", "type": "scout", "pane": "pane-sc1",
		"parent_id": "01-one", "harness": "pig", "model": "haiku",
	})
	writeJSON(t, filepath.Join(stateDir, "agents", "sc2.json"), map[string]any{
		"id": "sc2", "type": "scout", "pane": "pane-sc2",
		"parent_id": "sc1", "harness": "pig", "model": "haiku",
	})
	writeJSON(t, filepath.Join(stateDir, "agents", "sc3.json"), map[string]any{
		"id": "sc3", "type": "scout", "pane": "pane-sc3",
		"parent_id": "someone-else", "harness": "pig", "model": "haiku",
	})
	// A reviewer agent state must never double as a scout row.
	writeJSON(t, filepath.Join(stateDir, "agents", "rs1.json"), map[string]any{
		"id": "rs1", "type": "reviewer-standards", "pane": "pane-rs",
		"parent_id": "01-one",
	})

	writeUsageRows(t, stateDir, []statedb.UsageRow{
		{Agent: "01-one", At: "2026-10-06T10:00:00Z", Harness: "pig", Provider: "p", Model: "opus", Kind: statedb.KindRequest, Input: 100, Output: 50, ReportedCost: costp(0.01)},
		{Agent: "rs1", At: "2026-10-06T10:00:00Z", Harness: "pig", Provider: "p", Model: "sonnet", Kind: statedb.KindRequest, Input: 10, Output: 5, ReportedCost: costp(0.001)},
		{Agent: "sc1", At: "2026-10-06T10:00:00Z", Harness: "pig", Provider: "p", Model: "haiku", Kind: statedb.KindRequest, Input: 7, Output: 3, ReportedCost: costp(0.0001)},
	})
	return stateDir, repo
}

func reviewersScoutsLive() map[string]string {
	return map[string]string{
		"pane-w1": "working", "pane-orch": "working",
		"pane-rs": "idle", "pane-rp": "working",
		"pane-sc1": "working", "pane-sc2": "working", "pane-sc3": "working",
	}
}

func agentByID(t *testing.T, agents []AgentSnap, id string) AgentSnap {
	t.Helper()
	for _, a := range agents {
		if a.ID == id {
			return a
		}
	}
	t.Fatalf("agent %q missing in %+v", id, agents)
	return AgentSnap{}
}

func TestSnapshotTicketAgentsIncludeReviewersAndScouts(t *testing.T) {
	stateDir, repo := reviewersScoutsFixture(t)

	snap, err := Snapshot(stateDir, repo, func() (map[string]string, error) { return reviewersScoutsLive(), nil })
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	tk := findSpec(t, snap, repo, "alpha").Tickets[0]
	// Worker, two reviewers, and the two scouts in its spawner chain.
	// The foreign scout and the reviewer's own agent state stay out.
	if len(tk.Agents) != 5 {
		t.Fatalf("agents = %+v, want worker + 2 reviewers + 2 scouts", tk.Agents)
	}
	// The worker lists first, then reviewers, then scouts.
	if tk.Agents[0].Kind != AgentWorker || tk.Agents[0].ID != "01-one" {
		t.Errorf("first agent = %+v, want the worker", tk.Agents[0])
	}
	rs := agentByID(t, tk.Agents, "rs1")
	if rs.Kind != AgentReviewerStandards || rs.Harness != "pig" || rs.ModelEffort() != "sonnet · medium" {
		t.Errorf("standards reviewer = %+v, want reviewer-standards pig sonnet medium", rs)
	}
	if !rs.Alive || rs.Status != "idle" {
		t.Errorf("standards reviewer liveness = %v/%q, want alive idle", rs.Alive, rs.Status)
	}
	if rs.Input != 10 || !rs.HasCost {
		t.Errorf("standards reviewer funds = %+v, want 10 tokens costed", rs)
	}
	rp := agentByID(t, tk.Agents, "rp1")
	if rp.Kind != AgentReviewerSpec || rp.Harness != "claude" {
		t.Errorf("spec reviewer = %+v, want reviewer-spec claude", rp)
	}
	sc1 := agentByID(t, tk.Agents, "sc1")
	if sc1.Kind != AgentScout || sc1.Model != "haiku" {
		t.Errorf("scout = %+v, want scout haiku", sc1)
	}
	if sc1.Input != 7 {
		t.Errorf("scout tokens = %d, want 7 from its usage log", sc1.Input)
	}
	agentByID(t, tk.Agents, "sc2")

	if tk.Totals.Input != 117 {
		t.Errorf("ticket input = %d, want 100 + 10 + 7", tk.Totals.Input)
	}
}

func TestSnapshotTicketWithoutWorkerHasNoAgents(t *testing.T) {
	stateDir := t.TempDir()
	repo := t.TempDir()
	writeSpec(t, repo, "alpha", []ticketFile{{"01-one.md", "ready-for-agent"}})

	snap, err := Snapshot(stateDir, repo, liveHerdr())
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	tk := findSpec(t, snap, repo, "alpha").Tickets[0]
	if len(tk.Agents) != 0 {
		t.Errorf("agents = %+v, want none without a worker", tk.Agents)
	}
	if tk.Title == "" || tk.Body == "" {
		t.Errorf("a ticket with no agents still carries its Markdown: %+v", tk)
	}
}

// A Reader polling across ticks accumulates appended usage without
// double-counting: the second snapshot holds both batches' sums, and an
// agent whose rows appear later joins with only its own rows.
func TestReaderIncrementalPollsDoNotDoubleCount(t *testing.T) {
	stateDir, repo := costsFixture(t)
	r := new(Reader)
	herdr := func() (map[string]string, error) { return costsLive(), nil }

	first, err := r.Snapshot(stateDir, repo, herdr)
	if err != nil {
		t.Fatalf("first Snapshot: %v", err)
	}
	before := findSpec(t, first, repo, "alpha").Tickets[0].Totals.Input

	writeUsageRows(t, stateDir, []statedb.UsageRow{
		{Agent: "01-one", At: "2026-10-06T11:00:00Z", Harness: "pig", Provider: "p", Model: "opus", Kind: statedb.KindRequest, Input: 50, Output: 10, ReportedCost: costp(0.005)},
	})

	second, err := r.Snapshot(stateDir, repo, herdr)
	if err != nil {
		t.Fatalf("second Snapshot: %v", err)
	}
	after := findSpec(t, second, repo, "alpha").Tickets[0]
	if after.Totals.Input != before+50 {
		t.Errorf("input after append = %d, want first %d plus the new 50", after.Totals.Input, before)
	}
	if after.Agents[0].FindCalls != 2 {
		t.Errorf("find calls after append = %d, want the same 2, not 4", after.Agents[0].FindCalls)
	}

	// A fresh one-shot Snapshot over the same database agrees with the
	// incremental Reader: neither double-counts.
	fresh, err := Snapshot(stateDir, repo, herdr)
	if err != nil {
		t.Fatalf("fresh Snapshot: %v", err)
	}
	freshTotals := findSpec(t, fresh, repo, "alpha").Tickets[0].Totals
	if freshTotals != after.Totals {
		t.Errorf("incremental %+v != one-shot %+v", after.Totals, freshTotals)
	}
}

// Overlapping poll loads share one live Reader: concurrent Snapshots
// serialize on its mutex instead of racing on the high-water mark. Run with
// -race: without the lock this fails on concurrent map access.
func TestReaderConcurrentSnapshotsDoNotRace(t *testing.T) {
	stateDir, repo := costsFixture(t)
	r := new(Reader)
	herdr := func() (map[string]string, error) { return costsLive(), nil }

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := r.Snapshot(stateDir, repo, herdr); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()

	snap, err := r.Snapshot(stateDir, repo, herdr)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if got := findSpec(t, snap, repo, "alpha").Tickets[0].Totals.Input; got != 300 {
		t.Errorf("input after concurrent polls = %d, want 300 counted once", got)
	}
}

// Appending a claude cost-state snapshot moves the reported cost to the
// new cumulative total instead of adding snapshots together.
func TestReaderIncrementalCostSnapshotReplaces(t *testing.T) {
	stateDir, repo := costsFixture(t)
	r := new(Reader)
	herdr := func() (map[string]string, error) { return costsLive(), nil }

	if _, err := r.Snapshot(stateDir, repo, herdr); err != nil {
		t.Fatalf("first Snapshot: %v", err)
	}
	// A newer cost-state for the same session start replaces the opus
	// model row and leaves the sonnet row alone: 6.25 + 0.07.
	writeUsageRows(t, stateDir, []statedb.UsageRow{
		{Agent: "orch", At: "2026-10-06T09:00:00Z", Harness: "claude", Provider: "anthropic", Model: "claude-opus-5", Kind: statedb.KindCost, Input: 3, Output: 500, CacheRead: 18000, Reasoning: 180, ReportedCost: costp(6.25)},
	})

	second, err := r.Snapshot(stateDir, repo, herdr)
	if err != nil {
		t.Fatalf("second Snapshot: %v", err)
	}
	orch := findSpec(t, second, repo, "alpha").Orchestrator
	if orch == nil || !orch.HasCost || !approx(orch.Cost, 6.32) {
		t.Errorf("orchestrator cost = %+v, want replaced opus 6.25 + kept sonnet 0.07", orch)
	}
	// Request tokens still sum: the original request plus nothing new.
	if orch.Input != 2 {
		t.Errorf("orchestrator input = %d, want 2 (no new request rows)", orch.Input)
	}
}

// A deleted and recreated database restarts row ids at 1: the next poll
// notices the file identity changed and starts the accumulation over instead
// of skipping every row — even when the new file already reaches past the
// old high-water mark.
func TestReaderRestartsWhenDatabaseIsRecreated(t *testing.T) {
	stateDir, repo := costsFixture(t)
	r := new(Reader)
	herdr := func() (map[string]string, error) { return costsLive(), nil }

	first, err := r.Snapshot(stateDir, repo, herdr)
	if err != nil {
		t.Fatalf("first Snapshot: %v", err)
	}
	if got := findSpec(t, first, repo, "alpha").Tickets[0].Totals.Input; got != 300 {
		t.Fatalf("input = %d, want the fixture's 300", got)
	}
	for _, p := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(statedb.DBPathForDir(stateDir) + p); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	writeUsageRows(t, stateDir, []statedb.UsageRow{
		{Agent: "01-one", At: "2026-10-06T12:00:00Z", Harness: "pig", Provider: "p", Model: "opus", Kind: statedb.KindRequest, Input: 9, Output: 1},
	})

	second, err := r.Snapshot(stateDir, repo, herdr)
	if err != nil {
		t.Fatalf("second Snapshot: %v", err)
	}
	if got := findSpec(t, second, repo, "alpha").Tickets[0].Totals.Input; got != 9 {
		t.Errorf("input after recreate = %d, want only the new database's 9", got)
	}
	// Recreate once more, this time already past the old mark (ids 1-3
	// against a mark of 1): without the identity check the first row would
	// be skipped as already seen.
	for _, p := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(statedb.DBPathForDir(stateDir) + p); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	writeUsageRows(t, stateDir, []statedb.UsageRow{
		{Agent: "01-one", At: "2026-10-06T13:00:00Z", Harness: "pig", Provider: "p", Model: "opus", Kind: statedb.KindRequest, Input: 7, Output: 1},
		{Agent: "01-one", At: "2026-10-06T13:01:00Z", Harness: "pig", Provider: "p", Model: "opus", Kind: statedb.KindRequest, Input: 7, Output: 1},
		{Agent: "01-one", At: "2026-10-06T13:02:00Z", Harness: "pig", Provider: "p", Model: "opus", Kind: statedb.KindRequest, Input: 7, Output: 1},
	})

	third, err := r.Snapshot(stateDir, repo, herdr)
	if err != nil {
		t.Fatalf("third Snapshot: %v", err)
	}
	if got := findSpec(t, third, repo, "alpha").Tickets[0].Totals.Input; got != 21 {
		t.Errorf("input after second recreate = %d, want all three new rows' 21", got)
	}
}
