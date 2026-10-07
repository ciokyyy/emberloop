package emberloop

import (
	"example.com/emberloop/tools"
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

// skillsRoot is where emberloop keeps the skill libraries it installs. The env var
// names the root itself, so EMBERLOOP_SKILLS_DIR overrides the whole tree, not one
// library inside it.
func skillsRoot() string {
	if dir := os.Getenv("EMBERLOOP_SKILLS_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(os.Getenv("HOME"), ".emberloop", "skills")
}

// skillsNamespace is the library's directory under the skills root, and the
// namespace its emberloop:// addresses carry. The two must stay one string: an
// address that names a namespace the read tool cannot find reads as nothing.
const skillsNamespace = "matt-pocock"

// skillsDir is the library emberloop installs: the mattpocock skills, one
// directory per skill, under the root.
func skillsDir() string {
	return filepath.Join(skillsRoot(), skillsNamespace)
}

// skillAddress is the emberloop:// address the read tool serves one skill under.
// Reading the namespace itself returns the index of every skill.
func skillAddress(name string) string {
	return "emberloop://" + skillsNamespace + "/" + name + "/SKILL.md"
}

// SkillLibraryDir is where emberloop installs and reads the skill library, for
// callers outside the package.
func SkillLibraryDir() string { return skillsDir() }

// reportExtension locates the herdr-report extension, which is what makes a
// worker visible to herdr as an agent rather than just a pane. The env var
// wins; otherwise the first candidate directory that exists wins. exe and home
// are parameters so a test can point them at temp dirs.
func reportExtension(exe, home string) string {
	if ext := os.Getenv("EMBERLOOP_HERDR_REPORT"); ext != "" {
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

// reportExtensionWarning warns that a pig worker will read unknown to herdr.
// Empty for any other harness, and empty when the extension is found.
func reportExtensionWarning(harness string) string {
	if harness != "pig" {
		return ""
	}
	if reportExtension(runningExe(), os.Getenv("HOME")) != "" {
		return ""
	}
	return "\nwarning: the herdr-report extension was not found, so this worker will read " +
		"unknown to herdr. Install herdr-report beside emberloop or point EMBERLOOP_HERDR_REPORT at its directory."
}

// recall returns one worker's record from the store, under the cross-process
// lock, so a concurrent spawn or finish cannot leave it reading a store
// mid-update.
func recall(id string) (Worker, error) {
	store, err := loadWorkersLocked()
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

// briefPath is where a worktree agent's brief lives, beside the worker store.
func briefPath(id string) string { return filepath.Join(filepath.Dir(statePath), "briefs", id+".md") }

// writeBrief puts the worker's instructions on disk, beside the worker store.
// It points at the ticket, its spec and the skills by path and emberloop:// address
// rather than restating them, so an upstream rewrite lands without a code
// change here. target is the branch the worker merges into; it rebases onto
// it before its last review, so conflicts are resolved while it still has
// its context, and the review covers what will merge. harness decides
// whether the brief opens with the root-doc line: the context probe
// (ticket 15) found pig loads the root AGENTS.md as a context file while
// the stripped claude harness loads neither context file, so only claude
// workers are told to read the root AGENTS.md first.
func writeBrief(id, ticket, spec, target, harness string) (string, error) {
	dir := filepath.Dir(briefPath(id))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := briefPath(id)

	var b strings.Builder
	if harness == "claude" {
		b.WriteString("First, read the root AGENTS.md and follow it before anything else.\n\n")
	}
	fmt.Fprintf(&b, "Implement the ticket at %s.\n", ticket)
	if spec != "" {
		fmt.Fprintf(&b, "It belongs to the spec at %s; read it for the decisions the ticket implements.\n", spec)
	}
	b.WriteString("\nRead and follow these skills, in this order:\n\n")
	for i, s := range workerSkills {
		fmt.Fprintf(&b, "%d. %s — %s\n", i+1, skillAddress(s.name), s.when)
	}
	b.WriteString("\n" + executeDirection + "\n")
	b.WriteString("\nWhere a skill says /code-review, spawn both of your reviewer types in one spawn call: " +
		"spawn(agents=[{type: \"reviewer-standards\"}, {type: \"reviewer-spec\"}]). They review this worktree and report pass or changes. " +
		"The spawn tool is in your tool list: in pig its name is mcp__emberloop__spawn. Call it directly; do not decide it is missing, and never start reviewers through bash or a second emberloop-mcp.\n")
	b.WriteString("\nAfter spawn, end your turn; the result arrives as a system notice. Never poll with sleep or bash while reviewers run.\n")
	b.WriteString("\nBefore editing, read the AGENTS.md chain from the repo root down to each file you will touch, following each doc's Child DOX Index; the nearest doc is the local contract. " +
		"After a change that alters purpose, ownership, contracts, structure, workflow, inputs, outputs, or permissions, update the nearest owning AGENTS.md (plus affected parents and children, refreshing indexes) in the same task.\n")
	b.WriteString("\nStay in this worktree. Commit to the current branch when the gate passes.\n")
	if target != "" {
		fmt.Fprintf(&b, "\nBefore your final review, bring the branch up to date: `git rebase %s`. "+
			"Resolve any conflict keeping the intent of both sides, spawn the reviewers again, and commit. "+
			"Other tickets merge into %s while you work, so do this last.\n", target, target)
	}
	b.WriteString("\nA review finding you cannot fix within this ticket (outside what it may touch, or against the spec) " +
		"is declined, with the reason, in the prompt of the next review spawn. Name every declined finding in your submit summary after \"Follow-ups:\", a clause each.\n")
	b.WriteString("\nServers, watchers and log tails go to the background tool, never bash: it blocks until timeout.\n")

	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// renderFinish returns prose rather than a Go value: a struct crossing the
// extension boundary reaches the model as nothing at all, and prose is also
// fewer tokens than the equivalent JSON.

func renderFinish(r *FinishResult) string {
	switch {
	case r.Merged:
		return fmt.Sprintf("merged %s into the base as %s; its worktree is gone.", r.ID, r.SHA)
	case r.Kept:
		return fmt.Sprintf("%s did NOT merge; its worktree and pane are still there: %s\n"+
			"Send it back with tell(agent=%s, text=...) to fix this (rebase, resolve, gate, review); its next submit arrives as a notice, then finish it again.", r.ID, r.Detail, r.ID)
	default:
		return fmt.Sprintf("discarded %s and removed its worktree.", r.ID)
	}
}

// implementPrompt is what `emberloop implement <spec>` injects. The skill does
// the thinking; the adapter only maps its vocabulary onto emberloop's tools and
// drops what emberloop already does (review per ticket, the integration branch).
func implementPrompt(run SpecRun) string {
	maxWorkers := tools.LoadSettings().EffectiveMaxWorkers()
	return fmt.Sprintf(`Implement the spec at %s. Its tickets are under %s.

Read %s and follow it, with these emberloop adaptations:

- The integration branch is %s. It already exists and is checked out; never switch away from it.
- An implementer subagent is spawn(agents=[{type: "worker", ticket: <path>}]). The worker gets its own worktree from the integration tip, the skills, and the spec; tell it nothing else. Spawn returns at once with the agents still running, for every type.
- At most %d workers run at once; further workers from one spawn queue and start as earlier ones finish.
- An exploration subagent is spawn(agents=[{type: "scout", prompt: "..."}]). Ask several at once when they are independent; their results arrive together.
- After spawn, end your turn; the result arrives as a system notice. The results of one spawn call arrive as a single notice once every agent of that call has submitted. Waiting for it only spends tokens.
- The merger subagent is finish(agent=<worker id>): it rebases the worker onto the integration tip and fast-forwards. There is no merger agent.
- Skip the skill's final code-review step: every worker reviewed its own ticket before submitting. Never review a worker yourself.
- If finish does not merge a worker, tell(agent=<id>, text=<the reason finish gave>) it what failed, and its next submit arrives as a notice. Once it submits, finish it again. If the same worker fails again, leave it, report it, and carry on with every ticket it does not block.
- You never run code. You may edit and write only the paths listed under "orchestrator_paths" in .emberloop/settings.json; any other path is refused. Use that only to keep the spec and notes current: finish marks a merged worker's ticket done itself. Never touch source files, and leave the edits uncommitted.

When nothing more is ready, report the integration branch, each merged ticket with its sha, every worker left unmerged and why, and the follow-ups workers listed.`,
		run.Spec, run.Tickets, skillAddress("implement-spec"), run.Branch, maxWorkers)
}

func str(params map[string]any, key string) string {
	v, _ := params[key].(string)
	return v
}
