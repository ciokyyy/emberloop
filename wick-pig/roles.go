package wick

import (
	"fmt"
	"os"

	"example.com/wick/tools"
)

// roles name what a session may touch. The tool set is the enforcement: a
// reviewer has no edit tool, so no prompt has to ask it not to edit.
var roles = map[string][]string{
	// implement delegates: it reads and spawns, it never edits or runs code.
	"implement": {"read", "find", "spawn_worker", "wait_workers", "finish_worker", "review_worker", "tell_worker"},
	// review and scout can only look.
	"review": {"read", "find"},
	"scout":  {"read", "find"},
	// work is an implementer: every file tool, no delegation.
	"work": {"read", "edit", "write", "bash", "find", "background"},
	// plan is a wayfinder, grill or research session: file tools plus scouts.
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

// RoleFromEnv is the role a launcher chose for this process.
func RoleFromEnv() string {
	if r := os.Getenv("WICK_ROLE"); r != "" {
		return r
	}
	return "full"
}
