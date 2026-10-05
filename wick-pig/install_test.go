package wick

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
	statePath = filepath.Join(t.TempDir(), "workers.json")
	dir := t.TempDir()
	t.Setenv("WICK_INSTALL_DIR", dir)
	t.Setenv("WICK_SOURCE_DIR", t.TempDir())
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

func TestInstallRefusesWhileAWorkerIsLiveAndReplacesNothing(t *testing.T) {
	dir := installTestState(t)
	if err := save(map[string]Worker{
		"07-foo": {ID: "07-foo", Pane: "w1:p1"},
	}); err != nil {
		t.Fatal(err)
	}
	writeInstallSentinel(t, dir, "wick-pig", "old-pig")
	writeInstallSentinel(t, dir, "wick-mcp", "old-mcp")

	_, err := InstallBinaries(installPaneExec(t, "w1:p1"), InstallOptions{})
	if err == nil {
		t.Fatal("install with a live worker must fail")
	}
	if !strings.Contains(err.Error(), "07-foo") {
		t.Errorf("the refusal must name the live agent, got: %v", err)
	}
	if got := readInstallFile(t, dir, "wick-pig"); got != "old-pig" {
		t.Errorf("a refused install must replace nothing, wick-pig = %q", got)
	}
	if got := readInstallFile(t, dir, "wick-mcp"); got != "old-mcp" {
		t.Errorf("a refused install must replace nothing, wick-mcp = %q", got)
	}
}

// installBuildExec answers `herdr pane list` with panes alive and plays `go
// build` by writing a marker where -o points, so the test sees what would be
// installed without a toolchain.
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
			if o.Cwd != src {
				t.Errorf("go build must run in the source checkout %q, got %q", src, o.Cwd)
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
	src := os.Getenv("WICK_SOURCE_DIR")
	writeInstallSentinel(t, dir, "wick-pig", "old-pig")
	writeInstallSentinel(t, dir, "wick-mcp", "old-mcp")

	res, err := InstallBinaries(installBuildExec(t, src), InstallOptions{})
	if err != nil {
		t.Fatalf("install with no live agent must succeed: %v", err)
	}
	if res.Dir != dir {
		t.Errorf("install dir = %q, want %q", res.Dir, dir)
	}
	if got := readInstallFile(t, dir, "wick-pig"); got != "built ./cmd/wick-pig" {
		t.Errorf("wick-pig = %q, want the new build", got)
	}
	if got := readInstallFile(t, dir, "wick-mcp"); got != "built ./cmd/wick-mcp" {
		t.Errorf("wick-mcp = %q, want the new build", got)
	}
}

// Seam (ticket 13): install --force — force installs past a live agent and
// warns that running sessions keep the old binary until they restart.
func TestInstallForceInstallsPastALiveAgentWithWarning(t *testing.T) {
	dir := installTestState(t)
	src := os.Getenv("WICK_SOURCE_DIR")
	if err := save(map[string]Worker{
		"07-foo": {ID: "07-foo", Pane: "w1:p1"},
	}); err != nil {
		t.Fatal(err)
	}
	writeInstallSentinel(t, dir, "wick-pig", "old-pig")
	writeInstallSentinel(t, dir, "wick-mcp", "old-mcp")

	res, err := InstallBinaries(installBuildExec(t, src, "w1:p1"), InstallOptions{Force: true})
	if err != nil {
		t.Fatalf("a forced install must succeed: %v", err)
	}
	if !strings.Contains(res.Warning, "running sessions keep the old binary until they restart") {
		t.Errorf("a forced install must warn about stale sessions, got %q", res.Warning)
	}
	if got := readInstallFile(t, dir, "wick-pig"); got != "built ./cmd/wick-pig" {
		t.Errorf("a forced install must replace, wick-pig = %q", got)
	}
	if got := readInstallFile(t, dir, "wick-mcp"); got != "built ./cmd/wick-mcp" {
		t.Errorf("a forced install must replace, wick-mcp = %q", got)
	}
}

