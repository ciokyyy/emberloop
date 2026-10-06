package wick

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Worker is one delegated implementer, isolated in its own git worktree and
// terminal pane.
type Worker struct {
	ID     string `json:"id"`
	Ticket string `json:"ticket"`
	// Spec is the spec the ticket belongs to, when the worker was spawned by
	// an implement session; the spec reviewer judges the diff against it.
	Spec string `json:"spec,omitempty"`
	Repo string `json:"repo"`
	Base string `json:"base"`
	// Target is the branch the worker merges into: the branch the repo was on
	// when it spawned, or the one named as its base. Base stays the commit it
	// branched from, for diffs; finish rebases onto Target's current tip.
	Target    string `json:"target,omitempty"`
	Branch    string `json:"branch"`
	Worktree  string `json:"worktree"`
	Pane      string `json:"pane"`
	Tab       string `json:"tab,omitempty"`
	Workspace string `json:"workspace"`
	Harness   string `json:"harness"`
	Model     string `json:"model,omitempty"`
	// Effort is the reasoning effort the agent was launched with, in the
	// harness's own vocabulary.
	Effort    string `json:"effort,omitempty"`
	Started   string `json:"started"`
	SessionID string `json:"session_id,omitempty"`
	// AgentType is the type this worker was spawned as, whose requires_review
	// decides which reviewers gate its finish. Empty on records made before
	// types recorded their name, which fall back to the two built-in ones.
	AgentType string `json:"agent_type,omitempty"`
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

// save writes the store through a temp file and a rename, so a concurrent
// reader never sees a half-written file. Callers hold the workers
// cross-process lock across their read-modify-write, so no entry is lost.
func save(store map[string]Worker) error {
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	tmp := statePath + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, statePath)
}

