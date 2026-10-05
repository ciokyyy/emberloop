package wick

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShellArgQuotesOnlyWhenNeeded(t *testing.T) {
	if got := shellArg(""); got != `""` {
		t.Errorf("an empty argument must survive the shell, got %q", got)
	}
	if got := shellArg("--no-skills"); got != "--no-skills" {
		t.Errorf("a plain flag must not be quoted, got %q", got)
	}
	if got := shellArg("Read /a b/c.md and follow it."); !strings.HasPrefix(got, `"`) || !strings.HasSuffix(got, `"`) {
		t.Errorf("an argument with spaces must be quoted, got %q", got)
	}
	if got := shellArg(`say "hi"`); strings.Count(got, `"`) != 4 {
		t.Errorf("embedded quotes must be escaped, got %q", got)
	}
}

func TestWriteBriefPointsAtSkillsWithoutCopyingThem(t *testing.T) {
	t.Setenv("WICK_STATE", filepath.Join(t.TempDir(), "workers.json"))

	path, err := writeBrief("01-probe", "/repo/.scratch/p/issues/01-probe.md")
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)

	for _, want := range []string{"wick://matt-pocock/implement/SKILL.md", "wick://matt-pocock/tdd/SKILL.md"} {
		if !strings.Contains(text, want) {
			t.Errorf("brief must name %s:\n%s", want, text)
		}
	}
	implement := strings.Index(text, "wick://matt-pocock/implement/SKILL.md")
	tdd := strings.Index(text, "wick://matt-pocock/tdd/SKILL.md")
	if !(implement < tdd) {
		t.Errorf("order must be implement -> tdd, got %d/%d", implement, tdd)
	}
	if strings.Contains(text, "/skills/matt-pocock") {
		t.Errorf("brief must not name an absolute library path:\n%s", text)
	}
	if strings.Contains(text, "code-review") {
		t.Errorf("a worker must not review itself; the reviewer does:\n%s", text)
	}
	if strings.Contains(text, "Commit your work to the current branch") {
		t.Errorf("brief copied skill content instead of referencing it:\n%s", text)
	}
}

// Every harness reads skills through the read tool under one wick:// address:
// no session loads the library itself.
func TestBriefIsTheSameForEveryHarness(t *testing.T) {
	t.Setenv("WICK_STATE", filepath.Join(t.TempDir(), "workers.json"))
	path, err := writeBrief("01-probe", "/repo/.scratch/p/issues/01-probe.md")
	if err != nil {
		t.Fatal(err)
	}
	text := readFile(t, path)
	if !strings.Contains(text, "wick://matt-pocock/implement/SKILL.md") {
		t.Errorf("brief must name the implement skill by its wick:// address:\n%s", text)
	}
}

func TestBackgroundRuleReachesBriefAndMerger(t *testing.T) {
	t.Setenv("WICK_STATE", filepath.Join(t.TempDir(), "workers.json"))

	path, err := writeBrief("01-probe", "/repo/.scratch/p/issues/01-probe.md")
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "background") {
		t.Errorf("a worker brief must carry the background rule, it only lives in the system prompt:\n%s", body)
	}

	if got := implementPrompt("/repo/.scratch/p/issues"); !strings.Contains(got, "background") {
		t.Errorf("the merger prompt must carry the background rule for anything it runs:\n%s", got)
	}
}

// The merger never reviews: each worker already ran code_review on its own
// work, and a second review per worker only spends the merger's tokens.
func TestImplementPromptDoesNotReview(t *testing.T) {
	got := implementPrompt("/repo/.scratch/p/issues")
	if strings.Contains(got, "review_worker") || strings.Contains(strings.ToLower(got), "verdict") {
		t.Errorf("the merger prompt must not ask for a review:\n%s", got)
	}
}

func TestRenderStatusIsProse(t *testing.T) {
	if got := renderStatus(nil, nil, nil); !strings.Contains(got, "no workers") {
		t.Errorf("empty status must say so plainly, got %q", got)
	}
	workers := []Status{{ID: "01-a", State: "idle", Detail: "done"}}
	got := renderStatus(workers, nil, nil)
	if !strings.Contains(got, "01-a") || !strings.Contains(got, "idle") || !strings.Contains(got, "finish_worker") {
		t.Errorf("status must name the worker, its state and the next call, got %q", got)
	}
	if got := renderStatus([]Status{{ID: "01-a", State: "working"}}, []string{"01-a"}, nil); !strings.Contains(got, "still running") {
		t.Errorf("pending workers must be reported as such, got %q", got)
	}
}

