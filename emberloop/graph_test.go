package el

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

	"example.com/emberloop/tools"
	"example.com/emberloop/tools/mcpfake"
)

// graphFakeSettings points the graph synthesis at a fake MCP server offering
// cfg's tools, and returns the effective settings with the synthesis applied
// plus the fake's method log.
func graphFakeSettings(t *testing.T, cfg mcpfake.Config) (tools.Settings, string) {
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
	hermeticSettings(t)
	s := tools.WithGraphServer(tools.EffectiveSettings(t.TempDir()))
	if _, ok := s.MCPServers[tools.GraphServerName]; !ok {
		t.Fatal("graph synthesis must register the server")
	}
	return s, log
}

// graphFakeTools offers the exposed set plus every hidden tool.
func graphFakeTools() []mcpfake.ToolDef {
	return []mcpfake.ToolDef{
		{Name: "search_graph"}, {Name: "trace_path"}, {Name: "get_code_snippet"},
		{Name: "get_file_outline"}, {Name: "get_architecture"},
		{Name: "detect_changes"},
		{Name: "index_repository"}, {Name: "list_projects"}, {Name: "delete_project"},
		{Name: "index_status"}, {Name: "check_index_coverage"},
	}
}

// Review and scout serve the exposed graph tools in main; every other role
// keeps them behind execute; the management tools and detect_changes never
// serve anywhere.
func TestGraphExposedSetPerRole(t *testing.T) {
	s, _ := graphFakeSettings(t, mcpfake.Config{Tools: graphFakeTools()})
	reg := tools.NewRegistry(s.MCPServers)
	t.Cleanup(reg.Close)
	listed := MCPSnapshot(reg)
	if _, ok := listed[tools.GraphServerName]; !ok {
		t.Fatalf("snapshot must list graph: %v", listed)
	}
	for role, wantMain := range map[string]bool{
		"review": true, "scout": true,
		"work": false, "plan": false, "full": false, "agent": false,
	} {
		if role == "agent" {
			t.Setenv("EL_AGENT_TYPE", "reviewer-spec")
		}
		main, execute, err := ResolvedToolsWithMCP(role, listed)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range tools.GraphExposedTools {
			inMain, inExecute := containsName(toolNames(main), name), containsName(toolNames(execute), name)
			if wantMain && !inMain {
				t.Errorf("role %q must serve %q in main: main=%v execute=%v", role, name, toolNames(main), toolNames(execute))
			}
			if !wantMain && (inMain || !inExecute) {
				t.Errorf("role %q must serve %q behind execute: main=%v execute=%v", role, name, toolNames(main), toolNames(execute))
			}
		}
		for _, name := range tools.GraphHiddenTools {
			if containsName(toolNames(main), name) || containsName(toolNames(execute), name) {
				t.Errorf("role %q must never serve %q: main=%v execute=%v", role, name, toolNames(main), toolNames(execute))
			}
		}
	}
}

// A missing binary exposes no graph tools and raises no error, through the
// same resolution the server serves.
func TestGraphMissingExposesNothing(t *testing.T) {
	hermeticSettings(t)
	t.Setenv("EL_BIN_DIR", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	t.Setenv("EL_GRAPH_TEST_COMMAND", "")
	t.Setenv("EL_GRAPH_TEST_ARGS", "")
	t.Setenv("EL_STATE_DIR", t.TempDir())
	s := tools.WithGraphServer(tools.EffectiveSettings(t.TempDir()))
	if _, ok := s.MCPServers[tools.GraphServerName]; ok {
		t.Fatal("missing binary must register no graph server")
	}
	reg := tools.NewRegistry(s.MCPServers)
	t.Cleanup(reg.Close)
	listed := MCPSnapshot(reg)
	for _, role := range []string{"review", "scout", "work", "agent"} {
		if role == "agent" {
			t.Setenv("EL_AGENT_TYPE", "reviewer-spec")
		}
		main, execute, err := ResolvedToolsWithMCP(role, listed)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range append(append([]string{}, tools.GraphExposedTools...), tools.GraphHiddenTools...) {
			if containsName(toolNames(main), name) || containsName(toolNames(execute), name) {
				t.Errorf("role %q serves %q with no binary: main=%v execute=%v", role, name, toolNames(main), toolNames(execute))
			}
		}
	}
}

// graphCallRec records one lifecycle call the tests observe.
type graphCallRec struct {
	worktree, tool string
	args           map[string]any
}

// graphRecorder is a threadsafe sink for lifecycle calls: the async creation
// index lands after Spawn returns, so tests poll it.
type graphRecorder struct {
	mu    sync.Mutex
	calls []graphCallRec
}

func (r *graphRecorder) add(worktree, tool string, args map[string]any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, graphCallRec{worktree, tool, args})
}

