package el

import (
	"fmt"
	"log"
	"os"
	"path"
	"sort"
	"strings"
	"sync"

	"example.com/emberloop/tools"
)

// cwd is the working directory tools resolve against: the process's own,
// falling back to empty when it cannot be read.
func cwd() string {
	if dir, err := os.Getwd(); err == nil {
		return dir
	}
	return ""
}

// ResolvedTools is the final tool list for one role: the main list the MCP
// server serves, and the tools callable only inside execute. Both lists carry
// the settings renames. A disabled tool is in neither. This is the one
// function both the MCP server's tools/list (through RoleTools) and the
// execute stub generator call, so the two cannot disagree.
func ResolvedTools(role string) (main, execute []tools.Tool, err error) {
	return ResolvedToolsWithMCP(role, nil)
}

// ResolvedToolsWithMCP resolves a role with an explicit MCP tool listing,
// with the graph server synthesized in when it is enabled and installed.
func ResolvedToolsWithMCP(role string, mcp map[string][]tools.Tool) (main, execute []tools.Tool, err error) {
	base, err := baseToolsFor(role)
	if err != nil {
		return nil, nil, err
	}
	main, execute = resolveRoleTools(role, base, mcp, tools.WithGraphServer(tools.EffectiveSettings(cwd())), warnSettingsOnce)
	return main, execute, nil
}

// baseToolsFor is the role's tool set before settings narrow it: the role
// table's allowlist over every tool.
func baseToolsFor(role string) ([]tools.Tool, error) {
	names, ok := roles[role]
	if !ok {
		if role == "" {
			return nil, MissingRoleError()
		}
		return nil, fmt.Errorf("unknown role %q; choose one of: %s", role, strings.Join(RoleNames(), ", "))
	}
	all := tools.All()
	if names == nil {
		return all, nil
	}
	byName := map[string]tools.Tool{}
	for _, t := range all {
		byName[t.Name()] = t
	}
	out := make([]tools.Tool, 0, len(names))
	for _, n := range names {
		if t, ok := byName[n]; ok {
			out = append(out, t)
		}
	}
	out = withWebProxies(role, out)
	return out, nil
}

// WebSubmitEnabled offers the submit tool to web Daemon spawn children
// (web-worker, scout) and runs the Daemon submit gate for them. It is false:
// a spawned child's Group result is delivered when the child settles idle
// (or fails/ends), carrying its submit text when one exists, else its last
// assistant message. The Run worker/reviewer submit route is untouched by
// this flag. Re-enabling is one line: set it true (tests flip it per test,
// never globally).
var WebSubmitEnabled = false

// webProxyRoles is the Daemon-tool matrix for the single el MCP server: which
// Daemon-hosted tools each web role serves. A top-level parent manages its
// children (spawn, steer, check, cancel), reads and proposes its Profile
// (profile), proposes Rules to remember (remember) and searches memory
// (recall), and never needs submit; a spawned worker spawns its own (nesting
// kept) and searches memory but never steers, peeks at or stops, and never
// touches the Profile or proposes Rules; a scout only reads memory and
// settles. Submit stays listed for web-worker and scout so a
// one-line re-enable restores it, but withWebProxies filters it while
// WebSubmitEnabled is false. Children never hold the parent's full set:
// workers, scouts and Reviewers get recall only (ticket 18), never profile
// or remember.
var webProxyRoles = map[string][]string{
	"web":        {"spawn", "steer", "check", "cancel", "profile", "remember", "recall"},
	"web-worker": {"spawn", "submit", "recall"},
	"scout":      {"submit", "recall"},
	"run-worker": {"spawn", "submit", "recall"},
}

