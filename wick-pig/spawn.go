package wick

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"example.com/wick/tools"
)

// roleSpawns says which agent types a top-level role may spawn. A spawned
// agent instead spawns its own type's `spawns`.
var roleSpawns = map[string][]string{
	"implement": {"scout", "worker"},
	"plan":      {"scout"},
}

// spawnableNames is the types a role may spawn, filtered to those that exist.
func spawnableNames(role string, types map[string]AgentType) []string {
	if role == "full" {
		return allTypeNames(types)
	}
	var out []string
	for _, name := range roleSpawns[role] {
		if _, ok := types[name]; ok {
			out = append(out, name)
		}
	}
	return out
}

// agentTools is the tool set of a spawned agent: the MCP tools its type names,
// plus submit and, when the type may spawn, a spawn tool for its own types.
// The type is read from the environment the launcher set.
func agentTools() ([]tools.Tool, error) {
	typ := os.Getenv("WICK_AGENT_TYPE")
	if typ == "" {
		return nil, fmt.Errorf("WICK_AGENT_TYPE is required for a spawned agent")
	}
	agentID := os.Getenv("WICK_AGENT_ID")
	types, err := LoadAgentTypes(cwd())
	if err != nil {
		return nil, err
	}
	t, ok := types[typ]
	if !ok {
		return nil, fmt.Errorf("unknown agent type %q", typ)
	}
	out := tools.Named(t.Tools...)
	if len(t.Spawns) > 0 {
		out = append(out, spawnToolFor(types, filtered(t.Spawns, types)))
	}
	out = append(out, submitToolFor(t, agentID))
	return out, nil
}

// filtered keeps the names that name a known type.
func filtered(names []string, types map[string]AgentType) []string {
	var out []string
	for _, n := range names {
		if _, ok := types[n]; ok {
			out = append(out, n)
		}
	}
	return out
}

// cwd is the workspace a server was started in; type loading reads it.
func cwd() string {
	d, _ := os.Getwd()
	return d
}

// spawnRequest is one entry of a spawn call. A worktree type's entry carries
// the worker fields a worktree entry carries: the ticket, and how to run it.
type spawnRequest struct {
	Type   string `json:"type"`
	Prompt string `json:"prompt"`
	Ticket string `json:"ticket,omitempty"`
	Repo   string `json:"repo,omitempty"`
	Base   string `json:"base,omitempty"`
	// Target names the branch a worker merges back into; empty derives it from
	// the repo, the way Spawn derives it.
	Target  string `json:"target,omitempty"`
	Harness string `json:"harness,omitempty"`
	Model   string `json:"model,omitempty"`
	Effort  string `json:"effort,omitempty"`
}

// spawnToolFor builds the spawn tool a caller is given. allowed names the types
// it may spawn, which is what the description lists and what an entry is
// checked against.
func spawnToolFor(types map[string]AgentType, allowed []string) tools.Tool {
	return fnTool{
		name:     "spawn",
		desc:     spawnDescription(types, allowed),
		schema:   spawnSchema(),
		readOnly: false,
		runCtx: func(ctx context.Context, root string, params map[string]any) (string, error) {
			return runSpawnCtx(ctx, root, types, allowed, params)
		},
	}
}

func spawnDescription(types map[string]AgentType, allowed []string) string {
	var b strings.Builder
	b.WriteString("Spawn one or more agents, each by type, all in parallel. ")
	if len(allowed) == 0 {
		b.WriteString("You may not spawn any agent type.")
		return b.String()
	}
	b.WriteString("You may spawn: ")
	for i, name := range allowed {
		if i > 0 {
			b.WriteString("; ")
		}
		fmt.Fprintf(&b, "%s — %s", name, types[name].Description)
	}
	b.WriteString(". Returns at once with the agents still running, for every type. " +
		"One notice with every result arrives when the last agent of this call has submitted; after spawn, end your turn and wait for it. " +
		"A blocking type's result carries its full report, a non-blocking type's a one-line summary. " +
		"Each started entry names its agent id and its pane id; tell and finish take either. " +
		"A worktree entry also takes ticket, repo, base, target, harness, model and effort.")
	return b.String()
}

