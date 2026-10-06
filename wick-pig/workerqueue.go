package wick

import (
	"encoding/json"
	"example.com/wick/tools"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// queuedWorker is one worktree agent waiting for a worker slot. It stores the
// spawn request as given plus the resolved launch choice, so a later finish
// can start it without the original caller's types.
type queuedWorker struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Ticket   string `json:"ticket"`
	Spec     string `json:"spec,omitempty"`
	Repo     string `json:"repo"`
	Base     string `json:"base,omitempty"`
	Target   string `json:"target,omitempty"`
	Harness  string `json:"harness"`
	Model    string `json:"model,omitempty"`
	Effort   string `json:"effort,omitempty"`
	ParentID string `json:"parent_id,omitempty"`
	Parent   string `json:"parent_pane,omitempty"`
	// GroupID is the spawn group the queued worker belongs to: when it
	// starts, its submit joins the group's single notice.
	GroupID string `json:"group_id,omitempty"`
}

// workerQueuePath is where queued workers wait, beside the worker store.
func workerQueuePath() string { return filepath.Join(filepath.Dir(statePath), "worker-queue.json") }

// loadWorkerQueue reads the queue; a missing or corrupt file is empty, so a
// bad queue never blocks launches.
func loadWorkerQueue() []queuedWorker {
	raw, err := os.ReadFile(workerQueuePath())
	if err != nil {
		return nil
	}
	var out []queuedWorker
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

// saveWorkerQueue writes the queue atomically, so a concurrent reader never
// sees a half-written file.
func saveWorkerQueue(q []queuedWorker) error {
	if err := os.MkdirAll(filepath.Dir(workerQueuePath()), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(q, "", "  ")
	if err != nil {
		return err
	}
	tmp := workerQueuePath() + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, workerQueuePath())
}

// effectiveWorkerCap is the worker cap in force, defaulting to three.
func effectiveWorkerCap() int {
	return tools.LoadSettings().EffectiveMaxWorkers()
}

// queueWorker appends one entry, refusing a duplicate id.
func queueWorker(entry queuedWorker) error {
	return withFileLock("worker-queue", func() error {
		q := loadWorkerQueue()
		for _, e := range q {
			if e.ID == entry.ID {
				return fmt.Errorf("worker %q is already queued", entry.ID)
			}
		}
		return saveWorkerQueue(append(q, entry))
	})
}

// dequeueWorkers removes the named ids from the queue, for a failed spawn's
// rollback. An id that is not queued is not an error.
func dequeueWorkers(ids []string) {
	_ = withFileLock("worker-queue", func() error {
		q := loadWorkerQueue()
		keep := q[:0]
		for _, e := range q {
			if !contains(ids, e.ID) {
				keep = append(keep, e)
			}
		}
		return saveWorkerQueue(keep)
	})
}

// queuedIDs reports the ids currently queued, for tests.
func queuedIDs() []string {
	var out []string
	_ = withFileLock("worker-queue", func() error {
		for _, e := range loadWorkerQueue() {
			out = append(out, e.ID)
		}
		return nil
	})
	return out
}

// drainWorkerQueue starts queued workers while slots are free. It runs after a
// finish freed a slot: one slot freed starts at most one queued worker, which
// keeps the launch serialized with every spawn's own launches under the same
// worker-launch lock. A launch failure re-queues the entry, except an entry
// that can never launch (already running, ticket gone), which is dropped.
// Best effort: a drain that finds no slot or no queue does nothing.
func drainWorkerQueue(run Exec) {
	_ = withFileLock("worker-launch", func() error {
		var entry queuedWorker
		_ = withFileLock("worker-queue", func() error {
			q := loadWorkerQueue()
			if len(q) == 0 {
				return nil
			}
			store, err := loadWorkersLocked()
			if err != nil {
				return nil
			}
			if len(store) >= effectiveWorkerCap() {
				return nil
			}
			entry = q[0]
			_ = saveWorkerQueue(q[1:])
			return nil
		})
		if entry.ID == "" {
			return nil
		}
		agent, err := launchQueuedWorker(run, entry)
		if err != nil {
			if dropQueuedEntry(err) {
				return nil
			}
			_ = withFileLock("worker-queue", func() error {
				q := loadWorkerQueue()
				for _, e := range q {
					if e.ID == entry.ID {
						return nil
					}
				}
				return saveWorkerQueue(append([]queuedWorker{entry}, q...))
			})
			return nil
		}
		_ = agent
		return nil
	})
}

// dropQueuedEntry reports whether a failed queued launch must not be retried:
// the worker is already running, or its ticket can never be read.
func dropQueuedEntry(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "already exists") || strings.Contains(msg, "already queued") ||
		strings.Contains(msg, "cannot read ticket") || strings.Contains(msg, "needs ticket")
}

// launchQueuedWorker starts one queued worktree agent, rebuilding the launch
// the original spawn would have made from the stored resolved choice. The
// brief is rewritten, a fresh session names a fresh launch file, and the
// launch is spaced like any other.
func launchQueuedWorker(run Exec, e queuedWorker) (*spawnedAgent, error) {
	if e.Ticket == "" {
		return nil, fmt.Errorf("a %s entry needs ticket", e.Type)
	}
	if _, err := os.Stat(e.Ticket); err != nil {
		return nil, fmt.Errorf("cannot read ticket %s: %w", e.Ticket, err)
	}
	target := e.Target
	if target == "" {
		target = targetBranch(run, e.Repo, e.Base)
	}
	in := SpawnInput{
		Ticket:    e.Ticket,
		Spec:      e.Spec,
		Repo:      e.Repo,
		Base:      e.Base,
		Target:    target,
		Harness:   e.Harness,
		Model:     e.Model,
		Effort:    e.Effort,
		AgentType: e.Type,
	}
	brief, err := writeBrief(e.ID, in.Ticket, in.Spec, target, e.Harness)
	if err != nil {
		return nil, err
	}
	briefText, err := os.ReadFile(brief)
	if err != nil {
		return nil, err
	}
	sess := Session{
		ID:        newSessionID(),
		Role:      "agent",
		Cwd:       worktreePath(e.Repo, e.ID),
		Model:     e.Model,
		Effort:    e.Effort,
		Label:     e.ID,
		Spec:      in.Spec,
		AgentType: e.Type,
		AgentID:   e.ID,
		Prompt:    string(briefText),
	}
	launch, err := PrepareLaunch(e.Harness, sess)
	if err != nil {
		return nil, err
	}
	in.Launch, in.Late = []string{launch.Line}, launch.Late
	in.SessionID = sess.ID
	spaceSpawns()
	worker, err := Spawn(run, in)
	if err != nil {
		CleanupSession(sess)
		return nil, err
	}
	state := AgentState{
		ID:       worker.ID,
		Type:     e.Type,
		Ticket:   e.Ticket,
		Pane:     worker.Pane,
		Tab:      worker.Tab,
		ParentID: e.ParentID,
		Parent:   e.Parent,
		GroupID:  e.GroupID,
		// No spawn waits on a drained worker: the queuing call already
		// returned and told its parent to end the turn, so the submit joins
		// the queuing call's group notice whatever the type's blocking says.
		Deliver: true,
	}
	if err := WriteAgentState(state); err != nil {
		_, _ = finishWorker(run, worker.ID, false, false)
		return nil, err
	}
	return &spawnedAgent{
		ID: worker.ID, Pane: worker.Pane, Tab: worker.Tab, Sess: sess,
		Worktree: true, Warning: reportExtensionWarning(e.Harness),
	}, nil
}
