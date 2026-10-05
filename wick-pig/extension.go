package wick

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
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

// skillsRoot is where wick keeps the skill libraries it installs. The env var
// names the root itself, so WICK_SKILLS_DIR overrides the whole tree, not one
// library inside it.
func skillsRoot() string {
	if dir := os.Getenv("WICK_SKILLS_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(os.Getenv("HOME"), ".wick-pig", "skills")
}

// skillsNamespace is the library's directory under the skills root, and the
// namespace its wick:// addresses carry. The two must stay one string: an
// address that names a namespace the read tool cannot find reads as nothing.
const skillsNamespace = "matt-pocock"

// skillsDir is the library wick-pig installs: the mattpocock skills, one
// directory per skill, under the root.
func skillsDir() string {
	return filepath.Join(skillsRoot(), skillsNamespace)
}

// skillAddress is the wick:// address the read tool serves one skill under.
// Reading the namespace itself returns the index of every skill.
func skillAddress(name string) string {
	return "wick://" + skillsNamespace + "/" + name + "/SKILL.md"
}

// SkillLibraryDir is where wick-pig installs and reads the skill library, for
// callers outside the package.
func SkillLibraryDir() string { return skillsDir() }

// reportExtension locates the herdr-report extension, which is what makes a
// worker visible to herdr as an agent rather than just a pane. The env var
// wins; otherwise the first candidate directory that exists wins. exe and home
// are parameters so a test can point them at temp dirs.
func reportExtension(exe, home string) string {
	if ext := os.Getenv("WICK_HERDR_REPORT"); ext != "" {
		return ext
	}
	for _, candidate := range reportCandidates(exe, home) {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return filepath.Clean(candidate)
		}
	}
	return ""
}

// reportCandidates is every place reportExtension looks, in order. The env var
// is checked before these and is deliberately not one of them, so a caller can
// name the directories that were actually searched.
func reportCandidates(exe, home string) []string {
	dir := filepath.Dir(exe)
	return []string{
		filepath.Join(dir, "..", "herdr-report"),
		filepath.Join(dir, "herdr-report"),
		filepath.Join(home, ".pig", "agent", "extensions", "herdr-report"),
	}
}

// reportExtensionWarning warns that a pig worker will read unknown to herdr
// and never settle, which leaves wait_workers blocking on a state that cannot
// arrive. Empty for any other harness, and empty when the extension is found.
func reportExtensionWarning(harness string) string {
	if harness != "pig" {
		return ""
	}
	if reportExtension(runningExe(), os.Getenv("HOME")) != "" {
		return ""
	}
	return "\nwarning: the herdr-report extension was not found, so this worker will read " +
		"unknown to herdr and never settle on its own. Judge it by its branch commits and " +
		"an idle pane; install herdr-report beside wick or point WICK_HERDR_REPORT at its directory."
}

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