func spawnSchema() map[string]any {
	return schema(map[string]any{
		"agents": map[string]any{
			"type":        "array",
			"description": "Agents to start, each with its type and prompt.",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"type":    map[string]any{"type": "string", "description": "One of the types you may spawn."},
					"prompt":  map[string]any{"type": "string", "description": "What to ask or tell the agent. Required except for a worktree type, whose ticket is its brief."},
					"ticket":  map[string]any{"type": "string", "description": "worktree type: path to the ticket file."},
					"repo":    map[string]any{"type": "string", "description": "worktree type: repo root. Defaults to the session cwd."},
					"base":    map[string]any{"type": "string", "description": "worktree type: base ref. Defaults to HEAD."},
					"target":  map[string]any{"type": "string", "description": "worktree type: branch to merge into. Defaults to the base branch."},
					"harness": map[string]any{"type": "string", "enum": []any{"pig", "claude"}, "description": "worktree type: agent to run. Defaults to the type's or the implementer slot."},
					"model":   map[string]any{"type": "string", "description": "worktree type: model passed through to the harness."},
					"effort":  map[string]any{"type": "string", "description": "worktree type: reasoning effort for the harness."},
				},
				"required": []any{"type"},
			},
		},
	}, "agents")
}

func runSpawn(root string, types map[string]AgentType, allowed []string, params map[string]any) (string, error) {
	return runSpawnCtx(context.Background(), root, types, allowed, params)
}

func runSpawnCtx(ctx context.Context, root string, types map[string]AgentType, allowed []string, params map[string]any) (string, error) {
	reqs, err := spawnRequests(params)
	if err != nil {
		return "", err
	}
	for i, r := range reqs {
		if !contains(allowed, r.Type) {
			return "", fmt.Errorf("cannot spawn type %q; you may spawn %s", r.Type, strings.Join(allowed, ", "))
		}
		t, ok := types[r.Type]
		if !ok {
			return "", fmt.Errorf("unknown agent type %q", r.Type)
		}
		// A reviewer (parent checkout) runs in the caller's own worktree; its
		// prompt is an optional note for a follow-up, so it is not required.
		if t.Checkout == "none" && strings.TrimSpace(r.Prompt) == "" {
			return "", fmt.Errorf("agents[%d].prompt is required", i)
		}
	}
	return spawnAgentsCtx(ctx, spawnExec, root, types, reqs)
}

func spawnRequests(params map[string]any) ([]spawnRequest, error) {
	raw, _ := params["agents"].([]any)
	if len(raw) == 0 {
		return nil, fmt.Errorf("agents is required: give a list of {type, prompt} entries")
	}
	out := make([]spawnRequest, 0, len(raw))
	for i, e := range raw {
		m, ok := e.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("agents[%d] is not an object", i)
		}
		r := spawnRequest{
			Type:    str(m, "type"),
			Prompt:  str(m, "prompt"),
			Ticket:  str(m, "ticket"),
			Repo:    str(m, "repo"),
			Base:    str(m, "base"),
			Target:  str(m, "target"),
			Harness: str(m, "harness"),
			Model:   str(m, "model"),
			Effort:  str(m, "effort"),
		}
		if r.Type == "" {
			return nil, fmt.Errorf("agents[%d].type is required", i)
		}
		out = append(out, r)
	}
	return out, nil
}

