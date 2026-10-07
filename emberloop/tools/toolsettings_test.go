package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// globalProjectEnv points the global store at a temp file holding globalBody
// and the project store at dir/.emberloop/settings.json holding projectBody,
// so a test reads exactly the two layers it wrote.
func globalProjectEnv(t *testing.T, dir, globalBody, projectBody string) {
	t.Helper()
	g := filepath.Join(t.TempDir(), "global.json")
	if err := os.WriteFile(g, []byte(globalBody), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EMBERLOOP_SETTINGS", g)
	t.Setenv("EMBERLOOP_DECISION_MODEL", "")
	t.Setenv("EMBERLOOP_FIND_THRESHOLD", "")
	t.Setenv("EMBERLOOP_MAX_WORKERS", "")
	if projectBody == "" {
		t.Setenv("EMBERLOOP_PROJECT_SETTINGS", filepath.Join(t.TempDir(), "missing.json"))
		return
	}
	p := filepath.Join(dir, ".emberloop", "settings.json")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(projectBody), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EMBERLOOP_PROJECT_SETTINGS", p)
}

// The project file wins field by field: a project threshold beats the global
// one while the global decision model still shows through.
func TestEffectiveSettingsProjectWinsFieldByField(t *testing.T) {
	dir := t.TempDir()
	globalProjectEnv(t, dir,
		`{"decision_model":"global/model","threshold":0.3}`,
		`{"threshold":0.5}`)
	s := EffectiveSettings(dir)
	if s.DecisionModel != "global/model" {
		t.Fatalf("global decision model lost: %+v", s)
	}
	if s.Threshold != 0.5 {
		t.Fatalf("project threshold did not win: %+v", s)
	}
	if got := s.SourceOf("threshold"); got != "project" {
		t.Fatalf("threshold source = %q, want project", got)
	}
	if got := s.SourceOf("decision_model"); got != "file" {
		t.Fatalf("decision_model source = %q, want file", got)
	}
}

// Role slots merge field by field too: a project model replaces only the
// model, keeping the global harness and effort.
func TestEffectiveSettingsRolesMergeFieldByField(t *testing.T) {
	dir := t.TempDir()
	globalProjectEnv(t, dir,
		`{"roles":{"scout":{"harness":"pig","model":"global-m","effort":"low"}}}`,
		`{"roles":{"scout":{"model":"project-m"}}}`)
	s := EffectiveSettings(dir)
	got := s.Roles["scout"]
	if got.Harness != "pig" || got.Model != "project-m" || got.Effort != "low" {
		t.Fatalf("role slot merged to %+v, want harness pig model project-m effort low", got)
	}
}

// Tool entries merge by key and then field by field, with per-role overrides
// merging the same way: the project disables bash for scout while the global
// rename still shows through.
func TestEffectiveSettingsToolsMergeByKey(t *testing.T) {
	dir := t.TempDir()
	globalProjectEnv(t, dir,
		`{"tools":{"bash":{"enabled":true,"placement":"main","rename":"sh","roles":{"scout":{"placement":"execute"}}},"read":{"enabled":false}}}`,
		`{"tools":{"bash":{"roles":{"scout":{"enabled":false}}}}}`)
	s := EffectiveSettings(dir)
	bash := s.Tools["bash"]
	if bash.Enabled == nil || !*bash.Enabled {
		t.Fatalf("global bash enabled lost: %+v", bash)
	}
	if bash.Rename != "sh" {
		t.Fatalf("global bash rename lost: %+v", bash)
	}
	scout, ok := bash.Roles["scout"]
	if !ok || scout.Enabled == nil || *scout.Enabled {
		t.Fatalf("project scout disable lost: %+v", bash.Roles)
	}
	if scout.Placement != "execute" {
		t.Fatalf("global scout placement lost: %+v", scout)
	}
	if s.Tools["read"].Enabled == nil || *s.Tools["read"].Enabled {
		t.Fatalf("global read disable lost: %+v", s.Tools["read"])
	}
}

// MCP server entries merge by key: scalar fields win wholesale, env and
// rename maps merge by key, and include lists replace.
func TestEffectiveSettingsMCPServersMergeByKey(t *testing.T) {
	dir := t.TempDir()
	globalProjectEnv(t, dir,
		`{"mcp_servers":{"github":{"command":"npx","args":["a"],"env":{"K":"v"},"placement":"execute","include":["search_*"],"rename":{"a":"b"}}}}`,
		`{"mcp_servers":{"github":{"env":{"K2":"v2"},"include":["other_*"],"rename":{"c":"d"}}}}`)
	s := EffectiveSettings(dir)
	gh := s.MCPServers["github"]
	if gh.Command != "npx" || gh.Placement != "execute" {
		t.Fatalf("global scalar fields lost: %+v", gh)
	}
	if len(gh.Args) != 1 || gh.Args[0] != "a" {
		t.Fatalf("global args lost: %+v", gh)
	}
	if gh.Env["K"] != "v" || gh.Env["K2"] != "v2" {
		t.Fatalf("env maps did not merge by key: %+v", gh.Env)
	}
	if len(gh.Include) != 1 || gh.Include[0] != "other_*" {
		t.Fatalf("project include did not replace: %+v", gh.Include)
	}
	if gh.Rename["a"] != "b" || gh.Rename["c"] != "d" {
		t.Fatalf("rename maps did not merge by key: %+v", gh.Rename)
	}
}

// The environment still wins over both files.
func TestEffectiveSettingsEnvWinsOverTheProjectFile(t *testing.T) {
	dir := t.TempDir()
	globalProjectEnv(t, dir, `{"threshold":0.3}`, `{"threshold":0.5}`)
	t.Setenv("EMBERLOOP_FIND_THRESHOLD", "0.7")
	if got := EffectiveSettings(dir).Threshold; got != 0.7 {
		t.Fatalf("env threshold lost, got %v", got)
	}
}

// Unknown keys survive at the top level and inside tool and server entries,
// so a save never drops what a newer emberloop wrote.
func TestSaveSettingsKeepsUnknownKeysEverywhere(t *testing.T) {
	dir := t.TempDir()
	globalProjectEnv(t, dir,
		`{"threshold":0.3,"future_top":1,"tools":{"bash":{"enabled":true,"future_field":"kept"}},"mcp_servers":{"github":{"command":"npx","future_field":"kept"}}}`,
		`{}`)
	s := LoadSettings()
	s.Threshold = 0.4
	if err := SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(os.Getenv("EMBERLOOP_SETTINGS"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"future_top": 1`, `"future_field": "kept"`, `"threshold": 0.4`} {
		if !containsStr(string(raw), want) {
			t.Errorf("saved settings lost %q: %s", want, raw)
		}
	}
}

// Saving writes the global file only: project values never leak into it.
func TestSaveSettingsDoesNotWriteTheProjectFile(t *testing.T) {
	dir := t.TempDir()
	globalProjectEnv(t, dir, `{"threshold":0.3}`, `{"threshold":0.5}`)
	s := LoadSettings()
	s.Threshold = 0.4
	if err := SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(os.Getenv("EMBERLOOP_SETTINGS"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) == "" || !containsStr(string(raw), "0.4") {
		t.Fatalf("global save did not hold 0.4: %s", raw)
	}
	// The project file is untouched and still wins when merged.
	if got := EffectiveSettings(dir).Threshold; got != 0.5 {
		t.Fatalf("project file changed by a global save: %v", got)
	}
}

func containsStr(hay, needle string) bool {
	for i := 0; i+len(needle) <= len(hay); i++ {
		if hay[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// Merging a project entry that carries unknown fields over a global file
// without them must not panic: the extras maps start empty, not nil.
func TestMergeProjectUnknownEntryFieldsOverEmptyGlobal(t *testing.T) {
	dir := t.TempDir()
	globalProjectEnv(t, dir,
		`{"threshold":0.3}`,
		`{"roles":{"scout":{"harness":"pig","future":"kept"}},"tools":{"bash":{"enabled":true,"future":"kept"}},"mcp_servers":{"github":{"command":"npx","future":"kept"}}}`)
	s := EffectiveSettings(dir)
	if s.Roles["scout"].Harness != "pig" {
		t.Fatalf("project role slot lost: %+v", s.Roles["scout"])
	}
	if s.Tools["bash"].Enabled == nil || !*s.Tools["bash"].Enabled {
		t.Fatalf("project tool entry lost: %+v", s.Tools["bash"])
	}
	if s.MCPServers["github"].Command != "npx" {
		t.Fatalf("project server entry lost: %+v", s.MCPServers["github"])
	}
}

// Unknown fields inside a per-role override survive a save, like every other
// level.
func TestSaveSettingsKeepsUnknownOverrideFields(t *testing.T) {
	dir := t.TempDir()
	globalProjectEnv(t, dir,
		`{"tools":{"bash":{"roles":{"scout":{"enabled":false,"future":"kept"}}}},"mcp_servers":{"github":{"command":"npx","roles":{"review":{"placement":"main","future":"kept"}}}}}`,
		`{}`)
	s := LoadSettings()
	s.Threshold = 0.4
	if err := SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(os.Getenv("EMBERLOOP_SETTINGS"))
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(string(raw), `"future": "kept"`); count != 2 {
		t.Fatalf("override unknown fields lost, want 2 kept: %s", raw)
	}
}

// Seam (ticket 19): installer — the graph settings decode, default to
// enabled, merge project-over-global field by field, and round-trip unknown
// graph fields through a save.
func TestGraphSettingsDecodeMergeAndRoundTrip(t *testing.T) {
	dir := t.TempDir()
	globalProjectEnv(t, dir,
		`{"graph":{"enabled":true,"binary":"/global/cbm","future":"kept"}}`,
		`{"graph":{"enabled":false}}`)
	s := EffectiveSettings(dir)
	if s.GraphEnabled() {
		t.Fatalf("project graph.enabled=false must win, got %+v", s.Graph)
	}
	if s.Graph.Binary != "/global/cbm" {
		t.Fatalf("global graph.binary must show through, got %+v", s.Graph)
	}
	if got := s.SourceOf("graph.enabled"); got != "project" {
		t.Fatalf("graph.enabled source = %q, want project", got)
	}
	// Unknown graph fields survive a save.
	g := LoadSettings()
	if err := SaveSettings(g); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(os.Getenv("EMBERLOOP_SETTINGS"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `kept`) {
		t.Errorf("saved settings lost the unknown graph field: %s", raw)
	}
	if !strings.Contains(string(raw), `binary`) {
		t.Errorf("saved settings lost graph.binary: %s", raw)
	}
}

func TestGraphEnabledDefaultsToTrue(t *testing.T) {
	dir := t.TempDir()
	globalProjectEnv(t, dir, `{}`, `{}`)
	if !EffectiveSettings(dir).GraphEnabled() {
		t.Fatal("graph must be enabled by default")
	}
}

func TestSaveSettingsOmitsAnUnsetGraph(t *testing.T) {
	dir := t.TempDir()
	globalProjectEnv(t, dir, `{"threshold":0.3}`, `{}`)
	s := LoadSettings()
	if err := SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(os.Getenv("EMBERLOOP_SETTINGS"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"graph"`) {
		t.Errorf("a save with no graph settings must not write one, got: %s", raw)
	}
}
