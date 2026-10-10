package el

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Seam (ticket 13): install gate — with a live worker in the store and the
// fake multiplexer, install exits non-zero, names the agent and replaces
// nothing (the builder is never invoked).

func installTestState(t *testing.T) string {
	t.Helper()
	old := statePath
	statePath = filepath.Join(t.TempDir(), "workers.json")
	t.Cleanup(func() { statePath = old })
	dir := t.TempDir()
	t.Setenv("EL_INSTALL_DIR", dir)
	src := t.TempDir()
	t.Setenv("EL_SOURCE_DIR", src)
	// The web module sits beside the main module in a real checkout; the
	// builder fake asserts every build dir exists, so the fixture mirrors
	// it. The health probe is stubbed to pass: tests drive the rollback by
	// overriding it, and no test boots a real binary.
	if err := os.MkdirAll(filepath.Join(src, "..", "emberloop-web"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldProbe := probeWebHealth
	probeWebHealth = func(string) error { return nil }
	t.Cleanup(func() { probeWebHealth = oldProbe })
	return dir
}

func writeInstallSentinel(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func readInstallFile(t *testing.T, dir, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// installPaneExec answers `herdr pane list` with panes alive, and fails the
// test on any `go build` — a refused install must never build.
func installPaneExec(t *testing.T, panes ...string) Exec {
	t.Helper()
	return func(cmd string, args []string, o ExecOpts) (Result, error) {
		if cmd == "go" {
			t.Errorf("a refused install must not build, got go %v", args)
			return Result{Code: 1}, nil
		}
		if len(args) >= 2 && args[0] == "pane" && args[1] == "list" {
			var b strings.Builder
			b.WriteString(`{"result":{"panes":[`)
			for i, p := range panes {
				if i > 0 {
					b.WriteString(",")
				}
				b.WriteString(`{"pane_id":"` + p + `"}`)
			}
			b.WriteString(`]}}`)
			return Result{Code: 0, Stdout: b.String()}, nil
		}
		return Result{Code: 1, Stderr: "no such command"}, nil
	}
}

// installBuildExec answers `herdr pane list` with panes alive and plays `go
// build` by writing a marker where -o points, so the test sees what would be
// installed without a toolchain. Builds run in the source checkout, or in
// the web sibling module under it for the update's el-web target.
func installBuildExec(t *testing.T, src string, panes ...string) Exec {
	t.Helper()
	return func(cmd string, args []string, o ExecOpts) (Result, error) {
		if cmd == "go" {
			out := ""
			for i, a := range args {
				if a == "-o" && i+1 < len(args) {
					out = args[i+1]
				}
			}
			if out == "" {
				return Result{Code: 1, Stderr: "no -o"}, nil
			}
			if o.Cwd != src && o.Cwd != filepath.Join(src, "..", "emberloop-web") {
				t.Errorf("go build must run in the source checkout %q or the web sibling module under it, got %q", src, o.Cwd)
			}
			if info, err := os.Stat(o.Cwd); err != nil || !info.IsDir() {
				t.Errorf("go build must run in an existing checkout dir, got %q", o.Cwd)
			}
			pkg := args[len(args)-1]
			if err := os.WriteFile(out, []byte("built "+pkg), 0o755); err != nil {
				return Result{}, err
			}
			return Result{Code: 0}, nil
		}
		return installPaneExec(t, panes...)(cmd, args, o)
	}
}

// Seam (ticket 13): install — with no live agent the build runs and both
// binaries are replaced.
func TestInstallWithNoLiveAgentInstalls(t *testing.T) {
	dir := installTestState(t)
	src := os.Getenv("EL_SOURCE_DIR")
	writeInstallSentinel(t, dir, "el", "old-pig")
	writeInstallSentinel(t, dir, "el-mcp", "old-mcp")

	res, err := InstallBinaries(installBuildExec(t, src), InstallOptions{})
	if err != nil {
		t.Fatalf("install with no live agent must succeed: %v", err)
	}
	if res.Dir != dir {
		t.Errorf("install dir = %q, want %q", res.Dir, dir)
	}
	if got := readInstallFile(t, dir, "el"); got != "built ./cmd/emberloop" {
		t.Errorf("el = %q, want the new build", got)
	}
	if got := readInstallFile(t, dir, "el-mcp"); got != "built ./cmd/emberloop-mcp" {
		t.Errorf("el-mcp = %q, want the new build", got)
	}
}

var errInstallHerdrDown = errors.New("herdr down")

// Seam (ticket 13): install CLI — `el install` refuses live agents by
// name, installs cleanly, and --force installs with the stale-session
// warning; unknown flags fail with usage.
func installCLIExec(t *testing.T, panes ...string) Exec {
	t.Helper()
	src := os.Getenv("EL_SOURCE_DIR")
	return installBuildExec(t, src, panes...)
}

func TestRunInstallRefusesUnknownFlagsWithUsage(t *testing.T) {
	installTestState(t)
	var out strings.Builder
	if err := RunInstall([]string{"--bogus"}, &out); err == nil {
		t.Error("an unknown install flag must fail")
	} else if !strings.Contains(err.Error(), "usage") {
		t.Errorf("the failure must print usage, got: %v", err)
	}
	if err := RunInstall([]string{"--help"}, &out); err != nil {
		t.Errorf("--help must succeed, got: %v", err)
	} else if !strings.Contains(out.String(), "usage") {
		t.Errorf("--help must print usage, got %q", out.String())
	}
}

// Seam (ticket 13): install atomicity — a build that fails halfway replaces
// nothing and leaves no temp files behind.
func TestInstallFailedBuildReplacesNothingAndLeavesNoTemps(t *testing.T) {
	dir := installTestState(t)
	writeInstallSentinel(t, dir, "el", "old-pig")
	writeInstallSentinel(t, dir, "el-mcp", "old-mcp")
	builds := 0
	exec := func(cmd string, args []string, o ExecOpts) (Result, error) {
		if cmd == "go" {
			builds++
			if builds == 1 {
				out := ""
				for i, a := range args {
					if a == "-o" && i+1 < len(args) {
						out = args[i+1]
					}
				}
				if err := os.WriteFile(out, []byte("built"), 0o755); err != nil {
					return Result{}, err
				}
				return Result{Code: 0}, nil
			}
			out := ""
			for i, a := range args {
				if a == "-o" && i+1 < len(args) {
					out = args[i+1]
				}
			}
			_ = os.WriteFile(out, []byte("partial"), 0o755)
			return Result{Code: 1, Stderr: "boom"}, nil
		}
		return installPaneExec(t)(cmd, args, o)
	}
	if _, err := InstallBinaries(exec, InstallOptions{}); err == nil {
		t.Fatal("a failed build must fail the install")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		names := []string{}
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("a failed install must leave no temp files, dir holds %v", names)
	}
	if got := readInstallFile(t, dir, "el"); got != "old-pig" {
		t.Errorf("a failed install must replace nothing, el = %q", got)
	}
	if got := readInstallFile(t, dir, "el-mcp"); got != "old-mcp" {
		t.Errorf("a failed install must replace nothing, el-mcp = %q", got)
	}
}

// Seam (ticket 13): install rollback — when a later rename fails, destinations
// already replaced are restored, so a half-installed pair is never left. A
// directory at a destination fails its rename loudly instead of being
// clobbered.
func TestInstallFailedRenameRestoresWhatItReplaced(t *testing.T) {
	dir := installTestState(t)
	src := os.Getenv("EL_SOURCE_DIR")
	writeInstallSentinel(t, dir, "el", "old-pig")
	if err := os.MkdirAll(filepath.Join(dir, "el-mcp"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := InstallBinaries(installBuildExec(t, src), InstallOptions{})
	if err == nil {
		t.Fatal("a rename onto a directory must fail the install")
	}
	if got := readInstallFile(t, dir, "el"); got != "old-pig" {
		t.Errorf("a failed swap must restore el, got %q", got)
	}
	if info, err := os.Stat(filepath.Join(dir, "el-mcp")); err != nil || !info.IsDir() {
		t.Errorf("a failed swap must not clobber the directory at el-mcp: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		names := []string{}
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("a failed swap must leave no temps or backups, dir holds %v", names)
	}
}