// spawnedAgent is one agent a spawn call started.
type spawnedAgent struct {
	ID   string
	Type AgentType
	Pane string
	Tab  string
	Sess Session
	// Harness is the harness the agent runs on. The session carries the
	// model and effort; the harness lives here because no session field
	// names it, and the spawn caller records all three on scouts.
	Harness string
	// Worktree marks an agent that owns a git worktree, which has to be
	// cleaned up through finish rather than by closing its pane.
	Worktree bool
	// Warning is a launch-time caution, such as a pig worker with no report
	// extension; it is appended to the spawn result.
	Warning string
	// Notice is a line the spawn result carries beside the agent's report,
	// such as a dead reviewer having been replaced.
	Notice string
	// Reused marks an agent that was already running for this parent: the
	// spawn re-prompted it rather than starting it, so a failed call has
	// nothing to roll back.
	Reused bool
	// Queued marks a worktree agent that did not start: the worker cap was
	// full, so it waits in the queue and a later finish starts it. It owns
	// no pane yet, so rollback only dequeues it.
	Queued bool
}

// spawnAgents starts every entry, launches spaced as always, and returns at
// once with the agents still running, for every type. The results of one
// call are a group: each member's submit is recorded as it comes, and when
// the last member has submitted, one notice carrying every result goes to
// the parent pane. A cancelled call strands nothing: the group is already
// recorded, so every result still arrives as the group notice.
func spawnAgents(run Exec, root string, types map[string]AgentType, reqs []spawnRequest) (string, error) {
	return spawnAgentsCtx(context.Background(), run, root, types, reqs)
}

