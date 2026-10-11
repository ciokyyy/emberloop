package web

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// rebaseSpecBranch rebases a spec branch in an isolated detached worktree.
// The ref moves only after the rebase succeeds and only if it still points
// at the captured tip, so a concurrent Ticket merge cannot be overwritten.
// A conflict is returned as a sorted file list; the original branch remains
// untouched and the temporary worktree is removed.
func rebaseSpecBranch(repo, branch string) (master string, conflicts []string, err error) {
	master, err = masterTip(repo)
	if err != nil {
		return "", nil, err
	}
	oldTip, err := runGit(repo, "rev-parse", branch)
	if err != nil || oldTip == "" {
		return "", nil, fmt.Errorf("could not read spec branch %s: %v", branch, err)
	}
	root := filepath.Join(repo, ".el-worktrees")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", nil, err
	}
	if err := ensureWorktreeConfigExtension(repo); err != nil {
		return "", nil, fmt.Errorf("could not enable worktreeConfig: %w", err)
	}
	worktree, err := os.MkdirTemp(root, ".spec-rebase-")
	if err != nil {
		return "", nil, err
	}
	if err := os.Remove(worktree); err != nil {
		return "", nil, err
	}
	cleanup := func() error {
		_, removeErr := runGit(repo, "worktree", "remove", "--force", worktree)
		if removeErr != nil {
			return removeErr
		}
		return os.RemoveAll(worktree)
	}
	if _, err := runGit(repo, "worktree", "add", "--detach", worktree, oldTip); err != nil {
		_ = os.RemoveAll(worktree)
		return "", nil, err
	}
	if err := ensureWorktreeGitIdentity(worktree); err != nil {
		_ = cleanup()
		return "", nil, err
	}
	if _, rebaseErr := runGit(worktree, "rebase", master); rebaseErr != nil {
		if out, listErr := runGit(worktree, "diff", "--name-only", "--diff-filter=U"); listErr == nil {
			for _, path := range strings.Split(out, "\n") {
				if path = strings.TrimSpace(path); path != "" {
					conflicts = append(conflicts, path)
				}
			}
		}
		sort.Strings(conflicts)
		_, _ = runGit(worktree, "rebase", "--abort")
		if cleanErr := cleanup(); cleanErr != nil {
			return "", conflicts, fmt.Errorf("rebase conflicted and temporary worktree cleanup failed: %w", cleanErr)
		}
		if len(conflicts) == 0 {
			return "", nil, rebaseErr
		}
		return master, conflicts, nil
	}
	newTip, err := runGit(worktree, "rev-parse", "HEAD")
	if err != nil || newTip == "" {
		_ = cleanup()
		return "", nil, fmt.Errorf("could not read rebased spec tip: %v", err)
	}
	if err := cleanup(); err != nil {
		return "", nil, fmt.Errorf("could not remove temporary rebase worktree: %w", err)
	}
	if _, err := runGit(repo, "update-ref", "refs/heads/"+branch, newTip, oldTip); err != nil {
		return "", nil, fmt.Errorf("spec branch moved during rebase; retry: %s", strings.TrimSpace(err.Error()))
	}
	return master, nil, nil
}

// rebaseRun applies the explicit checkpoint rebase action. A content conflict
// leaves the spec ref unchanged and publishes a needs-you Run snapshot with
// the conflicting paths.
func (d *Daemon) rebaseRun(w http.ResponseWriter, r *http.Request, id string) {
	d.mu.Lock()
	rs, ok := d.runs[id]
	if !ok {
		d.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	if rs.Status == runStatusAbandoned {
		d.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]any{"error": "run is abandoned"})
		return
	}
	if rs.SpecBranch == "" {
		d.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]any{"error": "run has no spec branch"})
		return
	}
	repo, branch := rs.Repo, rs.SpecBranch
	d.mu.Unlock()

	master, conflicts, err := rebaseSpecBranch(repo, branch)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error()})
		return
	}

	d.mu.Lock()
	rs, ok = d.runs[id]
	if !ok {
		d.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	if rs.Status == runStatusAbandoned {
		d.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]any{"error": "run was abandoned during rebase"})
		return
	}
	if len(conflicts) != 0 {
		rs.Status = runStatusNeedsYou
		rs.Error = "rebase onto master conflicted in " + strings.Join(conflicts, ", ") + "; resolve the conflicts and rebase again"
	} else {
		rs.SpecBase = master
		if strings.HasPrefix(rs.Error, "rebase onto master conflicted") {
			rs.Error = ""
		}
	}
	d.persistRun(rs)
	out := d.runJSONLocked(rs)
	d.broadcastRunLocked(rs)
	d.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"run": out})
}

