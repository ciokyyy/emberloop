package wick

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkerCommandStartsWithTheLabelThenTheHarness(t *testing.T) {
	t.Setenv("WICK_SKILLS_DIR", "/skills")

	// pane run types its argv into the pane's shell, so the first word is what
	// the shell runs: an env assignment, then the program.
	for _, harness := range []string{"pig", "claude"} {
		argv := launchCommand(harness, "", "/state/briefs/01-x.md", "01-x")
		if len(argv) < 2 {
			t.Fatalf("%s: argv too short: %v", harness, argv)
		}
		if argv[0] != "WICK_AGENT_LABEL=01-x" {
			t.Errorf("%s: argv[0] = %q, want the agent label assignment", harness, argv[0])
		}
		if argv[1] != harness {
			t.Errorf("%s: argv[1] = %q, want the harness (pane run executes the first word)", harness, argv[1])
		}
	}
}

func TestWorkerCommandSuppressesOtherSkills(t *testing.T) {
	t.Setenv("WICK_SKILLS_DIR", "/skills")
	t.Setenv("WICK_HERDR_REPORT", "")

	argv := launchCommand("pig", "", "/state/briefs/01-x.md", "01-x")
	want := map[string]bool{"/skills/implement": false, "/skills/tdd": false, "/skills/code-review": false}
	for i, a := range argv {
		if a != "--skill" || i+1 >= len(argv) {
			continue
		}
		if _, ok := want[argv[i+1]]; ok {
			want[argv[i+1]] = true
		}
	}
	for dir, seen := range want {
		if !seen {
			t.Errorf("worker never offered %s; argv = %v", dir, argv)
		}
	}
	if !contains([]string{"--no-skills"}, argv[2]) {
		t.Errorf("--no-skills must follow the harness, got %v", argv)
	}
}

func TestWorkerCommandLoadsHerdrReport(t *testing.T) {
	t.Setenv("WICK_SKILLS_DIR", "/skills")
	t.Setenv("WICK_HERDR_REPORT", "/exts/herdr-report")

	argv := launchCommand("pig", "", "/state/briefs/01-x.md", "01-x")
	joined := strings.Join(argv, " ")
	if !strings.Contains(joined, "-e /exts/herdr-report") {
		t.Errorf("worker must load the reporting extension or herdr cannot see it as an agent: %v", argv)
	}
}

func TestWorkerCommandIsOneShellLine(t *testing.T) {
	t.Setenv("WICK_SKILLS_DIR", "/skills")

	argv := launchCommand("pig", "", "/state/briefs/01-x.md", "01-x")
	joined := strings.Join(argv, " ")
	if strings.ContainsAny(joined, "\n") {
		t.Errorf("pane run types argv as a single line; it must contain no newline: %q", joined)
	}
	// The brief is a quoted path, not inline prose, because newlines cannot
	// survive the trip.
	last := argv[len(argv)-1]
	if !strings.HasPrefix(last, `"`) || !strings.Contains(last, "/state/briefs/01-x.md") {
		t.Errorf("final argument must be a quoted reference to the brief, got %q", last)
	}
}

func TestNonPigHarnessGetsNoPigFlags(t *testing.T) {
	t.Setenv("WICK_SKILLS_DIR", "/skills")
	t.Setenv("WICK_HERDR_REPORT", "/exts/herdr-report")

	argv := launchCommand("claude", "some-model", "/state/briefs/01-x.md", "01-x")
	for _, a := range argv {
		if a == "--no-skills" || a == "--skill" || a == "-e" {
			t.Errorf("pig-only flag %q leaked into a claude invocation: %v", a, argv)
		}
	}
	if !strings.Contains(strings.Join(argv, " "), "some-model") {
		t.Errorf("model not forwarded: %v", argv)
	}
}

func TestShellArgQuotesOnlyWhenNeeded(t *testing.T) {
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
	t.Setenv("WICK_SKILLS_DIR", "/skills")
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

	if !strings.Contains(text, "/skills/implement/SKILL.md") {
		t.Errorf("brief must name the implement skill file:\n%s", text)
	}
	implement := strings.Index(text, "/skills/implement/SKILL.md")
	tdd := strings.Index(text, "/skills/tdd/SKILL.md")
	review := strings.Index(text, "/skills/code-review/SKILL.md")
	if !(implement < tdd && tdd < review) {
		t.Errorf("order must be implement -> tdd -> code-review, got %d/%d/%d", implement, tdd, review)
	}
	if strings.Contains(text, "Commit your work to the current branch") {
		t.Errorf("brief copied skill content instead of referencing it:\n%s", text)
	}
}

func TestRenderStatusIsProse(t *testing.T) {
	if got := renderStatus(nil, nil); !strings.Contains(got, "no workers") {
		t.Errorf("empty status must say so plainly, got %q", got)
	}
	workers := []Status{{ID: "01-a", State: "idle", Detail: "done"}}
	got := renderStatus(workers, nil)
	if !strings.Contains(got, "01-a") || !strings.Contains(got, "idle") || !strings.Contains(got, "finish_worker") {
		t.Errorf("status must name the worker, its state and the next call, got %q", got)
	}
	if got := renderStatus([]Status{{ID: "01-a", State: "working"}}, []string{"01-a"}); !strings.Contains(got, "still running") {
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