func (r *graphRecorder) withTool(tool string) []graphCallRec {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []graphCallRec
	for _, c := range r.calls {
		if c.tool == tool {
			out = append(out, c)
		}
	}
	return out
}

// stubGraphDial records every lifecycle call and answers through respond. A
// nil respond answers `indexed`.
func stubGraphDial(t *testing.T, respond func(tool string, args map[string]any) (string, error)) *graphRecorder {
	t.Helper()
	rec := &graphRecorder{}
	old := graphLifecycleDial
	graphLifecycleDial = func(ctx context.Context, worktree, tool string, args map[string]any) (string, error) {
		rec.add(worktree, tool, args)
		if respond != nil {
			return respond(tool, args)
		}
		return `{"status":"indexed"}`, nil
	}
	t.Cleanup(func() { graphLifecycleDial = old })
	return rec
}

// useRealGraphDial runs lifecycle through the real dial (and the fake server
// behind the command seam) for one test.
func useRealGraphDial(t *testing.T) {
	t.Helper()
	old := graphLifecycleDial
	graphLifecycleDial = graphDialReal
	t.Cleanup(func() { graphLifecycleDial = old })
}

// waitGraphCall polls for a lifecycle call, for the asynchronous creation
// index that lands after Spawn returns.
func waitGraphCall(t *testing.T, rec *graphRecorder, tool string) graphCallRec {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if got := rec.withTool(tool); len(got) > 0 {
			return got[0]
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no lifecycle call to %q arrived", tool)
	return graphCallRec{}
}

// The real dial reaches the fake server standing in for the binary: the index
// round trip carries the worktree path and its slug.
func TestGraphDialReachesFakeServer(t *testing.T) {
	s, log := graphFakeSettings(t, mcpfake.Config{Tools: []mcpfake.ToolDef{
		{Name: "index_repository"}, {Name: "index_status"}, {Name: "delete_project"},
	}})
	_ = s
	useRealGraphDial(t)
	tree := filepath.Join(t.TempDir(), "tree")
	text, err := graphIndex(context.Background(), tree)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, tree) || !strings.Contains(text, tools.GraphProjectSlug(tree)) {
		t.Fatalf("index echo = %q, want the worktree path and its slug", text)
	}
	raw, err := readGraphLog(log)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, "tools/call\n") {
		t.Fatalf("index never reached the server, log:\n%s", raw)
	}
}

// Spawning a worker indexes its worktree asynchronously, naming the project
// for the worktree path.
func TestGraphIndexOnWorktreeCreation(t *testing.T) {
	rec := stubGraphDial(t, nil)
	statePath = filepath.Join(t.TempDir(), "workers.json")
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	repo := graphGitRepo(t)
	ticket := filepath.Join(repo, "07-example.md")
	if err := writeGraphFile(ticket, "# Example\n\nBlocked by: \n"); err != nil {
		t.Fatal(err)
	}
	h := newTicketHerdr(osExec)
	worker, err := Spawn(h.run, SpawnInput{Ticket: ticket, Repo: repo, Base: "main", Harness: "stub", Launch: []string{"true"}})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	got := waitGraphCall(t, rec, "index_repository")
	if got.worktree != worker.Worktree {
		t.Fatalf("index worktree = %q, want %q", got.worktree, worker.Worktree)
	}
	if got.args["repo_path"] != worker.Worktree {
		t.Fatalf("index repo_path = %v, want %q", got.args["repo_path"], worker.Worktree)
	}
	if got.args["name"] != tools.GraphProjectSlug(worker.Worktree) {
		t.Fatalf("index name = %v, want the path slug", got.args["name"])
	}
}

