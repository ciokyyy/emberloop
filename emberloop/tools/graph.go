// Code graph integration (ticket 20): codebase-memory-mcp as the `graph` MCP
// server, with automatic per-worktree indexing driven by the harness.
//
// The binary is optional: graph.enabled=false in settings or a missing binary
// synthesizes nothing and breaks nothing. El finds it in the el bin
// directory first, then on PATH, or at graph.binary in settings, through the
// shared ResolveBinary order the installer, grep and the sandbox share. Its
// own `install` command never runs and no agent client config is touched.
//
// The server registers through the settings and client machinery as an
// mcp_servers entry named `graph`: its include list is exactly the exposed
// set below, its exclude the management tools plus detect_changes (which fails
// inside git worktrees), and its per-role placement puts it in main for
// review and scout and behind execute for every other role. An explicit user
// mcp_servers.graph entry wins over the synthesis.
//
// Lifecycle calls (index_repository, index_status, delete_project) go through
// the same client; the harness in the parent package drives them invisibly to
// agents. EL_GRAPH_TEST_COMMAND (with EL_GRAPH_TEST_ARGS as a JSON argv
// list) overrides the synthesized command for tests, so the fake MCP server
// stands in for the binary.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// GraphServerName is the mcp_servers entry the code graph registers as.
const GraphServerName = "graph"

// GraphBinaryName is the code graph binary the installer lays down and this
// package resolves. The harness installer names the same binary; both must
// stay one string.
const GraphBinaryName = "codebase-memory-mcp"

// GraphExposedTools is the closed set agents may call. The binary may list
// more (query_graph, search_code, manage_adr and others in some versions);
// the include list keeps those from every agent. get_file_outline is absent
// from older binaries (0.10.x lists no such tool): it simply serves nothing
// there and appears with newer ones.
var GraphExposedTools = []string{
	"search_graph", "trace_path", "get_code_snippet", "get_file_outline", "get_architecture",
}

// GraphHiddenTools are never exposed: the index management tools plus
// detect_changes, which fails inside git worktrees.
var GraphHiddenTools = []string{
	"detect_changes",
	"index_repository", "list_projects", "delete_project", "index_status", "check_index_coverage",
}

// DefaultGraphGateWait bounds the first-query wait on the index status: long
// enough for the asynchronous worktree index to land, short enough that a
// missing graph never stalls a session.
const DefaultGraphGateWait = 15 * time.Second

// graphGatePoll is the pause between index_status checks while waiting.
const graphGatePoll = 500 * time.Millisecond

