package wick

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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
		spawnToolFor(types, spawnable), waitTool, finishTool, tellTool, codeReviewTool,
	}
}

// AllTools is every tool wick serves, file tools first.
func AllTools() []tools.Tool {
	return append(tools.All(), OrchestrationTools()...)
}

// spawnExec is the exec the spawn tool drives. A var, like waitExec, so a test
// can spawn against a fake herdr and git rather than a real one.
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

// WaitCap is the longest wait_workers may block. A harness cuts a tool call at
// its own MCP_TOOL_TIMEOUT, so a wait that outlives it fails while the wait
// goroutine keeps running: the model sees an error and the worker is still
// going. Staying under the cap means the wait always answers, and a caller that
// wants longer calls again.
const WaitCap = ToolBudget

// waitCap is WaitCap in use, a var only so a test can shrink it.
var waitCap = WaitCap

// waitExec is the exec wait_workers polls through. A var so the tool can be
// driven by a fake exec rather than a real herdr.
var waitExec Exec = osExec

var waitTool = fnTool{
	name: "wait_workers",
	desc: fmt.Sprintf("Worker status. Omit workers for all; timeout_ms 0 returns immediately. "+
		"timeout_ms is capped at %d ms: a longer request is clamped to that and the result says so.",
		WaitCap/time.Millisecond),
	schema: schema(map[string]any{
		"workers":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Worker ids. Omit for all."},
		"timeout_ms": map[string]any{"type": "integer", "description": fmt.Sprintf("Block up to this long. 0 returns now. Capped at %d.", WaitCap/time.Millisecond)},
	}),
	run: func(_ string, params map[string]any) (string, error) {
		timeout := time.Duration(num(params, "timeout_ms")) * time.Millisecond
		clamped := timeout > waitCap
		if clamped {
			timeout = waitCap
		}
		workers, pending, err := Wait(waitExec, strs(params, "workers"), timeout)
		if err != nil {
			return "", err
		}
		out := renderStatus(workers, pending, reportCandidates(runningExe(), os.Getenv("HOME")))
		if clamped {
			out += fmt.Sprintf("\ntimeout_ms was clamped to the %d ms cap, so this wait ended early. Call wait_workers again for the rest.",
				waitCap/time.Millisecond)
		}
		return out, nil
	},
}

var finishTool = fnTool{
	name: "finish_worker",
	desc: "Take a finished worker: rebase and fast-forward it into its target branch, then drop its worktree and close its reviewers. " +
		"It merges only once both of the worker's reviewers passed at its tip; merge=false discards it regardless.",
	schema: schema(map[string]any{
		"worker": map[string]any{"type": "string", "description": "Worker id."},
		"merge":  map[string]any{"type": "boolean", "description": "False discards the branch. Defaults to true."},
	}, "worker"),
	run: func(_ string, params map[string]any) (string, error) {
		id := str(params, "worker")
		if id == "" {
			return "", fmt.Errorf("worker is required")
		}
		merge := true
		if b, ok := params["merge"].(bool); ok {
			merge = b
		}
		result, err := Finish(osExec, id, merge)
		if err != nil {
			return "", err
		}
		return renderFinish(result), nil
	},
}

// tellExec is the exec tell_worker drives. A var, like waitExec, so a test can
// stand in for herdr.
var tellExec Exec = osExec

// tell_worker messages a running worker in its own input, so a steer reaches
// the model instead of the shell.
var tellTool = fnTool{
	name: "tell_worker",
	desc: "Send a message to a running worker. Use for a steer, or to answer a question it raised.",
	schema: schema(map[string]any{
		"worker": map[string]any{"type": "string", "description": "Worker id."},
		"text":   map[string]any{"type": "string", "description": "What to tell it."},
	}, "worker", "text"),
	run: func(_ string, params map[string]any) (string, error) {
		id, text := str(params, "worker"), str(params, "text")
		if id == "" || text == "" {
			return "", fmt.Errorf("worker and text are required")
		}
		worker, err := recall(id)
		if err != nil {
			return "", err
		}
		settled, err := Tell(tellExec, worker.Pane, text, toolBudget)
		if err != nil {
			return "", fmt.Errorf("could not deliver to %s: %w", id, err)
		}
		if !settled {
			return fmt.Sprintf("delivered to %s; it is still working. Call wait_workers to see it settle.", id), nil
		}
		return fmt.Sprintf("delivered to %s and waited for it to settle", id), nil
	},
}

// reviewExec is the exec a self-review runs through. A var, like waitExec, so
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
