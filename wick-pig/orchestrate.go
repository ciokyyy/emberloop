package wick

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"example.com/wick/tools"
)

// fnTool is one orchestration tool: a name, a schema and a handler over the
// decoded arguments. They run through osExec, so the MCP server and any other
// plain process can serve them.
type fnTool struct {
	name, desc string
	schema     map[string]any
	readOnly   bool
	run        func(root string, params map[string]any) (string, error)
}

func (t fnTool) Name() string           { return t.name }
func (t fnTool) Description() string    { return t.desc }
func (t fnTool) Schema() map[string]any { return t.schema }
func (t fnTool) ReadOnly() bool         { return t.readOnly }
func (t fnTool) Execute(_ context.Context, root string, args json.RawMessage) (string, error) {
	params := map[string]any{}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &params); err != nil {
			return "", fmt.Errorf("invalid %s parameters: %w", t.name, err)
		}
	}
	return t.run(root, params)
}

func schema(properties map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": properties}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

// OrchestrationTools are the delegation tools, with spawn bound to every type.
// A role gets its own spawn, listing the types it may spawn, through RoleTools.
func OrchestrationTools() []tools.Tool {
	types := mustLoadAgentTypes(cwd())
	return orchestrationTools(types, allTypeNames(types))
}

// orchestrationTools builds the delegation tools with spawn bound to the given
// spawnable types.
func orchestrationTools(types map[string]AgentType, spawnable []string) []tools.Tool {
	return []tools.Tool{
		spawnToolFor(types, spawnable), finishTool, tellTool, codeReviewTool,
	}
}

// AllTools is every tool wick serves, file tools first.
func AllTools() []tools.Tool {
	return append(tools.All(), OrchestrationTools()...)
}

// spawnExec is the exec the spawn tool drives. A var, like finishExec, so a
// test can spawn against a fake herdr and git rather than a real one.
var spawnExec Exec = osExec

// spawnGap spaces launches: a run that opens every agent's first request at
// the same instant is what the provider answers with empty bodies. A var so
// tests can zero it.
var spawnGap = 3 * time.Second

var (
	spawnGate sync.Mutex
	lastSpawn time.Time
)

func spaceSpawns() {
	spawnGate.Lock()
	defer spawnGate.Unlock()
	if wait := spawnGap - time.Since(lastSpawn); wait > 0 {
		time.Sleep(wait)
	}
	lastSpawn = time.Now()
}

// finishExec is the exec the finish tool drives. A var, so a test can stand in
// for herdr and git.
var finishExec Exec = osExec

// requireWorktreeAgent refuses finish for any agent whose type has no worktree
// of its own: there is nothing to merge or discard.
func requireWorktreeAgent(id string) error {
	st, ok, err := ReadAgentState(id)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("unknown agent %q", id)
	}
	if t, ok := mustLoadAgentTypes(cwd())[st.Type]; ok {
		if t.Checkout == "worktree" {
			return nil
		}
		return fmt.Errorf("finish works only for a worktree agent; %s is a %s (checkout %q)", id, t.Name, t.Checkout)
	}
	// Its type definition is gone from this repo, but a worktree agent still
	// holds a worktree record: finish that rather than strand it.
	if w, err := recall(id); err == nil && w.Worktree != "" {
		return nil
	}
	return fmt.Errorf("agent %s has unknown type %q", id, st.Type)
}

var finishTool = fnTool{
	name: "finish",
	desc: "Take a finished worktree agent: rebase and fast-forward it into its target branch, then drop its worktree and close its reviewers. " +
		"It merges only once both of the agent's reviewers passed at its tip; merge=false discards it regardless. " +
		"Works only for a worktree agent.",
	schema: schema(map[string]any{
		"agent": map[string]any{"type": "string", "description": "Agent id."},
		"merge": map[string]any{"type": "boolean", "description": "False discards the branch. Defaults to true."},
	}, "agent"),
	run: func(_ string, params map[string]any) (string, error) {
		id := str(params, "agent")
		if id == "" {
			return "", fmt.Errorf("agent is required")
		}
		merge := true
		if b, ok := params["merge"].(bool); ok {
			merge = b
		}
		if err := requireWorktreeAgent(id); err != nil {
			return "", err
		}
		result, err := Finish(finishExec, id, merge)
		if err != nil {
			return "", err
		}
		return renderFinish(result), nil
	},
}

// tell messages a running agent in its own input, so a steer reaches the model
// instead of the shell. It works for any agent id: a non-blocking agent's
// result comes back later as a notice, a blocking agent's next submit is
// returned.
var tellTool = fnTool{
	name: "tell",
	desc: "Send a message to a running agent. For a non-blocking agent it returns at once, and the agent's next result arrives as a system notice. " +
		"For a blocking agent it waits for the agent's next submit and returns it; past the tool budget the agent is backgrounded and the result arrives as a notice.",
	schema: schema(map[string]any{
		"agent": map[string]any{"type": "string", "description": "Agent id."},
		"text":  map[string]any{"type": "string", "description": "What to tell it."},
	}, "agent", "text"),
	run: func(_ string, params map[string]any) (string, error) {
		id, text := str(params, "agent"), str(params, "text")
		if id == "" || text == "" {
			return "", fmt.Errorf("agent and text are required")
		}
		return tellAgent(tellExec, id, text)
	},
}

// reviewExec is the exec a self-review runs through. A var, like spawnExec, so
// a test can drive the tool with a fake reviewer instead of a real agent.
var reviewExec Exec = osExec

var codeReviewTool = fnTool{
	name: "code_review",
	desc: "Have this worktree reviewed by two read-only reviewers: code (correctness and standards) and spec (does it do the ticket, and only the ticket). " +
		"They stay open for the whole ticket: call again after fixing, and they review what changed. " +
		"By default a call goes to every reviewer still asking for changes. It covers committed and uncommitted work; commit first. " +
		"If it answers \"still running\", call it again with the same arguments.",
	schema: schema(map[string]any{
		"reviewer": map[string]any{"type": "string", "enum": []any{axisCode, axisSpec}, "description": "Only this reviewer. Default: every reviewer with open findings."},
		"note":     map[string]any{"type": "string", "description": "Your answer to the findings: what you fixed, and what you decline and why."},
	}),
	run: func(root string, params map[string]any) (string, error) {
		return once("code_review "+root, func() (string, error) {
			return CodeReview(reviewExec, root, str(params, "reviewer"), str(params, "note"))
		})
	},
}
