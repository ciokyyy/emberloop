package wick

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Worker is one delegated implementer, isolated in its own git worktree and
// terminal pane.
type Worker struct {
	ID        string `json:"id"`
	Ticket    string `json:"ticket"`
	Repo      string `json:"repo"`
	Base      string `json:"base"`
	Branch    string `json:"branch"`
	Worktree  string `json:"worktree"`
	Pane      string `json:"pane"`
	Tab       string `json:"tab,omitempty"`
	Workspace string `json:"workspace"`
	Harness   string `json:"harness"`
	Model     string `json:"model,omitempty"`
	Started   string `json:"started"`
	SessionID string `json:"session_id,omitempty"`
}

// Status is a worker's live state. Everything the caller needs to decide what to
// do next, in one value, so a single tool call answers it.
type Status struct {
	ID       string `json:"id"`
	State    string `json:"state"`
	Detail   string `json:"detail,omitempty"`
	Branch   string `json:"branch"`
	Worktree string `json:"worktree"`
}

type FinishResult struct {
	ID     string `json:"id"`
	Merged bool   `json:"merged"`
	SHA    string `json:"sha,omitempty"`
	Detail string `json:"detail"`
	Kept   bool   `json:"kept"`
}

var (
	// statePath follows PiG's own convention for extension-owned state.
	statePath = filepath.Join(os.Getenv("HOME"), ".pig", "state", "wick", "workers.json")
	storeMu   sync.Mutex
)

// ---------------------------------------------------------------------------
// persistence
// ---------------------------------------------------------------------------

func load() (map[string]Worker, error) {
	raw, err := os.ReadFile(statePath)
	if os.IsNotExist(err) {
		return map[string]Worker{}, nil
	}
	if err != nil {
		return nil, err
	}
	var out map[string]Worker
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("worker store is corrupt: %w", err)
	}
	if out == nil {
		out = map[string]Worker{}
	}
	return out, nil
}

func save(store map[string]Worker) error {
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(statePath, append(raw, '\n'), 0o644)
}

// ---------------------------------------------------------------------------
// herdr
// ---------------------------------------------------------------------------

type herdrReply struct {
	Result struct {
		Pane struct {
			PaneID string `json:"pane_id"`
			TabID  string `json:"tab_id"`
		} `json:"pane"`
		RootPane struct {
			PaneID string `json:"pane_id"`
			TabID  string `json:"tab_id"`
		} `json:"root_pane"`
		Workspace struct {
			WorkspaceID string `json:"workspace_id"`
		} `json:"workspace"`
		Layout paneLayout `json:"layout"`
		Agents []struct {
			PaneID string `json:"pane_id"`
			Status string `json:"agent_status"`
			Title  string `json:"terminal_title_stripped"`
			Name   string `json:"name"`
		} `json:"agents"`
		Panes []struct {
			PaneID    string `json:"pane_id"`
			TabID     string `json:"tab_id"`
			Workspace string `json:"workspace_id"`
			Cwd       string `json:"cwd"`
		} `json:"panes"`
	} `json:"result"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`

	// raw is herdr's untouched reply text, kept for diagnostics only.
	raw string
}

func herdrBin() string {
	if p := os.Getenv("HERDR_BIN_PATH"); p != "" {
		return p
	}
	return "herdr"
}

// herdrJSON runs a herdr subcommand and decodes its reply. herdr answers JSON on
// stdout when it succeeds and JSON on stderr when it fails.
func herdrJSON(run Exec, args []string, timeout time.Duration) (*herdrReply, error) {
	res, err := run(herdrBin(), args, ExecOpts{Timeout: timeout})
	if err != nil {
		return nil, err
	}

	text := strings.TrimSpace(res.Stdout)
	if text == "" {
		text = strings.TrimSpace(res.Stderr)
	}

	var reply herdrReply
	reply.raw = text
	if text != "" {
		if err := json.Unmarshal([]byte(text), &reply); err != nil {
			return nil, fmt.Errorf("herdr %s returned non-JSON: %.200s", args[0], text)
		}
	}
	if res.Code != 0 {
		if reply.Error.Message != "" {
			return nil, fmt.Errorf("herdr %s failed: %s", args[0], reply.Error.Message)
		}
		return nil, fmt.Errorf("herdr %s exited %d: %.200s", args[0], res.Code, text)
	}
	return &reply, nil
}

