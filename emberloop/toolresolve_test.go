package el

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"example.com/emberloop/tools"
)

// hermeticSettings points both settings layers at missing temp files, so a
// test asserting an exact tool list never reads the developer's own tools
// configuration.
func hermeticSettings(t *testing.T) {
	t.Helper()
	t.Setenv("EL_SETTINGS", filepath.Join(t.TempDir(), "missing.json"))
	t.Setenv("EL_PROJECT_SETTINGS", filepath.Join(t.TempDir(), "missing.json"))
	t.Setenv("EL_DECISION_MODEL", "")
	t.Setenv("EL_FIND_THRESHOLD", "")
	t.Setenv("EL_MAX_WORKERS", "")
	t.Setenv("EL_SEARCH_PROVIDER", "")
	t.Setenv("EL_SEARCH_MODE", "")
	// A Daemon binding would resolve Daemon tools into the web roles, so an
	// exact tool list never depends on the shell the test runs in.
	t.Setenv("EL_WEB_PARENT", "")
	t.Setenv("EL_WEB_DAEMON_URL", "")
}

// writeSettingsFile points the global store at a temp file holding body.
func writeSettingsFile(t *testing.T, body string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EL_SETTINGS", p)
	t.Setenv("EL_PROJECT_SETTINGS", filepath.Join(t.TempDir(), "missing.json"))
	t.Setenv("EL_DECISION_MODEL", "")
	t.Setenv("EL_FIND_THRESHOLD", "")
	t.Setenv("EL_MAX_WORKERS", "")
	t.Setenv("EL_SEARCH_PROVIDER", "")
	t.Setenv("EL_SEARCH_MODE", "")
	t.Setenv("EL_WEB_PARENT", "")
	t.Setenv("EL_WEB_DAEMON_URL", "")
}

// fakeTool is a named tool for the resolution tests.
type fakeTool struct{ name string }

func (f fakeTool) Name() string           { return f.name }
func (f fakeTool) Description() string    { return "fake " + f.name }
func (f fakeTool) Schema() map[string]any { return map[string]any{"type": "object"} }
func (f fakeTool) Execute(context.Context, string, json.RawMessage) (string, error) {
	return "fake " + f.name, nil
}

func fakeTools(names ...string) []tools.Tool {
	out := make([]tools.Tool, 0, len(names))
	for _, n := range names {
		out = append(out, fakeTool{name: n})
	}
	return out
}

func toolNames(ts []tools.Tool) []string {
	out := make([]string, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.Name())
	}
	return out
}

func disabled() *bool { f := false; return &f }
func enabled() *bool  { f := true; return &f }

// resolveWith runs the resolution over a fixed native base for one role,
// recording every warning.
func resolveWith(role string, s tools.Settings, mcp map[string][]tools.Tool) (main, execute []string, warns []string) {
	base := fakeTools("read", "bash", "find", "spawn", "finish", "tell", "submit")
	var w []string
	mainT, execT := resolveRoleTools(role, base, mcp, s, func(m string) { w = append(w, m) })
	return toolNames(mainT), toolNames(execT), w
}

