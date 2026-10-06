package wick

import (
	"fmt"
	"log"
	"path"
	"sort"
	"strings"
	"sync"

	"example.com/wick/tools"
)

// ResolvedTools is the final tool list for one role: the main list the MCP
// server serves, and the tools callable only inside execute. Both lists carry
// the settings renames. A disabled tool is in neither. This is the one
// function both the MCP server's tools/list (through RoleTools) and the
// execute stub generator call, so the two cannot disagree.
func ResolvedTools(role string) (main, execute []tools.Tool, err error) {
	return ResolvedToolsWithMCP(role, nil)
}

// ResolvedToolsWithMCP resolves a role with an explicit MCP tool listing,
func ResolvedToolsWithMCP(role string, mcp map[string][]tools.Tool) (main, execute []tools.Tool, err error) {
	base, err := baseToolsFor(role)
	if err != nil {
		return nil, nil, err
	}
	main, execute = resolveRoleTools(role, base, mcp, tools.EffectiveSettings(cwd()), warnSettingsOnce)
	return main, execute, nil
}

// baseToolsFor is the role's tool set before settings narrow it: the role
// table's allowlist over every tool, with the implement edit/write guard.
func baseToolsFor(role string) ([]tools.Tool, error) {
	if role == "agent" {
		return agentTools()
	}
	names, ok := roles[role]
	if !ok {
		if role == "" {
			return nil, MissingRoleError()
		}
		return nil, fmt.Errorf("unknown role %q; choose one of: %s", role, strings.Join(RoleNames(), ", "))
	}
	types, err := LoadAgentTypes(cwd())
	if err != nil {
		return nil, err
	}
	all := append(tools.All(), orchestrationTools(types, spawnableNames(role, types))...)
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
			if role == "implement" && (n == "edit" || n == "write") {
				t = tools.Guard(t, tools.ProjectWritePaths)
			}
			out = append(out, t)
		}
	}
	return out, nil
}

// resolveRoleTools applies settings to one role's base list plus its MCP
// listing. Settings only narrow: a settings entry for a known tool the role
// does not serve changes nothing, and an unknown name warns once and is
// ignored. submit always stays in main under its own name, because every
// agent brief addresses it by name. warn reports settings problems; nil
// skips the reports but still ignores the entries.
func resolveRoleTools(role string, base []tools.Tool, mcp map[string][]tools.Tool, s tools.Settings, warn func(string)) (main, execute []tools.Tool) {
	known := map[string]bool{"submit": true}
	for _, t := range AllTools() {
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
	for _, t := range base {
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
		final := name
		if rename != "" {
			final = rename
			t = tools.Renamed(t, final)
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
	log.Printf("wick: %s", msg)
}