// spawnAgentsCtx is spawnAgents for a call the harness can cancel. It never
// waits: the group is recorded before it returns, so a call cancelled on
// the way out still completes through its members' submits.
func spawnAgentsCtx(ctx context.Context, run Exec, root string, types map[string]AgentType, reqs []spawnRequest) (string, error) {
	_ = ctx
	// Reject the whole call before launching anything: a spawn that fails part
	// way leaves panes a parent never learned the ids of.
	for _, r := range reqs {
		t := types[r.Type]
		switch t.Checkout {
		case "none", "worktree", "parent":
		default:
			return "", fmt.Errorf("cannot spawn %q: checkout %q is not supported yet", r.Type, t.Checkout)
		}
	}
	parentPane := os.Getenv("HERDR_PANE_ID")
	parentID := os.Getenv("WICK_AGENT_ID")
	// The prune runs before any cap is read: a hand-closed worker pane frees
	// its slot, and the sweep after it reports the cancelled worker to this
	// parent through the dead-child sweep.
	pruneDeadWorkers(run)
	sweepDeadChildren(parentPane, parentID)
	groupID := newSessionID()
	// A worktree worker is its own group: its result is delivered the moment
	// it submits, so the parent merges each ticket as it lands and a freed
	// slot starts the next queued one, instead of everything waiting for the
	// slowest worker of the call. Every other agent of the call shares groupID.
	workerGroup := map[string]string{}
	groupFor := func(id string) string {
		if g, ok := workerGroup[id]; ok {
			return g
		}
		return groupID
	}
	agents := make([]*spawnedAgent, 0, len(reqs))
	// A reviewer spawn shares one packet and one check run across every
	// reviewer type in the call; it is prepared once, on the first reviewer.
	var review *reviewPacket
	// held says, for each reviewer left alone because its pass still stands,
	// that its answer is in: it was already judged at this exact state, it
	// is reported here and is not part of the group.
	var held []string
	// repeated says, for each reviewer that already judged exactly this
	// state and asked for changes, that its answer is repeated here and not
	// re-reviewed: it is not part of the group either.
	var repeated []string
	// reindexed dedupes the pre-spawn graph re-index by root: co-launched
	// reviewers of one tree share one re-index.
	reindexed := map[string]bool{}
	for _, r := range reqs {
		t := types[r.Type]
		if t.Checkout == "parent" {
			if review == nil {
				p, err := prepareReview(run, root, r.Prompt)
				if err != nil {
					closeSpawned(run, agents)
					return "", err
				}
				review = p
			}
			if review.noChanges {
				continue
			}
			// A reviewer that already passed at the tip is left alone, so a
			// re-spawn only reaches the reviewers that still have something
			// to look at. The re-index lives inside launchReviewer, at the
			// points that really launch, so a held pass indexes nothing and
			// co-launched reviewers share one.
			agent, err := launchReviewer(run, review, t, r.Prompt, parentPane, parentID, groupID, reindexed)
			if err != nil {
				closeSpawned(run, agents)
				return "", err
			}
			if agent != nil {
				// A reviewer that already judged exactly this state was not
				// re-prompted: its answer is repeated here and it is not
				// part of the group. Only that path carries the
				// already-judged notice; a re-prompted reviewer whose fake
				// submits synchronously during the launch still joins the
				// group, even though its new submit is already on disk.
				if agent.Reused && agent.Notice != "" {
					if s, ok, _ := ReadAgentSubmit(agent.ID); ok {
						line := renderSubmit(s)
						line = agent.Notice + "\n" + line
						repeated = append(repeated, line)
						continue
					}
				}
				agents = append(agents, agent)
			} else {
				held = append(held, heldPassLine(review, t))
			}
			continue
		}
		// A persistent type keeps one agent per parent: a re-spawn re-prompts
		// the live agent rather than starting a second one.
		if t.Persistent {
			if prev, ok := liveAgentFor(run, t, parentID, parentPane, r); ok {
				// Same re-index for a re-prompted reviewer or scout: the tree
				// may have moved since it last looked.
				graphReindexOnce(root, reindexed)
				agent, err := repromptAgent(run, t, prev, r, groupID)
				if err != nil {
					closeSpawned(run, agents)
					return "", err
				}
				agents = append(agents, agent)
				continue
			}
		}
		var (
			agent *spawnedAgent
			err   error
		)
		if t.Checkout == "worktree" {
			wgid := newSessionID()
			agent, err = launchWorktreeCapped(run, root, t, r, parentID, parentPane, wgid)
			if err == nil {
				workerGroup[agent.ID] = wgid
			}
		} else {
			// Same re-index for a scout: the graph matches the tree it runs in.
			graphReindexOnce(root, reindexed)
			agent, err = launchScout(run, root, t, r.Prompt)
		}
		if err != nil {
			// The call failed, so the parent never learns these ids: close
			// what was already started rather than leave it orphaned.
			closeSpawned(run, agents)
			return "", err
		}
		if agent.Queued {
			// No pane yet, so no agent state: the drain writes it with the
			// group when the worker starts. The queue entry already records
			// the parent and the group.
			agents = append(agents, agent)
			continue
		}
		// Every member joins the group and delivers through it: its submit
		// never comes back here, it joins the group's single notice.
		// A worktree agent's state also carries the ticket it was spawned
		// from, so finish can mark it done. A scout's state carries how it
		// runs instead: workers and reviewers keep theirs in their own
		// stores, but a scout has no other record for the TUI to read.
		state := AgentState{
			ID:       agent.ID,
			Type:     t.Name,
			Pane:     agent.Pane,
			Tab:      agent.Tab,
			ParentID: parentID,
			Parent:   parentPane,
			GroupID:  groupFor(agent.ID),
			Deliver:  true,
			// The harness session the launch started: transcript lookup, the
			// usage tailer and reply waiting read this current value, so a
			// restart only changes it here.
			SessionID: agent.Sess.ID,
			// The child starts now: an empty pane still counts as booting
			// rather than dead until the grace passes.
			LaunchedAt: time.Now().UTC().Format(time.RFC3339),
		}
		if t.Checkout == "worktree" {
			state.Ticket = r.Ticket
		} else {
			state.Harness, state.Model, state.Effort = agent.Harness, agent.Sess.Model, agent.Sess.Effort
		}
		if err := WriteAgentState(state); err != nil {
			closeSpawned(run, append(agents, agent))
			return "", err
		}
		agents = append(agents, agent)
	}

	// Stranded groups finalize after this call's launches, not before: a
	// dead member this call replaces waits on its replacement instead of
	// being reported lost first. It runs before every return below, so a
	// call that launches nothing still finalizes what is already complete.
	sweepOpenGroups(parentPane, parentID)
	// Nothing to review, or every reviewer already passed: report that and
	// start nothing.
	if review != nil && len(agents) == 0 {
		if review.noChanges {
			return "no changes: nothing differs from the base. Nothing to review.", nil
		}
		out := fmt.Sprintf("every reviewer passed at %s; nothing to review.\n%s", short(review.head), strings.Join(held, "\n"))
		if len(repeated) > 0 {
			out += "\n\n" + strings.Join(repeated, "\n\n")
		}
		return out, nil
	}
	if len(agents) == 0 {
		if len(held) > 0 || len(repeated) > 0 {
			out := strings.Join(append(held, repeated...), "\n\n")
			return out, nil
		}
		return "", fmt.Errorf("agents is required: give a list of {type, prompt} entries")
	}
	// One group per worktree worker, one shared group for the rest.
	var sharedMembers []string
	for _, a := range agents {
		if wg, ok := workerGroup[a.ID]; ok {
			if err := writeSpawnGroup(spawnGroup{
				ID: wg, Parent: parentPane, ParentID: parentID, Members: []string{a.ID},
			}); err != nil {
				closeSpawned(run, agents)
				return "", err
			}
			continue
		}
		sharedMembers = append(sharedMembers, a.ID)
	}
	if len(sharedMembers) > 0 {
		if err := writeSpawnGroup(spawnGroup{
			ID: groupID, Parent: parentPane, ParentID: parentID, Members: sharedMembers,
		}); err != nil {
			closeSpawned(run, agents)
			return "", err
		}
	}
	// Members that submitted synchronously during the launch (a fake herdr
	// answers a reviewer's prompt before the group file exists) already
	// complete the group: dispatch the notice in the background, through the
	// notice path, not the reply. Dispatch is asynchronous so the spawn
	// itself returns at once; the background delivery still holds for an
	// idle parent. A group that is still waiting simply stays recorded.
	if len(sharedMembers) > 0 {
		dispatchGroupDelivery(groupID)
	}
	for _, wg := range workerGroup {
		dispatchGroupDelivery(wg)
	}
	out := renderSpawnImmediate(agents)
	if len(held) > 0 {
		out += "\n\n" + strings.Join(held, "\n")
	}
	if len(repeated) > 0 {
		out += "\n\n" + strings.Join(repeated, "\n\n")
	}
	return out, nil
}

