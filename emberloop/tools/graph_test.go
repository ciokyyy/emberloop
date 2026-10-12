package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"example.com/emberloop/tools/mcpfake"
)

// graphFakeEnv points the graph synthesis at a fake MCP server and returns
// its method log. The fake's config travels in this process's environment,
// which the spawned child inherits; the command override names the re-executed
// test binary.
func graphFakeEnv(t *testing.T, cfg mcpfake.Config) string {
	t.Helper()
	log := filepath.Join(t.TempDir(), "graph-methods.log")
	cfg.LogPath = log
	command, args, env := mcpfake.Helper(t, cfg)
	t.Setenv("EL_GRAPH_TEST_COMMAND", command)
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("EL_GRAPH_TEST_ARGS", string(raw))
	for k, v := range env {
		t.Setenv(k, v)
	}
	return log
}

// graphHermeticEnv points both settings layers at missing files and hides
// every binary, so synthesis sees no real graph binary.
func graphHermeticEnv(t *testing.T) {
	t.Helper()
	t.Setenv("EL_SETTINGS", filepath.Join(t.TempDir(), "missing.json"))
	t.Setenv("EL_PROJECT_SETTINGS", filepath.Join(t.TempDir(), "missing.json"))
	t.Setenv("EL_BIN_DIR", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	t.Setenv("EL_GRAPH_TEST_COMMAND", "")
	t.Setenv("EL_GRAPH_TEST_ARGS", "")
}

// graphTools lists the fake's graph-shaped tools: the five exposed plus the
// management set and detect_changes, so shaping is proved, not assumed.
func graphTools() []mcpfake.ToolDef {
	return []mcpfake.ToolDef{
		{Name: "search_graph"}, {Name: "trace_path"}, {Name: "get_code_snippet"},
		{Name: "get_file_outline"}, {Name: "get_architecture"},
		{Name: "detect_changes"},
		{Name: "index_repository"}, {Name: "list_projects"}, {Name: "delete_project"},
		{Name: "index_status"}, {Name: "check_index_coverage"},
	}
}

// Two worktrees of one repo map to different projects, and the main checkout
// differs from both, so an index never answers for the wrong tree.
func TestGraphProjectSlugDiffersPerWorktree(t *testing.T) {
	a := GraphProjectSlug("/repo/.emberloop-worktrees/01-x")
	b := GraphProjectSlug("/repo/.emberloop-worktrees/02-y")
	main := GraphProjectSlug("/repo")
	if a == "" || b == "" || main == "" {
		t.Fatal("slug must not be empty")
	}
	if a == b {
		t.Fatalf("two worktrees share slug %q", a)
	}
	if a == main || b == main {
		t.Fatalf("worktree slug collides with main checkout: %q vs %q", a, main)
	}
	if a != GraphProjectSlug("/repo/.emberloop-worktrees/01-x") {
		t.Fatal("slug must be deterministic")
	}
	for _, s := range []string{a, b, main} {
		for _, r := range s {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '.' || r == '_') {
				t.Fatalf("slug %q holds unsafe rune %q", s, r)
			}
		}
	}
}

// graph.enabled=false synthesizes nothing, even with a server behind the seam.
func TestGraphDisabledSynthesizesNothing(t *testing.T) {
	graphHermeticEnv(t)
	graphFakeEnv(t, mcpfake.Config{Tools: graphTools()})
	t.Setenv("EL_SETTINGS", writeGraphSettings(t, `{"graph":{"enabled":false}}`))
	s := EffectiveSettings(t.TempDir())
	if _, ok := GraphServerSetting(s); ok {
		t.Fatal("disabled graph must synthesize no server")
	}
	got := WithGraphServer(s)
	if _, ok := got.MCPServers[GraphServerName]; ok {
		t.Fatal("disabled graph must not join the server map")
	}
}

// No binary behind the seam means no graph tools and no error.
func TestGraphMissingBinarySynthesizesNothing(t *testing.T) {
	graphHermeticEnv(t)
	s := EffectiveSettings(t.TempDir())
	if _, ok := GraphServerSetting(s); ok {
		t.Fatal("missing binary must synthesize no server")
	}
	if _, ok := WithGraphServer(s).MCPServers[GraphServerName]; ok {
		t.Fatal("missing binary must not join the server map")
	}
}

