package wick

import (
	"fmt"
	"os"

	"example.com/wick/tools"
)

// roles name what a session may touch. The tool set is the enforcement: a
// reviewer has no edit tool, so no prompt has to ask it not to edit.
var roles = map[string][]string{
	// implement delegates: it reads, explores with scouts, and spawns; it
	// never edits, runs code or reviews. Each worker reviews its own work.
	"implement": {"read", "find", "spawn_worker", "wait_workers", "finish_worker", "tell_worker", "scout"},
	// review and scout can only look.
	"review": {"read", "find"},
	"scout":  {"read", "find"},
	// work is an implementer: every file tool, plus code_review, which runs
	// its two persistent reviewers. No delegation.
	"work": {"read", "edit", "write", "bash", "find", "background", "code_review"},
	// plan is a wayfinder, grill, research or to-spec session: file tools plus
	// scouts. No review: review belongs where code is written, in the workers.
	"plan": {"read", "edit", "write", "bash", "find", "background", "scout"},
	// full is everything.
	"full": nil,
}

// RoleTools returns the tools a role serves, in the role's order.
func RoleTools(role string) ([]tools.Tool, error) {
	names, ok := roles[role]
	if !ok {
		return nil, fmt.Errorf("unknown role %q", role)
	}
	all := AllTools()
	if names == nil {
		return all, nil
	}
	byName := map[string]tools.Tool{}
	for _, t := range all {
		byName[t.Name()] = t
	}
	out := make([]tools.Tool, 0, len(names))
	for _, n := range names {
		out = append(out, byName[n])
	}
	return out, nil
}

// RoleFromEnv is the role a launcher chose for this process. wick-pig always
// sets WICK_ROLE, so a bare session arrives here as plan. An unset role is a
// wick-mcp started by hand -- the live scripts and debugging do this -- and
// that keeps the full tool set.
func RoleFromEnv() string {
	if r := os.Getenv("WICK_ROLE"); r != "" {
		return r
	}
	return "full"
}
