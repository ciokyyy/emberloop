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
// the worker fields spawn_worker took: the ticket, and how to run it.
type spawnRequest struct {
	Type   string `json:"type"`
	Prompt string `json:"prompt"`
	Ticket string `json:"ticket,omitempty"`
	Repo   string `json:"repo,omitempty"`
	Base   string `json:"base,omitempty"`
	// Target names the branch a worker merges back into; empty derives it from
	// the repo, as spawn_worker did.
	Target  string `json:"target,omitempty"`
	Harness string `json:"harness,omitempty"`
	Model   string `json:"model,omitempty"`
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
		"A worktree entry also takes ticket, repo, base, target, harness and model.")
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
					"harness": map[string]any{"type": "string", "enum": []any{"pig", "claude"}, "description": "worktree type: agent to run. Defaults to the type's or pig."},
					"model":   map[string]any{"type": "string", "description": "worktree type: model passed through to the harness."},
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
		if t.Checkout != "worktree" && strings.TrimSpace(r.Prompt) == "" {
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
		case "none", "worktree":
		default:
			return "", fmt.Errorf("cannot spawn %q: checkout %q is not supported yet", r.Type, t.Checkout)
		}
	}

	parentPane := os.Getenv("HERDR_PANE_ID")
	parentID := os.Getenv("WICK_AGENT_ID")
	agents := make([]*spawnedAgent, 0, len(reqs))
	for _, r := range reqs {
		t := types[r.Type]
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

func closeSpawned(run Exec, agents []*spawnedAgent) {
	for _, a := range agents {
		if a.Worktree {
			_, _ = Finish(run, a.ID, false)
			continue
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
	harness := t.Harness
	if harness == "" {
		harness = os.Getenv("WICK_READONLY_HARNESS")
	}
	if harness == "" {
		harness = "pig"
	}
	id := newSessionID()
	sess := Session{
		ID:        id,
		Role:      "agent",
		Cwd:       root,
		Model:     t.Model,
		AgentType: t.Name,
		AgentID:   id,
		Dir:       filepath.Join(readOnlyDir(), "sessions"),
	}
	spaceSpawns()
	agent, err := launchPlacedAgent(run, readOnlyPlace{Overflow: "scouts"}, harness, sess, renderPrompt(t, prompt))
	if err != nil {
		return nil, err
	}
	return &spawnedAgent{ID: id, Type: t, Pane: agent.Pane, Tab: agent.Tab, Sess: agent.Session}, nil
}

// launchWorktree starts one worktree agent: it gets its own git worktree,
// branch and ticket tab exactly as spawn_worker did, and runs the type's brief.
func launchWorktree(run Exec, root string, t AgentType, r spawnRequest) (*spawnedAgent, error) {
	repo := r.Repo
	if repo == "" {
		repo = root
	}
	harness := r.Harness
	if harness == "" {
		harness = t.Harness
	}
	if harness == "" {
		harness = "pig"
	}
	model := r.Model
	if model == "" {
		model = t.Model
	}
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
		Ticket:  r.Ticket,
		Repo:    repo,
		Base:    r.Base,
		Target:  target,
		Harness: harness,
		Model:   model,
	}
	// An implement session carries its spec in the environment; every worker
	// it spawns records it, for its brief and its spec reviewer.
	in.Spec = os.Getenv("WICK_SPEC")
	brief, err := writeBrief(id, in.Ticket, in.Spec, target)
	if err != nil {
		return nil, err
	}
	sess := Session{
		ID:        newSessionID(),
		Role:      "agent",
		Cwd:       worktreePath(repo, id),
		Model:     model,
		Label:     id,
		Spec:      in.Spec,
		AgentType: t.Name,
		AgentID:   id,
		// The brief sits beside the worker store, outside the worktree;
		// read opens it by absolute path.
		Prompt: fmt.Sprintf("Read %s and follow it exactly.", brief),
	}
	launch, err := PrepareLaunch(harness, sess)
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
		Worktree: true, Warning: reportExtensionWarning(harness),
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
			parts = append(parts, renderSubmit(s))
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
