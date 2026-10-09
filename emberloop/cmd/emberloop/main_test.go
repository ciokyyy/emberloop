package main

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"example.com/emberloop"
)

// A bare launch is a planning session. full is reachable, but only by asking
// for it with the role flag.
func TestBareLaunchPlansAndTheRoleFlagSelectsFull(t *testing.T) {
	cases := []struct {
		sub, flag, want string
	}{
		{"", "", "plan"},
		{"", "full", "full"},
		{"grill", "", "plan"},
		{"to-spec", "", "plan"},
		{"implement", "", "implement"},
	}
	for _, c := range cases {
		got, err := sessionRole(c.sub, c.flag)
		if err != nil {
			t.Errorf("sessionRole(%q, %q): %v", c.sub, c.flag, err)
			continue
		}
		if got != c.want {
			t.Errorf("sessionRole(%q, %q) = %q, want %q", c.sub, c.flag, got, c.want)
		}
	}
}

func TestRoleFlagRefusesAnUnknownRole(t *testing.T) {
	if _, err := sessionRole("", "nope"); err == nil {
		t.Error("an unknown role must be refused at the launcher, not a broken session")
	}
}

// The flag belongs to a bare session: a subcommand's prompt is written for its
// own role, so pairing it with a different one is refused rather than launched.
func TestRoleFlagIsRefusedBesideASubcommand(t *testing.T) {
	for _, sub := range []string{"implement", "grill"} {
		if _, err := sessionRole(sub, "full"); err == nil {
			t.Errorf("--role full beside %q must be refused", sub)
		}
	}
}

// implement must run in a herdr pane: its workers' results are delivered into
// that pane as notices, so a session outside herdr could never hear them. The
// refusal comes before anything is created or spawned.
func TestImplementRequiresAHerdrPane(t *testing.T) {
	t.Setenv("HERDR_PANE_ID", "")
	err := run([]string{"implement", "/nonexistent/spec.md"})
	if err == nil || !strings.Contains(err.Error(), "herdr") {
		t.Fatalf("implement outside herdr must fail naming herdr, got %v", err)
	}
}

// The headless parent path is gone: an implement session refuses -p rather
// than run one-shot where its workers' notices cannot reach it.
func TestImplementRefusesHeadless(t *testing.T) {
	t.Setenv("HERDR_PANE_ID", "w1:p1")
	err := run([]string{"implement", "-p", "/nonexistent/spec.md"})
	if err == nil || !strings.Contains(err.Error(), "-p") {
		t.Fatalf("implement -p must be refused, got %v", err)
	}
}

// A top-level session with no flags resolves the orchestrator slot; a flag
// still wins over it. This is the slot `el implement` launches with.
func TestTopLevelSessionResolvesTheOrchestratorSlot(t *testing.T) {
	p := settingsFile(t)
	write(t, p, `{"roles":{"orchestrator":{"harness":"claude","model":"opus","effort":"high"}}}`)
	if got := topLevelChoice(el.Choice{}); got.Harness != "claude" || got.Model != "opus" || got.Effort != "high" {
		t.Errorf("orchestrator slot = %+v, want claude/opus/high", got)
	}
	over := topLevelChoice(el.Choice{Harness: "pig", Model: "m", Effort: "low"})
	if over.Harness != "pig" || over.Model != "m" || over.Effort != "low" {
		t.Errorf("flags must win = %+v, want pig/m/low", over)
	}
}

// --claude against a pig orchestrator slot drops the slot's model and effort:
// a pig model and an effort claude does not have must not leak onto claude.
func TestClaudeFlagDropsThePigOrchestratorSlotModel(t *testing.T) {
	p := settingsFile(t)
	write(t, p, `{"roles":{"orchestrator":{"harness":"pig","model":"pig-model","effort":"high"}}}`)
	if got := topLevelChoice(el.Choice{Harness: "claude"}); got != (el.Choice{Harness: "claude", Model: "sonnet"}) {
		t.Errorf("orchestrator slot with --claude = %+v, want {claude}", got)
	}
}