// abandonRun ends the Run's agent Sessions and removes every recorded Ticket
// worktree/branch and the spec branch. The Run row remains as an abandoned
// audit record; repeating the action retries any cleanup that previously
// failed.
func (d *Daemon) abandonRun(w http.ResponseWriter, r *http.Request, id string) {
	d.mu.Lock()
	rs, ok := d.runs[id]
	if !ok {
		d.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	if rs.Discarded == nil {
		rs.Discarded = map[string]bool{}
	}
	for _, ticket := range rs.Tickets {
		rs.Discarded[ticket.ID] = true
	}
	worktrees := make([]string, 0, len(rs.Worktrees))
	branches := make([]string, 0, len(rs.Branches))
	for _, path := range rs.Worktrees {
		if path != "" {
			worktrees = append(worktrees, path)
		}
	}
	for _, branch := range rs.Branches {
		if branch != "" {
			branches = append(branches, branch)
		}
	}
	specBranch, repo := rs.SpecBranch, rs.Repo
	// Close every Run-owned Session, including Reviewer sessions and
	// worker-spawned Subagents, before removing a worktree they may use.
	owned := map[string]bool{}
	for _, sessionID := range rs.Sessions {
		if sessionID != "" {
			owned[sessionID] = true
		}
	}
	for _, reviewers := range rs.Reviewers {
		for _, sessionID := range reviewers {
			if sessionID != "" {
				owned[sessionID] = true
			}
		}
	}
	for changed := true; changed; {
		changed = false
		for sessionID, session := range d.sessions {
			if owned[session.meta.ParentID] && !owned[sessionID] {
				owned[sessionID] = true
				changed = true
			}
		}
	}
	sessions := make([]string, 0, len(owned))
	for sessionID := range owned {
		sessions = append(sessions, sessionID)
	}
	sort.Strings(sessions)
	rs.Status = runStatusAbandoned
	rs.Error = ""
	if rs.unpause != nil {
		select {
		case <-rs.unpause:
		default:
			close(rs.unpause)
		}
	}
	d.persistRun(rs)
	d.broadcastRunLocked(rs)
	d.mu.Unlock()

	for _, sessionID := range sessions {
		_, _ = d.endSession(sessionID)
	}

	var cleanupErrors []string
	for _, worktree := range worktrees {
		if !runWorktreePath(repo, worktree) {
			cleanupErrors = append(cleanupErrors, "refused worktree outside .el-worktrees: "+worktree)
			continue
		}
		if _, err := os.Stat(worktree); os.IsNotExist(err) {
			continue
		}
		if _, err := runGit(repo, "worktree", "remove", "--force", worktree); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Sprintf("remove worktree %s: %s", worktree, strings.TrimSpace(err.Error())))
		}
	}
	if _, err := runGit(repo, "worktree", "prune"); err != nil {
		cleanupErrors = append(cleanupErrors, "prune stale worktrees: "+strings.TrimSpace(err.Error()))
	}
	branches = append(branches, specBranch)
	seen := map[string]bool{}
	for _, branch := range branches {
		if branch == "" || seen[branch] {
			continue
		}
		seen[branch] = true
		if exists, err := specBranchExists(repo, branch); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Sprintf("inspect branch %s: %s", branch, err))
		} else if exists {
			if _, err := runGit(repo, "branch", "-D", branch); err != nil {
				cleanupErrors = append(cleanupErrors, fmt.Sprintf("remove branch %s: %s", branch, strings.TrimSpace(err.Error())))
			}
		}
	}

	d.mu.Lock()
	rs, ok = d.runs[id]
	if !ok {
		d.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	if len(cleanupErrors) != 0 {
		rs.Error = "abandon cleanup incomplete: " + strings.Join(cleanupErrors, "; ")
	}
	d.persistRun(rs)
	out := d.runJSONLocked(rs)
	d.broadcastRunLocked(rs)
	d.mu.Unlock()
	if len(cleanupErrors) != 0 {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": out["error"], "run": out})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": out})
}

func runWorktreePath(repo, worktree string) bool {
	root, err := filepath.Abs(filepath.Join(repo, ".el-worktrees"))
	if err != nil {
		return false
	}
	path, err := filepath.Abs(worktree)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
