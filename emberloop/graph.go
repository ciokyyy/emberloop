package el

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"path/filepath"
	"strings"
	"time"

	"example.com/emberloop/tools"
)

// Code graph lifecycle (ticket 20): el indexes each worktree automatically
// and removes its project when the worktree goes, invisibly to agents.
//
// A worker's worktree is indexed asynchronously once Spawn creates it; just
// before each reviewer or scout spawn the tree is re-indexed incrementally so
// the graph matches the tree under review (the file watcher is not relied
// on); finish and every other path that drops a worktree deletes its project.
// The project name is the path slug of the worktree, so worktrees never
// collide with each other or the main checkout.
//
// Every call is best-effort: indexing and deletion failures are logged and
// never block spawn or finish. With the graph disabled or the binary absent
// every hook is a silent no-op. The binary's `install` command never runs and
// no agent client config is touched.
//
// Test seam: graphLifecycleDial performs one lifecycle tool call. Production
// dials the graph server; the package tests run with it nil (set in TestMain,
// so the many Spawn tests never spawn a server) and stub or restore it per
// graph test.
var graphLifecycleDial func(ctx context.Context, worktree, tool string, args map[string]any) (string, error) = graphDialReal

// errGraphUnavailable skips a lifecycle call silently: the graph is disabled
// or its binary is absent, so there is nothing to index into.
var errGraphUnavailable = errors.New("el: code graph unavailable")

// Lifecycle bounds: the creation index runs in the background, the pre-spawn
// re-index paces a reviewer launch, and the deletion is one fast call.
const (
	graphIndexTimeout   = 5 * time.Minute
	graphReindexTimeout = 2 * time.Minute
	graphDeleteTimeout  = 30 * time.Second
)

// graphDialReal runs one graph-server tool against the synthesized server for
// worktree's settings. An explicit user graph entry dials that instead.
func graphDialReal(ctx context.Context, worktree, tool string, args map[string]any) (string, error) {
	s := tools.WithGraphServer(tools.EffectiveSettings(worktree))
	cfg, ok := s.MCPServers[tools.GraphServerName]
	if !ok {
		return "", errGraphUnavailable
	}
	reg := tools.NewRegistry(map[string]tools.MCPServerSetting{tools.GraphServerName: cfg})
	defer reg.Close()
	raw, err := json.Marshal(args)
	if err != nil {
		return "", err
	}
	return reg.Call(ctx, tools.GraphServerName, tool, raw)
}

// graphCall runs one lifecycle call, skipping silently when the graph is
// unwired (tests) or unavailable (disabled or missing binary).
func graphCall(ctx context.Context, worktree, tool string, args map[string]any) (string, error) {
	if graphLifecycleDial == nil {
		return "", errGraphUnavailable
	}
	text, err := graphLifecycleDial(ctx, worktree, tool, args)
	if err != nil {
		return "", err
	}
	return text, nil
}

// graphAbs resolves worktree to an absolute path for server arguments. Index
// arguments must be absolute: the server process may run anywhere.
func graphAbs(worktree string) string {
	if abs, err := filepath.Abs(worktree); err == nil {
		return abs
	}
	return worktree
}

// graphIndex indexes worktree's project, returning the server's answer. An
// unavailable graph is a silent skip, never an error. When the answer names a
// different project than requested, the mismatch is logged: the gate and the
// delete resolve the usable name through index_status and list_projects.
func graphIndex(ctx context.Context, worktree string) (string, error) {
	abs := graphAbs(worktree)
	slug := tools.GraphProjectSlug(abs)
	text, err := graphCall(ctx, abs, "index_repository", map[string]any{
		"repo_path": abs,
		"name":      slug,
	})
	if errors.Is(err, errGraphUnavailable) {
		return "", nil
	}
	if err == nil {
		var body struct {
			Project string `json:"project"`
		}
		if json.Unmarshal([]byte(text), &body) == nil && body.Project != "" && body.Project != slug {
			log.Printf("el: code graph indexed %s as %q, not %q", abs, body.Project, slug)
		}
	}
	return text, err
}

