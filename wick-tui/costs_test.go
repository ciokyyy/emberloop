package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeLines writes each string as its own line in path, creating parents.
func writeLines(t *testing.T, path string, lines []string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// approx reports whether two reported costs agree to a tenth of a cent:
// float64 cannot hold decimal sums exactly, so tests never compare them
// with ==.
func approx(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 0.0001
}

// costsFixture builds a state dir with one spec (alpha), one ticket with a
// pig worker, and an orchestrator: each with a usage log and a memory log.
// The worker's log holds two request rows (costed), one find row with usage,
// and one find row without; the orchestrator's holds one claude request row
// (never costed) plus two cost-state snapshots where only the latest counts.
func costsFixture(t *testing.T) (stateDir, repo string) {
	t.Helper()
	stateDir = t.TempDir()
	repo = t.TempDir()
	writeSpec(t, repo, "alpha", []ticketFile{{"01-one.md", "ready-for-agent"}})
	specPath := filepath.Join(repo, ".scratch", "alpha", "spec.md")
	issues := filepath.Join(repo, ".scratch", "alpha", "issues")
	wt := filepath.Join(repo, ".wick-worktrees", "01-one")

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
			"agent_id": "orch", "pane": "pane-orch",
			"harness": "claude", "model": "opus", "effort": "high",
		},
	})

	writeLines(t, filepath.Join(stateDir, "usage", "01-one.jsonl"), []string{
		`{"agent":"01-one","at":"2026-10-06T10:00:00Z","harness":"pig","provider":"p","model":"opus","input":100,"output":50,"cacheRead":10,"cacheWrite":20,"reasoning":5,"reportedCost":0.01,"kind":"request"}`,
		`{"agent":"01-one","at":"2026-10-06T10:01:00Z","harness":"pig","provider":"p","model":"opus","input":200,"output":70,"cacheRead":30,"cacheWrite":40,"reasoning":7,"reportedCost":0.02,"kind":"request"}`,
		`{"agent":"01-one","at":"2026-10-06T10:02:00Z","kind":"find","model":"typesafe/jev-1.13","find_call_id":"f1","questions":2,"usage":{"input_tokens":278,"output_tokens":21,"cost":0.000011676}}`,
		`{"agent":"01-one","at":"2026-10-06T10:03:00Z","kind":"find","model":"typesafe/jev-1.13","find_call_id":"f2","questions":1}`,
	})
	// Cost-state rows carry the session start as At (the core's cost-state
	// line has no timestamp), so both snapshots share one At: the second
	// replaces the first per model, and its two models sum.
	writeLines(t, filepath.Join(stateDir, "usage", "orch.jsonl"), []string{
		`{"agent":"orch","at":"2026-10-06T10:00:00Z","harness":"claude","provider":"anthropic","model":"claude-opus-5","input":2,"output":474,"cacheRead":0,"cacheWrite":17838,"reasoning":170,"kind":"request"}`,
		`{"agent":"orch","at":"2026-10-06T09:00:00Z","harness":"claude","provider":"anthropic","model":"claude-opus-5","input":1,"output":10,"cacheRead":0,"cacheWrite":100,"reasoning":0,"reportedCost":1.5,"kind":"cost"}`,
		`{"agent":"orch","at":"2026-10-06T09:00:00Z","harness":"claude","provider":"anthropic","model":"claude-opus-5","input":2,"output":474,"cacheRead":0,"cacheWrite":17838,"reasoning":170,"reportedCost":5.0,"kind":"cost"}`,
		`{"agent":"orch","at":"2026-10-06T09:00:00Z","harness":"claude","provider":"anthropic","model":"claude-sonnet-4","input":0,"output":0,"cacheRead":0,"cacheWrite":0,"reasoning":0,"reportedCost":0.07,"kind":"cost"}`,
	})

	writeLines(t, filepath.Join(stateDir, "archive", "alpha", "01-one", "memory.jsonl"), []string{
		`{"agent":"01-one","at":"2026-10-06T10:00:00Z","pid":10,"name":"pig","rss":1000}`,
		`{"agent":"01-one","at":"2026-10-06T10:05:00Z","pid":10,"name":"pig","rss":2000}`,
		`{"agent":"01-one","at":"2026-10-06T10:05:00Z","pid":11,"name":"wick-mcp","rss":500}`,
	})
	writeLines(t, filepath.Join(stateDir, "archive", "alpha", "orch", "memory.jsonl"), []string{
		`{"agent":"orch","at":"2026-10-06T10:05:00Z","pid":20,"name":"claude","rss":4000}`,
	})
	return stateDir, repo
}

