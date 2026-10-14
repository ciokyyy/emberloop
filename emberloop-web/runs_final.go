package web

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	el "example.com/emberloop-web/el"
)

type finalMergeOutcome struct {
	NeedsYou bool
	Reason   string
	Files    []string
}

var finalGeneratedPaths = []string{
	"emberloop-web/web/dist",
	"emberloop-web/web/package-lock.json",
	"emberloop-web/web/e2e/package-lock.json",
}

// finalMergeReadiness refuses to gate a partial spec. RunDriver can return
// cleanly with queued, parked or needs-you Tickets still unresolved.
func (d *Daemon) finalMergeReadiness(runID string, result el.DriverResult) finalMergeOutcome {
	d.mu.Lock()
	rs, ok := d.runs[runID]
	if !ok {
		d.mu.Unlock()
		return finalNeedsYou("Run state disappeared before final merge", []string{"HEAD"}, "resume the Run")
	}
	repo := rs.Repo
	discarded := make(map[string]bool, len(rs.Discarded))
	for id, yes := range rs.Discarded {
		discarded[id] = yes
	}
	tickets := append([]staticTicket(nil), rs.Tickets...)
	// The driver result lists only merges of this driver invocation. A
	// resumed or restarted Run holds earlier merges in rs.Merged, and those
	// count as resolved too.
	merged := make(map[string]bool, len(result.Merged)+len(rs.Merged))
	for id, sha := range rs.Merged {
		if sha != "" {
			merged[id] = true
		}
	}
	d.mu.Unlock()

	for _, item := range result.Merged {
		merged[item.ID] = true
	}
	var unresolved, names []string
	for _, ticket := range tickets {
		if merged[ticket.ID] || discarded[ticket.ID] {
			continue
		}
		path, err := filepath.Rel(repo, ticket.Path)
		if err != nil || path == ".." || strings.HasPrefix(path, ".."+string(filepath.Separator)) {
			path = ticket.Path
		}
		unresolved = append(unresolved, filepath.ToSlash(path))
		names = append(names, ticket.ID)
	}
	if len(unresolved) == 0 {
		return finalMergeOutcome{}
	}
	return finalNeedsYou(
		"final merge blocked until every Ticket is merged or discarded",
		unresolved,
		"unresolved Tickets: "+strings.Join(names, ", "),
	)
}

