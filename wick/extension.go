package wick

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
)

// Extension returns the wick extension.
//
// Three tools, deliberately. Every tool in the loadout costs schema tokens on
// every turn of every session, so "check status" and "wait for status" are one
// call, and "merge" and "clean up" are one call.
//
// The merger and the worker are deliberately different. The merger holds no
// implementation skills at all: it delegates, so carrying them would be dead
// tokens and a standing invitation to do the work itself. The worker gets them
// as file references on its own command line, resolved from disk at launch, so
// nothing here can drift from the upstream skill.
func Extension() *sdk.Extension {
	e := sdk.New("wick")
	registerHashline(e)

	e.Tool("spawn_worker",
		"Start an implementer on a ticket in its own git worktree and pane.",
		schema(map[string]any{
			"ticket":  map[string]any{"type": "string", "description": "Path to the ticket file."},
			"repo":    map[string]any{"type": "string", "description": "Repo root. Defaults to the session cwd."},
			"base":    map[string]any{"type": "string", "description": "Base ref. Defaults to HEAD."},
			"harness": map[string]any{"type": "string", "enum": []any{"pig", "claude"}, "description": "Agent to run. Defaults to pig."},
			"model":   map[string]any{"type": "string", "description": "Model passed through to the harness."},
		}, "ticket"),
		func(ctx sdk.Context, params map[string]any) (any, error) {
			in := SpawnInput{
				Ticket:  str(params, "ticket"),
				Repo:    str(params, "repo"),
				Base:    str(params, "base"),
				Harness: str(params, "harness"),
				Model:   str(params, "model"),
			}
			if in.Ticket == "" {
				return nil, fmt.Errorf("ticket is required")
			}
			if in.Harness == "" {
				in.Harness = "pig"
			}
			if _, err := os.Stat(in.Ticket); err != nil {
				return nil, fmt.Errorf("cannot read ticket %s: %w", in.Ticket, err)
			}
			if in.Repo == "" {
				in.Repo = ctx.Cwd()
			}
			brief, err := writeBrief(WorkerID(in.Ticket), in.Ticket)
			if err != nil {
				return nil, err
			}
			in.Launch = launchCommand(in.Harness, in.Model, brief, WorkerID(in.Ticket))

			worker, err := Spawn(sdkExec(ctx), in)
			if err != nil {
				return nil, err
			}
			// A Go pointer renders as nothing to the model. Hand back prose it
			// can act on, including the id finish_worker needs.
			return fmt.Sprintf(
				"spawned worker %s\n  pane:      %s\n  workspace: %s\n  branch:    %s\n  worktree:  %s\n  brief:     %s\n\nNext: wait_workers(workers=[\"%s\"]), then finish_worker(worker=\"%s\").",
				worker.ID, worker.Pane, worker.Workspace, worker.Branch, worker.Worktree, brief,
				worker.ID, worker.ID), nil
		})

	e.Tool("wait_workers",
		"Worker status. Omit workers for all; timeout_ms 0 returns immediately.",
		schema(map[string]any{
			"workers":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Worker ids. Omit for all."},
			"timeout_ms": map[string]any{"type": "integer", "description": "Block up to this long. 0 returns now."},
		}),
		func(ctx sdk.Context, params map[string]any) (any, error) {
			timeout := time.Duration(num(params, "timeout_ms")) * time.Millisecond
			workers, pending, err := Wait(sdkExec(ctx), strs(params, "workers"), timeout)
			if err != nil {
				return nil, err
			}
			return renderStatus(workers, pending), nil
		})

	e.Tool("finish_worker",
		"Take a finished worker: rebase and fast-forward it into the base, then drop its worktree.",
		schema(map[string]any{
			"worker": map[string]any{"type": "string", "description": "Worker id."},
			"merge":  map[string]any{"type": "boolean", "description": "False discards the branch. Defaults to true."},
		}, "worker"),
		func(ctx sdk.Context, params map[string]any) (any, error) {
			id := str(params, "worker")
			if id == "" {
				return nil, fmt.Errorf("worker is required")
			}
			merge := true
			if v, ok := params["merge"]; ok {
				if b, isBool := v.(bool); isBool {
					merge = b
				}
			}
			result, err := Finish(sdkExec(ctx), id, merge)
			if err != nil {
				return nil, err
			}
			return renderFinish(result), nil
		})

	// Message a running worker through herdr rather than typing into its pane:
	// the answer lands in the agent's own input, so a steer reaches the model
	// instead of the shell.
	e.Tool("tell_worker",
		"Send a message to a running worker. Use for a steer, or to answer a question it raised.",
		schema(map[string]any{
			"worker": map[string]any{"type": "string", "description": "Worker id."},
			"text":   map[string]any{"type": "string", "description": "What to tell it."},
		}, "worker", "text"),
		func(ctx sdk.Context, params map[string]any) (any, error) {
			id := str(params, "worker")
			text := str(params, "text")
			if id == "" || text == "" {
				return nil, fmt.Errorf("worker and text are required")
			}
			worker, err := recall(id)
			if err != nil {
				return nil, err
			}
			// No --until: herdr's default already matches idle, done or blocked,
			// and --until takes one state per flag, so the comma-list this used
			// to pass was rejected outright as "invalid agent status". A done
			// worker is still promptable; only a blocked one is refused.
			res, err := sdkExec(ctx)(herdrBin(),
				[]string{"agent", "prompt", worker.Pane, text, "--wait", "--timeout", "600000"},
				ExecOpts{})
			if err != nil {
				return nil, err
			}
			if res.Code != 0 {
				return nil, fmt.Errorf("herdr could not deliver to %s: %s", id, firstLine(res.Stderr, res.Stdout))
			}
			return fmt.Sprintf("delivered to %s and waited for it to settle", id), nil
		})

	// `/implement <path>` is the button. It implements nothing itself: it
	// injects the loop as a turn so the model runs it with the session's
	// context and the three wick tools already in its loadout. Bare
	// `/implement` stays a status read-out.
	e.Command("implement", "Start the delegation loop for a ticket path, or list workers.", func(ctx sdk.Context, args string) error {
		target := strings.TrimSpace(args)
		if target != "" {
			if _, err := os.Stat(target); err != nil {
				return fmt.Errorf("cannot read %s: %w", target, err)
			}
			return ctx.SendMessage("wick", implementPrompt(target), true, sdk.SendMessageOptions{
				TriggerTurn: sdk.Bool(true),
			})
		}

		store, err := load()
		if err != nil {
			return err
		}
		if len(store) == 0 {
			ctx.Notify("wick: no workers. /implement <ticket-path> to start one.", "info")
			return nil
		}
		workers, pending, err := Wait(sdkExec(ctx), nil, 0)
		if err != nil {
			return err
		}
		lines := []string{"wick workers:"}
		for _, w := range workers {
			lines = append(lines, fmt.Sprintf("  %-24s %-8s %s", w.ID, w.State, w.Detail))
		}
		if len(pending) > 0 {
			lines = append(lines, "pending: "+strings.Join(pending, ", "))
		}
		ctx.Notify(strings.Join(lines, "\n"), "info")
		return nil
	})

	e.OnSessionStart(func(ctx sdk.Context, _ map[string]any) (any, error) {
		ctx.SetStatus("wick", "ready")
		return nil, nil
	})

	return e
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// workerSkills is the sequence a worker follows. The path is given, never the
// content, so an edit upstream takes effect on the next spawn with no code
// change here.
var workerSkills = []struct{ name, when string }{
	{"implement", "do the work"},
	{"tdd", "at the pre-agreed seams, while doing the work"},
	{"code-review", "only after the work is done, before committing"},
}

func skillsDir() string {
	if dir := os.Getenv("WICK_SKILLS_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(os.Getenv("HOME"), ".agents", "skills")
}

func skillPath(name string) string {
	return filepath.Join(skillsDir(), name, "SKILL.md")
}

// launchCommand builds the argv pane run hands to a worker.
//
// pig gets --no-skills so the worker never loads the 63 skills on this machine,
// and explicit --skill flags so it knows about exactly the route skills. Those
// flags are PiG's, so they are not passed to other harnesses; the prompt below
// names the files either way, which works everywhere.
func launchCommand(harness, model, briefPath, agentLabel string) []string {
	// WICK_AGENT_LABEL is read by the herdr-report extension, which tells
	// herdr what this pane is. Without it every worker reports as "pig" and
	// they stop being distinguishable in `herdr agent list`.
	argv := []string{"WICK_AGENT_LABEL=" + agentLabel}

	// The harness is argv[0] as far as the shell is concerned: pane run
	// executes the first word, so the env assignment must come first and the
	// program must still be a plain executable name.
	argv = append(argv, harness)
	if harness == "pig" {
		// --no-skills keeps the worker off the 63 skills on this machine; the
		// explicit ones are the route. Both flags are PiG's, so they are not
		// passed to other harnesses.
		argv = append(argv, "--no-skills")
		// A named session is what makes the worker addressable. Without it
		// ctx.GetSessionFile() is nil, herdr never sees an agent_session, and
		// `herdr agent prompt` refuses the pane with agent_not_ready — which
		// leaves tell_worker with nothing to talk to.
		argv = append(argv, "--session-id", shellArg(agentLabel))
		for _, s := range workerSkills {
			argv = append(argv, "--skill", shellArg(filepath.Dir(skillPath(s.name))))
		}
		if ext := reportExtension(); ext != "" {
			argv = append(argv, "-e", shellArg(ext))
		}
	}
	if model != "" {
		argv = append(argv, "--model", shellArg(model))
	}
	// The brief is a file, not an argument. pane run types its argv into the
	// pane's shell as a single line, so a multi-line prompt arrives as one
	// run-on sentence.
	return append(argv, shellArg(fmt.Sprintf("Read %s and follow it exactly.", briefPath)))
}

// reportExtension locates the herdr-report extension, which is what makes a
// worker visible to herdr as an agent rather than just a pane.
func reportExtension() string { return os.Getenv("WICK_HERDR_REPORT") }

// recall returns one worker's record from the store.
func recall(id string) (Worker, error) {
	store, err := load()
	if err != nil {
		return Worker{}, err
	}
	worker, ok := store[id]
	if !ok {
		return Worker{}, fmt.Errorf("unknown worker %q", id)
	}
	return worker, nil
}

// shellArg quotes one argument for the pane's shell. pane run joins its
// arguments with spaces and types them raw, so anything containing a space or
// a shell metacharacter has to carry its own quotes.
func shellArg(s string) string {
	if !strings.ContainsAny(s, " \t\n\"'\\|&;<>()$*`?[]#~=%") {
		return s
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

// writeBrief puts the worker's instructions on disk, beside the worker store.
// It points at the skills rather than restating them, so an upstream rewrite
// lands without a code change here.
func writeBrief(id, ticket string) (string, error) {
	dir := filepath.Join(filepath.Dir(statePath), "briefs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, id+".md")

	var b strings.Builder
	fmt.Fprintf(&b, "Implement the ticket at %s.\n\n", ticket)
	b.WriteString("Read and follow these skills, in this order:\n\n")
	for i, s := range workerSkills {
		fmt.Fprintf(&b, "%d. %s — %s\n", i+1, skillPath(s.name), s.when)
	}
	b.WriteString("\nStay in this worktree. Commit to the current branch when the gate passes.\n")

	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// renderStatus and renderFinish return prose rather than a Go value. A struct
// or map crossing the extension boundary reaches the model as nothing at all,
// and prose is also fewer tokens than the equivalent JSON.
func renderStatus(workers []Status, pending []string) string {
	if len(workers) == 0 {
		return "no workers are running. Spawn one with spawn_worker(ticket=<path>)."
	}
	var b strings.Builder
	for _, w := range workers {
		fmt.Fprintf(&b, "%-24s %-8s %s\n", w.ID, w.State, w.Detail)
	}
	if len(pending) == 0 {
		b.WriteString("all settled. Next: finish_worker for each.")
	} else {
		fmt.Fprintf(&b, "still running: %s. Call wait_workers again, or do other work.", strings.Join(pending, ", "))
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderFinish(r *FinishResult) string {
	switch {
	case r.Merged:
		return fmt.Sprintf("merged %s into the base as %s; its worktree is gone.", r.ID, r.SHA)
	case r.Kept:
		return fmt.Sprintf("%s did NOT merge and its worktree is still on disk: %s\n"+
			"Report this rather than resolving it — a human decides.", r.ID, r.Detail)
	default:
		return fmt.Sprintf("discarded %s and removed its worktree.", r.ID)
	}
}

// implementPrompt is what `/implement <path>` injects into the merger.
func implementPrompt(target string) string {
	return fmt.Sprintf(`Run the wick implement loop over %s.

1. Read the tickets under %s and order them by their "Blocked by:" line.
   Never start a ticket whose blockers have not merged.
2. spawn_worker(ticket=<path>) for every ready ticket. Pass harness or model
   only to override the default. Each worker is given its own worktree and the
   skills it needs; you need none of them yourself.
3. wait_workers(timeout_ms=600000) once — do not poll in a loop.
4. finish_worker(worker=<id>) for each settled worker. If one returns
   kept:true it conflicted: stop, report it, do not resolve it yourself.
5. Recompute what is ready and go again. Stop when nothing is ready.

Report the merged shas when the run is done.`, target, target)
}

func schema(properties map[string]any, required ...string) sdk.Schema {
	s := sdk.Schema{"type": "object", "properties": properties}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func str(params map[string]any, key string) string {
	v, _ := params[key].(string)
	return v
}

func strs(params map[string]any, key string) []string {
	raw, _ := params[key].([]any)
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func num(params map[string]any, key string) int {
	switch v := params[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}