func TestRenderFinishIsProse(t *testing.T) {
	if got := renderFinish(&FinishResult{ID: "01-a", Merged: true, SHA: "abc123"}); !strings.Contains(got, "abc123") {
		t.Errorf("a merge must report the sha, got %q", got)
	}
	kept := renderFinish(&FinishResult{ID: "01-a", Detail: "rebase failed", Kept: true})
	if !strings.Contains(kept, "NOT merge") || !strings.Contains(kept, "rebase failed") {
		t.Errorf("a conflict must be reported and escalated, got %q", kept)
	}
}

func TestReportExtensionPrefersTheEnvVar(t *testing.T) {
	t.Setenv("WICK_HERDR_REPORT", "/exts/from-env")
	if got := reportExtension("/opt/wick/bin/pig", "/home/cioky"); got != "/exts/from-env" {
		t.Errorf("a set env var must win over any directory on disk, got %q", got)
	}
}

func TestReportExtensionFallsBackToTheFirstDirectoryThatExists(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	bin := filepath.Join(root, "opt", "wick", "bin")
	mk := func(p string) string {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}

	t.Setenv("WICK_HERDR_REPORT", "")
	exe := filepath.Join(bin, "pig")

	if got := reportExtension(exe, home); got != "" {
		t.Errorf("with nothing on disk the lookup must return empty, got %q", got)
	}

	homeExt := mk(filepath.Join(home, ".pig", "agent", "extensions", "herdr-report"))
	if got := reportExtension(exe, home); got != homeExt {
		t.Errorf("with only the home extension present, got %q want %q", got, homeExt)
	}

	beside := mk(filepath.Join(bin, "herdr-report"))
	if got := reportExtension(exe, home); got != beside {
		t.Errorf("the extension beside the executable must beat the home one, got %q", got)
	}

	above := mk(filepath.Join(root, "opt", "wick", "herdr-report"))
	if got := reportExtension(exe, home); got != above {
		t.Errorf("../herdr-report must be the first candidate, got %q", got)
	}
}

func TestRenderStatusExplainsAllUnknownWorkers(t *testing.T) {
	unknown := []Status{{ID: "01-a", State: "unknown"}, {ID: "01-b", State: "unknown"}}
	candidates := []string{"/opt/wick/herdr-report", "/opt/wick/bin/herdr-report", "/home/u/.pig/agent/extensions/herdr-report"}
	got := renderStatus(unknown, []string{"01-a", "01-b"}, candidates)
	if !strings.Contains(got, "not registered with herdr") || !strings.Contains(got, "report extension") {
		t.Errorf("workers herdr never heard of must be diagnosed, got %q", got)
	}
	for _, c := range candidates {
		if !strings.Contains(got, c) {
			t.Errorf("the diagnosis must name every place searched, missing %q in %q", c, got)
		}
	}

	oneKnown := []Status{{ID: "01-a", State: "unknown"}, {ID: "01-b", State: "working"}}
	if got := renderStatus(oneKnown, []string{"01-a", "01-b"}, candidates); strings.Contains(got, "not registered with herdr") {
		t.Errorf("a worker herdr can see means the others are just slow, got %q", got)
	}

	settled := renderStatus([]Status{{ID: "01-a", State: "merged"}}, nil, candidates)
	if strings.Contains(settled, "not registered with herdr") {
		t.Errorf("nothing is pending, so nothing is unexplained, got %q", settled)
	}
}

// reportExtensionWarning is the spawn-side counterpart to reportExtension: it
// must warn only for the harness that needs the extension, and only when the
// lookup really comes up empty.
func TestReportExtensionWarning(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WICK_HERDR_REPORT", "")

	if got := reportExtensionWarning("pig"); !strings.Contains(got, "unknown") {
		t.Errorf("a pig spawn with no extension must warn, got %q", got)
	}
	if got := reportExtensionWarning("claude"); got != "" {
		t.Errorf("only pig registers through the extension, got %q", got)
	}

	ext := filepath.Join(home, ".pig", "agent", "extensions", "herdr-report")
	if err := os.MkdirAll(ext, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := reportExtensionWarning("pig"); got != "" {
		t.Errorf("a found extension must not warn, got %q", got)
	}
}
