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

// skillsDir is the library wick-pig installs: the mattpocock skills, one
// directory per skill, under the root.
func skillsDir() string {
	return filepath.Join(skillsRoot(), "matt-pocock")
}

// SkillLibraryDir is where wick-pig installs and reads the skill library, for
// callers outside the package.
func SkillLibraryDir() string { return skillsDir() }

func skillPath(name string) string {
	return filepath.Join(skillsDir(), name, "SKILL.md")
}

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
3. wait_workers(timeout_ms=%d) once — do not poll in a loop.
4. review_worker(worker=<id>) for each settled worker, then finish_worker for
   it. A review never blocks the merge: keep every verdict and put it in your
   report. If finish_worker returns kept:true it conflicted: stop, report it,
   do not resolve it yourself.
5. Recompute what is ready and go again. Stop when nothing is ready.

Servers, watchers and log tails go to the background tool, never bash, for any command you run yourself.

Report the merged shas and each worker's review verdict when the run is done.`, target, target, WaitCap/time.Millisecond)
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