// runningExe is the path of this executable. An error means no lookup is
// possible, and an empty exe only costs us the two candidates beside it.
func runningExe() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return exe
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
// It points at the ticket, its spec and the skills by path and wick:// address
// rather than restating them, so an upstream rewrite lands without a code
// change here, and one brief serves every harness. target is the branch the
// worker merges into; it rebases onto it before its last review, so conflicts
// are resolved while it still has its context, and the review covers what
// will merge.
func writeBrief(id, ticket, spec, target string) (string, error) {
	dir := filepath.Join(filepath.Dir(statePath), "briefs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, id+".md")

	var b strings.Builder
	fmt.Fprintf(&b, "Implement the ticket at %s.\n", ticket)
	if spec != "" {
		fmt.Fprintf(&b, "It belongs to the spec at %s; read it for the decisions the ticket implements.\n", spec)
	}
	b.WriteString("\nRead and follow these skills, in this order:\n\n")
	for i, s := range workerSkills {
		fmt.Fprintf(&b, "%d. %s — %s\n", i+1, skillAddress(s.name), s.when)
	}
	b.WriteString("\nWhere a skill says /code-review, call the code_review tool instead.\n")
	b.WriteString("\nStay in this worktree. Commit to the current branch when the gate passes.\n")
	if target != "" {
		fmt.Fprintf(&b, "\nBefore your final code_review, bring the branch up to date: `git rebase %s`. "+
			"Resolve any conflict keeping the intent of both sides, run the gate again, and commit. "+
			"Other tickets merge into %s while you work, so do this last.\n", target, target)
	}
	b.WriteString("\nA review finding you cannot fix within this ticket (outside what it may touch, or against the spec) " +
		"is declined, with the reason, in code_review's note. List every declined finding under \"Follow-ups\" in your final message.\n")
	b.WriteString("\nServers, watchers and log tails go to the background tool, never bash: it blocks until timeout.\n")

	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// renderStatus and renderFinish return prose rather than a Go value. A struct
// or map crossing the extension boundary reaches the model as nothing at all,
// and prose is also fewer tokens than the equivalent JSON.
func renderStatus(workers []Status, pending []string, candidates []string) string {
	if len(workers) == 0 {
		return "no workers are running. Spawn one with spawn(agents=[{type: \"worker\", ticket: <path>}])."
	}
	var b strings.Builder
	for _, w := range workers {
		fmt.Fprintf(&b, "%-24s %-8s %s\n", w.ID, w.State, w.Detail)
	}
	if len(pending) == 0 {
		b.WriteString("all settled. Next: finish_worker for each.")
	} else {
		fmt.Fprintf(&b, "still running: %s. Call wait_workers again, or do other work.", strings.Join(pending, ", "))
		if allUnknown(workers, pending) {
			b.WriteString("\n" + missingReportExtension(candidates))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// missingReportExtension explains the all-unknown case and names every place
// reportExtension looked, so the reader can fix the lookup instead of guessing
// at it.
func missingReportExtension(candidates []string) string {
	return "They are not registered with herdr, which is why they never settle: " +
		"the herdr-report extension was not found, so pig was launched without `-e herdr-report`. " +
		"Looked in: " + strings.Join(candidates, ", ") + ". " +
		"Install it in one of those, or set WICK_HERDR_REPORT to its directory."
}

// allUnknown reports whether every pending worker is unknown to herdr. One
// worker herdr can see proves the extension is loaded, so the rest are just
// working, and saying otherwise would send the reader chasing the wrong fault.
func allUnknown(workers []Status, pending []string) bool {
	if len(pending) == 0 {
		return false
	}
	state := make(map[string]string, len(workers))
	for _, w := range workers {
		state[w.ID] = w.State
	}
	for _, id := range pending {
		if state[id] != "unknown" {
			return false
		}
	}
	return true
}

func renderFinish(r *FinishResult) string {
	switch {
	case r.Merged:
		return fmt.Sprintf("merged %s into the base as %s; its worktree is gone.", r.ID, r.SHA)
	case r.Kept:
		return fmt.Sprintf("%s did NOT merge; its worktree and pane are still there: %s\n"+
			"Send it back with tell_worker to fix this (rebase, resolve, gate, code_review), wait for it, then finish it again.", r.ID, r.Detail)
	default:
		return fmt.Sprintf("discarded %s and removed its worktree.", r.ID)
	}
}

// implementPrompt is what `wick-pig implement <spec>` injects. The skill does
// the thinking; the adapter only maps its vocabulary onto wick's tools and
// drops what wick already does (review per ticket, the integration branch).
func implementPrompt(run SpecRun) string {
	return fmt.Sprintf(`Implement the spec at %s. Its tickets are under %s.

Read %s and follow it, with these wick adaptations:

- The integration branch is %s. It already exists and is checked out; never switch away from it.
- An implementer subagent is spawn(agents=[{type: "worker", ticket: <path>}]). The worker gets its own worktree from the integration tip, the skills, and the spec; tell it nothing else.
- An exploration subagent is spawn(agents=[{type: "scout", prompt: "..."}]). Ask several at once when they are independent.
- The merger subagent is finish_worker(worker=<id>): it rebases the worker onto the integration tip and fast-forwards. There is no merger agent.
- Skip the skill's final code-review step: every worker reviewed its own ticket before settling. Never review a worker yourself.
- wait_workers(timeout_ms=%d) to see workers settle; one call, not a polling loop.
- If finish_worker does not merge a worker, tell_worker it what failed (the reason finish_worker gave), wait for it, and finish it again. If the same worker fails again, leave it, report it, and carry on with every ticket it does not block.
- You never edit files and never run code.

When nothing more is ready, report the integration branch, each merged ticket with its sha, every worker left unmerged and why, and the follow-ups workers listed.`,
		run.Spec, run.Tickets, skillAddress("implement-spec"), run.Branch, WaitCap/time.Millisecond)
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