// heldPassLine tells the caller a reviewer was not started because its pass
// already covers this work.
func heldPassLine(p *reviewPacket, t AgentType) string {
	at := short(p.head)
	if prev, ok := reviewersOf(p.tree)[t.Name]; ok && prev.VerdictAt != "" {
		at = short(prev.VerdictAt)
	}
	return fmt.Sprintf("%s: PASS already on record (judged %s) and it still holds; it was not run again and no result from it is coming.", t.Name, at)
}

// closeSpawned rolls back the agents a failed call launched, so a parent that
// never learns their ids is not left with panes it cannot reach. A reused agent
// was not launched here and its parent already knows it, so it is left running.
// A rolled-back worktree records no ticket history: its ticket never ran, so
// there is no finished ticket to keep.
func closeSpawned(run Exec, agents []*spawnedAgent) {
	var queued []string
	for _, a := range agents {
		if a.Reused {
			continue
		}
		if a.Queued {
			queued = append(queued, a.ID)
			continue
		}
		if a.Worktree {
			_, _ = finishWorker(run, a.ID, false, false)
			continue
		}
		// The pane is closed before its shell may have run the exec line, so
		// the launch file needs an owner here too: the pane close alone
		// would leave it behind forever.
		if a.Sess.ID != "" {
			CleanupSession(a.Sess)
		}
		closeAgent(run, readOnlyAgent{Pane: a.Pane, Tab: a.Tab})
	}
	dequeueWorkers(queued)
	// A concurrent finish may have drained an entry this call just queued and
	// started it: the parent never learned the id, so tear the worker down
	// rather than leave it running orphaned. Its ticket never ran, so no
	// history is recorded.
	for _, id := range queued {
		if _, err := recall(id); err == nil {
			_, _ = finishWorker(run, id, false, false)
		}
	}
}