func sorted(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

// Without settings every base tool serves in the main list and nothing hides
// behind execute.
func TestResolveDefaultsToMain(t *testing.T) {
	main, execute, _ := resolveWith("work", tools.Settings{}, nil)
	want := []string{"read", "bash", "find", "spawn", "finish", "tell", "submit"}
	if !equalNames(main, want) {
		t.Fatalf("main = %v, want %v", main, want)
	}
	if len(execute) != 0 {
		t.Fatalf("execute = %v, want empty", execute)
	}
}

// A globally disabled tool is absent from both the main list and execute.
func TestResolveDisabledToolIsAbsentEverywhere(t *testing.T) {
	s := tools.Settings{Tools: map[string]tools.ToolSetting{"bash": {Enabled: disabled()}}}
	main, execute, _ := resolveWith("work", s, nil)
	for _, got := range append(main, execute...) {
		if got == "bash" {
			t.Fatalf("disabled bash still served: main=%v execute=%v", main, execute)
		}
	}
}

// A per-role disable hides the tool only for that role.
func TestResolvePerRoleDisable(t *testing.T) {
	s := tools.Settings{Tools: map[string]tools.ToolSetting{
		"bash": {Roles: map[string]tools.ToolRoleOverride{"scout": {Enabled: disabled()}}},
	}}
	main, _, _ := resolveWith("scout", s, nil)
	for _, got := range main {
		if got == "bash" {
			t.Fatalf("bash still served for scout: %v", main)
		}
	}
	main, _, _ = resolveWith("work", s, nil)
	found := false
	for _, got := range main {
		if got == "bash" {
			found = true
		}
	}
	if !found {
		t.Fatalf("bash wrongly hidden from work: %v", main)
	}
}

// A renamed tool serves under its new name, in main and in execute alike.
func TestResolveRenameAppliesInBothLists(t *testing.T) {
	s := tools.Settings{Tools: map[string]tools.ToolSetting{
		"bash": {Rename: "sh"},
		"find": {Rename: "grep", Placement: tools.PlacementExecute},
	}}
	main, execute, _ := resolveWith("work", s, nil)
	if !equalNames(sorted(main), []string{"finish", "read", "sh", "spawn", "submit", "tell"}) {
		t.Fatalf("main = %v, want bash as sh and no find", main)
	}
	if !equalNames(execute, []string{"grep"}) {
		t.Fatalf("execute = %v, want find as grep", execute)
	}
}

// Placement execute hides a tool from the main list without disabling it.
func TestResolveExecutePlacementHidesFromMain(t *testing.T) {
	s := tools.Settings{Tools: map[string]tools.ToolSetting{
		"bash": {Placement: tools.PlacementExecute},
	}}
	main, execute, _ := resolveWith("work", s, nil)
	for _, got := range main {
		if got == "bash" {
			t.Fatalf("execute-placed bash still in main: %v", main)
		}
	}
	if !equalNames(execute, []string{"bash"}) {
		t.Fatalf("execute = %v, want bash", execute)
	}
}

// submit cannot be disabled, moved behind execute, or renamed: agents address
// it by name in every brief.
func TestResolveSubmitIsUndisableable(t *testing.T) {
	s := tools.Settings{Tools: map[string]tools.ToolSetting{
		"submit": {
			Enabled:   disabled(),
			Placement: tools.PlacementExecute,
			Rename:    "done",
			Roles:     map[string]tools.ToolRoleOverride{"scout": {Enabled: disabled(), Placement: tools.PlacementExecute}},
		},
	}}
	main, execute, warns := resolveWith("scout", s, nil)
	if !equalNames(main, []string{"read", "bash", "find", "spawn", "finish", "tell", "submit"}) {
		t.Fatalf("submit must stay in main under its own name: main=%v execute=%v", main, execute)
	}
	if len(execute) != 0 {
		t.Fatalf("submit must never move behind execute: %v", execute)
	}
	if len(warns) == 0 {
		t.Fatal("overriding submit should warn that it is protected")
	}
}

// Unknown tool names are ignored and reported: no role serves them, and each
// resolution says so once (the production warner folds repeats per process).
func TestResolveUnknownToolNamesWarnOnce(t *testing.T) {
	s := tools.Settings{Tools: map[string]tools.ToolSetting{"frobnicate": {Enabled: disabled()}}}
	var warns []string
	warn := func(m string) { warns = append(warns, m) }
	base := fakeTools("read", "submit")
	for _, role := range []string{"work", "review", "scout"} {
		main, _ := resolveRoleTools(role, base, nil, s, warn)
		for _, got := range toolNames(main) {
			if got == "frobnicate" {
				t.Fatalf("unknown tool served for %s", role)
			}
		}
	}
	if len(warns) != 3 {
		t.Fatalf("warns = %v, want one report per resolution", warns)
	}
	for _, w := range warns {
		if !strings.Contains(w, "frobnicate") {
			t.Fatalf("warns = %v, want each naming frobnicate", warns)
		}
	}
}

// The production warner folds repeats: the same message logs once per process,
// so an unknown name every role trips over appears a single time.
func TestWarnSettingsOnceLogsARepeatOnce(t *testing.T) {
	var buf strings.Builder
	orig := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(orig)
	warnedSettings.Lock()
	warnedSettings.seen = map[string]bool{}
	warnedSettings.Unlock()
	warnSettingsOnce("unknown tool \"frobnicate-test-once\" in settings tools; ignoring")
	warnSettingsOnce("unknown tool \"frobnicate-test-once\" in settings tools; ignoring")
	if got := strings.Count(buf.String(), "frobnicate-test-once"); got != 1 {
		t.Fatalf("repeat logged %d times, want once: %q", got, buf.String())
	}
}

// Settings only narrow a role: enabling a tool the role does not have changes
// nothing, and silently so, since a shared global file enables tools most
// roles never see.
func TestResolveNeverWidensARole(t *testing.T) {
	s := tools.Settings{Tools: map[string]tools.ToolSetting{"bash": {Enabled: enabled()}}}
	base := fakeTools("read", "find")
	var warns []string
	main, _ := resolveRoleTools("review", base, nil, s, func(m string) { warns = append(warns, m) })
	if !equalNames(toolNames(main), []string{"read", "find"}) {
		t.Fatalf("settings widened review with bash: %v", toolNames(main))
	}
	if len(warns) != 0 {
		t.Fatalf("a known-but-unlisted tool must stay silent, got %v", warns)
	}
}

// MCP tools default behind execute, honour include and exclude globs over
// their original names, and serve renamed.
func TestResolveMCPIncludeExcludeAndRename(t *testing.T) {
	mcp := map[string][]tools.Tool{
		"github": fakeTools("search_repositories", "delete_repo", "get_repo"),
	}
	s := tools.Settings{MCPServers: map[string]tools.MCPServerSetting{
		"github": {
			Include: []string{"search_*", "get_*"},
			Exclude: []string{"delete_*", "get_repo"},
			Rename:  map[string]string{"search_repositories": "gh_search"},
		},
	}}
	main, execute, _ := resolveWith("work", s, mcp)
	if len(main) != 7 {
		t.Fatalf("no MCP tool defaults to main: main=%v", main)
	}
	if !equalNames(execute, []string{"gh_search"}) {
		t.Fatalf("execute = %v, want the renamed search tool only", execute)
	}
}

// A per-role placement promotes an MCP tool to the main list for that role
// only; a per-role disable removes it there.
func TestResolveMCPPerRolePlacementAndDisable(t *testing.T) {
	mcp := map[string][]tools.Tool{"github": fakeTools("search_repositories")}
	s := tools.Settings{MCPServers: map[string]tools.MCPServerSetting{
		"github": {Roles: map[string]tools.MCPServerRoleOverride{
			"review": {Placement: tools.PlacementMain},
			"scout":  {Enabled: disabled()},
		}},
	}}
	main, execute, _ := resolveWith("review", s, mcp)
	if !containsName(main, "search_repositories") {
		t.Fatalf("review must see the promoted tool in main: main=%v execute=%v", main, execute)
	}
	main, execute, _ = resolveWith("scout", s, mcp)
	for _, got := range append(main, execute...) {
		if got == "search_repositories" {
			t.Fatalf("scout disable ignored: main=%v execute=%v", main, execute)
		}
	}
	main, execute, _ = resolveWith("work", s, mcp)
	if containsName(main, "search_repositories") || !containsName(execute, "search_repositories") {
		t.Fatalf("work must keep the default execute placement: main=%v execute=%v", main, execute)
	}
}

// A renamed tool that collides with a served name loses: the first tool keeps
// the name and the collision warns.
func TestResolveRenameCollisionKeepsTheFirst(t *testing.T) {
	s := tools.Settings{Tools: map[string]tools.ToolSetting{"bash": {Rename: "read"}}}
	main, _, warns := resolveWith("work", s, nil)
	count := 0
	for _, got := range main {
		if got == "read" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("collision must leave exactly one read: %v", main)
	}
	if len(warns) == 0 {
		t.Fatal("a rename collision should warn")
	}
}

func equalNames(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func containsName(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}

// A disabled tool is absent from the served tool list for its role, and a
// rename takes effect there: the MCP server serves RoleTools, so both flow
// through.
func TestRoleToolsHonoursDisabledAndRenamed(t *testing.T) {
	writeSettingsFile(t, `{"tools":{"bash":{"enabled":false},"read":{"rename":"rd"}}}`)
	served, err := RoleTools("work")
	if err != nil {
		t.Fatal(err)
	}
	names := toolNames(served)
	if containsName(names, "bash") {
		t.Fatalf("disabled bash still served: %v", names)
	}
	if !containsName(names, "rd") || containsName(names, "read") {
		t.Fatalf("read rename did not take effect: %v", names)
	}
}

// Placement execute hides a tool from the served list while keeping it in
// the execute bucket the future stub generator reads.
func TestRoleToolsHidesExecutePlacedTools(t *testing.T) {
	writeSettingsFile(t, `{"tools":{"bash":{"placement":"execute"}}}`)
	served, err := RoleTools("work")
	if err != nil {
		t.Fatal(err)
	}
	if containsName(toolNames(served), "bash") {
		t.Fatalf("execute-placed bash still served: %v", toolNames(served))
	}
	_, execute, err := ResolvedTools("work")
	if err != nil {
		t.Fatal(err)
	}
	if !containsName(toolNames(execute), "bash") {
		t.Fatalf("execute bucket lost bash: %v", toolNames(execute))
	}
}

// The project layer narrows serving too: a project disable hides the tool
// even though the global file enables it.
func TestRoleToolsHonoursTheProjectLayer(t *testing.T) {
	writeSettingsFile(t, `{"tools":{"bash":{"enabled":true}}}`)
	project := filepath.Join(t.TempDir(), "project.json")
	if err := os.WriteFile(project, []byte(`{"tools":{"bash":{"enabled":false}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EL_PROJECT_SETTINGS", project)
	served, err := RoleTools("work")
	if err != nil {
		t.Fatal(err)
	}
	if containsName(toolNames(served), "bash") {
		t.Fatalf("project-disabled bash still served: %v", toolNames(served))
	}
}