// GraphProjectSlug names the graph project for a worktree: the absolute path
// with separators folded to dashes, plus a short hash of the full path, so two
// worktrees of one repo never collide with each other or the main checkout.
// The projection mirrors the binary's own path slugs, with the hash closing
// the /a/b versus /a-b collision its style leaves open.
func GraphProjectSlug(worktree string) string {
	abs, err := filepath.Abs(worktree)
	if err != nil {
		abs = worktree
	}
	clean := filepath.Clean(abs)
	trimmed := strings.TrimLeft(clean, `/\`)
	var b strings.Builder
	dash := false
	for _, r := range trimmed {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
			dash = false
			continue
		}
		if !dash {
			b.WriteRune('-')
			dash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if len(slug) > 64 {
		slug = slug[:64]
		slug = strings.TrimRight(slug, "-")
	}
	if slug == "" {
		slug = "project"
	}
	sum := fnv.New32a()
	_, _ = sum.Write([]byte(clean))
	return fmt.Sprintf("%s-%08x", slug, sum.Sum32())
}

// ResolveGraphBinary finds the graph binary: graph.binary in settings first
// (an executable path or anything on PATH), then the el bin directory,
// then PATH. Empty means absent, never an error.
func ResolveGraphBinary(s Settings) string {
	if s.Graph.Binary != "" {
		if isExecutable(s.Graph.Binary) {
			return s.Graph.Binary
		}
		if p, err := exec.LookPath(s.Graph.Binary); err == nil {
			return p
		}
	}
	if p, err := ResolveBinary(GraphBinaryName); err == nil {
		return p
	}
	return ""
}

// graphTestCommand is the test seam: EL_GRAPH_TEST_COMMAND names the fake
// server command outright, with EL_GRAPH_TEST_ARGS carrying its argv as a
// JSON list. The fake's mode config travels in the process environment, which
// the client passes to the child.
func graphTestCommand() (string, []string, bool) {
	cmd := os.Getenv("EL_GRAPH_TEST_COMMAND")
	if cmd == "" {
		return "", nil, false
	}
	var args []string
	if raw := strings.TrimSpace(os.Getenv("EL_GRAPH_TEST_ARGS")); raw != "" {
		_ = json.Unmarshal([]byte(raw), &args)
	}
	return cmd, args, true
}

// GraphServerSetting synthesizes the graph mcp_servers entry: the resolved
// binary (or the test override) as command, CBM_CACHE_DIR from
// graph.cache_dir, the exposed set as include, the hidden set as exclude, and
// per-role placement putting review and scout in main. False when the graph is
// disabled or the binary is absent.
func GraphServerSetting(s Settings) (MCPServerSetting, bool) {
	if !s.GraphEnabled() {
		return MCPServerSetting{}, false
	}
	cfg := MCPServerSetting{
		Summary:   "Code graph: structural search over the indexed worktree.",
		Placement: PlacementExecute,
		Include:   append([]string(nil), GraphExposedTools...),
		Exclude:   append([]string(nil), GraphHiddenTools...),
		Roles: map[string]MCPServerRoleOverride{
			"review": {Placement: PlacementMain},
			"scout":  {Placement: PlacementMain},
		},
	}
	if s.Graph.CacheDir != "" {
		cfg.Env = map[string]string{"CBM_CACHE_DIR": s.Graph.CacheDir}
	}
	if cmd, args, ok := graphTestCommand(); ok {
		cfg.Command, cfg.Args = cmd, args
		return cfg, true
	}
	bin := ResolveGraphBinary(s)
	if bin == "" {
		return MCPServerSetting{}, false
	}
	cfg.Command = bin
	return cfg, true
}

// WithGraphServer returns settings with the synthesized graph entry added.
// Disabled, a missing binary, or an explicit user graph entry leave the
// settings untouched. The returned maps are fresh, so callers never mutate
// the input through the output.
func WithGraphServer(s Settings) Settings {
	cfg, ok := GraphServerSetting(s)
	if !ok {
		return s
	}
	if s.MCPServers != nil {
		if _, exists := s.MCPServers[GraphServerName]; exists {
			return s
		}
	}
	out := s
	out.MCPServers = map[string]MCPServerSetting{}
	for name, e := range s.MCPServers {
		out.MCPServers[name] = cloneMCPServerSetting(e)
	}
	out.MCPServers[GraphServerName] = cfg
	return out
}

// Call runs one tool on one configured server, starting it on first use. It
// is the harness's handle for lifecycle calls (index, status, delete) that
// agents never see.
func (r *Registry) Call(ctx context.Context, server, tool string, args json.RawMessage) (string, error) {
	r.mu.Lock()
	c, ok := r.clients[server]
	h, hok := r.httpClients[server]
	bad := r.bad[server]
	r.mu.Unlock()
	switch {
	case bad != nil:
		return "", bad
	case ok:
		return c.CallTool(ctx, tool, args)
	case hok:
		return h.CallTool(ctx, tool, args)
	default:
		return "", fmt.Errorf("unknown mcp server %q", server)
	}
}

// GraphStatusCall runs one graph-server tool by name; the gate and the
// resolver bind it to the graph server.
type GraphStatusCall func(ctx context.Context, tool string, args json.RawMessage) (string, error)

// graphReadyRe matches a wrapped status payload naming a usable index. It
// matches the status field only, so "not_ready" or "not_indexed" never count.
var graphReadyRe = regexp.MustCompile(`"status"\s*:\s*"(ready|indexed)"`)

// ResolveGraphProject names the usable project for worktree. The path slug
// answers when its index status is ready or indexed; otherwise list_projects
// is matched by root path, covering servers that silently ignore the `name`
// override of index_repository (upstream issue #1317: a repo outside the
// server's allowed root is indexed under a path-derived name). Anything
// unrecognized falls back to the slug, so resolution never fails a query.
func ResolveGraphProject(ctx context.Context, call GraphStatusCall, worktree string) string {
	slug := GraphProjectSlug(graphAbsOf(worktree))
	if call == nil {
		return slug
	}
	if graphReady(ctx, call, slug) {
		return slug
	}
	if name, ok := graphProjectForRoot(ctx, call, graphAbsOf(worktree)); ok {
		return name
	}
	return slug
}

// graphAbsOf resolves worktree to an absolute path for server arguments.
func graphAbsOf(worktree string) string {
	if abs, err := filepath.Abs(worktree); err == nil {
		return abs
	}
	return worktree
}

// graphProjectForRoot finds the indexed project whose root path is worktree.
func graphProjectForRoot(ctx context.Context, call GraphStatusCall, abs string) (string, bool) {
	ctx, cancel := context.WithTimeout(ctx, graphGatePoll*4)
	defer cancel()
	text, err := call(ctx, "list_projects", json.RawMessage(`{}`))
	if err != nil {
		return "", false
	}
	var body struct {
		Projects []struct {
			Name     string `json:"name"`
			RootPath string `json:"root_path"`
		} `json:"projects"`
	}
	if err := json.Unmarshal([]byte(text), &body); err != nil {
		return "", false
	}
	for _, p := range body.Projects {
		if p.Name != "" && filepath.Clean(p.RootPath) == abs {
			return p.Name, true
		}
	}
	return "", false
}

// GateGraphTools wraps listed graph tools so the first query of the snapshot
// waits on the index status for up to wait instead of answering from a
// missing graph. One wait is shared by every tool of the snapshot; later
// queries pass straight through, and an index that never reports ready still
// answers when the bound lapses. Hidden tools (index_status and the rest)
// never reach this wrapper: only the exposed listing is gated.
func GateGraphTools(listed []Tool, call GraphStatusCall, worktree string, wait time.Duration) []Tool {
	shared := &graphGateShared{call: call, worktree: worktree, wait: wait}
	out := make([]Tool, 0, len(listed))
	for _, t := range listed {
		out = append(out, &gatedGraphTool{Tool: t, shared: shared})
	}
	return out
}

// graphGateShared holds the one wait every gated tool of a snapshot shares.
type graphGateShared struct {
	call     GraphStatusCall
	worktree string
	wait     time.Duration
	once     sync.Once
}

// gatedGraphTool serves one graph tool behind the index-status wait. It
// embeds Tool, so renames, server markers and read-only hints survive; only
// Execute is overridden.
type gatedGraphTool struct {
	Tool
	shared *graphGateShared
}

// Execute waits for the index once per snapshot, then answers through the
// inner tool. The wait never fails the query: it only delays it, bounded.
func (g *gatedGraphTool) Execute(ctx context.Context, root string, args json.RawMessage) (string, error) {
	g.shared.once.Do(func() {
		project := ResolveGraphProject(ctx, g.shared.call, g.shared.worktree)
		waitGraphReady(ctx, g.shared.call, project, g.shared.wait)
	})
	return g.Tool.Execute(ctx, root, args)
}

// waitGraphReady polls index_status until the project reports ready or indexed
// or the bound lapses. Unknown shapes, errors and a deadlined context all end
// the wait: the query that follows answers from whatever the graph holds.
func waitGraphReady(ctx context.Context, call GraphStatusCall, project string, wait time.Duration) {
	if wait <= 0 || call == nil || project == "" {
		return
	}
	deadline := time.Now().Add(wait)
	for {
		if graphReady(ctx, call, project) {
			return
		}
		if time.Now().After(deadline) {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(graphGatePoll):
		}
	}
}

// graphReady reports whether one index_status answer names a usable index.
func graphReady(ctx context.Context, call GraphStatusCall, project string) bool {
	raw, err := json.Marshal(map[string]string{"project": project})
	if err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, graphGatePoll*4)
	defer cancel()
	text, err := call(ctx, "index_status", raw)
	if err != nil {
		return false
	}
	return GraphStatusReady(text)
}

// GraphStatusReady reports whether an index_status answer names a usable
// index: a ready or indexed status, parsed or matched as a wrapped payload.
func GraphStatusReady(text string) bool {
	var body struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(text), &body); err != nil {
		// Some servers wrap the payload in MCP content JSON; match the
		// status field only as a last resort.
		return graphReadyRe.MatchString(text)
	}
	return body.Status == "ready" || body.Status == "indexed"
}
