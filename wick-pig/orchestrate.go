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

// OrchestrationTools are the delegation tools: workers, reviews, scouts.
func OrchestrationTools() []tools.Tool {
	return []tools.Tool{
		spawnTool, waitTool, finishTool, reviewTool, scoutTool, tellTool, codeReviewTool,
	}
}

// AllTools is every tool wick serves, file tools first.
func AllTools() []tools.Tool {
	return append(tools.All(), OrchestrationTools()...)
}

// spawnExec is the exec spawn_worker drives. A var, like waitExec, so a test
// can spawn against a fake herdr and git rather than a real one.
var spawnExec Exec = osExec

var spawnTool = fnTool{
	name: "spawn_worker",
	desc: "Start an implementer on a ticket in its own git worktree, as a pane of the run's tab.",
	schema: schema(map[string]any{
		"ticket":  map[string]any{"type": "string", "description": "Path to the ticket file."},
		"repo":    map[string]any{"type": "string", "description": "Repo root. Defaults to the session cwd."},
		"base":    map[string]any{"type": "string", "description": "Base ref. Defaults to HEAD."},
		"harness": map[string]any{"type": "string", "enum": []any{"pig", "claude"}, "description": "Agent to run. Defaults to pig."},
		"model":   map[string]any{"type": "string", "description": "Model passed through to the harness."},
	}, "ticket"),
	run: func(root string, params map[string]any) (string, error) {
		in := SpawnInput{
			Ticket:  str(params, "ticket"),
			Repo:    str(params, "repo"),
			Base:    str(params, "base"),
			Harness: str(params, "harness"),
			Model:   str(params, "model"),
			// The workers of one run share the tab the caller is in, so every
			// spawn splits it and the run's workers tile together there.
			RunTab: os.Getenv("HERDR_TAB_ID"),
			Anchor: os.Getenv("HERDR_PANE_ID"),
		}
		if in.Ticket == "" {
			return "", fmt.Errorf("ticket is required")
		}
		if in.Harness == "" {
			in.Harness = "pig"
		}
		if _, err := os.Stat(in.Ticket); err != nil {
			return "", fmt.Errorf("cannot read ticket %s: %w", in.Ticket, err)
		}
		if in.Repo == "" {
			in.Repo = root
		}
		brief, err := writeBrief(WorkerID(in.Ticket), in.Ticket)
		if err != nil {
			return "", err
		}
		sess := Session{
			ID:   newSessionID(),
			Role: "work", Cwd: worktreePath(in.Repo, WorkerID(in.Ticket)), Model: in.Model, Label: WorkerID(in.Ticket),
			// The brief sits beside the worker store, outside the worktree;
			// read opens it by absolute path.
			Prompt: fmt.Sprintf("Read %s and follow it exactly.", brief),
		}
		launch, err := PrepareLaunch(in.Harness, sess)
		if err != nil {
			return "", err
		}
		in.Launch, in.Late = []string{launch.Line}, launch.Late

		in.SessionID = sess.ID
		spaceSpawns()
		worker, err := Spawn(spawnExec, in)
		if err != nil {
			// No worker means nothing ever reads the config it was given.
			CleanupSession(sess)
			return "", err
		}
		// Tiling is cosmetic: an uneven tab is still a working tab, so a
		// failure is reported and the spawn stands. A missing report extension
		// is a bigger fault and is warned about up front.
		warning := reportExtensionWarning(in.Harness)
		if err := tilePanes(spawnExec, worker.Tab); err != nil {
			warning += fmt.Sprintf("\nwarning: could not even out the panes in %s: %v", worker.Tab, err)
		}
		return fmt.Sprintf(
			"spawned worker %s\n  pane:      %s\n  workspace: %s\n  branch:    %s\n  worktree:  %s\n  brief:     %s\n\nNext: wait_workers(workers=[\"%s\"]), then finish_worker(worker=\"%s\").%s",
			worker.ID, worker.Pane, worker.Workspace, worker.Branch, worker.Worktree, brief,
			worker.ID, worker.ID, warning), nil
	},
}

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
	desc: "Take a finished worker: rebase and fast-forward it into the base, then drop its worktree.",
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

var reviewTool = fnTool{
	name: "review_worker",
	desc: "Have a read-only reviewer judge a settled worker's diff. Call before finish_worker. " +
		"A review can outlast one call: if it answers \"still running\", call review_worker again with the same worker, never start another.",
	schema: schema(map[string]any{
		"worker": map[string]any{"type": "string", "description": "Worker id."},
	}, "worker"),
	run: func(_ string, params map[string]any) (string, error) {
		id := str(params, "worker")
		if id == "" {
			return "", fmt.Errorf("worker is required")
		}
		verdict, err := Review(osExec, id)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("review of %s:\n%s", id, verdict), nil
	},
}

var scoutTool = fnTool{
	name: "scout",
	desc: "Ask a read-only scout a factual question about this repo. It reports facts with file:line; it does not decide.",
	schema: schema(map[string]any{
		"question": map[string]any{"type": "string", "description": "One specific question."},
	}, "question"),
	readOnly: true,
	run: func(root string, params map[string]any) (string, error) {
		q := str(params, "question")
		if q == "" {
			return "", fmt.Errorf("question is required")
		}
		return readOnlyOnce(osExec, keyOf("scout", q), root, "", scoutPrompt(q))
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
	desc: "Have a read-only reviewer judge this worktree's diff against the base it was branched from, " +
		"covering committed and uncommitted work alike. Call it once, after committing, and fix the real " +
		"findings before you finish. It returns the reviewer's reply.",
	schema: schema(map[string]any{}),
	run: func(root string, _ map[string]any) (string, error) {
		return SelfReview(reviewExec, root)
	},
}