// finalizeSpecRun rebases the integration branch onto the current master,
// rebuilds generated UI files, checks that exact result, then fast-forwards
// a clean checkout of master. Failures are recoverable Needs-you states.
func (d *Daemon) finalizeSpecRun(runID string) (finalMergeOutcome, error) {
	d.mu.Lock()
	rs, ok := d.runs[runID]
	if !ok {
		d.mu.Unlock()
		return finalMergeOutcome{}, fmt.Errorf("unknown run %q", runID)
	}
	repo, specBranch := rs.Repo, rs.SpecBranch
	d.mu.Unlock()
	if specBranch == "" {
		return finalMergeOutcome{}, nil
	}

	masterRef, masterSHA, err := masterBranchRef(repo)
	if err != nil {
		return finalMergeOutcome{}, err
	}
	specRef := "refs/heads/" + specBranch
	originalSpecSHA, err := runGit(repo, "rev-parse", specRef)
	if err != nil {
		return finalMergeOutcome{}, fmt.Errorf("could not read spec branch %s: %w", specBranch, err)
	}
	worktree, err := os.MkdirTemp("", "emberloop-final-merge-")
	if err != nil {
		return finalMergeOutcome{}, err
	}
	if err := os.Remove(worktree); err != nil {
		return finalMergeOutcome{}, err
	}
	added := false
	defer func() {
		if added {
			_, _ = runGit(repo, "worktree", "remove", "--force", worktree)
		}
		_ = os.RemoveAll(worktree)
	}()
	if _, err := runGit(repo, "worktree", "add", "--detach", worktree, originalSpecSHA); err != nil {
		return finalMergeOutcome{}, fmt.Errorf("could not prepare final merge worktree: %w", err)
	}
	added = true

	containsMaster, err := isAncestorRef(repo, masterSHA, originalSpecSHA)
	if err != nil {
		return finalMergeOutcome{}, err
	}
	if !containsMaster {
		if _, err := runGit(worktree, "rebase", masterSHA); err != nil {
			conflicts, _ := runGit(worktree, "diff", "--name-only", "--diff-filter=U")
			files := splitFileList(conflicts)
			_, _ = runGit(worktree, "rebase", "--abort")
			if len(files) == 0 {
				files = changedFilesBetween(repo, masterSHA, originalSpecSHA)
			}
			return finalNeedsYou("final rebase onto master conflicted", files, err.Error()), nil
		}
	}

	buildOutput, err := d.runGeneratedBuild(worktree)
	if err != nil {
		files := workingTreeFiles(worktree)
		if len(files) == 0 {
			files = append([]string(nil), finalGeneratedPaths...)
		}
		return finalNeedsYou("generated-file build failed", files, buildOutput+"\n"+err.Error()), nil
	}
	if out, err := runGit(worktree, "status", "--porcelain"); err != nil {
		return finalMergeOutcome{}, err
	} else if strings.TrimSpace(out) != "" {
		if _, err := runGit(worktree, "add", "-A"); err != nil {
			return finalMergeOutcome{}, fmt.Errorf("could not stage generated files: %w", err)
		}
		if changed, err := runStagedDiff(worktree, "."); err != nil {
			return finalMergeOutcome{}, err
		} else if changed {
			if _, err := runGit(worktree, "-c", "user.name=emberloop", "-c", "user.email=emberloop@localhost", "commit", "-m", "Build generated files for final merge"); err != nil {
				return finalMergeOutcome{}, fmt.Errorf("could not commit generated files: %w", err)
			}
		}
	}
	candidateSHA, err := runGit(worktree, "rev-parse", "HEAD")
	if err != nil {
		return finalMergeOutcome{}, err
	}
	if candidateSHA != originalSpecSHA {
		if _, err := runGit(repo, "update-ref", specRef, candidateSHA, originalSpecSHA); err != nil {
			files := changedFilesBetween(repo, masterSHA, candidateSHA)
			return finalNeedsYou("spec branch moved during final merge", files, err.Error()), nil
		}
	}

	checkOutput, err := d.runFinalRepoCheck(worktree)
	if err != nil {
		files := changedFilesBetween(repo, masterSHA, candidateSHA)
		if len(files) == 0 {
			files = []string{".emberloop/check"}
		}
		return finalNeedsYou("repo check failed on the final merge result", files, checkOutput+"\n"+err.Error()), nil
	}

	d.pauseGate(runID)
	return advanceCheckedMaster(repo, masterRef, masterSHA, candidateSHA)
}

