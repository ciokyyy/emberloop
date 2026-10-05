package wick

import (
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
		run: func(root string, params map[string]any) (string, error) {
			return runSpawn(root, types, allowed, params)
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
	b.WriteString(". Blocking types return their reports together, in the order given, once all have submitted. " +
		"A non-blocking type comes back as started at once and reports later as a system notice. " +
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
	return spawnAgents(spawnExec, root, types, reqs)
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
}

// spawnAgents starts every entry, launches spaced as always, then waits at most
// the tool budget for the blocking ones to submit. Only the blocking entries
// are waited on; a non-blocking one is reported as started at once. When the
// budget runs out, the reports already in come back and the rest are
// backgrounded: their submits are delivered as notices, so the caller never
// calls the tool again.
func spawnAgents(run Exec, root string, types map[string]AgentType, reqs []spawnRequest) (string, error) {
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
	agents := make([]*spawnedAgent, 0, len(reqs))
	// A reviewer spawn shares one packet and one check run across every
	// reviewer type in the call; it is prepared once, on the first reviewer.
	var review *reviewPacket
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
			// to look at.
			agent, err := launchReviewer(run, review, t, r.Prompt, parentPane, parentID)
			if err != nil {
				closeSpawned(run, agents)
				return "", err
			}
			if agent != nil {
				agents = append(agents, agent)
			}
			continue
		}
		// A persistent type keeps one agent per parent: a re-spawn re-prompts
		// the live agent rather than starting a second one.
		if t.Persistent {
			if prev, ok := liveAgentFor(run, t, parentID, parentPane, r); ok {
				agent, err := repromptAgent(run, t, prev, r)
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
			agent, err = launchWorktree(run, root, t, r)
		} else {
			agent, err = launchScout(run, root, t, r.Prompt)
		}
		if err != nil {
			// The call failed, so the parent never learns these ids: close
			// what was already started rather than leave it orphaned.
			closeSpawned(run, agents)
			return "", err
		}
		// The child's own state says where its submit goes: a non-blocking
		// agent's from the start, a blocking one's once it is backgrounded.
		state := AgentState{
			ID:       agent.ID,
			Type:     t.Name,
			Pane:     agent.Pane,
			ParentID: parentID,
			Parent:   parentPane,
			Deliver:  !t.Blocking,
		}
		if err := WriteAgentState(state); err != nil {
			closeSpawned(run, append(agents, agent))
			return "", err
		}
		agents = append(agents, agent)
	}

	deadline := time.Now().Add(toolBudget)
	// Nothing to review, or every reviewer already passed: report that and
	// start nothing.
	if review != nil && len(agents) == 0 {
		if review.noChanges {
			return "no changes: nothing differs from the base. Nothing to review.", nil
		}
		return fmt.Sprintf("every reviewer passed at %s; nothing to review.", short(review.head)), nil
	}
	for {
		if allSubmitted(agents) || !time.Now().Before(deadline) {
			break
		}
		time.Sleep(pollEvery)
	}

	// Whatever blocking agent is still out became backgrounded: from now on
	// its submit is delivered to the parent as a notice. backgroundAgent takes
	// the child's lock and re-checks its submit, so an agent that submitted in
	// the meantime is returned here instead of being reported twice.
	var backgrounded []string
	for _, a := range agents {
		if !a.Type.Blocking {
			continue
		}
		bg, err := backgroundAgent(a.ID)
		if err != nil {
			closeSpawned(run, agents)
			return "", err
		}
		if bg {
			backgrounded = append(backgrounded, a.ID)
		}
	}
	return renderSpawn(agents, backgrounded), nil
}

// closeSpawned rolls back the agents a failed call launched, so a parent that
// never learns their ids is not left with panes it cannot reach. A reused agent
// was not launched here and its parent already knows it, so it is left running.
func closeSpawned(run Exec, agents []*spawnedAgent) {
	for _, a := range agents {
		if a.Reused {
			continue
		}
		if a.Worktree {
			_, _ = Finish(run, a.ID, false)
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
}

func allSubmitted(agents []*spawnedAgent) bool {
	for _, a := range agents {
		if !a.Type.Blocking {
			continue
		}
		if _, ok, _ := ReadAgentSubmit(a.ID); !ok {
			return false
		}
	}
	return true
}

// launchScout starts one read-only agent in the caller's tab and leaves it
// running, so a parent can send it a follow-up.
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
	}
	spaceSpawns()
	agent, err := launchPlacedAgent(run, readOnlyPlace{Overflow: "scouts"}, choice.Harness, sess, renderPrompt(t, prompt))
	if err != nil {
		return nil, err
	}
	return &spawnedAgent{ID: id, Type: t, Pane: agent.Pane, Tab: agent.Tab, Sess: agent.Session}, nil
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
	brief, err := writeBrief(id, in.Ticket, in.Spec, target)
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

// renderSpawn is the spawn result: each non-blocking agent reported as started,
// each blocking agent's report with it, and a closing line for the callers who
// must end their turn because a result is still to arrive. A backgrounded
// agent's result is not repeated here: it arrives as a notice. It never tells
// the caller to call again.
func renderSpawn(agents []*spawnedAgent, backgrounded []string) string {
	var parts []string
	startedNonBlocking := false
	for _, a := range agents {
		if !a.Type.Blocking {
			startedNonBlocking = true
			line := fmt.Sprintf("%s %s started.", a.Type.Name, a.ID)
			if a.Tab != "" {
				line = fmt.Sprintf("%s %s started in %s.", a.Type.Name, a.ID, a.Tab)
			}
			parts = append(parts, line+a.Warning)
			continue
		}
		if contains(backgrounded, a.ID) {
			continue
		}
		if s, ok, _ := ReadAgentSubmit(a.ID); ok {
			line := renderSubmit(s)
			if a.Notice != "" {
				line = a.Notice + "\n" + line
			}
			parts = append(parts, line)
		}
	}
	if len(backgrounded) > 0 {
		parts = append(parts, fmt.Sprintf("still running: %s. Their results will arrive as system notices; end your turn.",
			strings.Join(backgrounded, ", ")))
	} else if startedNonBlocking {
		parts = append(parts, "Their results will arrive as system notices; end your turn.")
	}
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
// new one: it hands the agent the prompt the launch would have given it,
// forgets the previous turn's submit and points the agent's next result back at
// the waiting spawn, so the caller waits on the new turn exactly as it would on
// a fresh launch.
func repromptAgent(run Exec, t AgentType, s AgentState, r spawnRequest) (*spawnedAgent, error) {
	promptPath := filepath.Join(readOnlyDir(), s.ID+".prompt")
	if t.Checkout == "worktree" {
		// A worktree agent's brief is already on disk, beside the worker store.
		promptPath = briefPath(s.ID)
	} else {
		if err := os.MkdirAll(readOnlyDir(), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(promptPath, []byte(renderPrompt(t, r.Prompt)), 0o644); err != nil {
			return nil, err
		}
	}
	clearAgentSubmit(s.ID)
	s.Deliver = !t.Blocking
	if err := WriteAgentState(s); err != nil {
		return nil, err
	}
	if err := Nudge(run, s.Pane, fmt.Sprintf("Read %s and follow it exactly.", promptPath)); err != nil {
		return nil, err
	}
	return &spawnedAgent{ID: s.ID, Type: t, Pane: s.Pane, Worktree: t.Checkout == "worktree", Reused: true}, nil
}
