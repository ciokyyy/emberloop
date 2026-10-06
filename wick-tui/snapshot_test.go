package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// snapshotFixture builds a state dir and two repos:
// repoA (the current repo) holds specs alpha (running, 1/2 merged) and beta
// (no run yet, 0/1); repoB holds gamma (ended, 2/2). The fake herdr reports
// only alpha's orchestrator pane alive.
func snapshotFixture(t *testing.T) (stateDir, repoA, repoB string) {
	t.Helper()
	stateDir = t.TempDir()
	repoA = t.TempDir()
	repoB = t.TempDir()

	writeSpec(t, repoA, "alpha", []ticketFile{
		{"01-one.md", "done"},
		{"02-two.md", "ready-for-agent"},
	})
	writeSpec(t, repoA, "beta", []ticketFile{
		{"01-only.md", "ready-for-agent"},
	})
	writeSpec(t, repoB, "gamma", []ticketFile{
		{"01-a.md", "done"},
		{"02-b.md", "done"},
	})

	runs := map[string]any{
		"alpha": map[string]any{
			"slug": "alpha", "spec": filepath.Join(repoA, ".scratch", "alpha", "spec.md"),
			"repo": repoA, "branch": "spec/alpha",
			"agent_id": "orch-alpha", "pane": "w1:alive", "harness": "pig",
		},
		"gamma": map[string]any{
			"slug": "gamma", "spec": filepath.Join(repoB, ".scratch", "gamma", "spec.md"),
			"repo": repoB, "branch": "spec/gamma",
			"agent_id": "orch-gamma", "pane": "w1:dead", "harness": "pig",
		},
	}
	raw, err := json.Marshal(runs)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "runs.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	history := []any{
		map[string]any{"ticket": "01-one.md", "outcome": "merged"},
	}
	raw, err = json.Marshal(history)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(stateDir, "history"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "history", "alpha.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "history", "gamma.json"), []byte("[]"), 0o644); err != nil {
		t.Fatal(err)
	}
	return stateDir, repoA, repoB
}

type ticketFile struct {
	name   string
	status string
}

