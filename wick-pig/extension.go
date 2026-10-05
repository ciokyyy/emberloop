package wick

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// workerSkills is the sequence a worker follows. The path is given, never the
// content, so an edit upstream takes effect on the next spawn with no code
// change here.
var workerSkills = []struct{ name, when string }{
	{"implement", "do the work"},
	{"tdd", "at the pre-agreed seams, while doing the work"},
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
	if s == "" {
		return `""`
	}
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
4. review_worker(worker=<id>) for each settled worker, then finish_worker for
   it. A review never blocks the merge: keep every verdict and put it in your
   report. If finish_worker returns kept:true it conflicted: stop, report it,
   do not resolve it yourself.
5. Recompute what is ready and go again. Stop when nothing is ready.

Report the merged shas and each worker's review verdict when the run is done.`, target, target)
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
