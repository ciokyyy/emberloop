package wick

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"example.com/wick/tools"
)

// roles name what a session may touch. The tool set is the enforcement: a
// reviewer has no edit tool, so no prompt has to ask it not to edit.
var roles = map[string][]string{
	// implement delegates: it reads, explores with scouts, and spawns; it
	// never runs code or reviews. Each worker reviews its own work. It can
	// edit and write only the paths its project's settings list (see
	// tools.Guard), enough to keep tickets and specs current.
	"implement": {"read", "edit", "write", "find", "websearch", "ctx_search", "spawn", "finish", "tell"},
	// review and scout can only look, plus search the web and what is
	// stored. review also re-runs the repo's checks; check defaults behind
	// execute, so it is reachable there, not in main.
	"review": {"read", "find", "websearch", "ctx_search", "check"},
	"scout":  {"read", "find", "websearch", "ctx_search"},
	// work is an implementer: every file tool. Its review is a spawn of the
	// reviewer types its agent type allows, not a tool of its own.
	"work": {"read", "edit", "write", "bash", "find", "websearch", "background", "ctx_search", "check"},
	// plan is a wayfinder, grill, research or to-spec session: file tools plus
	// spawn, so it can start scouts. No review: review belongs where code is
	// written, in the workers.
	"plan": {"read", "edit", "write", "bash", "find", "websearch", "background", "ctx_search", "spawn", "check"},
	// full is everything.
	"full": nil,
}

// RoleNames lists every valid role, sorted, for messages and usage.
func RoleNames() []string {
	names := make([]string, 0, len(roles)+1)
	for n := range roles {
		names = append(names, n)
	}
	names = append(names, "agent")
	sort.Strings(names)
	return names
}

// MissingRoleError is the refusal a wick MCP server starts with when no role
// was chosen: it names every valid role, so a hand-started server can never
// silently serve spawn, finish or tell. Ask for the full tool set explicitly,
// with --role full or WICK_ROLE=full.
func MissingRoleError() error {
	return fmt.Errorf("no role set; set WICK_ROLE or --role to one of: %s", strings.Join(RoleNames(), ", "))
}

// RoleTools returns the tools a role serves in its main list, in the role's
// order, after settings narrow, rename and place them. It is the main list of
// ResolvedTools: the MCP server serves exactly this. The role "agent" is a
// spawned agent: its tool set comes from the agent type in the environment,
// not from the role table. An empty or unknown role is an error naming every
// valid role.
func RoleTools(role string) ([]tools.Tool, error) {
	main, _, err := ResolvedTools(role)
	return main, err
}

// RoleFromEnv is the role a launcher chose for this process. wick-pig and
// the pig and claude harnesses always set WICK_ROLE, so every launch carries
// one. An empty value means a wick-mcp started by hand with no role: the
// caller refuses to start instead of falling back to the full tool set. The
// full tool set stays available when asked for explicitly, with --role full
// or WICK_ROLE=full, which is how live debugging reaches it.
func RoleFromEnv() string {
	return os.Getenv("WICK_ROLE")
}