// A reviewer spawn re-indexes the tree under review before launching.
func TestGraphReindexBeforeReviewerSpawn(t *testing.T) {
	rec := stubGraphDial(t, nil)
	_, tree := reviewRepo(t)
	f := newFakeReviewers(answers("pass", "pass"))
	if _, err := reviewSpawn(t, f, tree, map[string]any{"type": reviewerStandards}); err != nil {
		t.Fatalf("reviewer spawn: %v", err)
	}
	got := rec.withTool("index_repository")
	if len(got) == 0 {
		t.Fatal("reviewer spawn indexed nothing")
	}
	if got[0].args["repo_path"] != tree {
		t.Fatalf("reindex repo_path = %v, want %q", got[0].args["repo_path"], tree)
	}
}

// A scout spawn re-indexes the tree it runs in before launching.
func TestGraphReindexBeforeScoutSpawn(t *testing.T) {
	rec := stubGraphDial(t, nil)
	spawnTestState(t)
	g := newFakeGrid()
	t.Setenv("HERDR_TAB_ID", g.tabs[0].tab)
	t.Setenv("HERDR_PANE_ID", "wP")
	t.Setenv("EL_AGENT_ID", "parent1")
	ex := &scoutExec{g: g, writeOn: 1}
	spawnExec = ex.run
	oldDeliver := deliverExec
	recorder := &recorder{reply: func(string, []string) Result { return Result{Stdout: "{}"} }}
	deliverExec = recorder.run
	t.Cleanup(func() { deliverExec = oldDeliver })
	spawn, _ := spawnToolForRole(t, "implement")
	root := t.TempDir()
	if _, err := spawn(root, map[string]any{"agents": []any{map[string]any{"type": "scout", "prompt": "where?"}}}); err != nil {
		t.Fatalf("scout spawn: %v", err)
	}
	got := rec.withTool("index_repository")
	if len(got) == 0 {
		t.Fatal("scout spawn indexed nothing")
	}
	if got[0].args["repo_path"] != root {
		t.Fatalf("reindex repo_path = %v, want %q", got[0].args["repo_path"], root)
	}
}

// Finish deletes the worktree's project after dropping the worktree.
func TestGraphDeleteOnFinish(t *testing.T) {
	rec := stubGraphDial(t, nil)
	statePath = filepath.Join(t.TempDir(), "workers.json")
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	repo := graphGitRepo(t)
	ticket := filepath.Join(repo, "07-example.md")
	if err := writeGraphFile(ticket, "# Example\n\nBlocked by: \n"); err != nil {
		t.Fatal(err)
	}
	h := newTicketHerdr(osExec)
	worker, err := Spawn(h.run, SpawnInput{Ticket: ticket, Repo: repo, Base: "main", Harness: "stub", Launch: []string{"true"}})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if err := writeGraphFile(filepath.Join(worker.Worktree, "feature.txt"), "done\n"); err != nil {
		t.Fatal(err)
	}
	gitOut(t, worker.Worktree, "add", ".")
	gitOut(t, worker.Worktree, "commit", "-qm", "worker work")
	passReviews(t, worker.Worktree, gitOut(t, worker.Worktree, "rev-parse", "HEAD"))
	if _, err := Finish(h.run, worker.ID, true); err != nil {
		t.Fatalf("finish: %v", err)
	}
	got := waitGraphCall(t, rec, "delete_project")
	if got.args["project"] != tools.GraphProjectSlug(worker.Worktree) {
		t.Fatalf("delete project = %v, want the worktree slug", got.args["project"])
	}
}

