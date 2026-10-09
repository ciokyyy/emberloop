package el

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func workerSession() Session {
	return Session{Role: "work", Label: "01-x", Prompt: "Read /state/briefs/01-x.md and follow it exactly."}
}

// isolateState keeps a build that writes harness files (claude's MCP config)
// out of the real worker store.
func isolateState(t *testing.T) {
	t.Helper()
	statePath = filepath.Join(t.TempDir(), "workers.json")
	// Hermetic panes: the runner itself sits in a herdr pane, so a leaked
	// parent would send grouped delivery at the real herdr and hang.
	t.Setenv("HERDR_PANE_ID", "")
	t.Setenv("HERDR_TAB_ID", "")
	t.Setenv("EL_AGENT_ID", "")
}

func TestEveryHarnessBuildsOneShellLineWithRoleFirst(t *testing.T) {
	t.Setenv("EL_SKILLS_DIR", "/skills")
	isolateState(t)
	for _, name := range HarnessNames() {
		cmd, err := BuildCommand(name, workerSession())
		if err != nil {
			t.Fatal(err)
		}
		line := cmd.Line()
		// The system prompt holds newlines (the register is the plugin's
		// text); the launch file carries them, so the line the pane types
		// is only the exec line, and that stays one line.
		launch, err := PrepareLaunch(name, workerSession())
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(launch.Line, "\n") {
			t.Errorf("%s: pane run types one line: %q", name, launch.Line)
		}
		if !strings.HasPrefix(line, "EL_ROLE=work ") && !strings.Contains(line, " EL_ROLE=work ") {
			t.Errorf("%s: role missing: %q", name, line)
		}
		if !strings.HasSuffix(line, `"Read /state/briefs/01-x.md and follow it exactly."`) {
			t.Errorf("%s: prompt must end the line, quoted: %q", name, line)
		}
	}
}

// Every session, pig or claude, must be able to find the skills it is told
// about: the one address the read tool serves the library under.
func TestBothHarnessesCarryTheSkillLibraryAddress(t *testing.T) {
	isolateState(t)
	t.Setenv("EL_SKILLS_DIR", "/skills")
	for _, name := range HarnessNames() {
		cmd, err := BuildCommand(name, Session{Role: "full"})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(cmd.Line(), "el://matt-pocock") {
			t.Errorf("%s: the system prompt must name the skill library, got %q", name, cmd.Line())
		}
	}
}

// No harness loads global skills: pig keeps --no-skills and loads nothing from
// the library, so a session sees only what its prompt points it at.
func TestPigWorkerGetsNoSkillsAndTheReporter(t *testing.T) {
	t.Setenv("EL_SKILLS_DIR", "/skills")
	t.Setenv("EL_HERDR_REPORT", "/exts/herdr-report")
	cmd, _ := BuildCommand("pig", workerSession())
	line := cmd.Line()
	for _, want := range []string{"--no-skills", "-e /exts/herdr-report", "--session-id 01-x",
		"EL_AGENT_LABEL=01-x", "--no-builtin-tools"} {
		if !strings.Contains(line, want) {
			t.Errorf("missing %q in %s", want, line)
		}
	}
	if strings.Contains(line, "--skill") {
		t.Errorf("pig must load no skills of its own: %s", line)
	}
}

func TestClaudeRunsStripped(t *testing.T) {
	isolateState(t)
	cmd, _ := BuildCommand("claude", Session{Role: "review", Model: "m", Prompt: "p"})
	line := cmd.Line()
	for _, want := range []string{`--tools ""`, `--setting-sources ""`, "--strict-mcp-config", "--model m",
		"CLAUDE_CODE_DISABLE_AUTO_MEMORY=1",
		"CLAUDE_CODE_DISABLE_BACKGROUND_TASKS=1", "CLAUDE_CODE_MCP_AUTO_BACKGROUND_MS=0"} {
		if !strings.Contains(line, want) {
			t.Errorf("missing %q in %s", want, line)
		}
	}
	for _, leak := range []string{"--skill", "--no-skills", "-e "} {
		if strings.Contains(line, leak) {
			t.Errorf("pig-only flag %q leaked: %s", leak, line)
		}
	}
	// The Stop hook is passed inline, and it still applies under
	// --setting-sources "": a stop without a submit runs el's gate.
	settings := argValue(cmd.Args, "--settings")
	var cfg struct {
		Hooks struct {
			Stop []struct {
				Hooks []struct {
					Type    string `json:"type"`
					Command string `json:"command"`
				} `json:"hooks"`
			} `json:"Stop"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(settings), &cfg); err != nil {
		t.Fatalf("--settings %q is not JSON: %v", settings, err)
	}
	if settings == "" || len(cfg.Hooks.Stop) == 0 || len(cfg.Hooks.Stop[0].Hooks) == 0 ||
		cfg.Hooks.Stop[0].Hooks[0].Type != "command" ||
		!strings.Contains(cfg.Hooks.Stop[0].Hooks[0].Command, "stop-gate") {
		t.Errorf("--settings must carry a Stop hook that runs the stop-gate: %s", settings)
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

// Effort is the harness's own word and becomes its own flag: pig's --thinking
// and claude's --effort. No effort means no flag.
func TestBuildCommandTurnsEffortIntoTheHarnessFlag(t *testing.T) {
	isolateState(t)
	pig, err := BuildCommand("pig", Session{Role: "work", Effort: "high"})
	if err != nil {
		t.Fatal(err)
	}
	if got := argValue(pig.Args, "--thinking"); got != "high" {
		t.Errorf("pig effort must be --thinking high, got %q", got)
	}
	if strings.Contains(pig.Line(), "--effort") {
		t.Errorf("claude's flag must not leak into pig: %s", pig.Line())
	}
	cl, err := BuildCommand("claude", Session{Role: "review", Effort: "minimal"})
	if err != nil {
		t.Fatal(err)
	}
	if got := argValue(cl.Args, "--effort"); got != "minimal" {
		t.Errorf("claude effort must be --effort minimal, got %q", got)
	}
	if strings.Contains(cl.Line(), "--thinking") {
		t.Errorf("pig's flag must not leak into claude: %s", cl.Line())
	}
	none, _ := BuildCommand("pig", Session{Role: "work"})
	if strings.Contains(none.Line(), "--thinking") {
		t.Errorf("no effort must add no flag: %s", none.Line())
	}
}

// --claude against a pig orchestrator slot launches claude with no --model or
// --effort: the resolver drops the pig slot's model and effort, so claude
// starts on its own defaults instead of a pig model it cannot run.
func TestClaudeFlagDropsThePigSlotModelAtLaunch(t *testing.T) {
	isolateState(t)
	resolverSettings(t, `{"roles":{"orchestrator":{"harness":"pig","model":"pig-model","effort":"high"}}}`)
	got := Resolve("orchestrator", Choice{Harness: "claude"}, Choice{})
	if got != (Choice{Harness: "claude"}) {
		t.Fatalf("Resolve = %+v, want {claude}", got)
	}
	cmd, err := BuildCommand(got.Harness, Session{Role: "plan", Model: got.Model, Effort: got.Effort})
	if err != nil {
		t.Fatal(err)
	}
	if v := argValue(cmd.Args, "--model"); v != "" {
		t.Errorf("claude launch must carry no --model, got %q in %q", v, cmd.Args)
	}
	if v := argValue(cmd.Args, "--effort"); v != "" {
		t.Errorf("claude launch must carry no --effort, got %q in %q", v, cmd.Args)
	}
}