// --effort is a launcher flag, and the harness turns it into its own: a pig
// session launches with --thinking <level>.
func TestEffortFlagReachesTheHarness(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("HERDR_PANE_ID", "")
	out := filepath.Join(home, "args")
	script := filepath.Join(home, "pig")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > "+out+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EL_PIG", script)
	if err := run([]string{"-p", "--effort", "high", "what is it"}); err != nil {
		t.Fatalf("run with --effort: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Fields(string(got))
	i := indexOf(args, "--thinking")
	if i < 0 || i+1 >= len(args) || args[i+1] != "high" {
		t.Errorf("pig must launch with --thinking high, got %q", string(got))
	}
}

func indexOf(values []string, want string) int {
	for i, v := range values {
		if v == want {
			return i
		}
	}
	return -1
}

// `el exec` is the one line every pane types: it takes exactly one
// launch file. The argv/env round trip itself is covered in the el package;
// here the dispatch and its usage error are pinned.
func TestExecNeedsExactlyOneLaunchFile(t *testing.T) {
	for _, argv := range [][]string{{"exec"}, {"exec", "a", "b"}} {
		if err := run(argv); err == nil || !strings.Contains(err.Error(), "usage") {
			t.Errorf("run(%q) must fail with usage, got %v", argv, err)
		}
	}
}

func TestExecFailsOnAMissingLaunchFile(t *testing.T) {
	if err := run([]string{"exec", filepath.Join(t.TempDir(), "no-such-file.json")}); err == nil {
		t.Error("exec of a missing launch file must fail")
	}
}

// `el install` is the one command that replaces the installed binaries.
// Here its dispatch and its usage error are pinned; the gate itself is
// covered in the el package. The unknown flag fails before any exec, so
// this runs no herdr and no builder.
func TestInstallRefusesUnknownFlags(t *testing.T) {
	if err := run([]string{"install", "--bogus"}); err == nil || !strings.Contains(err.Error(), "usage") {
		t.Errorf("run(install --bogus) must fail with usage, got %v", err)
	}
}

// `el tui` opens the dashboard with no arguments. Here its dispatch
// and its usage error are pinned; the binary lookup and starter are covered
// in the el package. The unknown arg fails before any exec, so this starts
// no terminal.
func TestTuiRefusesArgs(t *testing.T) {
	if err := run([]string{"tui", "--bogus"}); err == nil || !strings.Contains(err.Error(), "usage") {
		t.Errorf("run(tui --bogus) must fail with usage, got %v", err)
	}
}

// `el deps` checks external dependencies. Here its dispatch and usage
// error are pinned; the installer itself is covered in the el package. The
// unknown flag fails before any install or download.
func TestDepsRefusesUnknownFlags(t *testing.T) {
	if err := run([]string{"deps", "--bogus"}); err == nil {
		t.Error("run(deps --bogus) must fail")
	}
}

// --driver and --llm pick the implement mode explicitly, so pairing them is
// refused before anything runs.
func TestImplementDriverAndLlmExcludeEachOther(t *testing.T) {
	err := run([]string{"implement", "--driver", "--llm", "some-spec"})
	if err == nil || !strings.Contains(err.Error(), "exclude") {
		t.Fatalf("implement --driver --llm must be refused, got %v", err)
	}
}

// --check is the driver's graph check, so it needs --driver.
func TestImplementCheckNeedsDriver(t *testing.T) {
	err := run([]string{"implement", "--check", "some-spec"})
	if err == nil || !strings.Contains(err.Error(), "--check needs --driver") {
		t.Fatalf("implement --check without --driver must be refused, got %v", err)
	}
}

// The check still needs a spec to read.
func TestImplementDriverCheckNeedsASpec(t *testing.T) {
	err := run([]string{"implement", "--driver", "--check"})
	if err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("implement --driver --check without a spec must fail with usage, got %v", err)
	}
}

