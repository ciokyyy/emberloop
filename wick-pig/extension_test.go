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
	if !(implement < tdd) {
		t.Errorf("order must be implement -> tdd, got %d/%d", implement, tdd)
	}
	if strings.Contains(text, "code-review") {
		t.Errorf("a worker must not review itself; the reviewer does:\n%s", text)
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