// loadWorkersLocked is load under the workers cross-process lock, for readers
// that must not see a store mid-update.
func loadWorkersLocked() (map[string]Worker, error) {
	var out map[string]Worker
	err := withFileLock("workers", func() error {
		store, err := load()
		if err != nil {
			return err
		}
		out = store
		return nil
	})
	return out, err
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
		Tab struct {
			TabID       string `json:"tab_id"`
			Label       string `json:"label"`
			WorkspaceID string `json:"workspace_id"`
		} `json:"tab"`
		Tabs []struct {
			TabID       string `json:"tab_id"`
			Label       string `json:"label"`
			WorkspaceID string `json:"workspace_id"`
		} `json:"tabs"`
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
		ProcessInfo struct {
			ShellPID int `json:"shell_pid"`
			GroupID  int `json:"foreground_process_group_id"`
			// Foreground is whatever runs on top of the shell: an agent, or a
			// command pane run started.
			Foreground []struct {
				PID int `json:"pid"`
			} `json:"foreground_processes"`
		} `json:"process_info"`
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
	Ticket string
	Spec   string
	Repo   string
	Base   string
	// Target overrides the branch the worker merges back into. Empty derives
	// it from Base, the way spawn always did.
	Target  string
	Harness string
	Model   string
	Effort  string
	// Launch is the exact command pane run receives. The extension derives it
	// from Harness and Model; tests supply a stub agent instead.
	Launch []string
	// Late is a first message sent once the agent is ready; see Harness.LatePrompt.
	Late string
	// SessionID names the harness session so Wait can find its transcript.
	SessionID string
	// AgentType is the type this worker is spawned as; finish reads its
	// requires_review to know which reviewers the merge gate needs.
	AgentType string
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

// targetBranch is the branch a worker spawned from base merges back into: base
// itself when it names a local branch, otherwise the branch the repo has
// checked out. A detached repo has none, and finish falls back to the base.
func targetBranch(run Exec, repo, base string) string {
	if base != "" && base != "HEAD" {
		res, err := run("git", []string{"show-ref", "--verify", "--quiet", "refs/heads/" + base}, ExecOpts{Cwd: repo, Timeout: 30 * time.Second})
		if err == nil && res.Code == 0 {
			return base
		}
		return ""
	}
	res, err := run("git", []string{"symbolic-ref", "--quiet", "--short", "HEAD"}, ExecOpts{Cwd: repo, Timeout: 30 * time.Second})
	if err != nil || res.Code != 0 {
		return ""
	}
	return strings.TrimSpace(res.Stdout)
}

// reusableBranch reports whether a spawn may reuse an implement branch finish
// kept. The branch is reusable when it is at or behind base, so `git worktree
// add <path> <branch>` checks it out with no -b. A branch carrying commits
// base does not have is refused by name: reusing it would drop that work, and
// re-creating it would silently start the branch over from base.
func reusableBranch(run Exec, repo, branch, base string) (bool, error) {
	res, err := run("git", []string{"show-ref", "--verify", "--quiet", "refs/heads/" + branch},
		ExecOpts{Cwd: repo, Timeout: 30 * time.Second})
	if err != nil {
		return false, err
	}
	if res.Code != 0 {
		return false, nil
	}
	res, err = run("git", []string{"merge-base", "--is-ancestor", branch, base},
		ExecOpts{Cwd: repo, Timeout: 30 * time.Second})
	if err != nil {
		return false, err
	}
	if res.Code != 0 {
		return false, fmt.Errorf("branch %s has commits %s does not have; finish or delete it before spawning it again",
			branch, base)
	}
	return true, nil
}

// Spawn creates a git worktree on branch implement/<id>, opens the ticket's tab
// in the project workspace, and starts the worker in that tab's root pane.
func Spawn(run Exec, in SpawnInput) (*Worker, error) {
	// Workers live in the workspace the orchestrator runs in. Without it there
	// is nowhere to put the ticket's tab, so nothing is created first.
	workspace := os.Getenv("HERDR_WORKSPACE_ID")
	if workspace == "" {
		return nil, fmt.Errorf("HERDR_WORKSPACE_ID is not set: wick spawns workers into the herdr workspace it runs in")
	}

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
	target := in.Target
	if target == "" {
		target = targetBranch(run, repo, in.Base)
	}

	// finish keeps the implement branch, so a respawn after finish finds it
	// already there. Reuse it when the base still contains it; refuse when it
	// carries work the base does not, before anything is created.
	branch := "implement/" + id
	reuse, err := reusableBranch(run, repo, branch, base)
	if err != nil {
		return nil, err
	}

	storeMu.Lock()
	defer storeMu.Unlock()

	// The store is shared between sessions, which are separate processes: the
	// cross-process lock holds across the whole spawn, from the duplicate
	// check to the final save, so two simultaneous spawns never load the same
	// map and drop each other's entry.
	var out *Worker
	if err := withFileLock("workers", func() error {
		store, err := load()
		if err != nil {
			return err
		}
		if old, exists := store[id]; exists {
			// A record whose worktree is gone is left over from a cleanup done
			// outside finish; it must not block the id for ever.
			if _, err := os.Stat(old.Worktree); err == nil {
				return fmt.Errorf("worker %q already exists", id)
			}
			delete(store, id)
		}

		w := &Worker{
			ID:        id,
			Ticket:    in.Ticket,
			Spec:      in.Spec,
			Repo:      repo,
			Base:      base,
			Target:    target,
			Branch:    branch,
			Worktree:  worktreePath(repo, id),
			Workspace: workspace,
			Harness:   in.Harness,
			Model:     in.Model,
			Effort:    in.Effort,
			SessionID: in.SessionID,
			AgentType: in.AgentType,
			Started:   time.Now().UTC().Format(time.RFC3339),
		}

		// Plain git owns the worktree now, at today's path and branch: herdr is
		// only tabs and panes, never worktrees. An existing branch is reused with
		// no -b; -b would fail on a name that is already there.
		add := []string{"worktree", "add"}
		if reuse {
			add = append(add, w.Worktree, w.Branch)
		} else {
			add = append(add, "-b", w.Branch, w.Worktree, base)
		}
		if res, err := run("git", add, ExecOpts{Cwd: repo, Timeout: 60 * time.Second}); err != nil {
			return err
		} else if res.Code != 0 {
			return fmt.Errorf("git worktree add %s failed: %s", w.Worktree, firstLine(res.Stderr, "unknown error"))
		}

		// Anything that fails from here on has to undo the worktree, and the
		// branch only when this spawn made it, or the ticket can never be spawned
		// again. A branch finish kept is left for the next respawn. The tab is
		// closed only when this spawn created it: an existing one may hold a
		// previous implementor.
		tabCreated := false
		undo := func() {
			if tabCreated && w.Tab != "" {
				_, _ = herdrJSON(run, []string{"tab", "close", w.Tab}, 30*time.Second)
			}
			_, _ = run("git", []string{"worktree", "remove", "--force", w.Worktree}, ExecOpts{Cwd: repo, Timeout: 60 * time.Second})
			if !reuse {
				_, _ = run("git", []string{"branch", "-D", w.Branch}, ExecOpts{Cwd: repo, Timeout: 30 * time.Second})
			}
		}

		// Finds the tab labelled with the ticket id in the project workspace, or
		// creates it with the worktree as cwd. A respawn reuses the tab and replaces
		// the implementor in its root pane rather than opening a second one.
		w.Tab, w.Pane, tabCreated, err = ticketTab(run, workspace, id, w.Worktree)
		if err != nil {
			undo()
			return err
		}

		// A respawn reuses the tab; its root pane may still hold the previous
		// implementor, so whatever runs there is stopped first. The launch line
		// must never be typed into a live agent.
		if !tabCreated {
			if err := stopPane(run, w.Pane); err != nil {
				undo()
				return err
			}
		}

		// pane run rather than `agent start --kind`: herdr's kinds are compiled into
		// its binary and include no entry for pig. Workers self-report instead.
		if res, err := run(herdrBin(), append([]string{"pane", "run", w.Pane}, in.Launch...), ExecOpts{}); err != nil {
			undo()
			return err
		} else if res.Code != 0 {
			undo()
			return fmt.Errorf("could not launch worker in %s: %.200s", w.Pane, res.Stderr)
		}

		if err := DeliverLate(run, w.Pane, in.Late); err != nil {
			undo()
			return err
		}

		store[id] = *w
		if err := save(store); err != nil {
			undo()
			return err
		}
		out = w
		return nil
	}); err != nil {
		return nil, err
	}
	return out, nil
}

// ticketTab finds the tab labelled id in workspace, or creates it there with
// cwd, and returns its root pane: the pane in the tab's first grid slot. A new
// tab's root pane is recorded as slot 1, so a later respawn finds it even when
// herdr lists another pane first. created is false when the tab already
// existed, so a failed spawn knows not to close a tab it did not make.
func ticketTab(run Exec, workspace, id, cwd string) (tab, pane string, created bool, err error) {
	tabs, err := herdrJSON(run, []string{"tab", "list", "--workspace", workspace}, 30*time.Second)
	if err != nil {
		return "", "", false, err
	}
	for _, t := range tabs.Result.Tabs {
		if t.Label != id {
			continue
		}
		pane, err := gridRoot(run, t.TabID)
		if err != nil {
			return "", "", false, err
		}
		return t.TabID, pane, false, nil
	}

	reply, err := herdrJSON(run, []string{
		"tab", "create",
		"--workspace", workspace,
		"--cwd", cwd,
		"--label", id,
		"--no-focus",
	}, 30*time.Second)
	if err != nil {
		return "", "", false, err
	}
	tab = reply.Result.RootPane.TabID
	if tab == "" {
		tab = reply.Result.Tab.TabID
	}
	pane = reply.Result.RootPane.PaneID
	if tab == "" {
		return "", "", false, fmt.Errorf("herdr tab create returned no tab for %q; raw: %.600s", id, reply.raw)
	}
	if pane == "" {
		// The tab exists now, so report it as created: the caller's undo
		// closes it rather than leaking a pane-less tab.
		return tab, "", true, fmt.Errorf("herdr tab create returned no pane for %q; raw: %.600s", id, reply.raw)
	}
	if err := recordGridSlot(tab, 1, pane); err != nil {
		// The tab exists now, so report it as created: the caller's undo
		// closes it rather than leaking a pane-less tab.
		return tab, "", true, err
	}
	return tab, pane, true, nil
}

// gridRoot is the pane in a tab's first grid slot, slot 1: the tab's root and
// the implementor's pane. herdr cannot say which pane that is -- it lists
// panes in creation order, which a replacement disturbs -- so the grid's own
// record is the authority, falling back to the first pane herdr reports for a
// tab the grid never placed.
func gridRoot(run Exec, tab string) (string, error) {
	slots, err := gridSlots(run, tab)
	if err != nil {
		return "", err
	}
	if pane := slots[1]; pane != "" {
		return pane, nil
	}
	return "", fmt.Errorf("herdr reports no panes in tab %s", tab)
}

// paneProcess is what herdr reports of a pane's processes: the shell that owns
// the pane, the process group in the foreground, and the processes in it.
type paneProcess struct {
	ShellPID   int
	Group      int
	Foreground []int
}

// stopped says nothing runs in the pane but its shell, so a launch line typed
// there reaches the shell.
func (p paneProcess) stopped() bool {
	for _, pid := range p.Foreground {
		if pid != p.ShellPID {
			return false
		}
	}
	return true
}

// stopPane ends whatever runs in a pane before a respawn types into it. A
// reused tab's root pane may still hold the previous implementor, and pane run
// writes its command to whatever is in the foreground, so that process is
// ended and the pane waited on until herdr reports the shell back. herdr has no
// stop verb; its process-info gives the group to kill.
func stopPane(run Exec, pane string) error {
	proc, err := paneProcesses(run, pane)
	if err != nil {
		return err
	}
	if proc.stopped() {
		return nil
	}
	group := proc.Group
	if group <= 0 && len(proc.Foreground) > 0 {
		group = proc.Foreground[0]
	}
	if group <= 0 {
		return nil
	}
	// SIGTERM first, so an agent can clean up; SIGKILL for one that will not.
	last := ""
	for _, sig := range []string{"-TERM", "-KILL"} {
		said, err := signalPane(run, pane, sig, group)
		if err != nil {
			return err
		}
		if waitPaneStop(run, pane, paneStopWait) {
			return nil
		}
		last = sig
		if said != "" {
			last += ": " + said
		}
	}
	return fmt.Errorf("the process in %s would not stop (kill %s), so the launch line was not typed into it", pane, last)
}

// signalPane sends sig to a pane's foreground process group. The group is
// negative in the kill call so the agent and anything it spawned go together.
// It returns what kill said, or "" when it said nothing. A kill that finds
// nothing to signal is not a failure -- the process being gone is the point --
// so a non-zero exit is reported rather than returned, and waitPaneStop decides
// whether the process actually went.
func signalPane(run Exec, pane, sig string, group int) (string, error) {
	res, err := run("kill", []string{sig, "--", "-" + strconv.Itoa(group)}, ExecOpts{Timeout: 15 * time.Second})
	if err != nil {
		return "", fmt.Errorf("could not signal the process in %s: %w", pane, err)
	}
	if res.Code != 0 {
		return firstLine(res.Stderr, fmt.Sprintf("%s exited %d", sig, res.Code)), nil
	}
	return "", nil
}

// waitPaneStop waits until herdr reports only the pane's shell left. It reads
// process-info immediately, so a process that went at once costs no wait.
func waitPaneStop(run Exec, pane string, within time.Duration) bool {
	deadline := time.Now().Add(within)
	for {
		proc, err := paneProcesses(run, pane)
		if err != nil {
			return false
		}
		if proc.stopped() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(paneStopPoll)
	}
}

// paneProcesses reads what runs in a pane.
func paneProcesses(run Exec, pane string) (paneProcess, error) {
	reply, err := herdrJSON(run, []string{"pane", "process-info", "--pane", pane}, 15*time.Second)
	if err != nil {
		return paneProcess{}, err
	}
	info := reply.Result.ProcessInfo
	proc := paneProcess{ShellPID: info.ShellPID, Group: info.GroupID}
	for _, fg := range info.Foreground {
		proc.Foreground = append(proc.Foreground, fg.PID)
	}
	return proc, nil
}

var (
	// paneStopWait is how long each signal gets to end the foreground process.
	paneStopWait = 10 * time.Second
	paneStopPoll = 200 * time.Millisecond
)

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
// on disk, because a human still has to look at it. Every finish call — merged,
// discarded, or kept — leaves a ticket history entry under its run first, with
// the reviewer verdicts as they stand.
func Finish(run Exec, id string, merge bool) (*FinishResult, error) {
	return finishWorker(run, id, merge, true)
}

// finishWorker is Finish, with record naming whether the call leaves a ticket
// history entry. Spawn's rollback of a failed spawn tears a worker down through
// here, and a ticket that never ran is not a finished ticket, so it records
// nothing.
func finishWorker(run Exec, id string, merge, record bool) (*FinishResult, error) {
	// storeMu is released before the queue drain below: the drain launches
	// through Spawn, which takes storeMu itself.
	storeMu.Lock()
	// The store is shared between sessions, which are separate processes: the
	// cross-process lock holds across the whole finish, from load to the final
	// delete, so two simultaneous finishes never load the same map and resurrect
	// each other's entry.
	var out *FinishResult
	lockErr := withFileLock("workers", func() error {
		store, err := load()
		if err != nil {
			return err
		}
		w, ok := store[id]
		if !ok {
			return fmt.Errorf("unknown worker %q", id)
		}

		var sha, detail string
		merged := false

		// kept records a refused merge before sending the worker back: the verdicts
		// as they stand, and why it was kept. The worker stays, so a later finish
		// records again.
		kept := func(detail string) (*FinishResult, error) {
			if record {
				if err := recordFinish(&w, outcomeKept, "", detail); err != nil {
					return nil, err
				}
			}
			return &FinishResult{ID: id, Kept: true, Detail: detail}, nil
		}

		if merge {
			// Uncommitted work is not on the branch, so merging would drop it
			// with the worktree.
			if res, err := run("git", []string{"status", "--porcelain"}, ExecOpts{Cwd: w.Worktree}); err != nil {
				return err
			} else if strings.TrimSpace(res.Stdout) != "" {
				r, err := kept("the worktree has uncommitted changes, which would be lost; have it commit them and call spawn of its reviewers again")
				out = r
				return err
			}
			// Both reviewers must have passed the tip they judged; a worker that
			// stopped before its review is sent back, not merged. The check is
			// on the worker's own tip, before the rebase below rewrites it.
			res, err := run("git", []string{"rev-parse", w.Branch}, ExecOpts{Cwd: w.Repo})
			if err != nil {
				return err
			}
			tip := strings.TrimSpace(res.Stdout)
			tipTree := ""
			if t, err := run("git", []string{"rev-parse", tip + "^{tree}"}, ExecOpts{Cwd: w.Repo}); err == nil && t.Code == 0 {
				tipTree = strings.TrimSpace(t.Stdout)
			}
			tipPatch := workerPatchID(run, &w, tip, tipTree)
			if why := reviewGate(w.Worktree, tip, tipTree, tipPatch, requiredReviewers(&w), passHolds(w.Repo)); why != "" {
				r, err := kept("not reviewed to a pass: " + why + ". Have it spawn its reviewer types until they pass at its tip")
				out = r
				return err
			}

			// Rebase onto where the target is *now*: another worker may have moved
			// it since this one spawned. Old records without a target fall back to
			// the base they recorded.
			onto := w.Target
			if onto == "" {
				onto = w.Base
			}
			if res, err := run("git", []string{"rebase", onto}, ExecOpts{Cwd: w.Worktree}); err != nil {
				return err
			} else if res.Code != 0 {
				// Name the conflicts, then abort: the worker that is sent back to
				// resolve them starts from its own clean branch, not a half rebase.
				files := ""
				if u, err := run("git", []string{"diff", "--name-only", "--diff-filter=U"}, ExecOpts{Cwd: w.Worktree}); err == nil {
					files = strings.Join(strings.Fields(u.Stdout), ", ")
				}
				_, _ = run("git", []string{"rebase", "--abort"}, ExecOpts{Cwd: w.Worktree})
				detail := "rebase onto " + onto + " conflicted"
				if files != "" {
					detail += " in " + files
				} else {
					detail += ": " + gitFullText(res.Stderr, "rebase failed")
				}
				r, err := kept(detail + "; the rebase was aborted, the branch is unchanged")
				out = r
				return err
			}

			// The fast-forward lands on whatever the repo has checked out, so that
			// must be the worker's target.
			if w.Target != "" {
				if res, err := run("git", []string{"symbolic-ref", "--quiet", "--short", "HEAD"}, ExecOpts{Cwd: w.Repo}); err != nil {
					return err
				} else if on := strings.TrimSpace(res.Stdout); on != w.Target {
					r, err := kept(fmt.Sprintf("the repo is on %q, not the worker's target %q; switch it back and finish again", on, w.Target))
					out = r
					return err
				}
			}

			// The integration checkout may hold uncommitted work of its own: the
			// orchestrator's notes and ticket edits. That is fine unless it
			// overlaps what the merge would change, in which case git would
			// refuse halfway with its own error. Refuse up front instead, naming
			// the files, so the orchestrator knows to commit or stash them.
			// -uall lists untracked files individually: the default collapses a
			// wholly-untracked directory to `dir/`, which never matches an
			// incoming `dir/file` the merge would overwrite.
			if res, err := run("git", []string{"status", "--porcelain", "-uall"}, ExecOpts{Cwd: w.Repo}); err == nil && res.Code == 0 {
				if dirty := porcelainFiles(res.Stdout); len(dirty) > 0 {
					if diff, err := run("git", []string{"diff", "--name-only", onto + "..." + w.Branch}, ExecOpts{Cwd: w.Repo}); err == nil && diff.Code == 0 {
						incoming := map[string]bool{}
						for _, f := range strings.Fields(diff.Stdout) {
							incoming[strings.Trim(strings.TrimSpace(f), `"`)] = true
						}
						var overlap []string
						for _, f := range dirty {
							if incoming[f] {
								overlap = append(overlap, f)
							}
						}
						if len(overlap) > 0 {
							sort.Strings(overlap)
							r, err := kept(fmt.Sprintf("the integration checkout has uncommitted changes overlapping the incoming change in %s; commit or stash them and finish again", strings.Join(overlap, ", ")))
							out = r
							return err
						}
					}
				}
			}

			if res, err := run("git", []string{"merge", "--ff-only", w.Branch}, ExecOpts{Cwd: w.Repo}); err != nil {
				return err
			} else if res.Code != 0 {
				r, err := kept(gitFullText(res.Stderr, "merge failed"))
				out = r
				return err
			}

			if res, err := run("git", []string{"rev-parse", w.Branch}, ExecOpts{Cwd: w.Repo}); err == nil {
				sha = strings.TrimSpace(res.Stdout)
			}
			merged, detail = true, "merged"
			// The merge moved the target; the ticket's own completion lands on top
			// of it, in one commit: done status, ticked boxes. Every earlier return
			// (refused, conflicting) left the ticket untouched.
			if err := completeTicket(run, &w); err != nil {
				return err
			}
		} else {
			detail = "discarded"
		}

		// Only reached when the worker merged or was discarded. A refusal returns
		// above with the worktree, tab and reviewers left for the worker to use.
		// The history entry goes first, while the reviewers are still on record; a
		// failed record keeps the worker, so nothing is lost with it.
		if record {
			outcome := outcomeDiscarded
			if merged {
				outcome = outcomeMerged
			}
			if err := recordFinish(&w, outcome, sha, detail); err != nil {
				return err
			}
		}
		closeReviewers(run, w.Worktree)
		// The ticket's tab holds the implementor and its reviewers; the overflow
		// tabs hold the reviewers that did not fit. Closing both closes every
		// agent of the ticket. Best-effort: a tab already gone must not block
		// dropping the worktree. Records without a tab predate the ticket tab;
		// skip them.
		if w.Tab != "" {
			_, _ = herdrJSON(run, []string{"tab", "close", w.Tab}, 30*time.Second)
			closeOverflowTabs(run, w.Tab, w.ID)
		}
		// herdr never owned the worktree: plain git made it, plain git drops it.
		if res, err := run("git", []string{"worktree", "remove", "--force", w.Worktree},
			ExecOpts{Cwd: w.Repo, Timeout: 60 * time.Second}); err != nil {
			return err
		} else if res.Code != 0 {
			return fmt.Errorf("git worktree remove %s failed: %s", w.Worktree, gitFullText(res.Stderr, "unknown error"))
		}
		// The worker's per-session files go with it: the MCP config and the
		// launch file `wick-pig exec` read at startup. Either is already gone when
		// the agent consumed it; what remains is the backup for a launch that
		// never started.
		CleanupSession(Session{ID: w.SessionID, Label: id})
		CleanupSession(Session{Label: id})
		delete(store, id)
		if err := save(store); err != nil {
			return err
		}
		out = &FinishResult{ID: id, Merged: merged, SHA: sha, Detail: detail}
		return nil
	})
	storeMu.Unlock()
	if lockErr != nil {
		return nil, lockErr
	}
	// A freed slot starts the next queued worker, if any. Only a real finish
	// (recorded) drains: rollbacks free slots too, but the next real finish
	// picks those up, and draining from a rollback would nest drains.
	if record && out != nil && !out.Kept {
		drainWorkerQueue(run)
	}
	return out, nil
}

// completeTicket marks the worker's ticket done after its merge: the status
// line reads done and every checkbox is ticked, committed on the target branch
// in one commit. A ticket that is missing, outside the repo, or without a
// status line is not a ticket finish can complete, and is left alone: records
// made before workers stored their ticket, and fixtures, carry none.
func completeTicket(run Exec, w *Worker) error {
	if w.Ticket == "" {
		return nil
	}
	abs := w.Ticket
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(w.Repo, abs)
	}
	rel, err := filepath.Rel(w.Repo, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil
	}
	raw, err := os.ReadFile(abs)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	done, ok := tickTicket(string(raw))
	if !ok || done == string(raw) {
		return nil
	}
	if err := os.WriteFile(abs, []byte(done), 0o644); err != nil {
		return err
	}
	// The commit names only the ticket: dirt in other paths stays uncommitted,
	// the way the merge left it. Uncommitted edits to the ticket file itself
	// are included, since the commit takes the file as it stands.
	if res, err := run("git", []string{"add", "--", rel}, ExecOpts{Cwd: w.Repo}); err != nil {
		return err
	} else if res.Code != 0 {
		return fmt.Errorf("git add %s failed: %s", rel, gitFullText(res.Stderr, "unknown error"))
	}
	if res, err := run("git", []string{"commit", "-m", "mark " + w.ID + " done", "--", rel}, ExecOpts{Cwd: w.Repo}); err != nil {
		return err
	} else if res.Code != 0 {
		return fmt.Errorf("git commit %s failed: %s", rel, gitFullText(res.Stderr, "unknown error"))
	}
	return nil
}