// --llm keeps the default path: outside a herdr pane it fails naming herdr,
// like a bare run.
func TestImplementLlmKeepsTheOrchestratorPath(t *testing.T) {
	t.Setenv("HERDR_PANE_ID", "")
	err := run([]string{"implement", "--llm", "/nonexistent/spec.md"})
	if err == nil || !strings.Contains(err.Error(), "herdr") {
		t.Fatalf("implement --llm outside herdr must fail naming herdr, got %v", err)
	}
}

// writeSpec lays out a spec dir with an issues directory holding the given
// ticket files, and returns the spec dir for `implement --driver --check`.
func writeSpec(t *testing.T, docs map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "spec.md"), []byte("# Spec\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	issues := filepath.Join(dir, "issues")
	if err := os.MkdirAll(issues, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, doc := range docs {
		if err := os.WriteFile(filepath.Join(issues, name), []byte(doc), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// captureStdout runs fn with os.Stdout pointed at a pipe and returns what it
// wrote.
func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()
	runErr := fn()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = old
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out), runErr
}

// --driver --check prints the start order and needs no herdr pane: it reads
// files and starts no worker.
func TestImplementDriverCheckPrintsStartOrder(t *testing.T) {
	t.Setenv("HERDR_PANE_ID", "")
	dir := writeSpec(t, map[string]string{
		"01-first.md":  "# 01\n\n**Blocked by:** None (can start immediately)\n",
		"02-second.md": "# 02\n\n**Blocked by:** 01\n",
	})
	out, err := captureStdout(t, func() error {
		return run([]string{"implement", "--driver", "--check", dir})
	})
	if err != nil {
		t.Fatalf("implement --driver --check: %v", err)
	}
	if out != "01-first\n02-second\n" {
		t.Errorf("check printed %q, want %q", out, "01-first\n02-second\n")
	}
}

// --driver --check fails a bad graph before any worker would start, still with
// no herdr pane.
func TestImplementDriverCheckFailsABadGraph(t *testing.T) {
	t.Setenv("HERDR_PANE_ID", "")
	dir := writeSpec(t, map[string]string{
		"01-first.md":  "# 01\n\n**Blocked by:** None\n",
		"02-second.md": "# 02\n\n**Blocked by:** 09 Typo\n",
	})
	_, err := captureStdout(t, func() error {
		return run([]string{"implement", "--driver", "--check", dir})
	})
	if err == nil || !strings.Contains(err.Error(), "09") {
		t.Fatalf("check of a bad graph must fail naming the unknown id, got %v", err)
	}
}

// `implement --driver` without --check runs the driver loop: in a fresh repo
// with no herdr workspace the first worker fails fast naming the workspace,
// which proves dispatch reached the live run rather than the old stub.
func TestImplementDriverRunStartsWorkersThroughTheLiveLoop(t *testing.T) {
	t.Setenv("HERDR_PANE_ID", "w1:p1")
	t.Setenv("HERDR_WORKSPACE_ID", "")
	t.Setenv("HOME", t.TempDir())
	repo := t.TempDir()
	if out, err := exec.Command("git", "-C", repo, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	dir := writeSpec(t, map[string]string{
		"01-first.md": "# 01\n\n**Blocked by:** None\n",
	})
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	err = run([]string{"implement", "--driver", dir})
	if err == nil || !strings.Contains(err.Error(), "HERDR_WORKSPACE_ID") {
		t.Fatalf("implement --driver must reach the live run, got %v", err)
	}
}

// The driver checks the graph first: a bad graph fails on the graph, never
// reaching the live run.
func TestImplementDriverRunChecksTheGraphFirst(t *testing.T) {
	t.Setenv("HERDR_PANE_ID", "w1:p1")
	dir := writeSpec(t, map[string]string{
		"01-first.md": "# 01\n\n**Blocked by:** 09 Typo\n",
	})
	err := run([]string{"implement", "--driver", dir})
	if err == nil || !strings.Contains(err.Error(), "09") {
		t.Fatalf("implement --driver on a bad graph must fail naming the unknown id, got %v", err)
	}
}