// The synthesis names the binary, scopes the cache, and places review and
// scout in main with the rest behind execute.
func TestGraphSynthesisShapesServer(t *testing.T) {
	graphHermeticEnv(t)
	graphFakeEnv(t, mcpfake.Config{Tools: graphTools()})
	s := EffectiveSettings(t.TempDir())
	cfg, ok := GraphServerSetting(s)
	if !ok {
		t.Fatal("override behind the seam must synthesize a server")
	}
	if cfg.Command == "" {
		t.Fatal("synthesis must name a command")
	}
	for _, want := range GraphExposedTools {
		if !matchMCPGlobs(cfg.Include, want) {
			t.Fatalf("include must cover exposed tool %q: %v", want, cfg.Include)
		}
	}
	for _, want := range GraphHiddenTools {
		if !matchMCPGlobs(cfg.Exclude, want) {
			t.Fatalf("exclude must cover hidden tool %q: %v", want, cfg.Exclude)
		}
	}
	if cfg.Placement != PlacementExecute {
		t.Fatalf("default placement = %q, want execute", cfg.Placement)
	}
	for _, role := range []string{"review", "scout"} {
		ov, ok := cfg.Roles[role]
		if !ok || ov.Placement != PlacementMain {
			t.Fatalf("role %q must place graph in main: %+v", role, cfg.Roles)
		}
	}
}

// graph.cache_dir reaches the server as CBM_CACHE_DIR.
func TestGraphCacheDirBecomesEnv(t *testing.T) {
	graphHermeticEnv(t)
	graphFakeEnv(t, mcpfake.Config{Tools: graphTools()})
	t.Setenv("EL_SETTINGS", writeGraphSettings(t, `{"graph":{"cache_dir":"/tmp/cbm"}}`))
	cfg, ok := GraphServerSetting(EffectiveSettings(t.TempDir()))
	if !ok {
		t.Fatal("override must synthesize a server")
	}
	if cfg.Env["CBM_CACHE_DIR"] != "/tmp/cbm" {
		t.Fatalf("env = %v, want CBM_CACHE_DIR=/tmp/cbm", cfg.Env)
	}
}

// An explicit user graph entry wins over the synthesis.
func TestGraphUserEntryWins(t *testing.T) {
	graphHermeticEnv(t)
	graphFakeEnv(t, mcpfake.Config{Tools: graphTools()})
	t.Setenv("EL_SETTINGS", writeGraphSettings(t, `{"mcp_servers":{"graph":{"command":"/user/graph"}}}`))
	got := WithGraphServer(EffectiveSettings(t.TempDir()))
	if got.MCPServers[GraphServerName].Command != "/user/graph" {
		t.Fatalf("user entry lost: %+v", got.MCPServers[GraphServerName])
	}
}

// The registry shaping keeps the exposed tools and drops the management set
// and detect_changes; per-role placement is proved in the el package,
// where the role resolution lives.
func TestGraphRegistryShapesExposedSet(t *testing.T) {
	graphHermeticEnv(t)
	graphFakeEnv(t, mcpfake.Config{Tools: graphTools()})
	s := WithGraphServer(EffectiveSettings(t.TempDir()))
	reg := NewRegistry(s.MCPServers)
	t.Cleanup(reg.Close)
	got, err := reg.Tools(context.Background(), GraphServerName)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range GraphExposedTools {
		if !containsToolName(got, name) {
			t.Errorf("graph must list exposed tool %q: %v", name, toolNameList(got))
		}
	}
	for _, name := range GraphHiddenTools {
		if containsToolName(got, name) {
			t.Errorf("graph must never list %q: %v", name, toolNameList(got))
		}
	}
}

// The first graph query waits on the index status before answering, the
// status tool itself stays hidden, and one wait covers every tool of the
// snapshot.
func TestGraphGateWaitsOnIndexStatus(t *testing.T) {
	graphHermeticEnv(t)
	log := graphFakeEnv(t, mcpfake.Config{
		Tools: []mcpfake.ToolDef{{Name: "search_graph"}, {Name: "get_architecture"}, {Name: "index_status"}, {Name: "list_projects"}},
		Calls: map[string]mcpfake.Result{
			"index_status":     {Text: `{"status":"ready"}`},
			"search_graph":     {Text: "hits"},
			"get_architecture": {Text: "arch"},
		},
	})
	tree := t.TempDir()
	s := WithGraphServer(EffectiveSettings(tree))
	reg := NewRegistry(s.MCPServers)
	t.Cleanup(reg.Close)
	listed, errs := reg.Snapshot(context.Background())
	if len(errs) != 0 {
		t.Fatalf("snapshot errs = %v", errs)
	}
	call := func(ctx context.Context, tool string, args json.RawMessage) (string, error) {
		return reg.Call(ctx, GraphServerName, tool, args)
	}
	gated := GateGraphTools(listed[GraphServerName], call, tree, 5*time.Second)
	if len(gated) != 2 {
		t.Fatalf("gated = %v, want the two exposed tools", toolNameList(gated))
	}
	if text, err := gated[0].Execute(context.Background(), tree, json.RawMessage(`{}`)); err != nil || text != "hits" {
		t.Fatalf("first gated call = %q, %v, want hits", text, err)
	}
	if text, err := gated[1].Execute(context.Background(), tree, json.RawMessage(`{}`)); err != nil || text != "arch" {
		t.Fatalf("second gated call = %q, %v, want arch without another wait", text, err)
	}
	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	// Resolve checks once, the wait checks once, then one call per query:
	// a per-tool wait would cost two more.
	if n := strings.Count(string(raw), "tools/call\n"); n != 4 {
		t.Fatalf("want 4 server calls (2 status + 2 queries), log:\n%s", raw)
	}
}