// launchScout starts one read-only agent in the caller's tab. Its pane
// closes as soon as its result is delivered; a follow-up means a new scout.
func launchScout(run Exec, root string, t AgentType, prompt string) (*spawnedAgent, error) {
	// A scout resolves the scout slot: WICK_READONLY_HARNESS is a flag layer,
	// the type's frontmatter the typeOrSpawn layer, then the slot's settings,
	// then pig.
	choice := Resolve(slotScout, readonlyHarnessChoice(),
		Choice{Harness: t.Harness, Model: t.Model, Effort: t.Effort})
	id := newSessionID()
	sess := Session{
		ID:        id,
		Role:      "agent",
		Cwd:       root,
		Model:     choice.Model,
		Effort:    choice.Effort,
		AgentType: t.Name,
		AgentID:   id,
		Dir:       filepath.Join(readOnlyDir(), "sessions"),
		// A scout inside a run archives under it: the spawning session
		// carries its spec in the environment, the way workers record it.
		Spec: os.Getenv("WICK_SPEC"),
	}
	spaceSpawns()
	agent, err := launchPlacedAgent(run, readOnlyPlace{Overflow: "scouts"}, choice.Harness, sess, renderPrompt(t, prompt))
	if err != nil {
		return nil, err
	}
	return &spawnedAgent{ID: id, Type: t, Pane: agent.Pane, Tab: agent.Tab, Sess: agent.Session, Harness: choice.Harness}, nil
}

// launchWorktree starts one worktree agent: it gets its own git worktree,
// branch and ticket tab, the way Spawn gives a worker one, and runs the type's
// brief.
func launchWorktree(run Exec, root string, t AgentType, r spawnRequest) (*spawnedAgent, error) {
	repo := r.Repo
	if repo == "" {
		repo = root
	}
	// The implementer slot resolves per field: the spawn entry beats the
	// type's frontmatter, which beats the settings file, which beats pig.
	choice := Resolve(slotImplementer,
		Choice{Harness: r.Harness, Model: r.Model, Effort: r.Effort},
		Choice{Harness: t.Harness, Model: t.Model, Effort: t.Effort})
	if r.Ticket == "" {
		return nil, fmt.Errorf("a %s entry needs ticket", t.Name)
	}
	if _, err := os.Stat(r.Ticket); err != nil {
		return nil, fmt.Errorf("cannot read ticket %s: %w", r.Ticket, err)
	}

	id := WorkerID(r.Ticket)
	target := r.Target
	if target == "" {
		target = targetBranch(run, repo, r.Base)
	}
	in := SpawnInput{
		Ticket:    r.Ticket,
		Repo:      repo,
		Base:      r.Base,
		Target:    target,
		Harness:   choice.Harness,
		Model:     choice.Model,
		Effort:    choice.Effort,
		AgentType: t.Name,
	}
	// An implement session carries its spec in the environment; every worker
	// it spawns records it, for its brief and its spec reviewer.
	in.Spec = os.Getenv("WICK_SPEC")
	brief, err := writeBrief(id, in.Ticket, in.Spec, target, choice.Harness)
	if err != nil {
		return nil, err
	}
	// The brief file stays for the archive, but the worker's first message is
	// the brief itself, not a "read this file" indirection: the launch file
	// carries it as a real argument through `wick-pig exec`.
	briefText, err := os.ReadFile(brief)
	if err != nil {
		return nil, err
	}
	sess := Session{
		ID:        newSessionID(),
		Role:      "agent",
		Cwd:       worktreePath(repo, id),
		Model:     choice.Model,
		Effort:    choice.Effort,
		Label:     id,
		Spec:      in.Spec,
		AgentType: t.Name,
		AgentID:   id,
		Prompt:    string(briefText),
	}
	launch, err := PrepareLaunch(choice.Harness, sess)
	if err != nil {
		return nil, err
	}
	in.Launch, in.Late = []string{launch.Line}, launch.Late
	in.SessionID = sess.ID

	spaceSpawns()
	worker, err := Spawn(run, in)
	if err != nil {
		// No worker means nothing ever reads the config it was given.
		CleanupSession(sess)
		return nil, err
	}
	return &spawnedAgent{
		ID: worker.ID, Type: t, Pane: worker.Pane, Tab: worker.Tab, Sess: sess,
		Worktree: true, Warning: reportExtensionWarning(choice.Harness),
	}, nil
}

