package wick

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// skillsRepo is the upstream library wick-pig installs.
const skillsRepo = "mattpocock/skills"

// errNoBunx tells the reader what is missing and what to do about it.
var errNoBunx = errors.New("bunx was not found: install bun from https://bun.sh, " +
	"then run `wick-pig skills install` again")

// InstallSkills fills skillsDir from skillsRepo using the `skills` CLI.
//
// That CLI only installs into an agent directory, so it runs in a staging git
// repo beside the library and the result is moved into place. The staging repo
// is a sibling so the move is a rename on one filesystem, not a copy across
// two.
func InstallSkills(run Exec) error {
	root := skillsRoot()
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(root, "install-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)

	if _, err := run("git", []string{"init"}, ExecOpts{Cwd: stage}); err != nil {
		return err
	}

	res, err := run("bunx", []string{
		"skills@latest", "add", skillsRepo,
		"--skill", "*", "--agent", "universal", "--copy", "-y",
	}, ExecOpts{Cwd: stage})
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return errNoBunx
		}
		return err
	}
	if res.Code != 0 {
		return fmt.Errorf("bunx skills add %s failed:\n%s", skillsRepo, combined(res))
	}

	staged := filepath.Join(stage, ".agents", "skills")
	if info, err := os.Stat(staged); err != nil || !info.IsDir() {
		return fmt.Errorf("the skills CLI wrote no .agents/skills under %s", stage)
	}
	// Replace the whole library: a skill dropped upstream must not linger.
	if err := os.RemoveAll(skillsDir()); err != nil {
		return err
	}
	if err := os.Rename(staged, skillsDir()); err != nil {
		return err
	}
	return keepLock(stage)
}

// keepLock moves the CLI's lockfile out of the staging repo, beside the library
// it describes. The old lock is dropped even when the new install brought none,
// so a replaced library never keeps a lockfile that describes the old one.
func keepLock(stage string) error {
	dst := filepath.Join(skillsRoot(), "skills-lock.json")
	if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
		return err
	}
	src := filepath.Join(stage, "skills-lock.json")
	if _, err := os.Stat(src); err != nil {
		return nil
	}
	return os.Rename(src, dst)
}

// combined is a command's two streams as one message for the reader, trimmed.
func combined(res Result) string {
	return strings.TrimSpace(res.Stdout + res.Stderr)
}
