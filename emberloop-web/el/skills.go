package el

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// skillsRoot is where el keeps the skill libraries it installs. The env var
// names the root itself, so EL_SKILLS_DIR overrides the whole tree, not one
// library inside it.
func skillsRoot() string {
	if dir := os.Getenv("EL_SKILLS_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(os.Getenv("HOME"), ".emberloop", "skills")
}

// skillsNamespace is the library's directory under the skills root, and the
// namespace its el:// addresses carry. The two must stay one string: an
// address that names a namespace the read tool cannot find reads as nothing.
const skillsNamespace = "matt-pocock"

// skillsDir is the library el installs: the mattpocock skills, one
// directory per skill, under the root.
func skillsDir() string {
	return filepath.Join(skillsRoot(), skillsNamespace)
}

// SkillLibraryDir is where el installs and reads the skill library, for
// callers outside the package.
func SkillLibraryDir() string { return skillsDir() }

// skillsRepo is the upstream library el installs.
const skillsRepo = "mattpocock/skills"

// errNoBunx tells the reader what is missing and what to do about it.
var errNoBunx = errors.New("bunx was not found: install bun from https://bun.sh, " +
	"then run `el skills install` again")

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

	if init, err := run("git", []string{"init"}, ExecOpts{Cwd: stage}); err != nil {
		return err
	} else if init.Code != 0 {
		return fmt.Errorf("git init failed:\n%s", combined(init))
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
	// Replace the whole library: a skill dropped upstream must not linger. The
	// old tree is kept until the new one is in place, so a failed move leaves
	// the previous library rather than none.
	if err := swapIn(staged, skillsDir()); err != nil {
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

// swapIn puts staged in dest, keeping the old tree until the new one arrives.
// If the second move fails the old tree is restored and the error returned.
func swapIn(staged, dest string) error {
	old := dest + ".old"
	if err := os.RemoveAll(old); err != nil {
		return err
	}
	if err := os.Rename(dest, old); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(staged, dest); err != nil {
		_ = os.Rename(old, dest)
		return err
	}
	return os.RemoveAll(old)
}