func advanceCheckedMaster(repo, masterRef, masterSHA, candidateSHA string) (finalMergeOutcome, error) {
	masterCheckout, checkedOut, err := masterBranchWorktree(repo, masterRef)
	if err != nil {
		return finalMergeOutcome{}, err
	}
	if !checkedOut {
		masterCheckout, err = os.MkdirTemp("", "emberloop-master-merge-")
		if err != nil {
			return finalMergeOutcome{}, err
		}
		if err := os.Remove(masterCheckout); err != nil {
			return finalMergeOutcome{}, err
		}
		branch := strings.TrimPrefix(masterRef, "refs/heads/")
		if _, err := runGit(repo, "worktree", "add", masterCheckout, branch); err != nil {
			_ = os.RemoveAll(masterCheckout)
			return finalNeedsYou("could not prepare a clean master checkout", []string{"HEAD"}, err.Error()), nil
		}
		defer func() {
			_, _ = runGit(repo, "worktree", "remove", "--force", masterCheckout)
			_ = os.RemoveAll(masterCheckout)
		}()
	}
	branch, branchErr := runGit(masterCheckout, "symbolic-ref", "--quiet", "HEAD")
	if branchErr != nil || branch != masterRef {
		detail := "expected a checkout of " + masterRef
		if branchErr != nil {
			detail += ": " + branchErr.Error()
		} else {
			detail += ", found " + branch
		}
		return finalNeedsYou("final merge requires the selected master checkout", []string{"HEAD"}, detail), nil
	}
	var dirtyFiles []string
	var status string
	if checkedOut {
		dirtyFiles, status, err = masterCheckoutDirtyFiles(masterCheckout)
	} else {
		status, err = runGit(masterCheckout, "status", "--porcelain", "--untracked-files=all")
		if err == nil && strings.TrimSpace(status) != "" {
			dirtyFiles = workingTreeFiles(masterCheckout)
			if len(dirtyFiles) == 0 {
				dirtyFiles = []string{"."}
			}
		}
	}
	if err != nil {
		return finalMergeOutcome{}, err
	}
	if len(dirtyFiles) != 0 {
		return finalNeedsYou("final merge requires a clean master checkout", dirtyFiles, status), nil
	}
	currentMaster, err := runGit(masterCheckout, "rev-parse", masterRef)
	if err != nil {
		return finalMergeOutcome{}, err
	}
	if currentMaster != masterSHA {
		files := changedFilesBetween(repo, masterSHA, currentMaster)
		if len(files) == 0 {
			files = changedFilesBetween(repo, currentMaster, candidateSHA)
		}
		if len(files) == 0 {
			files = []string{"HEAD"}
		}
		return finalNeedsYou("master moved while the final check was running", files, "checked "+masterSHA+", current "+currentMaster), nil
	}
	if _, err := runGit(masterCheckout, "merge", "--ff-only", candidateSHA); err != nil {
		files := changedFilesBetween(repo, masterSHA, candidateSHA)
		return finalNeedsYou("could not fast-forward the checked-out master", files, err.Error()), nil
	}
	mergedHEAD, err := runGit(masterCheckout, "rev-parse", "HEAD")
	if err != nil {
		return finalMergeOutcome{}, err
	}
	if mergedHEAD != candidateSHA {
		return finalNeedsYou("master moved during the final merge", changedFilesBetween(repo, masterSHA, mergedHEAD), "expected "+candidateSHA+", found "+mergedHEAD), nil
	}
	return finalMergeOutcome{}, nil
}

func masterBranchWorktree(repo, branchRef string) (string, bool, error) {
	output, err := runGit(repo, "worktree", "list", "--porcelain")
	if err != nil {
		return "", false, err
	}
	var path, branch string
	for _, line := range append(strings.Split(output, "\n"), "") {
		if line == "" {
			if path != "" && branch == branchRef {
				return path, true, nil
			}
			path, branch = "", ""
			continue
		}
		if strings.HasPrefix(line, "worktree ") {
			path = strings.TrimPrefix(line, "worktree ")
		} else if strings.HasPrefix(line, "branch ") {
			branch = strings.TrimPrefix(line, "branch ")
		}
	}
	return "", false, nil
}

// masterCheckoutDirtyFiles ignores registered linked worktrees nested in an
// existing master checkout. Temporary checkouts have no nested worktrees and
// use plain git status. Other tracked and untracked changes block promotion.
func masterCheckoutDirtyFiles(root string) ([]string, string, error) {
	status, err := runGit(root, "status", "--porcelain", "--untracked-files=all")
	if err != nil || strings.TrimSpace(status) == "" {
		return nil, status, err
	}
	files := workingTreeFiles(root)
	worktrees, err := runGit(root, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, status, err
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return nil, status, err
	}
	var nested []string
	for _, line := range strings.Split(worktrees, "\n") {
		if !strings.HasPrefix(line, "worktree ") {
			continue
		}
		worktree, err := filepath.Abs(strings.TrimPrefix(line, "worktree "))
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(rootAbs, worktree)
		if err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
			nested = append(nested, filepath.ToSlash(filepath.Clean(rel)))
		}
	}
	var dirty []string
	for _, file := range files {
		file = filepath.ToSlash(filepath.Clean(file))
		inWorktree := false
		for _, worktree := range nested {
			if file == worktree || strings.HasPrefix(file, worktree+"/") {
				inWorktree = true
				break
			}
		}
		if !inWorktree {
			dirty = append(dirty, file)
		}
	}
	if len(dirty) == 0 && len(files) == 0 {
		dirty = []string{"."}
	}
	return sortedUnique(dirty), status, nil
}

