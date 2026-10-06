package wick

import (
	"fmt"
	"strings"
)

// skillCommands gives each skill on the wayfinder route its own slash command.
// The command injects "read this skill file and follow it" as a turn, so the
// skill text is never copied here and an upstream edit lands on the next call.
var skillCommands = []struct {
	name, skill, desc string
	scout             bool
}{
	{"wayfinder", "wayfinder", "Map the work: chart the fog and the open questions.", true},
	{"grill", "grilling", "Grill a plan or decision until it holds. May research.", true},
	{"research", "research", "Research a question and write the notes down.", true},
	{"domain", "domain-modeling", "Model the domain language before building on it.", true},
	{"prototype", "prototype", "Build a throwaway prototype to answer a question.", false},
	{"to-spec", "to-spec", "Turn the conversation into a spec.", false},
	{"to-tickets", "to-tickets", "Break a spec into a ticket graph with Blocked-by edges.", false},
}

// executeDirection is the one positive execute line every worker-class prompt
// carries: search and process content with execute.
const executeDirection = "Search and process content with `execute`, combining related tool calls into one script."

func skillPrompt(skill, args string, scout bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Read %s and follow it exactly.", skillAddress(skill))
	b.WriteString("\n\n" + executeDirection)
	if args != "" {
		fmt.Fprintf(&b, "\n\nInput: %s", args)
	}
	if scout {
		b.WriteString("\n\nFor facts about this repo, spawn a scout rather than reading broadly yourself: spawn(agents=[{type: \"scout\", prompt: \"...\"}]). Scouts report facts only; decisions stay with you and the user.")
	}
	return b.String()
}