// launchWorktreeCapped starts one worktree agent, or queues it when the worker
// cap is full. Worktree launches are serialized under the worker-launch lock,
// so two sessions deciding at the same instant cannot both take the last slot:
// the check and the launch (or queue) are one critical section. The launch
// itself runs inside the lock, the way Spawn already holds the workers lock
// across its own herdr calls. A queued worker joins groupID, so its submit
// joins the group's single notice when the drain starts it.
func launchWorktreeCapped(run Exec, root string, t AgentType, r spawnRequest, parentID, parentPane, groupID string) (*spawnedAgent, error) {
	var out *spawnedAgent
	err := withFileLock("worker-launch", func() error {
		// A hand-closed pane frees its slot here, before the cap is read: the
		// prune cancels it, and the count below sees only live panes.
		pruneDeadWorkers(run)
		if r.Ticket != "" {
			id := WorkerID(r.Ticket)
			store, err := loadWorkersLocked()
			if err != nil {
				return err
			}
			if _, exists := store[id]; !exists {
				queued := false
				_ = withFileLock("worker-queue", func() error {
					for _, e := range loadWorkerQueue() {
						if e.ID == id {
							queued = true
						}
					}
					return nil
				})
				if queued {
					return fmt.Errorf("worker %q is already queued", id)
				}
				if liveWorkerCount(store, run) >= effectiveWorkerCap() {
					repo := r.Repo
					if repo == "" {
						repo = root
					}
					choice := Resolve(slotImplementer,
						Choice{Harness: r.Harness, Model: r.Model, Effort: r.Effort},
						Choice{Harness: t.Harness, Model: t.Model, Effort: t.Effort})
					if _, err := os.Stat(r.Ticket); err != nil {
						return fmt.Errorf("cannot read ticket %s: %w", r.Ticket, err)
					}
					entry := queuedWorker{
						ID: id, Type: t.Name, Ticket: r.Ticket, Spec: os.Getenv("WICK_SPEC"),
						Repo: repo, Base: r.Base, Target: r.Target,
						Harness: choice.Harness, Model: choice.Model, Effort: choice.Effort,
						ParentID: parentID, Parent: parentPane, GroupID: groupID,
					}
					if err := queueWorker(entry); err != nil {
						return err
					}
					out = &spawnedAgent{ID: id, Type: t, Worktree: true, Queued: true}
					return nil
				}
			}
		}
		agent, err := launchWorktree(run, root, t, r)
		if err != nil {
			return err
		}
		out = agent
		return nil
	})
	return out, err
}

// renderPrompt fills a type's prompt template with the prompt the parent gave.
// A body with no {{prompt}} placeholder gets the prompt appended.
func renderPrompt(t AgentType, prompt string) string {
	if strings.Contains(t.Body, "{{prompt}}") {
		return strings.ReplaceAll(t.Body, "{{prompt}}", prompt)
	}
	if prompt == "" {
		return t.Body
	}
	return strings.TrimSpace(t.Body) + "\n\n" + prompt
}

