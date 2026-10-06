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

// liveHerdr is the fake herdr: exactly the panes named alive are running.
func liveHerdr(alive ...string) Herdr {
	set := map[string]bool{}
	for _, p := range alive {
		set[p] = true
	}
	return func() (map[string]bool, error) { return set, nil }
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
