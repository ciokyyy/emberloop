package wick

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Finished-ticket history, one entry per finish call, kept under the ticket's
// run so merged work and its cost stay visible after the worker entry is gone.
// Finish writes the entry before deleting the worker entry, copying the
// reviewer verdicts in before the reviewers are forgotten.

// The outcomes a history entry records: merged or discarded (the worker entry
// is deleted after), kept (a refused merge — the worker stays, and the
// entry says why), or cancelled (the worker's pane was closed by hand — the
// entry is gone but its worktree and branch stay for a respawn).
const (
	outcomeMerged    = "merged"
	outcomeDiscarded = "discarded"
	outcomeKept      = "kept"
	outcomeCancelled = "cancelled"
)

// ReviewVerdict is one reviewer's verdict as it stood at finish: what it
// judged, what it said, and who said it, so usage can be summed later.
type ReviewVerdict struct {
	Type      string `json:"type"`
	Verdict   string `json:"verdict,omitempty"`
	VerdictAt string `json:"verdict_at,omitempty"`
	Reviewed  string `json:"reviewed,omitempty"`
	Findings  string `json:"findings,omitempty"`
	AgentID   string `json:"agent_id,omitempty"`
	Harness   string `json:"harness,omitempty"`
	Model     string `json:"model,omitempty"`
	Effort    string `json:"effort,omitempty"`
}

// TicketHistory is one finished ticket: what it was, how it ended, what the
// reviewers said, and whose tokens it spent.
type TicketHistory struct {
	Ticket  string `json:"ticket"`
	Spec    string `json:"spec,omitempty"`
	Outcome string `json:"outcome"`
	// SHA is the merge commit for a merged ticket, empty otherwise.
	SHA    string `json:"sha,omitempty"`
	Detail string `json:"detail,omitempty"`
	Branch string `json:"branch,omitempty"`
	// Worker is the worker's agent id (its ticket id, the WICK_AGENT_ID its
	// usage is logged under).
	Worker  string          `json:"worker,omitempty"`
	Harness string          `json:"harness,omitempty"`
	Model   string          `json:"model,omitempty"`
	Effort  string          `json:"effort,omitempty"`
	Started string          `json:"started,omitempty"`
	Ended   string          `json:"ended,omitempty"`
	Reviews []ReviewVerdict `json:"reviews,omitempty"`
}

// historyPath holds one run's ticket history, beside the worker store.
func historyPath(slug string) string {
	return filepath.Join(filepath.Dir(statePath), "history", slug+".json")
}

func loadHistory(slug string) ([]TicketHistory, error) {
	raw, err := os.ReadFile(historyPath(slug))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []TicketHistory
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("ticket history is corrupt: %w", err)
	}
	return out, nil
}

func saveHistory(slug string, entries []TicketHistory) error {
	path := historyPath(slug)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// historySlug is the run a worker's history is filed under, from the spec it
// was spawned with. A worker with no spec predates runs, and has no run to
// file under.
func historySlug(w *Worker) string {
	if w == nil || w.Spec == "" {
		return ""
	}
	return specSlug(w.Spec)
}

// recordFinish appends one entry for w to its run's history, capturing the
// reviewers as they stand now — before closeReviewers forgets them. The
// verdicts are read with their errors, not through the swallowing reviewersOf:
// a verdict that cannot be read fails the finish and keeps the worker, rather
// than writing an entry with no reviews and then forgetting them. A worker
// with no spec has no run, and records nothing.
func recordFinish(w *Worker, outcome, sha, detail string) error {
	slug := historySlug(w)
	if slug == "" {
		return nil
	}
	have, err := reviewersOfErr(w.Worktree)
	if err != nil {
		return err
	}
	reviews := make([]ReviewVerdict, 0, len(have))
	for typ, r := range have {
		reviews = append(reviews, ReviewVerdict{
			Type: typ, Verdict: r.Verdict, VerdictAt: r.VerdictAt,
			Reviewed: r.Reviewed, Findings: r.Findings, AgentID: r.SessionID,
			Harness: r.Harness, Model: r.Model, Effort: r.Effort,
		})
	}
	sort.Slice(reviews, func(i, j int) bool { return reviews[i].Type < reviews[j].Type })
	entry := TicketHistory{
		Ticket: w.Ticket, Spec: w.Spec, Outcome: outcome, SHA: sha, Detail: detail,
		Branch: w.Branch, Worker: w.ID,
		Harness: w.Harness, Model: w.Model, Effort: w.Effort,
		Started: w.Started, Ended: time.Now().UTC().Format(time.RFC3339),
		Reviews: reviews,
	}
	return withFileLock("history-"+slug, func() error {
		entries, err := loadHistory(slug)
		if err != nil {
			return err
		}
		return saveHistory(slug, append(entries, entry))
	})
}

// ListTicketHistory lists a run's finished tickets, oldest first. slug is the
// run slug, the key FinishRun takes — never a spec path, so there is no
// guessing which of the two the caller meant.
func ListTicketHistory(slug string) ([]TicketHistory, error) {
	entries, err := loadHistory(slug)
	if err != nil {
		return nil, err
	}
	// Stable: entries stamped in the same second keep the order they were
	// recorded in, so a kept entry always reads before the merged one after it.
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Ended != entries[j].Ended {
			return entries[i].Ended < entries[j].Ended
		}
		return entries[i].Ticket < entries[j].Ticket
	})
	return entries, nil
}

// reviewersOfErr is reviewersOf with its errors: the reviewers of the
// worktree at tree, failing on a lock or a read the swallowing reviewersOf
// would hide.
func reviewersOfErr(tree string) (map[string]reviewer, error) {
	var out map[string]reviewer
	err := withFileLock("reviewers", func() error {
		all, err := loadReviewers()
		if err != nil {
			return err
		}
		out = all[filepath.Clean(tree)]
		return nil
	})
	if err != nil {
		return nil, err
	}
	if out == nil {
		return map[string]reviewer{}, nil
	}
	return out, nil
}
