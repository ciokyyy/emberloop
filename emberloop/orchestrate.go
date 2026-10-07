package emberloop

import (
	"context"
	"encoding/json"
	"example.com/emberloop/tools"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// fnTool is one orchestration tool: a name, a schema and a handler over the
// decoded arguments. They run through osExec, so the MCP server and any other
// plain process can serve them.
type fnTool struct {
	name, desc string
	schema     map[string]any
	readOnly   bool
	run        func(root string, params map[string]any) (string, error)
	// runCtx, when set, is used instead of run and receives the call's
	// context, which ends when the harness cancels the call.
	runCtx func(ctx context.Context, root string, params map[string]any) (string, error)
}

func (t fnTool) Name() string           { return t.name }
func (t fnTool) Description() string    { return t.desc }
func (t fnTool) Schema() map[string]any { return t.schema }
func (t fnTool) ReadOnly() bool         { return t.readOnly }
func (t fnTool) Execute(ctx context.Context, root string, args json.RawMessage) (string, error) {
	params := map[string]any{}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &params); err != nil {
			return "", fmt.Errorf("invalid %s parameters: %w", t.name, err)
		}
	}
	if t.runCtx != nil {
		return t.runCtx(ctx, root, params)
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
		spawnToolFor(types, spawnable), finishTool, tellTool,
	}
}

// AllTools is every tool emberloop serves, file tools first.
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

var spawnGate sync.Mutex

// spawnTimestampPath is the shared launch timestamp, beside the worker store so
// every session (every emberloop-mcp process) shares it. A per-process timestamp
// cannot space launches from separate sessions.
func spawnTimestampPath() string { return filepath.Join(filepath.Dir(statePath), "last-spawn") }

// readSpawnTimestamp returns the last reserved launch time, or zero when none
// was recorded yet. A corrupt file reads as zero rather than blocking launches.
func readSpawnTimestamp() time.Time {
	raw, err := os.ReadFile(spawnTimestampPath())
	if err != nil {
		return time.Time{}
	}
	nanos, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(0, nanos)
}

// writeSpawnTimestamp records t as the last reserved launch time, atomically,
// so a concurrent reader never sees a half-written file.
func writeSpawnTimestamp(t time.Time) {
	_ = os.MkdirAll(filepath.Dir(spawnTimestampPath()), 0o755)
	tmp := spawnTimestampPath() + ".tmp"
	raw := []byte(strconv.FormatInt(t.UnixNano(), 10) + "\n")
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, spawnTimestampPath())
}

// spaceSpawns spaces launches by the configured gap, across sessions. The next
// slot is reserved under a cross-process lock before sleeping, so two processes
// launching at the same instant take successive slots: the first proceeds at
// once and records now, the second records now+gap and waits for it. The sleep
// happens outside both locks, so a slow launcher never blocks a reservation.
func spaceSpawns() {
	spawnGate.Lock()
	var wait time.Duration
	_ = withFileLock("spawn-spacing", func() error {
		last := readSpawnTimestamp()
		now := time.Now()
		scheduled := now
		if last.Add(spawnGap).After(now) {
			scheduled = last.Add(spawnGap)
			wait = scheduled.Sub(now)
		}
		writeSpawnTimestamp(scheduled)
		return nil
	})
	spawnGate.Unlock()
	if wait > 0 {
		time.Sleep(wait)
	}
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
		"It merges only once every reviewer type the agent's type requires passed at the agent's tip; merge=false discards it regardless. " +
		"Works only for a worktree agent. Give exactly one of agent (agent id) or pane (the herdr pane id).",
	schema: schema(map[string]any{
		"agent": map[string]any{"type": "string", "description": "Agent id. Exactly one of agent or pane is required."},
		"pane":  map[string]any{"type": "string", "description": "Herdr pane id. Exactly one of agent or pane is required."},
		"merge": map[string]any{"type": "boolean", "description": "False discards the branch. Defaults to true."},
	}),
	run: func(_ string, params map[string]any) (string, error) {
		// Each parent action sweeps its stranded groups first, in the
		// background: a vanished member with no further submit is finalized
		// after this call ends instead of waiting forever. The dead-child sweep
		// runs beside it, so a cancelled worker is reported here too.
		sweepOpenGroups(os.Getenv("HERDR_PANE_ID"), os.Getenv("EMBERLOOP_AGENT_ID"))
		sweepDeadChildren(os.Getenv("HERDR_PANE_ID"), os.Getenv("EMBERLOOP_AGENT_ID"))
		id, err := resolveAgentParam(params, finishExec)
		if err != nil {
			return "", err
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
// instead of the shell. It works for any agent, addressed by id or by pane:
// a non-blocking agent's result comes back later as a notice, a blocking
// agent's next submit is returned.
var tellTool = fnTool{
	name: "tell",
	desc: "Send a message to a running agent. For a non-blocking agent it returns at once, and the agent's next result arrives as a system notice. " +
		"For a blocking agent it waits for the agent's next submit and returns it; past the tool budget the agent is backgrounded and the result arrives as a notice. " +
		"Give exactly one of agent (agent id) or pane (the herdr pane id).",
	schema: schema(map[string]any{
		"agent": map[string]any{"type": "string", "description": "Agent id. Exactly one of agent or pane is required."},
		"pane":  map[string]any{"type": "string", "description": "Herdr pane id. Exactly one of agent or pane is required."},
		"text":  map[string]any{"type": "string", "description": "What to tell it."},
	}, "text"),
	runCtx: func(ctx context.Context, _ string, params map[string]any) (string, error) {
		id, err := resolveAgentParam(params, tellExec)
		if err != nil {
			return "", err
		}
		text := str(params, "text")
		if text == "" {
			return "", fmt.Errorf("text is required")
		}
		return tellAgentCtx(ctx, tellExec, id, text)
	},
}
