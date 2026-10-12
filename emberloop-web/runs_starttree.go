package web

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// This file makes ticket worktree creation robust against leftovers from
// earlier Runs. A previous Run died after creating implement/<ticket> (or
// its .el-worktrees/<ticket> directory) but before removing it, so the next
// Run's `git worktree add -b` failed with "a branch named ... already
// exists" and the whole Run failed at its first ticket. The reconcile below
// reuses or cleans what is safe and reports the rest as a ticket Needs-you
// with a remedy, instead of failing the Run.

// listRegisteredWorktrees maps each registered worktree path to the branch
// checked out there ("" when detached), parsed from
// `git worktree list --porcelain`.
func listRegisteredWorktrees(repo string) (map[string]string, error) {
	out, err := runGit(repo, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	paths := map[string]string{}
	var cur string
	branch := ""
	flush := func() {
		if cur != "" {
			if _, ok := paths[cur]; !ok {
				paths[cur] = branch
			}
			cur, branch = "", ""
		}
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			cur = strings.TrimSpace(strings.TrimPrefix(line, "worktree "))
		case strings.HasPrefix(line, "branch "):
			branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		case line == "":
			flush()
		}
	}
	flush()
	return paths, nil
}

// worktreePathDirty reports whether the checkout at dir holds uncommitted
// changes.
func worktreePathDirty(dir string) bool {
	out, err := runGit(dir, "status", "--porcelain")
	return err == nil && strings.TrimSpace(out) != ""
}

// branchMergedInto reports whether branch is an ancestor of base. A false
// covers both a branch with unique commits and an unresolvable comparison:
// neither is safe to delete, so both read as unmerged.
func branchMergedInto(repo, branch, base string) bool {
	cmd := exec.Command("git", "merge-base", "--is-ancestor", branch, base)
	cmd.Dir = repo
	return cmd.Run() == nil
}

// reconcileTicketWorktree prepares a fresh `git worktree add -b branch
// worktree base` after an earlier Run may have left its branch or directory
// behind. It returns a blocked reason when the leftovers hold work only a
// human can resolve (the caller surfaces it as Needs-you); any other error
// fails the start as before.
func reconcileTicketWorktree(repo, branch, worktree, base string) (blocked string, err error) {
	if st, statErr := os.Stat(worktree); statErr == nil && st.IsDir() {
		paths, err := listRegisteredWorktrees(repo)
		if err != nil {
			return "", err
		}
		if _, registered := paths[worktree]; registered {
			// A clean registered worktree from a dead Run is safe to
			// drop; the branch check below decides the branch. A dirty
			// one may hold a worker's uncommitted work, so it stays.
			if worktreePathDirty(worktree) {
				return fmt.Sprintf("existing worktree %s holds uncommitted changes; commit or remove them, then retry the ticket", worktree), nil
			}
			if _, err := runGit(repo, "worktree", "remove", "--force", worktree); err != nil {
				return "", err
			}
		} else {
			// Present on disk but unknown to git: stale output of a
			// crashed Run, never a live checkout. It always lives
			// under the repo's own .el-worktrees directory; refuse
			// anything else rather than deleting blindly.
			root := filepath.Join(repo, ".el-worktrees")
			rel, relErr := filepath.Rel(root, worktree)
			if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return "", fmt.Errorf("worktree path %s is outside %s", worktree, root)
			}
			if err := os.RemoveAll(worktree); err != nil {
				return "", err
			}
		}
	}
	exists, err := specBranchExists(repo, branch)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", nil
	}
	// The branch survived an earlier Run. When another worktree still has
	// it checked out, only its owner can resolve that.
	if paths, err := listRegisteredWorktrees(repo); err != nil {
		return "", err
	} else {
		for path, br := range paths {
			if br == branch {
				return fmt.Sprintf("branch %s is still checked out at %s; remove that worktree, then retry the ticket", branch, path), nil
			}
		}
	}
	// Merged (or empty of unique commits) means no Run work would be
	// lost: drop it and recreate from the base below. Anything else keeps
	// commits some Run made, so it waits for a human.
	if !branchMergedInto(repo, branch, base) {
		return fmt.Sprintf("branch %s holds unmerged work from an earlier Run; delete it (git branch -D %s) after saving what matters, or finish it, then retry the ticket", branch, branch), nil
	}
	if _, err := runGit(repo, "branch", "-D", branch); err != nil {
		return "", err
	}
	return "", nil
}

// startBlockedReason builds the ticket Needs-you text for a worktree the
// start could not safely reuse. It names the remedy so the banner is
// actionable on its own.
func startBlockedReason(ticketID, blocked string) string {
	return fmt.Sprintf("ticket %s needs you: could not start a fresh worktree: %s", ticketID, blocked)
}

// markStartBlockedNeedsYou records a worktree block on the ticket and
// publishes the usual Run notification. The driver treats the blocked
// ticket as parked (no start, no finish, slot freed) until a retry clears
// it. Callers hold no lock.
func (d *Daemon) markStartBlockedNeedsYou(runID, ticketID, reason string) {
	d.mu.Lock()
	rs := d.runs[runID]
	if rs == nil {
		d.mu.Unlock()
		return
	}
	if rs.StartBlocked == nil {
		rs.StartBlocked = map[string]string{}
	}
	rs.StartBlocked[ticketID] = reason
	if rs.NeedsYouReason == nil {
		rs.NeedsYouReason = map[string]string{}
	}
	rs.NeedsYouReason[ticketID] = reason
	push := d.noteNeedsYouPushLocked(rs, ticketID, reason)
	num, title := ticketNumTitleLocked(rs, ticketID)
	d.persistRun(rs)
	d.broadcastRunLocked(rs)
	d.mu.Unlock()
	if push {
		d.pushRunNeedsYou(runID, ticketID, num, title, reason)
	}
}