// A failing graph never blocks the spawn it precedes or the finish it follows.
func TestGraphFailureNeverBlocks(t *testing.T) {
	stubGraphDial(t, func(tool string, args map[string]any) (string, error) {
		return "", errGraphBoom
	})
	_, tree := reviewRepo(t)
	f := newFakeReviewers(answers("pass", "pass"))
	if _, err := reviewSpawn(t, f, tree, map[string]any{"type": reviewerStandards}); err != nil {
		t.Fatalf("reviewer spawn must survive a graph failure: %v", err)
	}
	statePath = filepath.Join(t.TempDir(), "workers.json")
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	repo := graphGitRepo(t)
	ticket := filepath.Join(repo, "08-example.md")
	if err := writeGraphFile(ticket, "# Example\n\nBlocked by: \n"); err != nil {
		t.Fatal(err)
	}
	h := newTicketHerdr(osExec)
	worker, err := Spawn(h.run, SpawnInput{Ticket: ticket, Repo: repo, Base: "main", Harness: "stub", Launch: []string{"true"}})
	if err != nil {
		t.Fatalf("spawn must survive a graph failure: %v", err)
	}
	if err := writeGraphFile(filepath.Join(worker.Worktree, "feature.txt"), "done\n"); err != nil {
		t.Fatal(err)
	}
	gitOut(t, worker.Worktree, "add", ".")
	gitOut(t, worker.Worktree, "commit", "-qm", "worker work")
	passReviews(t, worker.Worktree, gitOut(t, worker.Worktree, "rev-parse", "HEAD"))
	if _, err := Finish(h.run, worker.ID, true); err != nil {
		t.Fatalf("finish must survive a graph failure: %v", err)
	}
}

// With no binary and no override every lifecycle call is a silent no-op.
func TestGraphUnavailableSkipsSilently(t *testing.T) {
	hermeticSettings(t)
	t.Setenv("EL_BIN_DIR", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	t.Setenv("EL_GRAPH_TEST_COMMAND", "")
	t.Setenv("EL_GRAPH_TEST_ARGS", "")
	useRealGraphDial(t)
	tree := filepath.Join(t.TempDir(), "tree")
	if _, err := graphIndex(context.Background(), tree); err != nil {
		t.Fatalf("unavailable index must skip silently: %v", err)
	}
	if err := graphDelete(context.Background(), tree); err != nil {
		t.Fatalf("unavailable delete must skip silently: %v", err)
	}
	graphReindexForSpawn(tree)
	graphDeleteForWorktree(tree)
	graphIndexWorktreeAsync(tree)
}

// errGraphBoom is the failing dial the never-blocks test answers with.
var errGraphBoom = errorString("graph boom")

type errorString string

func (e errorString) Error() string { return string(e) }

// graphGitRepo is a temp git repo with one commit, for Spawn fixtures.
func graphGitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	gitOut(t, repo, "init", "-q", "-b", "main")
	gitOut(t, repo, "config", "user.email", "el@test.local")
	gitOut(t, repo, "config", "user.name", "el")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitOut(t, repo, "add", ".")
	gitOut(t, repo, "commit", "-qm", "base")
	return repo
}

func writeGraphFile(path, body string) error {
	return os.WriteFile(path, []byte(body), 0o644)
}