// tickTicket returns the ticket with its status done and its boxes ticked. ok
// is false when the file holds no status line, so prose and fixtures never
// gain one here. Only the header field counts: a "Status:" mentioned in the
// body is left alone.
func tickTicket(raw string) (done string, ok bool) {
	var out []string
	seen := false
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Status:") || strings.HasPrefix(trimmed, "**Status:") {
			seen = true
			if i := strings.Index(line, "Status:"); i >= 0 {
				// Keep the markup between the key and the value (`**Status:** x`
				// stays bold); only the value itself becomes done.
				rest := line[i+len("Status:"):]
				j := 0
				for j < len(rest) && (rest[j] == '*' || rest[j] == ' ' || rest[j] == '\t') {
					j++
				}
				sep := strings.TrimRight(rest[:j], " \t") + " "
				line = line[:i+len("Status:")] + sep + "done"
			}
		}
		line = strings.ReplaceAll(line, "[ ]", "[x]")
		out = append(out, line)
	}
	if !seen {
		return "", false
	}
	return strings.Join(out, "\n"), true
}

func firstLine(text, fallback string) string {
	if trimmed := strings.TrimSpace(text); trimmed != "" {
		return strings.SplitN(trimmed, "\n", 2)[0]
	}
	return fallback
}

// gitFullText passes git's own error text through whole, never truncated, so a
// refusal says why. Empty text falls back, so the reply still says something.
func gitFullText(text, fallback string) string {
	if trimmed := strings.TrimSpace(text); trimmed != "" {
		return trimmed
	}
	return fallback
}

// porcelainFiles lists the paths `git status --porcelain` reports as dirty, in
// the repo-relative form `git diff --name-only` uses, so finish can compare
// the two. Renames contribute both sides: either overlapping blocks the merge.
func porcelainFiles(stdout string) []string {
	var out []string
	for _, line := range strings.Split(stdout, "\n") {
		if len(line) < 4 {
			continue
		}
		path := strings.TrimSpace(line[3:])
		if path == "" {
			continue
		}
		if strings.Contains(path, " -> ") {
			for _, p := range strings.Split(path, " -> ") {
				if p = strings.Trim(strings.TrimSpace(p), `"`); p != "" {
					out = append(out, p)
				}
			}
			continue
		}
		out = append(out, strings.Trim(path, `"`))
	}
	return out
}