func (d *Daemon) runGeneratedBuild(worktree string) (string, error) {
	if d.runFinalBuild != nil {
		return d.runFinalBuild(worktree)
	}
	webDir := filepath.Join(worktree, "emberloop-web", "web")
	packageJSON := filepath.Join(webDir, "package.json")
	info, err := os.Stat(packageJSON)
	if errors.Is(err, os.ErrNotExist) {
		return "skipped UI build: emberloop-web/web/package.json is absent", nil
	}
	if err != nil {
		return "", fmt.Errorf("could not inspect emberloop-web/web/package.json: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("emberloop-web/web/package.json is not a regular file")
	}

	var output strings.Builder
	commands := []struct {
		dir  string
		args []string
	}{
		{dir: webDir, args: []string{"install", "--no-audit", "--no-fund"}},
		{dir: webDir, args: []string{"run", "build"}},
	}
	e2ePackage := filepath.Join(webDir, "e2e", "package.json")
	if _, err := os.Stat(e2ePackage); err == nil {
		commands = append(commands, struct {
			dir  string
			args []string
		}{dir: filepath.Dir(e2ePackage), args: []string{"install", "--package-lock-only", "--no-audit", "--no-fund"}})
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("could not inspect emberloop-web/web/e2e/package.json: %w", err)
	}
	for _, command := range commands {
		cmd := exec.Command("npm", command.args...)
		cmd.Dir = command.dir
		out, err := cmd.CombinedOutput()
		output.Write(out)
		if err != nil {
			return output.String(), fmt.Errorf("npm %s failed: %w", strings.Join(command.args, " "), err)
		}
	}
	return output.String(), nil
}

func (d *Daemon) runFinalRepoCheck(worktree string) (string, error) {
	if d.runRepoCheck != nil {
		return d.runRepoCheck(worktree)
	}
	checkPath := filepath.Join(worktree, ".emberloop", "check")
	info, err := os.Stat(checkPath)
	if errors.Is(err, os.ErrNotExist) {
		return "", errors.New("required repository check .emberloop/check is missing")
	}
	if err != nil {
		return "", fmt.Errorf("could not inspect .emberloop/check: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("required repository check .emberloop/check is not a regular file")
	}
	cmd := exec.Command("sh", ".emberloop/check")
	cmd.Dir = worktree
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf(".emberloop/check failed: %w", err)
	}
	return string(out), nil
}

func masterBranchRef(repo string) (ref, sha string, err error) {
	for _, candidate := range []string{"refs/heads/master", "refs/heads/main"} {
		if got, e := runGit(repo, "rev-parse", candidate); e == nil && got != "" {
			return candidate, got, nil
		}
	}
	return "", "", fmt.Errorf("could not read the master tip in %s: neither master nor main exists", repo)
}

func finalNeedsYou(reason string, files []string, detail string) finalMergeOutcome {
	files = sortedUnique(files)
	if len(files) == 0 {
		files = append([]string(nil), finalGeneratedPaths...)
	}
	message := reason + " in: " + strings.Join(files, ", ")
	if detail = strings.TrimSpace(detail); detail != "" {
		if len(detail) > 4000 {
			detail = detail[:4000] + "…"
		}
		message += "\n" + detail
	}
	return finalMergeOutcome{NeedsYou: true, Reason: message, Files: files}
}

func changedFilesBetween(repo, base, head string) []string {
	out, err := runGit(repo, "diff", "--name-only", base, head)
	if err != nil {
		return nil
	}
	return splitFileList(out)
}

func workingTreeFiles(dir string) []string {
	tracked, _ := runGit(dir, "diff", "--name-only", "HEAD")
	untracked, _ := runGit(dir, "ls-files", "--others", "--exclude-standard")
	return sortedUnique(append(splitFileList(tracked), splitFileList(untracked)...))
}

func splitFileList(output string) []string {
	var files []string
	for _, line := range strings.Split(output, "\n") {
		if file := strings.TrimSpace(line); file != "" {
			files = append(files, file)
		}
	}
	return sortedUnique(files)
}

func sortedUnique(files []string) []string {
	seen := make(map[string]bool, len(files))
	clean := make([]string, 0, len(files))
	for _, file := range files {
		file = strings.TrimSpace(file)
		if file != "" && !seen[file] {
			seen[file] = true
			clean = append(clean, file)
		}
	}
	sort.Strings(clean)
	return clean
}