// A gate whose index never reports ready still answers within its bound.
func TestGraphGateProceedsWhenIndexNeverReady(t *testing.T) {
	graphHermeticEnv(t)
	oldPoll := graphGatePoll
	graphGatePoll = 20 * time.Millisecond
	t.Cleanup(func() { graphGatePoll = oldPoll })
	graphFakeEnv(t, mcpfake.Config{
		Tools: []mcpfake.ToolDef{{Name: "search_graph"}, {Name: "index_status"}},
		Calls: map[string]mcpfake.Result{
			"index_status": {Text: `{"project":"p","status":"indexing"}`},
			"search_graph": {Text: "hits"},
		},
	})
	tree := t.TempDir()
	s := WithGraphServer(EffectiveSettings(tree))
	reg := NewRegistry(s.MCPServers)
	t.Cleanup(reg.Close)
	listed, errs := reg.Snapshot(context.Background())
	if len(errs) != 0 {
		t.Fatalf("snapshot errs = %v", errs)
	}
	call := func(ctx context.Context, tool string, args json.RawMessage) (string, error) {
		return reg.Call(ctx, GraphServerName, tool, args)
	}
	gated := GateGraphTools(listed[GraphServerName], call, tree, 250*time.Millisecond)
	text, err := gated[0].Execute(context.Background(), tree, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if text != "hits" {
		t.Fatalf("gated call = %q, want hits despite indexing", text)
	}
}

// A server that indexed under a path-derived name still answers: the gate
// resolves the project through list_projects matched by root path.
func TestGraphGateResolvesRenamedProject(t *testing.T) {
	graphHermeticEnv(t)
	oldPoll := graphGatePoll
	graphGatePoll = 20 * time.Millisecond
	t.Cleanup(func() { graphGatePoll = oldPoll })
	tree := t.TempDir()
	projects := `{"projects":[{"name":"real-proj","root_path":` + strconv.Quote(tree) + `}]}`
	graphFakeEnv(t, mcpfake.Config{
		Tools: []mcpfake.ToolDef{{Name: "search_graph"}, {Name: "index_status"}, {Name: "list_projects"}},
		Calls: map[string]mcpfake.Result{
			"index_status":  {Message: "project not found or not indexed"},
			"list_projects": {Text: projects},
			"search_graph":  {Text: "hits"},
		},
	})
	s := WithGraphServer(EffectiveSettings(tree))
	reg := NewRegistry(s.MCPServers)
	t.Cleanup(reg.Close)
	listed, errs := reg.Snapshot(context.Background())
	if len(errs) != 0 {
		t.Fatalf("snapshot errs = %v", errs)
	}
	var mu sync.Mutex
	var called []string
	call := func(ctx context.Context, tool string, args json.RawMessage) (string, error) {
		mu.Lock()
		called = append(called, tool)
		mu.Unlock()
		return reg.Call(ctx, GraphServerName, tool, args)
	}
	gated := GateGraphTools(listed[GraphServerName], call, tree, 250*time.Millisecond)
	text, err := gated[0].Execute(context.Background(), tree, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	// "hits" only comes from search_graph, so the query went out; the
	// recorded calls prove the resolution path that preceded it.
	if text != "hits" {
		t.Fatalf("gated call = %q, want hits through the resolved project", text)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, want := range []string{"index_status", "list_projects"} {
		found := false
		for _, got := range called {
			if got == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("gate never called %q: %v", want, called)
		}
	}
}

func writeGraphSettings(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func containsToolName(ts []Tool, name string) bool {
	for _, tl := range ts {
		if tl.Name() == name {
			return true
		}
	}
	return false
}

func toolNameList(ts []Tool) []string {
	out := make([]string, 0, len(ts))
	for _, tl := range ts {
		out = append(out, tl.Name())
	}
	return out
}
