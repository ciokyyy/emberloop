package el

import (
	"fmt"
	"strings"
)

// CompressPrompt is the pinned combined summarizer prompt: the Matt Pocock
// handoff structure with caveman ultra for prose. Code, paths, errors and
// IDs stay verbatim. Stored with el, never read from the skill library at
// runtime, so the summary shape cannot drift under a compress.
const CompressPrompt = `Summarize the session transcript below into a handoff for the agent that continues this work. Write prose the caveman ultra way: drop articles, filler, pleasantries and hedging. Fragments over full sentences. State each fact once. No preamble or recap. Code, paths, errors and IDs stay exact and verbatim, never paraphrased. No tool output pruned, no tail appended: the handoff is the whole context the next session gets.

Use exactly these sections, in this order:

## Task
What the session was asked to do. One fragment per goal.

## Context
Repo, branch, ticket, specs and decisions the work depends on.

## Progress
What changed so far: files touched, commands run, results seen.

## Next steps
Open tasks in order, each directly actionable.

## Reference
Verbatim code, paths, commands, error strings and session or agent IDs the next session needs. Quote exact.
`

// handoffSections are the sections CheckHandoff requires: the pinned
// prompt's own structure.
var handoffSections = []string{"## Task", "## Context", "## Progress", "## Next steps", "## Reference"}

// CheckHandoff sanity-checks a summary before any restart: non-empty, with
// every expected section. A failed summary leaves the old session untouched.
func CheckHandoff(handoff string) error {
	if strings.TrimSpace(handoff) == "" {
		return fmt.Errorf("empty summary: the old session stays untouched")
	}
	var missing []string
	for _, s := range handoffSections {
		if !strings.Contains(handoff, s) {
			missing = append(missing, s)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("summary lacks %s: the old session stays untouched", strings.Join(missing, ", "))
	}
	return nil
}

// HandoffSections returns the section headings a handoff carries, in the
// pinned order. The result is a copy: callers cannot change the list.
func HandoffSections() []string { return append([]string(nil), handoffSections...) }

// ContinueLine is the final handoff line after an interrupted step: the new
// session resumes without waiting for the user.
const ContinueLine = "Continue the interrupted step above where you left off."

// FinishHandoff appends the continue line when the compress interrupted a
// step (a tool call still open). An idle compress resumes nothing: a manual
// compress adds the line only if a task was open.
func FinishHandoff(handoff string, interrupted bool) string {
	if !interrupted {
		return handoff
	}
	return strings.TrimRight(handoff, "\n") + "\n\n" + ContinueLine + "\n"
}
