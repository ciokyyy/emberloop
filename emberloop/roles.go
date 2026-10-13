package el

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"example.com/emberloop/tools"
)

// roles name what a session may touch. The tool set is the enforcement: a
// scout has no edit tool, so no prompt has to ask it not to edit. These are
// the roles the web Daemon serves through the single el MCP server.
var roles = map[string][]string{
	// web is the browser Daemon's top-level session: the plan file tools with
	// no spawn, finish or tell. The Daemon hosts its own spawn, so serving
	// el's would start panes instead of Sessions. Bound to a Daemon
	// Session, it also serves the Daemon's spawn, steer, check and cancel.
	"web": {"read", "edit", "write", "bash", "find", "websearch", "background", "ctx_search", "check", "grep", "git", "execute"},
	// web-worker is a spawned Daemon Subagent: the web file tools, plus submit
	// to settle its turn and, nesting kept, spawn for its own children. It
	// never manages them the way a parent does: no steer, check or cancel.
	"web-worker": {"read", "edit", "write", "bash", "find", "websearch", "background", "ctx_search", "check", "grep", "git", "execute"},
	// run-worker is a Run ticket Worker or Reviewer: the web-worker tools. It
	// always serves submit, since the Run submit route and the Run gate need
	// it whatever WebSubmitEnabled says.
	"run-worker": {"read", "edit", "write", "bash", "find", "websearch", "background", "ctx_search", "check", "grep", "git", "execute"},
	// scout reads and searches only: facts with citations. grep and git live
	// behind execute; execute itself is in main, sandboxed for this role.
	"scout": {"read", "find", "websearch", "ctx_search", "grep", "git", "execute"},
	// full is everything.
	"full": nil,
}

// RoleNames lists every valid role, sorted, for messages and usage.
func RoleNames() []string {
	names := make([]string, 0, len(roles))
	for n := range roles {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// MissingRoleError is the refusal a el MCP server starts with when no role
// was chosen: it names every valid role, so a hand-started server can never
// silently serve the full tool set. Ask for the full tool set explicitly,
// with --role full or EL_ROLE=full.
func MissingRoleError() error {
	return fmt.Errorf("no role set; set EL_ROLE or --role to one of: %s", strings.Join(RoleNames(), ", "))
}

// RoleTools returns the tools a role serves in its main list, in the role's
// order, after settings narrow, rename and place them. It is the main list of
// ResolvedTools: the MCP server serves exactly this. An empty or unknown
// role is an error naming every valid role.
func RoleTools(role string) ([]tools.Tool, error) {
	main, _, err := ResolvedTools(role)
	return main, err
}

// RoleFromEnv is the role the Daemon chose for this process. The Daemon
// always sets EL_ROLE, so every session carries one. An empty value means a
// el-mcp started by hand with no role: the caller refuses to start instead
// of falling back to the full tool set. The full tool set stays available
// when asked for explicitly, with --role full or EL_ROLE=full, which is how
// live debugging reaches it.
func RoleFromEnv() string {
	return os.Getenv("EL_ROLE")
}