// graphReindexOnce re-indexes the tree for one spawn, at most once per root:
// co-launched reviewers share one re-index, and a held pass that launches
// nothing indexes nothing.
func graphReindexOnce(worktree string, done map[string]bool) {
	if done != nil {
		if done[worktree] {
			return
		}
		done[worktree] = true
	}
	graphReindexForSpawn(worktree)
}

// graphDelete deletes worktree's project, returning nil when the graph is
// unavailable or the project was never indexed. A slug the server does not
// know falls back to the list_projects entry for the worktree root, covering
// servers that indexed under a path-derived name.
func graphDelete(ctx context.Context, worktree string) error {
	abs := graphAbs(worktree)
	slug := tools.GraphProjectSlug(abs)
	_, err := graphCall(ctx, abs, "delete_project", map[string]any{
		"project": slug,
	})
	if errors.Is(err, errGraphUnavailable) {
		return nil
	}
	if err != nil && strings.Contains(err.Error(), "not_found") {
		if resolved := graphResolveProjectName(ctx, abs, slug); resolved != slug {
			return graphDeleteProject(ctx, abs, resolved)
		}
		return nil
	}
	return err
}

// graphDeleteProject deletes one named project, swallowing unknown names.
func graphDeleteProject(ctx context.Context, abs, project string) error {
	_, err := graphCall(ctx, abs, "delete_project", map[string]any{
		"project": project,
	})
	if errors.Is(err, errGraphUnavailable) {
		return nil
	}
	if err != nil && strings.Contains(err.Error(), "not_found") {
		return nil
	}
	return err
}

// graphResolveProjectName names the usable project for a worktree: the slug
// when its status is ready, else the list_projects entry whose root path
// matches, else the slug. It never errors; unrecognized answers keep the slug.
func graphResolveProjectName(ctx context.Context, abs, slug string) string {
	if graphLifecycleDial == nil {
		return slug
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if text, err := graphCall(ctx, abs, "index_status", map[string]any{"project": slug}); err == nil && tools.GraphStatusReady(text) {
		return slug
	}
	if text, err := graphCall(ctx, abs, "list_projects", map[string]any{}); err == nil {
		var body struct {
			Projects []struct {
				Name     string `json:"name"`
				RootPath string `json:"root_path"`
			} `json:"projects"`
		}
		if err := json.Unmarshal([]byte(text), &body); err == nil {
			for _, p := range body.Projects {
				if p.Name != "" && filepath.Clean(p.RootPath) == abs {
					return p.Name
				}
			}
		}
	}
	return slug
}

// graphIndexWorktreeAsync indexes the worktree in the background once Spawn
// has created it. Failures are logged and forgotten. The host is always the
// session's el-mcp process (the spawn tool launches through it
// run inside it), which outlives the index, so the goroutine is never cut
// short by an exiting CLI.
func graphIndexWorktreeAsync(worktree string) {
	if graphLifecycleDial == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), graphIndexTimeout)
		defer cancel()
		if _, err := graphIndex(ctx, worktree); err != nil {
			log.Printf("el: code graph index of %s failed: %v", worktree, err)
		}
	}()
}

// graphReindexForSpawn re-indexes the tree incrementally just before a
// reviewer or scout spawns, so the graph matches the tree under review. It
// runs synchronously in the spawn and never fails it.
func graphReindexForSpawn(worktree string) {
	if graphLifecycleDial == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), graphReindexTimeout)
	defer cancel()
	if _, err := graphIndex(ctx, worktree); err != nil {
		log.Printf("el: code graph re-index of %s failed: %v", worktree, err)
	}
}

// graphDeleteForWorktree deletes the worktree's project after the worktree is
// dropped. It never fails its caller.
func graphDeleteForWorktree(worktree string) {
	if graphLifecycleDial == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), graphDeleteTimeout)
	defer cancel()
	if err := graphDelete(ctx, worktree); err != nil {
		log.Printf("el: code graph delete of %s failed: %v", worktree, err)
	}
}
