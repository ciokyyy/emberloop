package wick

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func workerSession() Session {
	return Session{Role: "work", Label: "01-x", Skills: RouteSkills(), Prompt: "Read /state/briefs/01-x.md and follow it exactly."}
}

// isolateState keeps a build that writes harness files (claude's MCP config)
// out of the real worker store.
func isolateState(t *testing.T) {
	t.Helper()
	statePath = filepath.Join(t.TempDir(), "workers.json")
}

func TestEveryHarnessBuildsOneShellLineWithRoleFirst(t *testing.T) {
	t.Setenv("WICK_SKILLS_DIR", "/skills")
	isolateState(t)
	for _, name := range HarnessNames() {
		cmd, err := BuildCommand(name, workerSession())
		if err != nil {
			t.Fatal(err)
		}
		line := cmd.Line()
		if strings.Contains(line, "\n") {
			t.Errorf("%s: pane run types one line: %q", name, line)
		}
		if !strings.HasPrefix(line, "WICK_ROLE=work ") && !strings.Contains(line, " WICK_ROLE=work ") {
			t.Errorf("%s: role missing: %q", name, line)
		}
		if !strings.HasSuffix(line, `"Read /state/briefs/01-x.md and follow it exactly."`) {
			t.Errorf("%s: prompt must end the line, quoted: %q", name, line)
		}
	}
}

func TestPigWorkerGetsRouteSkillsAndReporter(t *testing.T) {
	t.Setenv("WICK_SKILLS_DIR", "/skills")
	t.Setenv("WICK_HERDR_REPORT", "/exts/herdr-report")
	cmd, _ := BuildCommand("pig", workerSession())
	line := cmd.Line()
	for _, want := range []string{"--no-skills", "--skill /skills/matt-pocock/implement", "--skill /skills/matt-pocock/tdd",
		"-e /exts/herdr-report", "--session-id 01-x", "WICK_AGENT_LABEL=01-x", "--no-builtin-tools"} {
		if !strings.Contains(line, want) {
			t.Errorf("missing %q in %s", want, line)
		}
	}
}

func TestClaudeRunsStripped(t *testing.T) {
	isolateState(t)
	cmd, _ := BuildCommand("claude", Session{Role: "review", Model: "m", Prompt: "p"})
	line := cmd.Line()
	for _, want := range []string{`--tools ""`, `--setting-sources ""`, "--strict-mcp-config", "--model m",
		"CLAUDE_CODE_DISABLE_AUTO_MEMORY=1"} {
		if !strings.Contains(line, want) {
			t.Errorf("missing %q in %s", want, line)
		}
	}
	for _, leak := range []string{"--skill", "--no-skills", "-e "} {
		if strings.Contains(line, leak) {
			t.Errorf("pig-only flag %q leaked: %s", leak, line)
		}
	}
}

func TestUnknownHarnessIsAnError(t *testing.T) {
	if _, err := BuildCommand("nope", Session{}); err == nil {
		t.Error("want an error")
	}
}

func TestClaudeTrustWritesOnlyTheFlagAndKeepsTheRest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.WriteFile(filepath.Join(home, ".claude.json"), []byte(`{"theme":"dark","projects":{"/a":{"x":1}}}`), 0o600)
	if err := (claudeHarness{}).Trust("/b"); err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	b, _ := os.ReadFile(filepath.Join(home, ".claude.json"))
	json.Unmarshal(b, &cfg)
	p := cfg["projects"].(map[string]any)
	if p["/b"].(map[string]any)["hasTrustDialogAccepted"] != true || p["/a"].(map[string]any)["x"] != float64(1) || cfg["theme"] != "dark" {
		t.Errorf("config = %s", b)
	}
}
