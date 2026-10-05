package wick

import (
	"encoding/json"
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
	"implement": {"scout"},
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

// spawnRequest is one entry of a spawn call.
type spawnRequest struct {
	Type   string `json:"type"`
	Prompt string `json:"prompt"`
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
	b.WriteString(". Blocking types return their reports together, in the order given, once all have submitted.")
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
					"type":   map[string]any{"type": "string", "description": "One of the types you may spawn."},
					"prompt": map[string]any{"type": "string", "description": "What to ask or tell the agent."},
				},
				"required": []any{"type", "prompt"},
			},
		},
	}, "agents")
}

func runSpawn(root string, types map[string]AgentType, allowed []string, params map[string]any) (string, error) {
	reqs, err := spawnRequests(params)
	if err != nil {
		return "", err
	}
	for _, r := range reqs {
		if !contains(allowed, r.Type) {
			return "", fmt.Errorf("cannot spawn type %q; you may spawn %s", r.Type, strings.Join(allowed, ", "))
		}
		if _, ok := types[r.Type]; !ok {
			return "", fmt.Errorf("unknown agent type %q", r.Type)
		}
	}
	// A re-call with the same agents rejoins the first call rather than
	// starting them twice: the answer is keyed by what was asked.
	return once(keyOf("spawn", canonicalRequests(reqs)), func() (string, error) {
		return spawnAgents(spawnExec, root, types, reqs)
	})
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
		r := spawnRequest{Type: str(m, "type"), Prompt: str(m, "prompt")}
		if r.Type == "" {
			return nil, fmt.Errorf("agents[%d].type is required", i)
		}
		if r.Prompt == "" {
			return nil, fmt.Errorf("agents[%d].prompt is required", i)
		}
		out = append(out, r)
	}
	return out, nil
}

func canonicalRequests(reqs []spawnRequest) string {
	raw, _ := json.Marshal(reqs)
	return string(raw)
}

// spawnedAgent is one agent a spawn call started.
type spawnedAgent struct {
	ID   string
	Type AgentType
	Pane string
	Tab  string
	Sess Session
}

// spawnAgents starts every entry, launches spaced as always, then waits for the
// blocking ones to submit and returns their reports in list order.
func spawnAgents(run Exec, root string, types map[string]AgentType, reqs []spawnRequest) (string, error) {
	// Reject the whole call before launching anything: a spawn that fails part
	// way leaves panes a parent never learned the ids of.
	for _, r := range reqs {
		if t := types[r.Type]; t.Checkout != "none" {
			return "", fmt.Errorf("cannot spawn %q: checkout %q is not supported yet", r.Type, t.Checkout)
		}
	}
	agents := make([]*spawnedAgent, 0, len(reqs))
	for _, r := range reqs {
		agent, err := launchScout(run, root, types[r.Type], r.Prompt)
		if err != nil {
			// The call failed, so the parent never learns these ids: close
			// the panes already started rather than leave them orphaned.
			closeSpawned(run, agents)
			return "", err
		}
		agents = append(agents, agent)
	}

	deadline := time.Now().Add(readOnlyTimeout)
	for {
		if allSubmitted(agents) || !time.Now().Before(deadline) {
			break
		}
		time.Sleep(pollEvery)
	}
	return renderSpawn(agents), nil
}

func closeSpawned(run Exec, agents []*spawnedAgent) {
	for _, a := range agents {
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

func renderSpawn(agents []*spawnedAgent) string {
	var b strings.Builder
	for i, a := range agents {
		if i > 0 {
			b.WriteString("\n\n")
		}
		if !a.Type.Blocking {
			fmt.Fprintf(&b, "%s %s started.", a.Type.Name, a.ID)
			continue
		}
		if s, ok, _ := ReadAgentSubmit(a.ID); ok {
			body := s.Report
			if s.Report == "" {
				body = s.Summary
			}
			fmt.Fprintf(&b, "%s %s submitted (%s):\n%s", a.Type.Name, a.ID, s.Status, body)
		} else {
			fmt.Fprintf(&b, "%s %s is still running; call spawn again with the same agents for its report.",
				a.Type.Name, a.ID)
		}
	}
	return b.String()
}