// withWebProxies serves the Daemon-hosted tools for one web role, but only
// when the process is bound to a Daemon Session: without EL_WEB_PARENT and
// EL_WEB_DAEMON_URL the role resolves to the core tools alone, so
// hand-started servers never see them. A proxy replaces the same-named core
// tool (check): the role cannot serve both the repo checks and the child
// peek under one name, and the Daemon tool wins.
func withWebProxies(role string, base []tools.Tool) []tools.Tool {
	names, ok := webProxyRoles[role]
	if !ok {
		return base
	}
	if _, _, ok := tools.WebDaemonBinding(); !ok {
		return base
	}
	if !WebSubmitEnabled && role != "run-worker" {
		kept := names[:0:0]
		for _, n := range names {
			if n != "submit" {
				kept = append(kept, n)
			}
		}
		names = kept
	}
	drop := map[string]bool{}
	for _, n := range names {
		drop[n] = true
	}
	out := make([]tools.Tool, 0, len(base)+len(names))
	for _, t := range base {
		if drop[t.Name()] {
			continue
		}
		out = append(out, t)
	}
	for _, n := range names {
		if p := tools.WebProxyTool(n); p != nil {
			out = append(out, p)
		}
	}
	return out
}

// resolveRoleTools applies settings to one role's base list plus its MCP
// listing. Settings only narrow: a settings entry for a known tool the role
// does not serve changes nothing, and an unknown name warns once and is
// ignored. submit stays known under its own name, because the Daemon serves
// it as a proxy. warn reports settings problems; nil skips the reports but
// still ignores the entries.
func resolveRoleTools(role string, base []tools.Tool, mcp map[string][]tools.Tool, s tools.Settings, warn func(string)) (main, execute []tools.Tool) {
	known := map[string]bool{"submit": true}
	for _, t := range tools.All() {
		known[t.Name()] = true
	}
	for _, listed := range mcp {
		for _, t := range listed {
			known[t.Name()] = true
		}
	}
	taken := map[string]bool{}
	claim := func(final string, t tools.Tool, placement string, where *[]tools.Tool) {
		if taken[final] {
			warnf(warn, "settings rename %q collides with a served tool; keeping the first", final)
			return
		}
		taken[final] = true
		*where = append(*where, t)
	}
	// Ticket 18: the served read refusal names what the caller can use, so
	// whether the role serves bash is decided once, before the buckets split.
	hasBash := roleServesBash(role, base, s)
	hasExecute := roleServesExecute(role, base, s)
	wantExecute := false
	for _, t := range base {
		// execute is bound after the buckets resolve, carrying this role's
		// allowed tools in its stub, so the served list and the stub agree.
		if t.Name() == "execute" {
			wantExecute = true
			continue
		}
		name := t.Name()
		enabled, placement, rename := true, tools.DefaultPlacement(name), ""
		if e, ok := s.Tools[name]; ok {
			enabled, placement, rename = effectiveToolSetting(name, e, role)
		}
		if name == "submit" {
			if !enabled || placement != tools.PlacementMain || rename != "" {
				warnf(warn, "submit cannot be disabled, moved or renamed; keeping submit in the main list")
			}
			enabled, placement, rename = true, tools.PlacementMain, ""
		}
		placement = validPlacement(placement, warn)
		// Daemon proxies always serve in the main list: they are the Session's
		// own management tools, and a settings placement written for the
		// same-named core tool (check) must not bury them behind execute.
		if tools.IsDaemonProxy(t) {
			placement = tools.PlacementMain
		}
		final := name
		if rename != "" {
			final = rename
			t = tools.Renamed(t, final)
		}
		// The refusal of an oversized file names bash for a role that serves it
		// and tools.grep in execute or read with offset and limit otherwise.
		if name == "read" {
			t = readHintTool{inner: t, hasBash: hasBash}
		}
		// A stored bash output tells a role that also serves execute to
		// filter in a script next time; bash behind execute needs no tip.
		if name == "bash" && hasExecute && placement == tools.PlacementMain {
			t = bashHintTool{inner: t}
		}
		if !enabled {
			continue
		}
		if placement == tools.PlacementExecute {
			claim(final, t, placement, &execute)
		} else {
			claim(final, t, placement, &main)
		}
	}
	for name := range s.Tools {
		if !known[name] {
			warnf(warn, "unknown tool %q in settings tools; ignoring", name)
		}
	}
	servers := make([]string, 0, len(mcp))
	for name := range mcp {
		servers = append(servers, name)
	}
	sort.Strings(servers)
	for _, server := range servers {
		cfg := s.MCPServers[server]
		for _, t := range mcp[server] {
			// Include, exclude and rename match over the tool's original
			// server-side name, so a listing the registry already shaped
			// resolves to the same set instead of filtering its renames out.
			original := mcpOriginalName(t)
			if !mcpIncluded(original, cfg) {
				continue
			}
			enabled, placement := true, tools.PlacementExecute
			if cfg.Placement != "" {
				placement = cfg.Placement
			}
			if ov, ok := cfg.Roles[role]; ok {
				if ov.Enabled != nil {
					enabled = *ov.Enabled
				}
				if ov.Placement != "" {
					placement = ov.Placement
				}
			}
			if !enabled {
				continue
			}
			if placement != tools.PlacementMain && placement != tools.PlacementExecute {
				warnf(warn, "mcp server %q has invalid placement %q; serving its tools behind execute", server, placement)
				placement = tools.PlacementExecute
			}
			final := t.Name()
			if renamed, ok := cfg.Rename[original]; ok && renamed != "" {
				final = renamed
				if final != t.Name() {
					t = tools.Renamed(t, final)
				}
			}
			if placement == tools.PlacementExecute {
				claim(final, t, placement, &execute)
			} else {
				claim(final, t, placement, &main)
			}
		}
	}
	if wantExecute {
		main = append(main, boundExecuteTool(role, s, main, execute, taken, warn)...)
	}
	return main, execute
}

