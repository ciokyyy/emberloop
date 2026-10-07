package emberloop

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// RunRecord is one `emberloop implement` run, keyed by spec slug. It names the
// spec, the repo and integration branch, the orchestrator (agent id, pane and
// tab), the harness it runs on, and when it started and — when the session
// exits normally — ended. Readers decide "running" from whether the
// orchestrator pane is alive in herdr, never from Ended alone.
type RunRecord struct {
	Slug    string `json:"slug"`
	Spec    string `json:"spec"`
	Repo    string `json:"repo"`
	Branch  string `json:"branch"`
	AgentID string `json:"agent_id"`
	Pane    string `json:"pane,omitempty"`
	Tab     string `json:"tab,omitempty"`
	Harness string `json:"harness"`
	Model   string `json:"model,omitempty"`
	Effort  string `json:"effort,omitempty"`
	Started string `json:"started"`
	Ended   string `json:"ended,omitempty"`
}

// runsPath holds the run records, beside the worker store.
func runsPath() string { return filepath.Join(filepath.Dir(statePath), "runs.json") }

func loadRuns() (map[string]RunRecord, error) {
	raw, err := os.ReadFile(runsPath())
	if os.IsNotExist(err) {
		return map[string]RunRecord{}, nil
	}
	if err != nil {
		return nil, err
	}
	var out map[string]RunRecord
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("run store is corrupt: %w", err)
	}
	if out == nil {
		out = map[string]RunRecord{}
	}
	return out, nil
}

// saveRuns writes the store through a temp file and a rename, so a reader
// never sees a half-written file.
func saveRuns(store map[string]RunRecord) error {
	if err := os.MkdirAll(filepath.Dir(runsPath()), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	tmp := runsPath() + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, runsPath())
}

// updateRuns runs fn on the store under a cross-process lock. Two implement
// runs for different specs write the same file, so the in-process mutex the
// worker store holds could not stop them dropping each other's record; the
// file lock does.
func updateRuns(fn func(map[string]RunRecord) error) error {
	return withFileLock("runs", func() error {
		store, err := loadRuns()
		if err != nil {
			return err
		}
		if err := fn(store); err != nil {
			return err
		}
		return saveRuns(store)
	})
}

// RecordRun writes the run record for spec, or updates it when the spec
// already has one: re-running implement on the same spec never duplicates.
// Pane comes from the orchestrator's herdr pane; tab prefers HERDR_TAB_ID and
// falls back to the pane list through run, the way scouts place themselves.
func RecordRun(run Exec, spec SpecRun, repo, agentID, harness, model, effort string) (RunRecord, error) {
	slug := specSlug(spec.Spec)
	rec := RunRecord{
		Slug:    slug,
		Spec:    spec.Spec,
		Repo:    repo,
		Branch:  spec.Branch,
		AgentID: agentID,
		Pane:    os.Getenv("HERDR_PANE_ID"),
		Tab:     callerTab(run),
		Harness: harness,
		Model:   model,
		Effort:  effort,
		Started: time.Now().UTC().Format(time.RFC3339),
	}
	if err := updateRuns(func(store map[string]RunRecord) error {
		store[slug] = rec
		return nil
	}); err != nil {
		return RunRecord{}, err
	}
	return rec, nil
}

// FinishRun stamps the run's end time, for a session that exits normally.
// Liveness still comes from the orchestrator pane, not from this field.
func FinishRun(slug string) error {
	return updateRuns(func(store map[string]RunRecord) error {
		rec, ok := store[slug]
		if !ok {
			return fmt.Errorf("unknown run %q", slug)
		}
		rec.Ended = time.Now().UTC().Format(time.RFC3339)
		store[slug] = rec
		return nil
	})
}

// ListRunRecords lists every run record, sorted by slug, for the TUI and stats.
func ListRunRecords() ([]RunRecord, error) {
	store, err := loadRuns()
	if err != nil {
		return nil, err
	}
	out := make([]RunRecord, 0, len(store))
	for _, r := range store {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out, nil
}