// Seam (ticket 13): install gate — a live reviewer and scout are named, a
// record whose pane is gone blocks nothing, and one pane is named once even
// when two stores know it.
func TestInstallNamesReviewerAndScoutAndIgnoresDeadPanes(t *testing.T) {
	dir := installTestState(t)
	tree := filepath.Join(t.TempDir(), "tree")
	if err := save(map[string]Worker{
		"07-dead": {ID: "07-dead", Pane: "w1:p9"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := saveReviewers(map[string]map[string]reviewer{
		tree: {"reviewer-standards": {Pane: "w1:p2"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := WriteAgentState(AgentState{ID: "rev-state", Type: "reviewer-standards", Pane: "w1:p2"}); err != nil {
		t.Fatal(err)
	}
	if err := WriteAgentState(AgentState{ID: "scout-1", Type: "scout", Pane: "w1:p3"}); err != nil {
		t.Fatal(err)
	}
	writeInstallSentinel(t, dir, "wick-pig", "old-pig")

	_, err := InstallBinaries(installPaneExec(t, "w1:p2", "w1:p3"), InstallOptions{})
	if err == nil {
		t.Fatal("install with live reviewer and scout must fail")
	}
	for _, want := range []string{"reviewer-standards", "scout-1"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal must name %q, got: %v", want, err)
		}
	}
	if strings.Contains(err.Error(), "07-dead") {
		t.Errorf("a record whose pane is gone must not block, got: %v", err)
	}
	if n := strings.Count(err.Error(), "w1:p2"); n != 1 {
		t.Errorf("one pane must be named once, w1:p2 appears %d times in: %v", n, err)
	}
	if got := readInstallFile(t, dir, "wick-pig"); got != "old-pig" {
		t.Errorf("a refused install must replace nothing, wick-pig = %q", got)
	}
}

var errInstallHerdrDown = errors.New("herdr down")

// Seam (ticket 13): install gate — when herdr cannot be asked, the install
// refuses rather than guesses; --force installs anyway with the warning.
func TestInstallWithUnknownLivenessRefusesUnlessForced(t *testing.T) {
	dir := installTestState(t)
	broken := func(cmd string, args []string, o ExecOpts) (Result, error) {
		if cmd == "go" {
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
		return Result{}, errInstallHerdrDown
	}

	if _, err := InstallBinaries(broken, InstallOptions{}); err == nil {
		t.Error("an install that cannot check liveness must refuse")
	} else if !strings.Contains(err.Error(), "cannot check") {
		t.Errorf("the refusal must say liveness is unknown, got: %v", err)
	}
	res, err := InstallBinaries(broken, InstallOptions{Force: true})
	if err != nil {
		t.Fatalf("a forced install must succeed: %v", err)
	}
	if !strings.Contains(res.Warning, "running sessions keep the old binary until they restart") {
		t.Errorf("a forced install past unknown liveness must warn, got %q", res.Warning)
	}
	if got := readInstallFile(t, dir, "wick-pig"); got != "built" {
		t.Errorf("a forced install must replace, wick-pig = %q", got)
	}
}

// Seam (ticket 13): install CLI — `wick-pig install` refuses live agents by
// name, installs cleanly, and --force installs with the stale-session
// warning; unknown flags fail with usage.
func installCLIExec(t *testing.T, panes ...string) Exec {
	t.Helper()
	src := os.Getenv("WICK_SOURCE_DIR")
	return installBuildExec(t, src, panes...)
}

func TestRunInstallRefusesLiveAgentsByName(t *testing.T) {
	installTestState(t)
	oldExec := installExec
	installExec = installCLIExec(t, "w1:p1")
	t.Cleanup(func() { installExec = oldExec })
	if err := save(map[string]Worker{
		"07-foo": {ID: "07-foo", Pane: "w1:p1"},
	}); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	err := RunInstall([]string{}, &out)
	if err == nil {
		t.Fatal("install with a live worker must fail")
	}
	if !strings.Contains(err.Error(), "07-foo") {
		t.Errorf("the refusal must name the live agent, got: %v", err)
	}
}

func TestRunInstallInstallsCleanlyAndForceWarns(t *testing.T) {
	dir := installTestState(t)
	oldExec := installExec
	installExec = installCLIExec(t)
	t.Cleanup(func() { installExec = oldExec })
	writeInstallSentinel(t, dir, "wick-pig", "old-pig")

	var out strings.Builder
	if err := RunInstall([]string{}, &out); err != nil {
		t.Fatalf("clean install must succeed: %v", err)
	}
	if !strings.Contains(out.String(), "installed") || !strings.Contains(out.String(), dir) {
		t.Errorf("a clean install must say what it installed where, got %q", out.String())
	}
	if got := readInstallFile(t, dir, "wick-pig"); got != "built ./cmd/wick-pig" {
		t.Errorf("wick-pig = %q, want the new build", got)
	}

	if err := save(map[string]Worker{
		"07-foo": {ID: "07-foo", Pane: "w1:p1"},
	}); err != nil {
		t.Fatal(err)
	}
	installExec = installCLIExec(t, "w1:p1")
	out.Reset()
	if err := RunInstall([]string{"--force"}, &out); err != nil {
		t.Fatalf("a forced install must succeed: %v", err)
	}
	if !strings.Contains(out.String(), "running sessions keep the old binary until they restart") {
		t.Errorf("a forced install must print the stale-session warning, got %q", out.String())
	}
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
	writeInstallSentinel(t, dir, "wick-pig", "old-pig")
	writeInstallSentinel(t, dir, "wick-mcp", "old-mcp")
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
	if got := readInstallFile(t, dir, "wick-pig"); got != "old-pig" {
		t.Errorf("a failed install must replace nothing, wick-pig = %q", got)
	}
	if got := readInstallFile(t, dir, "wick-mcp"); got != "old-mcp" {
		t.Errorf("a failed install must replace nothing, wick-mcp = %q", got)
	}
}