// mcpOriginalName is the name a server listed a tool under, before any
// rename. A plain tool has no original distinct from its name.
func mcpOriginalName(t tools.Tool) string {
	if o, ok := t.(interface{ OriginalName() string }); ok {
		if original := o.OriginalName(); original != "" {
			return original
		}
	}
	return t.Name()
}

// effectiveToolSetting folds a tool entry's top level with its override for
// role: enabled defaults true, placement defaults to DefaultPlacement(name).
func effectiveToolSetting(name string, e tools.ToolSetting, role string) (enabled bool, placement, rename string) {
	enabled, placement, rename = true, tools.DefaultPlacement(name), e.Rename
	if e.Enabled != nil {
		enabled = *e.Enabled
	}
	if e.Placement != "" {
		placement = e.Placement
	}
	if ov, ok := e.Roles[role]; ok {
		if ov.Enabled != nil {
			enabled = *ov.Enabled
		}
		if ov.Placement != "" {
			placement = ov.Placement
		}
	}
	return enabled, placement, rename
}

// validPlacement keeps main and execute, warning and keeping main for anything
// else: a typo must not silently hide a tool the role relies on.
func validPlacement(placement string, warn func(string)) string {
	if placement != tools.PlacementMain && placement != tools.PlacementExecute {
		warnf(warn, "invalid tool placement %q; serving the tool in the main list", placement)
		return tools.PlacementMain
	}
	return placement
}

// mcpIncluded reports whether an MCP tool survives its server's include and
// exclude globs, matched over the tool's original name. An empty include
// means all; exclude wins.
func mcpIncluded(name string, cfg tools.MCPServerSetting) bool {
	if len(cfg.Include) > 0 && !matchAny(cfg.Include, name) {
		return false
	}
	return !matchAny(cfg.Exclude, name)
}

// matchAny reports whether any glob matches name. An invalid glob never
// matches: it is a settings typo, not a pattern.
func matchAny(patterns []string, name string) bool {
	for _, p := range patterns {
		if ok, err := path.Match(p, name); err == nil && ok {
			return true
		}
	}
	return false
}

func warnf(warn func(string), format string, args ...any) {
	if warn == nil {
		return
	}
	warn(fmt.Sprintf(format, args...))
}

// warnedSettings remembers every settings warning already reported, so an
// unknown tool name that every role trips over is logged once per process.
var warnedSettings = struct {
	sync.Mutex
	seen map[string]bool
}{seen: map[string]bool{}}

// warnSettingsOnce logs one settings warning per process per message.
func warnSettingsOnce(msg string) {
	warnedSettings.Lock()
	defer warnedSettings.Unlock()
	if warnedSettings.seen[msg] {
		return
	}
	warnedSettings.seen[msg] = true
	log.Printf("el: %s", msg)
}