// ---------------------------------------------------------------------------
// identity
// ---------------------------------------------------------------------------

// WorkerID turns a ticket path into a branch-safe identifier:
// `/repo/.scratch/x/issues/07-some-thing.md` -> `07-some-thing`
func WorkerID(ticket string) string {
	base := filepath.Base(ticket)
	base = strings.TrimSuffix(base, ".md")

	var b strings.Builder
	for _, r := range base {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	id := strings.Trim(b.String(), "-")
	if id == "" {
		return "worker"
	}
	return id
}

// herdrName turns a worker id into a legal herdr agent name. herdr requires the
// name to start with a lowercase letter and hold only lowercase letters,
// digits, '-' or '_' — so the ticket numbering to-tickets produces (01-probe)
// has to be prefixed before it can be one.
func herdrName(id string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(id) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	name := strings.Trim(b.String(), "-")
	if name == "" || name[0] < 'a' || name[0] > 'z' {
		name = "w" + name
	}
	if len(name) > 32 {
		name = name[:32]
	}
	return name
}

// ---------------------------------------------------------------------------
// spawn
// ---------------------------------------------------------------------------

type SpawnInput struct {
	Ticket  string
	Repo    string
	Base    string
	Harness string
	Model   string
	// Launch is the exact command pane run receives. The extension derives it
	// from Harness and Model; tests supply a stub agent instead.
	Launch []string
	// Late is a first message sent once the agent is ready; see Harness.LatePrompt.
	Late string
	// SessionID names the harness session so Wait can find its transcript.
	SessionID string
	// RunTab and Anchor point at the tab a run's workers share: the pane is
	// split in RunTab next to Anchor so the worker joins it. Empty means no
	// shared tab, and the worker stays in the pane its worktree opened.
	RunTab string
	Anchor string
}

func worktreePath(repo, id string) string { return filepath.Join(repo, ".wick-worktrees", id) }

// resolveRef turns a git ref into the full SHA it currently points at.
func resolveRef(run Exec, repo, ref string) (string, error) {
	res, err := run("git", []string{"rev-parse", ref}, ExecOpts{Cwd: repo, Timeout: 30 * time.Second})
	if err != nil {
		return "", err
	}
	if res.Code != 0 {
		return "", fmt.Errorf("git rev-parse %s in %s failed: %s", ref, repo, firstLine(res.Stderr, "unknown ref"))
	}
	sha := strings.TrimSpace(res.Stdout)
	if sha == "" {
		return "", fmt.Errorf("git rev-parse %s in %s returned nothing", ref, repo)
	}
	return sha, nil
}

// Spawn creates a worktree, opens a pane on it, and starts the worker there.
func Spawn(run Exec, in SpawnInput) (*Worker, error) {
	id := WorkerID(in.Ticket)
	repo := in.Repo
	if repo == "" {
		var err error
		if repo, err = os.Getwd(); err != nil {
			return nil, err
		}
	}
	base := in.Base
	if base == "" {
		base = "HEAD"
	}

	// Resolve the base to a commit *here*, in the source repo. "HEAD" means the
	// worker's own tip once it is inside its worktree, which makes every later
	// `git diff <base>` empty. A branch name resolves to whatever it points at
	// now, so a worker's base stays put while the branch moves.
	resolved, err := resolveRef(run, repo, base)
	if err != nil {
		return nil, err
	}
	base = resolved

	storeMu.Lock()
	defer storeMu.Unlock()

	store, err := load()
	if err != nil {
		return nil, err
	}
	if old, exists := store[id]; exists {
		// A record whose worktree is gone is left over from a cleanup done
		// outside finish_worker; it must not block the id for ever.
		if _, err := os.Stat(old.Worktree); err == nil {
			return nil, fmt.Errorf("worker %q already exists", id)
		}
		delete(store, id)
	}

	w := &Worker{
		ID:        id,
		Ticket:    in.Ticket,
		Repo:      repo,
		Base:      base,
		Branch:    "implement/" + id,
		Worktree:  worktreePath(repo, id),
		Harness:   in.Harness,
		Model:     in.Model,
		SessionID: in.SessionID,
		Started:   time.Now().UTC().Format(time.RFC3339),
	}

	// pane run rather than `agent start --kind`: herdr's kinds are compiled into
	// its binary and include no entry for pig. Workers self-report instead.
	reply, err := herdrJSON(run, []string{
		"worktree", "create",
		"--cwd", repo,
		"--branch", w.Branch,
		"--base", base,
		"--path", w.Worktree,
		"--label", id,
		"--no-focus",
		"--trust-repository",
	}, 60*time.Second)
	if err != nil {
		return nil, err
	}

	w.Pane = reply.Result.RootPane.PaneID
	w.Tab = reply.Result.RootPane.TabID
	w.Workspace = reply.Result.Workspace.WorkspaceID
	if w.Pane == "" || w.Workspace == "" {
		return nil, fmt.Errorf("herdr worktree create returned no pane or workspace for %q; raw: %.600s", id, reply.raw)
	}

	// Anything that fails after the worktree exists has to undo it, or the
	// branch, the checkout and the workspace are left behind and the ticket can
	// never be spawned again. The worktree's own pane goes with its workspace;
	// a pane split into the run's tab has to be closed by hand.
	undo := func() {
		if w.Pane != "" && w.Pane != reply.Result.RootPane.PaneID {
			_, _ = run(herdrBin(), []string{"pane", "close", w.Pane}, ExecOpts{Timeout: 15 * time.Second})
		}
		_, _ = herdrJSON(run, []string{
			"worktree", "remove", "--workspace", w.Workspace, "--force", "--trust-repository",
		}, 60*time.Second)
	}

	// A worktree opens in a tab of its own, so tiling that tab would have
	// nothing to even out. When the caller names the run's tab, split it here:
	// every worker of the run lands as a pane of that one tab. The worktree's
	// own pane is left in place, because it is what keeps the worktree
	// workspace open for finish to remove.
	if in.RunTab != "" && in.Anchor != "" && in.RunTab != w.Tab {
		split, err := herdrJSON(run, []string{"pane", "split",
			"--pane", in.Anchor,
			"--direction", "down",
			"--ratio", "0.5",
			"--cwd", w.Worktree,
			"--no-focus",
		}, 15*time.Second)
		if err != nil {
			undo()
			return nil, err
		}
		pane := split.Result.Pane.PaneID
		if pane == "" {
			undo()
			return nil, fmt.Errorf("herdr split returned no pane id for %q", id)
		}
		w.Pane = pane
		w.Tab = split.Result.Pane.TabID
		if w.Tab == "" {
			w.Tab = in.RunTab
		}
	}

	if res, err := run(herdrBin(), append([]string{"pane", "run", w.Pane}, in.Launch...), ExecOpts{}); err != nil {
		undo()
		return nil, err
	} else if res.Code != 0 {
		undo()
		return nil, fmt.Errorf("could not launch worker in %s: %.200s", w.Pane, res.Stderr)
	}

	if err := DeliverLate(run, w.Pane, in.Late); err != nil {
		undo()
		return nil, err
	}

	store[id] = *w
	return w, save(store)
}

// ---------------------------------------------------------------------------
// wait
// ---------------------------------------------------------------------------

// settled is the vocabulary herdr reports once a worker stops moving.
var settled = []string{"idle", "done", "blocked", "failed"}

// Wait reports status for every known worker, or for the named ones.
//
// timeout of zero returns immediately, which is why there is no separate
// status tool: one call covers both "check" and "block until done". The polling
// happens here, in the extension, so the model spends no tokens on it.
func Wait(run Exec, ids []string, timeout time.Duration) ([]Status, []string, error) {
	storeMu.Lock()
	store, err := load()
	storeMu.Unlock()
	if err != nil {
		return nil, nil, err
	}

	chosen := make([]Worker, 0, len(store))
	for _, id := range ids {
		w, ok := store[id]
		if !ok {
			return nil, nil, fmt.Errorf("unknown worker %q", id)
		}
		chosen = append(chosen, w)
	}
	if len(ids) == 0 {
		for _, w := range store {
			chosen = append(chosen, w)
		}
	}

	deadline := time.Now().Add(timeout)
	for {
		reply, err := herdrJSON(run, []string{"agent", "list"}, 30*time.Second)
		if err != nil {
			return nil, nil, err
		}

		// Match on pane, never on the name a worker reports: every pig worker
		// reports the same name, so the name is ambiguous the moment there are two.
		byPane := map[string]struct {
			state string
			title string
			name  string
		}{}
		for _, a := range reply.Result.Agents {
			byPane[a.PaneID] = struct {
				state string
				title string
				name  string
			}{a.Status, a.Title, a.Name}
		}

		out := make([]Status, 0, len(chosen))
		var pending []string
		for _, w := range chosen {
			live, ok := byPane[w.Pane]
			state := "unknown"
			detail := ""
			if ok {
				state, detail = live.state, live.title
			}
			if ok && live.name == "" {
				// The worker is registered but unnamed. Rename belongs here
				// rather than at spawn: a worker takes seconds to boot before it
				// first reports, so a rename issued at spawn finds nothing.
				// Naming it makes `herdr agent get <id>` resolve.
				_, _ = herdrJSON(run, []string{"agent", "rename", w.Pane, herdrName(w.ID)}, 30*time.Second)
			}
			if !ok && paneGone(run, w) {
				state, detail = "failed", "pane is gone: the worker was closed or its herdr workspace died"
			}
			if contains(settled, state) && state != "failed" {
				state, detail = checkTurn(run, w, state, detail)
			}
			out = append(out, Status{
				ID: w.ID, State: state, Detail: detail,
				Branch: w.Branch, Worktree: w.Worktree,
			})
			if !contains(settled, state) {
				pending = append(pending, w.ID)
			}
		}

		if len(pending) == 0 || !time.Now().Before(deadline) {
			return out, pending, nil
		}
		time.Sleep(2 * time.Second)
	}
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// finish
// ---------------------------------------------------------------------------

// Finish takes a finished worker: merge it into the base when it will merge
// cleanly, then drop its worktree. A conflicting worktree is deliberately left
// on disk, because a human still has to look at it.
func Finish(run Exec, id string, merge bool) (*FinishResult, error) {
	storeMu.Lock()
	defer storeMu.Unlock()

	store, err := load()
	if err != nil {
		return nil, err
	}
	w, ok := store[id]
	if !ok {
		return nil, fmt.Errorf("unknown worker %q", id)
	}

	var sha, detail string
	merged := false

	if merge {
		if res, err := run("git", []string{"rebase", w.Base}, ExecOpts{Cwd: w.Worktree}); err != nil {
			return nil, err
		} else if res.Code != 0 {
			return &FinishResult{ID: id, Detail: firstLine(res.Stderr, "rebase failed"), Kept: true}, nil
		}

		if res, err := run("git", []string{"merge", "--ff-only", w.Branch}, ExecOpts{Cwd: w.Repo}); err != nil {
			return nil, err
		} else if res.Code != 0 {
			return &FinishResult{ID: id, Detail: firstLine(res.Stderr, "merge failed"), Kept: true}, nil
		}

		if res, err := run("git", []string{"rev-parse", w.Branch}, ExecOpts{Cwd: w.Repo}); err == nil {
			sha = strings.TrimSpace(res.Stdout)
		}
		merged, detail = true, "merged"
	} else {
		detail = "discarded"
	}

	// Only reached when the worker did not conflict. A conflict returns above
	// with its worktree deliberately left on disk for a human to look at.
	if _, err := herdrJSON(run, []string{
		"worktree", "remove", "--workspace", w.Workspace, "--force", "--trust-repository",
	}, 60*time.Second); err != nil {
		return nil, err
	}
	// The worker's pane belongs to the run's tab, not the worktree workspace,
	// so removing the worktree leaves it behind. Close it, and shrug at a pane
	// that is already gone (the worktree's own pane, when there was no run tab).
	_, _ = run(herdrBin(), []string{"pane", "close", w.Pane}, ExecOpts{Timeout: 15 * time.Second})
	// The worker's MCP config goes with it: nothing reads the file once the
	// worktree is gone.
	CleanupSession(Session{Label: id})
	delete(store, id)
	if err := save(store); err != nil {
		return nil, err
	}
	return &FinishResult{ID: id, Merged: merged, SHA: sha, Detail: detail}, nil
}

func firstLine(text, fallback string) string {
	if trimmed := strings.TrimSpace(text); trimmed != "" {
		return strings.SplitN(trimmed, "\n", 2)[0]
	}
	return fallback
}

// retryTrack is where one worker stands in its provider-error retries.
type retryTrack struct {
	nudges int
	next   time.Time // when the next nudge is due; zero when none is pending
	settle time.Time // the error record is stale until the nudge lands
}

var (
	retryMu     sync.Mutex
	retryTracks = map[string]*retryTrack{}
)

// checkTurn corrects a settled pane state with what the transcript says. A
// turn that died on a provider error looks idle to herdr, so without this
// check the merger would take an unfinished worker as finished.
func checkTurn(run Exec, w Worker, state, detail string) (string, string) {
	h, ok := registry[w.Harness]
	if !ok || w.SessionID == "" {
		return state, detail
	}
	p := findSession(h.SessionRoot(Session{ID: w.SessionID, Cwd: w.Worktree, Model: w.Model}), w.SessionID)
	if p == "" {
		return state, detail
	}
	turn, why := h.TurnState(p)

	retryMu.Lock()
	defer retryMu.Unlock()
	t := retryTracks[w.ID]
	if t == nil {
		t = &retryTrack{}
		retryTracks[w.ID] = t
	}
	now := time.Now()
	switch {
	case turn == TurnRunning:
		return "working", "turn in progress"
	case turn != TurnFailed:
		t.next = time.Time{}
		return state, detail
	case now.Before(t.settle):
		return "retrying", why
	case t.nudges >= len(retryBackoff):
		return "failed", fmt.Sprintf("provider error after %d retries: %s", t.nudges, why)
	case t.next.IsZero():
		t.next = now.Add(retryBackoff[t.nudges])
	case !now.Before(t.next):
		t.nudges++
		t.next = time.Time{}
		t.settle = now.Add(nudgeSettle)
		if err := Nudge(run, w.Pane, "continue"); err != nil {
			return "failed", "could not retry: " + err.Error()
		}
	}
	return "retrying", why
}

// paneGone reports a worker whose pane no longer exists. An unlisted pane is
// normal for the first seconds after spawn, while the agent boots, so only a
// worker older than spawnGrace counts, and only when herdr itself says the pane
// is not found rather than failing for some other reason.
func paneGone(run Exec, w Worker) bool {
	started, err := time.Parse(time.RFC3339, w.Started)
	if err != nil || time.Since(started) < spawnGrace {
		return false
	}
	res, err := run(herdrBin(), []string{"pane", "get", w.Pane}, ExecOpts{Timeout: 15 * time.Second})
	return err == nil && strings.Contains(res.Stdout+res.Stderr, "pane_not_found")
}

// spawnGrace is how long a worker may stay unlisted before its absence counts.
var spawnGrace = 30 * time.Second