func costsLive() map[string]string {
	return map[string]string{"pane-w1": "working", "pane-orch": "working"}
}

func TestSnapshotTicketAgentsCarryUsageTotals(t *testing.T) {
	stateDir, repo := costsFixture(t)

	snap, err := Snapshot(stateDir, repo, func() (map[string]string, error) { return costsLive(), nil })
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	alpha := findSpec(t, snap, repo, "alpha")
	if len(alpha.Tickets) != 1 {
		t.Fatalf("tickets = %+v, want the one ticket", alpha.Tickets)
	}
	tk := alpha.Tickets[0]
	if len(tk.Agents) != 1 {
		t.Fatalf("agents = %+v, want the worker", tk.Agents)
	}
	w := tk.Agents[0]
	if w.Kind != AgentWorker || w.Harness != "pig" || w.Model != "opus" || w.Effort != "high" {
		t.Errorf("worker identity = %+v, want worker pig opus high", w)
	}
	if w.ModelEffort() != "opus · high" {
		t.Errorf("model·effort = %q, want %q", w.ModelEffort(), "opus · high")
	}
	if w.Input != 300 || w.Output != 120 || w.CacheRead != 40 || w.CacheWrite != 60 || w.Reasoning != 12 {
		t.Errorf("worker tokens = %d/%d/%d/%d/%d, want 300/120/40/60/12",
			w.Input, w.Output, w.CacheRead, w.CacheWrite, w.Reasoning)
	}
	if !w.HasCost || !approx(w.Cost, 0.03) {
		t.Errorf("worker cost = %v/%v, want 0.03 reported", w.Cost, w.HasCost)
	}
	if w.FindCalls != 2 {
		t.Errorf("worker find calls = %d, want 2", w.FindCalls)
	}
	if w.FindInput != 278 || w.FindOutput != 21 || !w.HasFindCost || !approx(w.FindCost, 0.000011676) {
		t.Errorf("worker find usage = %d/%d/%v/%v, want 278/21/costed 0.000011676",
			w.FindInput, w.FindOutput, w.FindCost, w.HasFindCost)
	}
	if !w.Alive || w.Status != "working" {
		t.Errorf("worker liveness = %v/%q, want alive working", w.Alive, w.Status)
	}

	if tk.Totals.Input != 300 || !approx(tk.Totals.Cost, 0.03) || !tk.Totals.HasCost {
		t.Errorf("ticket totals = %+v, want the worker's 300 tokens and 0.03", tk.Totals)
	}
	if tk.Totals.FindCalls != 2 {
		t.Errorf("ticket find calls = %d, want 2", tk.Totals.FindCalls)
	}
}