func writeSpec(t *testing.T, repo, slug string, tickets []ticketFile) {
	t.Helper()
	dir := filepath.Join(repo, ".scratch", slug)
	issues := filepath.Join(dir, "issues")
	if err := os.MkdirAll(issues, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "spec.md"), []byte("# "+slug+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tf := range tickets {
		body := "# " + tf.name + "\n\n**Status:** " + tf.status + "\n"
		if err := os.WriteFile(filepath.Join(issues, tf.name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// liveHerdr is the fake herdr: exactly the panes named alive are running,
// with no agent status. Tests needing working/idle build their own map.
func liveHerdr(alive ...string) Herdr {
	set := map[string]string{}
	for _, p := range alive {
		set[p] = ""
	}
	return func() (map[string]string, error) { return set, nil }
}

func findSpec(t *testing.T, snap Snap, repo, slug string) SpecSnap {
	t.Helper()
	for _, r := range snap.Repos {
		if r.Path != repo {
			continue
		}
		for _, s := range r.Specs {
			if s.Slug == slug {
				return s
			}
		}
	}
	t.Fatalf("spec %s/%s not in snapshot %+v", repo, slug, snap)
	return SpecSnap{}
}

func TestSnapshotListsSpecsGroupedByRepoCurrentFirst(t *testing.T) {
	stateDir, repoA, repoB := snapshotFixture(t)

	snap, err := Snapshot(stateDir, repoA, liveHerdr("w1:alive"))
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(snap.Repos) != 2 {
		t.Fatalf("repos = %+v, want 2", snap.Repos)
	}
	if snap.Repos[0].Path != repoA {
		t.Errorf("first repo = %q, want current repo %q", snap.Repos[0].Path, repoA)
	}
	if snap.Repos[1].Path != repoB {
		t.Errorf("second repo = %q, want %q", snap.Repos[1].Path, repoB)
	}

	alpha := findSpec(t, snap, repoA, "alpha")
	if !alpha.HasRun || !alpha.Running {
		t.Errorf("alpha must have a run and be running: %+v", alpha)
	}
	if alpha.Merged != 1 || alpha.Total != 2 {
		t.Errorf("alpha progress = %d/%d, want 1/2: %+v", alpha.Merged, alpha.Total, alpha)
	}

	beta := findSpec(t, snap, repoA, "beta")
	if beta.HasRun || beta.Running {
		t.Errorf("beta has no run and must read ended: %+v", beta)
	}
	if beta.Merged != 0 || beta.Total != 1 {
		t.Errorf("beta progress = %d/%d, want 0/1: %+v", beta.Merged, beta.Total, beta)
	}

	gamma := findSpec(t, snap, repoB, "gamma")
	if !gamma.HasRun || gamma.Running {
		t.Errorf("gamma's pane is gone so it must read ended: %+v", gamma)
	}
	if gamma.Merged != 2 || gamma.Total != 2 {
		t.Errorf("gamma progress = %d/%d, want 2/2: %+v", gamma.Merged, gamma.Total, gamma)
	}
}

func TestSnapshotWithNoStateListsOnlyCurrentRepoSpecs(t *testing.T) {
	repo := t.TempDir()
	writeSpec(t, repo, "solo", []ticketFile{{"01-x.md", "ready-for-agent"}})

	snap, err := Snapshot(filepath.Join(t.TempDir(), "missing-state"), repo, liveHerdr())
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(snap.Repos) != 1 || snap.Repos[0].Path != repo {
		t.Fatalf("repos = %+v, want only the current repo", snap.Repos)
	}
	solo := findSpec(t, snap, repo, "solo")
	if solo.HasRun || solo.Running {
		t.Errorf("a spec with no record must read ended: %+v", solo)
	}
	if solo.Merged != 0 || solo.Total != 1 {
		t.Errorf("solo progress = %d/%d, want 0/1", solo.Merged, solo.Total)
	}
}

func TestSnapshotKeepsARunWhoseSpecFileIsGone(t *testing.T) {
	stateDir := t.TempDir()
	repo := t.TempDir()
	runs := map[string]any{
		"gone": map[string]any{
			"slug": "gone", "spec": filepath.Join(repo, ".scratch", "gone", "spec.md"),
			"repo": repo, "branch": "spec/gone",
			"agent_id": "orch-gone", "pane": "w1:alive", "harness": "pig",
		},
	}
	raw, _ := json.Marshal(runs)
	if err := os.WriteFile(filepath.Join(stateDir, "runs.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	snap, err := Snapshot(stateDir, repo, liveHerdr("w1:alive"))
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	gone := findSpec(t, snap, repo, "gone")
	if !gone.HasRun || !gone.Running {
		t.Errorf("a recorded run must stay listed and running: %+v", gone)
	}
	if gone.Merged != 0 || gone.Total != 0 {
		t.Errorf("gone progress = %d/%d, want 0/0", gone.Merged, gone.Total)
	}
}

// A run recorded from a subdirectory (implement's cwd) groups with its
// checkout: the spec file decides the repo, so the spec lists once, under
// the current repo, with its run. A symlinked current repo meets the record
// on the normalized path, and an oddly named spec directory meets its record
// on the slugified key.
func TestSnapshotGroupsASubdirectoryRunWithItsCheckout(t *testing.T) {
	stateDir := t.TempDir()
	repo := t.TempDir()
	writeSpec(t, repo, "alpha", []ticketFile{{"01-one.md", "ready-for-agent"}})
	writeSpec(t, repo, "My Spec!", []ticketFile{{"01-one.md", "ready-for-agent"}})
	sub := filepath.Join(repo, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(repo, link); err != nil {
		t.Fatal(err)
	}
	runs := map[string]any{
		"alpha": map[string]any{
			"slug": "alpha", "spec": filepath.Join(repo, ".scratch", "alpha", "spec.md"),
			"repo": sub, "branch": "spec/alpha",
			"agent_id": "orch-alpha", "pane": "w1:alive", "harness": "pig",
		},
		"my-spec": map[string]any{
			"slug": "my-spec", "spec": filepath.Join(repo, ".scratch", "My Spec!", "spec.md"),
			"repo": repo, "branch": "spec/my-spec",
			"agent_id": "orch-mine", "pane": "w1:alive", "harness": "pig",
		},
	}
	raw, _ := json.Marshal(runs)
	if err := os.WriteFile(filepath.Join(stateDir, "runs.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	snap, err := Snapshot(stateDir, link, liveHerdr("w1:alive"))
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(snap.Repos) != 1 {
		t.Fatalf("repos = %+v, want the single checkout", snap.Repos)
	}
	if snap.Repos[0].Path != repo {
		t.Errorf("repo = %q, want normalized %q", snap.Repos[0].Path, repo)
	}
	alpha := findSpec(t, snap, repo, "alpha")
	if !alpha.HasRun || !alpha.Running {
		t.Errorf("alpha must carry its run: %+v", alpha)
	}
	mine := findSpec(t, snap, repo, "my-spec")
	if !mine.HasRun {
		t.Errorf("the oddly named dir must meet its record: %+v", mine)
	}
	if len(snap.Repos[0].Specs) != 2 {
		t.Errorf("specs = %+v, want alpha and my-spec once each", snap.Repos[0].Specs)
	}
}

// Re-homing is not current-repo-only: a run launched from a subdirectory of
// a repo groups with that repo's root even when another repo is current, so
// no spec lists twice.
func TestSnapshotRehomesSubdirectoryRunsOfOtherRepos(t *testing.T) {
	stateDir := t.TempDir()
	repoA := t.TempDir()
	repoC := t.TempDir()
	writeSpec(t, repoA, "alpha", []ticketFile{{"01-one.md", "ready-for-agent"}})
	writeSpec(t, repoA, "beta", []ticketFile{{"01-one.md", "ready-for-agent"}})
	sub := filepath.Join(repoA, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	runs := map[string]any{
		"alpha": map[string]any{
			"slug": "alpha", "spec": filepath.Join(repoA, ".scratch", "alpha", "spec.md"),
			"repo": repoA, "branch": "spec/alpha",
			"agent_id": "o1", "pane": "w1:alive", "harness": "pig",
		},
		"beta": map[string]any{
			"slug": "beta", "spec": filepath.Join(repoA, ".scratch", "beta", "spec.md"),
			"repo": sub, "branch": "spec/beta",
			"agent_id": "o2", "pane": "w1:alive", "harness": "pig",
		},
	}
	raw, _ := json.Marshal(runs)
	if err := os.WriteFile(filepath.Join(stateDir, "runs.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	snap, err := Snapshot(stateDir, repoC, liveHerdr("w1:alive"))
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(snap.Repos) != 2 {
		t.Fatalf("repos = %+v, want repoA and the current repo", snap.Repos)
	}
	if snap.Repos[0].Path != repoC {
		t.Errorf("first repo = %q, want current repo %q", snap.Repos[0].Path, repoC)
	}
	if got := len(snap.Repos[1].Specs); got != 2 {
		t.Fatalf("repoA specs = %+v, want alpha and beta once each", snap.Repos[1].Specs)
	}
	for _, slug := range []string{"alpha", "beta"} {
		if s := findSpec(t, snap, repoA, slug); !s.HasRun {
			t.Errorf("%s must carry its run: %+v", slug, s)
		}
	}
}

// Even when every run of a repo was recorded from subdirectories — no root
// candidate anywhere — the standard spec layout still homes them, so the
// repo groups and its runless specs are discovered.
func TestSnapshotHomesRunsWithNoRootCandidate(t *testing.T) {
	stateDir := t.TempDir()
	repoA := t.TempDir()
	repoC := t.TempDir()
	writeSpec(t, repoA, "alpha", []ticketFile{{"01-one.md", "ready-for-agent"}})
	writeSpec(t, repoA, "beta", []ticketFile{{"01-one.md", "ready-for-agent"}})
	sub := filepath.Join(repoA, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	runs := map[string]any{
		"alpha": map[string]any{
			"slug": "alpha", "spec": filepath.Join(repoA, ".scratch", "alpha", "spec.md"),
			"repo": sub, "branch": "spec/alpha",
			"agent_id": "o1", "pane": "w1:alive", "harness": "pig",
		},
	}
	raw, _ := json.Marshal(runs)
	if err := os.WriteFile(filepath.Join(stateDir, "runs.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	snap, err := Snapshot(stateDir, repoC, liveHerdr("w1:alive"))
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	alpha := findSpec(t, snap, repoA, "beta")
	if alpha.Total != 1 {
		t.Errorf("beta must be discovered under the derived root: %+v", alpha)
	}
	if s := findSpec(t, snap, repoA, "alpha"); !s.HasRun {
		t.Errorf("alpha must carry its run: %+v", s)
	}
}

// Every ticket status from its fixture signals: one spec with ten tickets,
// each driven by a different combination of worker entries, herdr state,
// submits, reviewer records and history. The derivation lives in one function
// inside Snapshot.
func TestSnapshotTicketsDeriveEveryStatus(t *testing.T) {
	stateDir := t.TempDir()
	repo := t.TempDir()
	tickets := []ticketFile{
		{"01-notstarted.md", "ready-for-agent"},
		{"02-working.md", "ready-for-agent"},
		{"03-retrying.md", "ready-for-agent"},
		{"04-idle.md", "ready-for-agent"},
		{"05-submitted-done.md", "ready-for-agent"},
		{"06-submitted-blocked.md", "ready-for-agent"},
		{"07-inreview.md", "ready-for-agent"},
		{"08-merged.md", "ready-for-agent"},
		{"09-discarded.md", "ready-for-agent"},
		{"10-failed.md", "ready-for-agent"},
	}
	writeSpec(t, repo, "alpha", tickets)
	specPath := filepath.Join(repo, ".scratch", "alpha", "spec.md")
	issues := filepath.Join(repo, ".scratch", "alpha", "issues")

	workers := map[string]any{}
	addWorker := func(id, ticket, pane, worktree string) {
		workers[id] = map[string]any{
			"id": id, "ticket": filepath.Join(issues, ticket),
			"spec": specPath, "repo": repo, "branch": "implement/" + id,
			"worktree": worktree, "pane": pane,
			"harness": "pig",
		}
	}
	wt := func(id string) string { return filepath.Join(repo, ".wick-worktrees", id) }
	addWorker("02-working", "02-working.md", "pane-working", wt("02-working"))
	addWorker("03-retrying", "03-retrying.md", "pane-retrying", wt("03-retrying"))
	addWorker("04-idle", "04-idle.md", "pane-idle", wt("04-idle"))
	addWorker("05-submitted-done", "05-submitted-done.md", "pane-subdone", wt("05-submitted-done"))
	addWorker("06-submitted-blocked", "06-submitted-blocked.md", "pane-subblocked", wt("06-submitted-blocked"))
	addWorker("07-inreview", "07-inreview.md", "pane-inreview", wt("07-inreview"))
	addWorker("10-failed", "10-failed.md", "pane-gone", wt("10-failed"))
	writeJSON(t, filepath.Join(stateDir, "workers.json"), workers)

	runs := map[string]any{
		"alpha": map[string]any{
			"slug": "alpha", "spec": specPath, "repo": repo, "branch": "spec/alpha",
			"agent_id": "orch-alpha", "pane": "pane-orch", "harness": "pig",
		},
	}
	writeJSON(t, filepath.Join(stateDir, "runs.json"), runs)

	writeJSON(t, filepath.Join(stateDir, "submits", "05-submitted-done.json"), map[string]any{
		"agent_id": "05-submitted-done", "type": "worker", "status": "done",
	})
	writeJSON(t, filepath.Join(stateDir, "submits", "06-submitted-blocked.json"), map[string]any{
		"agent_id": "06-submitted-blocked", "type": "worker", "status": "blocked",
	})

	writeJSON(t, filepath.Join(stateDir, "reviewers.json"), map[string]any{
		filepath.Clean(wt("07-inreview")): map[string]any{
			"reviewer-standards": map[string]any{"pane": "pane-rs", "session_id": "rs1", "verdict": "pass"},
			"reviewer-spec":      map[string]any{"pane": "pane-rp", "session_id": "rp1", "verdict": "changes"},
		},
	})

	if err := os.MkdirAll(filepath.Join(stateDir, "history"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(stateDir, "history", "alpha.json"), []any{
		map[string]any{"ticket": "08-merged.md", "outcome": "merged"},
		map[string]any{"ticket": "09-discarded.md", "outcome": "discarded"},
	})

	live := map[string]string{
		"pane-orch": "working", "pane-working": "working", "pane-retrying": "blocked",
		"pane-idle": "idle", "pane-subdone": "idle", "pane-subblocked": "idle",
		"pane-inreview": "working",
	}
	herdr := func() (map[string]string, error) { return live, nil }

	snap, err := Snapshot(stateDir, repo, herdr)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	alpha := findSpec(t, snap, repo, "alpha")
	got := map[string]TicketSnap{}
	for _, tk := range alpha.Tickets {
		got[tk.Name] = tk
	}
	want := map[string]TicketStatus{
		"01-notstarted.md": TicketNotStarted, "02-working.md": TicketWorking,
		"03-retrying.md": TicketRetrying, "04-idle.md": TicketIdle,
		"05-submitted-done.md": TicketSubmittedDone, "06-submitted-blocked.md": TicketSubmittedBlocked,
		"07-inreview.md": TicketInReview, "08-merged.md": TicketMerged,
		"09-discarded.md": TicketDiscarded, "10-failed.md": TicketFailed,
	}
	for name, status := range want {
		tk, ok := got[name]
		if !ok {
			t.Errorf("ticket %s missing, got %+v", name, alpha.Tickets)
			continue
		}
		if tk.Status != status {
			t.Errorf("ticket %s status = %q, want %q", name, tk.Status, status)
		}
	}
	if tk := got["07-inreview.md"]; tk.CodeVerdict != "pass" || tk.SpecVerdict != "changes" {
		t.Errorf("in-review verdicts = %q/%q, want pass/changes", tk.CodeVerdict, tk.SpecVerdict)
	}
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

// A ticket whose worker's pane was closed by hand shows cancelled: its
// history holds a cancelled entry and its worker entry is gone.
func TestCancelledTicketShowsCancelledStatus(t *testing.T) {
	stateDir := t.TempDir()
	repo := t.TempDir()
	writeSpec(t, repo, "alpha", []ticketFile{{"11-cancelled.md", "ready-for-agent"}})
	writeJSON(t, filepath.Join(stateDir, "history", "alpha.json"), []any{
		map[string]any{
			"ticket":  filepath.Join(repo, ".scratch", "alpha", "issues", "11-cancelled.md"),
			"outcome": "cancelled", "detail": "pane closed manually",
			"branch": "implement/11-cancelled", "worker": "11-cancelled",
		},
	})
	snap, err := Snapshot(stateDir, repo, liveHerdr())
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	alpha := findSpec(t, snap, repo, "alpha")
	if len(alpha.Tickets) != 1 {
		t.Fatalf("tickets = %+v, want one", alpha.Tickets)
	}
	if got := alpha.Tickets[0].Status; got != TicketCancelled {
		t.Errorf("cancelled ticket status = %q, want %q", got, TicketCancelled)
	}
}

// A cancelled ticket that was respawned follows its live worker, not its
// history: the cancellation loses to a running worker entry.
func TestRespawnedTicketBeatsCancelledHistory(t *testing.T) {
	stateDir := t.TempDir()
	repo := t.TempDir()
	writeSpec(t, repo, "alpha", []ticketFile{{"11-cancelled.md", "ready-for-agent"}})
	specPath := filepath.Join(repo, ".scratch", "alpha", "spec.md")
	issues := filepath.Join(repo, ".scratch", "alpha", "issues")
	writeJSON(t, filepath.Join(stateDir, "history", "alpha.json"), []any{
		map[string]any{
			"ticket": filepath.Join(issues, "11-cancelled.md"), "outcome": "cancelled",
		},
	})
	writeJSON(t, filepath.Join(stateDir, "workers.json"), map[string]any{
		"11-cancelled": map[string]any{
			"id": "11-cancelled", "ticket": filepath.Join(issues, "11-cancelled.md"),
			"spec": specPath, "pane": "pane-live", "worktree": "/tmp/wt",
		},
	})
	snap, err := Snapshot(stateDir, repo, liveHerdr("pane-live"))
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	alpha := findSpec(t, snap, repo, "alpha")
	if len(alpha.Tickets) != 1 {
		t.Fatalf("tickets = %+v, want one", alpha.Tickets)
	}
	if got := alpha.Tickets[0].Status; got == TicketCancelled {
		t.Errorf("a respawned ticket must follow its live worker, got %q", got)
	}
}
