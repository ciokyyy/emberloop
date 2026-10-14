package el

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// firstLine is the first line of text, or fallback when text is blank: build
// failures report one line, not a log.
func firstLine(text, fallback string) string {
	if trimmed := strings.TrimSpace(text); trimmed != "" {
		return strings.SplitN(trimmed, "\n", 2)[0]
	}
	return fallback
}

// `el update` (alias `el install`) is the only path that replaces the
// installed el binaries (el, el-mcp and el-web; see update.go).
// Nothing else in this package writes to the install directory.

// InstallOptions tunes InstallBinaries. It has no fields today.
type InstallOptions struct{}

// InstallResult is where InstallBinaries put the new binaries. Warning is
// a notice to print.
type InstallResult struct {
	Dir     string
	Warning string
}

// installExec runs the install's build commands, so tests drive it with a
// fake builder.
var installExec Exec = OSExec

// installDir is where the el binaries live. The install updates the
// directory the running binary came from, so it follows a relocated setup;
// EL_INSTALL_DIR points it at a temp dir in tests.
func installDir() string {
	if dir := os.Getenv("EL_INSTALL_DIR"); dir != "" {
		return dir
	}
	if exe, err := os.Executable(); err == nil && exe != "" {
		return filepath.Dir(exe)
	}
	return filepath.Join(os.Getenv("HOME"), ".local", "bin")
}

// installSourceDir is the checkout the install builds from: the module
// directory — run `el install` from the module source — or
// EL_SOURCE_DIR when set, which is what tests use.
func installSourceDir() string {
	if dir := os.Getenv("EL_SOURCE_DIR"); dir != "" {
		return dir
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return "."
}

// buildTarget is one binary to build: name is the installed file, pkg the
// package within its module, dir the module's directory relative to the
// main module ("" for the main module itself).
type buildTarget struct{ name, pkg, dir string }

// installTargets are the binaries one install replaces.
var installTargets = []buildTarget{
	{"el", "./cmd/el", ""},
	{"el-mcp", "./cmd/el-mcp", ""},
}

// buildTargets builds every target to a temp file with the same ldflags
// hook `el update` uses for its stamps ("" builds plain). It returns the
// staged set, swapped only when all built, so a failed build replaces
// nothing; the caller discards the temps on refusal.
func buildTargets(run Exec, src, dir string, targets []buildTarget, ldflags func(buildTarget) string) ([]staged, error) {
	var done []staged
	discard := func() {
		for _, s := range done {
			_ = os.Remove(s.tmp)
		}
	}
	for _, target := range targets {
		f, err := os.CreateTemp(dir, target.name+".*")
		if err != nil {
			discard()
			return nil, err
		}
		tmp := f.Name()
		_ = f.Close()
		_ = os.Remove(tmp)
		buildDir := src
		if target.dir != "" {
			buildDir = filepath.Join(src, target.dir)
		}
		args := []string{"build"}
		if flags := ldflags(target); flags != "" {
			args = append(args, "-ldflags", flags)
		}
		args = append(args, "-o", tmp, target.pkg)
		res, err := run("go", args, ExecOpts{Cwd: buildDir, Timeout: 5 * time.Minute})
		if err != nil {
			_ = os.Remove(tmp)
			discard()
			return nil, err
		}
		if res.Code != 0 {
			_ = os.Remove(tmp)
			discard()
			return nil, fmt.Errorf("go build %s failed: %s", target.pkg, firstLine(res.Stderr, res.Stdout))
		}
		done = append(done, staged{tmp: tmp, dst: filepath.Join(dir, target.name)})
	}
	return done, nil
}

// staged is one built binary waiting to replace its destination: the temp
// file holding the new build, and the installed path it renames over.
type staged struct{ tmp, dst string }

// InstallBinaries rebuilds el and el-mcp from the source
// checkout and installs them into installDir.
func InstallBinaries(run Exec, opts InstallOptions) (InstallResult, error) {
	dir, src := installDir(), installSourceDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return InstallResult{}, err
	}
	done, err := buildTargets(run, src, dir, installTargets, func(buildTarget) string { return "" })
	if err != nil {
		return InstallResult{}, err
	}
	if err := swapInstalled(done); err != nil {
		return InstallResult{}, err
	}
	res := InstallResult{Dir: dir}
	warn := func(w string) {
		if w == "" {
			return
		}
		if res.Warning != "" {
			res.Warning += "\n"
		}
		res.Warning += w
	}
	if _, err := SyncAgentDocs(); err != nil {
		warn("warning: could not refresh el://docs: " + err.Error())
	}
	return res, nil
}

// placedBinary is a destination the swap already replaced, with the backup
// of what was there ("" when the destination is new).
type placedBinary struct {
	dst  string
	prev string
}

// swapInstalled renames every staged build over its destination. Each
// destination the swap already replaced is restored when a later rename
// fails, so a half-installed set is never left: all binaries go, or the
// old ones stay. Only regular files are backed up; anything else at a
// destination fails the rename loudly instead of being clobbered.
func swapInstalled(done []staged) error {
	var placed []placedBinary
	restore := func() {
		for i := len(placed) - 1; i >= 0; i-- {
			p := placed[i]
			if p.prev == "" {
				_ = os.Remove(p.dst)
			} else {
				_ = os.Rename(p.prev, p.dst)
			}
		}
	}
	discard := func() {
		for _, s := range done {
			_ = os.Remove(s.tmp)
		}
	}
	fail := func(err error) error {
		restore()
		discard()
		return err
	}
	for _, s := range done {
		if err := os.Chmod(s.tmp, 0o755); err != nil {
			return fail(err)
		}
		prev := ""
		if info, err := os.Stat(s.dst); err == nil && info.Mode().IsRegular() {
			prev = s.tmp + ".prev"
			if err := os.Rename(s.dst, prev); err != nil {
				return fail(err)
			}
		}
		if err := os.Rename(s.tmp, s.dst); err != nil {
			if prev != "" {
				_ = os.Rename(prev, s.dst)
			}
			return fail(err)
		}
		placed = append(placed, placedBinary{dst: s.dst, prev: prev})
	}
	for _, p := range placed {
		if p.prev != "" {
			_ = os.Remove(p.prev)
		}
	}
	return nil
}

// runInstallDeps is the dependency step at the end of `el update`.
// A var so tests stub it with a fake instead of real package installs and
// downloads.
var runInstallDeps = func(out io.Writer) error {
	return InstallDeps(out, DepsConfig{})
}