// renderSpawnImmediate is the spawn reply: every member reported as started
// (blocking or not: the spawn never waits), each queued worktree agent
// reported as queued behind the worker cap, each launch notice beside its
// agent, and a closing line saying one notice with every result arrives and
// the caller should end its turn. It never carries a report and never tells
// the caller to call again. Each started entry names its agent id and its
// pane id, so the caller can address it either way.
func renderSpawnImmediate(agents []*spawnedAgent) string {
	var parts []string
	for _, a := range agents {
		if a.Queued {
			parts = append(parts, fmt.Sprintf("%s %s queued behind the worker cap (%d max); it starts as earlier workers finish.",
				a.Type.Name, a.ID, effectiveWorkerCap()))
			continue
		}
		line := fmt.Sprintf("%s %s started (pane %s).", a.Type.Name, a.ID, a.Pane)
		if a.Tab != "" {
			line = fmt.Sprintf("%s %s started in %s (pane %s).", a.Type.Name, a.ID, a.Tab, a.Pane)
		}
		if a.Notice != "" {
			line += "\n" + a.Notice
		}
		parts = append(parts, line+a.Warning)
	}
	parts = append(parts, "One notice with every result arrives when the last agent of this call has submitted; after spawn, end your turn and wait for it.")
	return strings.Join(parts, "\n\n")
}

// liveAgentFor finds the live agent a persistent type is re-prompted as rather
// than replaced: the agent of that type this parent already has running. A
// worktree type is looked up by the id its ticket derives, so only a re-spawn
// of the same ticket finds the agent working it; a read-only type keeps one
// agent per parent.
func liveAgentFor(run Exec, t AgentType, parentID, parentPane string, r spawnRequest) (AgentState, bool) {
	if t.Checkout == "worktree" {
		if r.Ticket == "" {
			return AgentState{}, false
		}
		id := WorkerID(r.Ticket)
		s, ok, err := ReadAgentState(id)
		if err != nil || !ok || !liveFor(s, t.Name, parentID, parentPane, run) {
			return AgentState{}, false
		}
		s.ID = id
		return s, true
	}
	entries, err := os.ReadDir(agentStateDir())
	if err != nil {
		return AgentState{}, false
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		s, ok, err := ReadAgentState(id)
		if err != nil || !ok || !liveFor(s, t.Name, parentID, parentPane, run) {
			continue
		}
		s.ID = id
		return s, true
	}
	return AgentState{}, false
}

// liveFor reports whether an agent's state is the live agent of this type for
// this parent. A session with an agent id names its children by it; a
// hand-started wick-mcp with no id falls back to its pane, so two sessions
// sharing a state dir keep their agents apart.
func liveFor(s AgentState, typ, parentID, parentPane string, run Exec) bool {
	if s.Type != typ || s.Pane == "" {
		return false
	}
	if parentID != "" {
		if s.ParentID != parentID {
			return false
		}
	} else if s.Parent != parentPane {
		return false
	}
	return readOnlyAgent{Pane: s.Pane}.alive(run)
}

// repromptAgent reuses a live agent of a persistent type instead of starting a
// new one: it sends the prompt the launch would have given it as the message
// itself through the shared DeliverPrompt path, forgets the previous turn's
// submit and points the agent's next submit at groupID's single notice, so
// the caller ends its turn exactly as for a fresh launch. The prompt file
// stays as an archive copy; the message never names it.
func repromptAgent(run Exec, t AgentType, s AgentState, r spawnRequest, groupID string) (*spawnedAgent, error) {
	prompt := renderPrompt(t, r.Prompt)
	// The archive copy stays; the message never names it. A worktree agent's
	// brief is already on disk beside the worker store and stays untouched:
	// the steering text gets its own copy.
	if err := os.MkdirAll(readOnlyDir(), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(readOnlyDir(), s.ID+".prompt"), []byte(prompt), 0o644); err != nil {
		return nil, err
	}
	clearAgentSubmit(s.ID)
	clearChildError(s.ID)
	s.GroupID, s.Deliver = groupID, true
	s.Errored, s.ErrorDetail, s.LostNotified = false, "", false
	if err := WriteAgentState(s); err != nil {
		return nil, err
	}
	// The reused pane keeps the agent's name, so a stale name never
	// lingers. Silent when the rename fails.
	renamePane(run, s.Pane, AgentPaneName(t))
	if err := DeliverPrompt(run, s.Pane, prompt); err != nil {
		return nil, err
	}
	return &spawnedAgent{ID: s.ID, Type: t, Pane: s.Pane, Worktree: t.Checkout == "worktree", Reused: true}, nil
}