func readGraphLog(log string) (string, error) {
	raw, err := os.ReadFile(log)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// TestLiveGraphBinaryRoundTrip drives the real graph binary end to end: index
// a temp tree, wait for ready, search it, delete the project. It needs the
// binary installed and never runs in the standard check (`-skip TestLive`).
func TestLiveGraphBinaryRoundTrip(t *testing.T) {
	tree := t.TempDir()
	if err := os.WriteFile(filepath.Join(tree, "main.go"), []byte("package main\n\nfunc hello() string { return \"hi\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cache := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tree, ".emberloop"), 0o755); err != nil {
		t.Fatal(err)
	}
	settingsBody := `{"graph":{"cache_dir":` + strconv.Quote(cache) + `}}`
	if err := os.WriteFile(filepath.Join(tree, ".emberloop", "settings.json"), []byte(settingsBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if tools.ResolveGraphBinary(tools.EffectiveSettings(tree)) == "" {
		t.Skip("graph binary not installed, skipping live graph probe")
	}
	useRealGraphDial(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	text, err := graphIndex(ctx, tree)
	if err != nil {
		t.Fatalf("live index: %v", err)
	}
	if !strings.Contains(text, tools.GraphProjectSlug(tree)) {
		t.Fatalf("live index answer names no project: %q", text)
	}
	project := tools.GraphProjectSlug(tree)
	deadline := time.Now().Add(2 * time.Minute)
	for {
		out, err := graphLifecycleDial(ctx, tree, "index_status", map[string]any{"project": project})
		if err == nil && (strings.Contains(out, `"ready"`) || strings.Contains(out, `"indexed"`)) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("live index never ready: out=%q err=%v", out, err)
		}
		time.Sleep(2 * time.Second)
	}
	hits, err := graphLifecycleDial(ctx, tree, "search_graph", map[string]any{"project": project, "query": "hello"})
	if err != nil {
		t.Fatalf("live search: %v", err)
	}
	if !strings.Contains(hits, "hello") {
		t.Fatalf("live search missed the fixture: %q", hits)
	}
	if err := graphDelete(ctx, tree); err != nil {
		t.Fatalf("live delete: %v", err)
	}
}

// Co-launched reviewers of one tree share a single re-index.
func TestGraphReindexOncePerSpawnCall(t *testing.T) {
	rec := stubGraphDial(t, nil)
	_, tree := reviewRepo(t)
	f := newFakeReviewers(answers("pass", "pass"))
	if _, err := reviewSpawn(t, f, tree, bothReviewers()...); err != nil {
		t.Fatalf("reviewer spawn: %v", err)
	}
	// Count this tree only: a worker test's background index goroutine can
	// outlive its test and land on this test's stub.
	var got []graphCallRec
	for _, c := range rec.withTool("index_repository") {
		if c.worktree == graphAbs(tree) {
			got = append(got, c)
		}
	}
	if len(got) != 1 {
		t.Fatalf("one spawn call indexed %d times, want once: %v", len(got), rec.withTool("index_repository"))
	}
}

// Delete falls back to the list_projects entry when the server does not know
// the slug, covering servers that indexed under a path-derived name.
func TestGraphDeleteFallsBackToListedProject(t *testing.T) {
	tree := t.TempDir()
	slug := tools.GraphProjectSlug(tree)
	projects := `{"projects":[{"name":"real-proj","root_path":` + strconv.Quote(tree) + `}]}`
	var mu sync.Mutex
	var deleted []string
	stubGraphDial(t, func(tool string, args map[string]any) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		switch tool {
		case "delete_project":
			name, _ := args["project"].(string)
			deleted = append(deleted, name)
			if name == slug {
				return "", errorString("project not_found or not indexed")
			}
			return `{"status":"deleted"}`, nil
		case "index_status":
			return "", errorString("project not_found or not indexed")
		case "list_projects":
			return projects, nil
		}
		return `{}`, nil
	})
	if err := graphDelete(context.Background(), tree); err != nil {
		t.Fatalf("delete with fallback: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(deleted) != 2 || deleted[0] != slug || deleted[1] != "real-proj" {
		t.Fatalf("deleted = %v, want [slug real-proj]", deleted)
	}
}

// The served snapshot gates the worktree's own project: the status check must
// carry the slug of the root, never a slug of a slug.
func TestGraphSnapshotGatesForWorktreeProject(t *testing.T) {
	s, _ := graphFakeSettings(t, mcpfake.Config{Tools: []mcpfake.ToolDef{
		{Name: "search_graph"}, {Name: "index_status"},
	}, Calls: map[string]mcpfake.Result{
		"index_status": {Text: `{"status":"ready"}`},
		"search_graph": {Text: "hits"},
	}})
	reg := tools.NewRegistry(s.MCPServers)
	t.Cleanup(reg.Close)
	listed := MCPSnapshot(reg)
	tree := t.TempDir()
	want := tools.GraphProjectSlug(tree)
	var mu sync.Mutex
	var statusProjects []string
	call := func(ctx context.Context, tool string, args json.RawMessage) (string, error) {
		if tool == "index_status" {
			var body struct {
				Project string `json:"project"`
			}
			if err := json.Unmarshal(args, &body); err == nil {
				mu.Lock()
				statusProjects = append(statusProjects, body.Project)
				mu.Unlock()
			}
		}
		return reg.Call(ctx, tools.GraphServerName, tool, args)
	}
	gated := GateListedGraph(listed, call, tree)
	text, err := gated[tools.GraphServerName][0].Execute(context.Background(), tree, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if text != "hits" {
		t.Fatalf("gated call = %q, want hits", text)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(statusProjects) == 0 {
		t.Fatal("gate checked no index status")
	}
	for _, got := range statusProjects {
		if got != want {
			t.Fatalf("status project = %q, want worktree slug %q", got, want)
		}
	}
}