func TestSnapshotOrchestratorCostUsesLatestCostSnapshot(t *testing.T) {
	stateDir, repo := costsFixture(t)

	snap, err := Snapshot(stateDir, repo, func() (map[string]string, error) { return costsLive(), nil })
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	alpha := findSpec(t, snap, repo, "alpha")
	if alpha.Orchestrator == nil {
		t.Fatalf("orchestrator = nil, want the run's agent: %+v", alpha)
	}
	o := alpha.Orchestrator
	if o.Kind != AgentOrchestrator || o.Harness != "claude" {
		t.Errorf("orchestrator = %+v, want orchestrator claude", o)
	}
	// Tokens come from the single request row; cost comes from the latest
	// cost-state snapshot only, never the sum of snapshots.
	if o.Input != 2 || o.Output != 474 || o.CacheWrite != 17838 || o.Reasoning != 170 {
		t.Errorf("orchestrator tokens = %d/%d/%d/%d, want 2/474/17838/170",
			o.Input, o.Output, o.CacheWrite, o.Reasoning)
	}
	if !o.HasCost || !approx(o.Cost, 5.07) {
		t.Errorf("orchestrator cost = %v/%v, want latest snapshot 5.07", o.Cost, o.HasCost)
	}

	// Per-spec totals include the orchestrator beside the ticket agents.
	if alpha.Totals.Input != 302 {
		t.Errorf("spec input = %d, want worker 300 + orch 2", alpha.Totals.Input)
	}
	if !alpha.Totals.HasCost || !approx(alpha.Totals.Cost, 5.10) {
		t.Errorf("spec cost = %v/%v, want 0.03 + 5.07", alpha.Totals.Cost, alpha.Totals.HasCost)
	}
}

func TestSnapshotMemoryLatestSampleAndMachineTotal(t *testing.T) {
	stateDir, repo := costsFixture(t)

	snap, err := Snapshot(stateDir, repo, func() (map[string]string, error) { return costsLive(), nil })
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	alpha := findSpec(t, snap, repo, "alpha")
	w := alpha.Tickets[0].Agents[0]
	// Latest sample only: the older 1000-byte sample is superseded.
	if !w.HasRSS || w.RSS != 2500 {
		t.Errorf("worker RSS = %d/%v, want latest sample 2000+500", w.RSS, w.HasRSS)
	}
	if alpha.Orchestrator == nil || !alpha.Orchestrator.HasRSS || alpha.Orchestrator.RSS != 4000 {
		t.Errorf("orchestrator RSS = %+v, want 4000", alpha.Orchestrator)
	}
	// Machine-wide total covers every running agent: worker plus orchestrator.
	if snap.MachineRSS != 6500 {
		t.Errorf("machine RSS = %d, want 2500 + 4000", snap.MachineRSS)
	}
}

func TestSnapshotMissingCostReadsNoCostReported(t *testing.T) {
	stateDir := t.TempDir()
	repo := t.TempDir()
	writeSpec(t, repo, "alpha", []ticketFile{{"01-one.md", "ready-for-agent"}})
	specPath := filepath.Join(repo, ".scratch", "alpha", "spec.md")
	issues := filepath.Join(repo, ".scratch", "alpha", "issues")
	writeJSON(t, filepath.Join(stateDir, "workers.json"), map[string]any{
		"01-one": map[string]any{
			"id": "01-one", "ticket": filepath.Join(issues, "01-one.md"),
			"spec": specPath, "repo": repo, "pane": "pane-w1",
			"harness": "pig", "model": "opus",
		},
	})
	writeLines(t, filepath.Join(stateDir, "usage", "01-one.jsonl"), []string{
		`{"agent":"01-one","at":"2026-10-06T10:00:00Z","harness":"pig","provider":"p","model":"opus","input":100,"output":50,"cacheRead":0,"cacheWrite":0,"reasoning":0,"kind":"request"}`,
	})

	snap, err := Snapshot(stateDir, repo, func() (map[string]string, error) {
		return map[string]string{"pane-w1": "working"}, nil
	})
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	w := findSpec(t, snap, repo, "alpha").Tickets[0].Agents[0]
	if w.HasCost {
		t.Errorf("worker cost = %v, want no cost reported", w.Cost)
	}
	if w.Input != 100 {
		t.Errorf("tokens without cost must still count: input = %d, want 100", w.Input)
	}
}
