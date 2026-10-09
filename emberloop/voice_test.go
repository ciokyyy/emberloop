package el

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every el session reads the register as part of its system prompt, under
// both harnesses, so no role or agent type is left out and no skill body can
// outrank it.
func TestSystemPromptCarriesTheRegisterOnBothHarnesses(t *testing.T) {
	if !strings.Contains(systemPrompt, voiceRegister) {
		t.Fatal("the system prompt must carry the register in full")
	}
	for name, h := range map[string]Harness{"pig": pigHarness{}, "claude": claudeHarness{}} {
		isolateState(t)
		cmd, err := h.Command(Session{Role: "work", Cwd: t.TempDir(), ID: "voice-" + name})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !strings.Contains(strings.Join(cmd.Args, "\x00"), voiceRegister) {
			t.Errorf("%s launch must pass the register in its system prompt args", name)
		}
	}
}

// The register is the plugin's text at ultra and only ultra: the banner, the
// skill body byte for byte, and one intensity row and one example per prompt.
func TestRegisterIsThePluginSkillAtUltra(t *testing.T) {
	if !strings.HasPrefix(voiceRegister, "CAVEMAN MODE ACTIVE \u2014 level: ultra\n\n") {
		t.Errorf("the register must open with the plugin banner, got %.60q", voiceRegister)
	}
	for _, want := range []string{
		"Respond terse like smart caveman. All technical substance stay. Only fluff die.",
		"Clarity register: mix ASD-STE100 Simplified Technical English into caveman, always.",
		"| **ultra** | Strip conjunctions when cause-then-effect stay unambiguous.",
		"- ultra: \"Inline obj prop, new ref, re-render. `useMemo`.\"",
		"Persisted outside chat: write normal prose code, comments, commits, docs",
	} {
		if !strings.Contains(voiceRegister, want) {
			t.Errorf("the register must carry the plugin line %q", want)
		}
	}
	for _, gone := range []string{"| **lite** |", "| **full** |", "| **wenyan", "- lite:", "- full:", "- wenyan", "name: caveman"} {
		if strings.Contains(voiceRegister, gone) {
			t.Errorf("the register must carry only the ultra level, found %q", gone)
		}
	}
}

// The per-turn reminder is the plugin's ultra line, and both harnesses send
// it: claude through a UserPromptSubmit hook, pig through the extension that
// asks `el voice-reminder`.
func TestVoiceReminderReachesEveryTurn(t *testing.T) {
	want := "CAVEMAN MODE ACTIVE (ultra). Enforce this reply: Drop articles, filler, pleasantries, hedging, and excess conjunctions. " +
		"Prefer fragments over full natural-prose sentences. State each fact once. No preamble or recap. " +
		"Technical terms, code, commands, paths, and errors stay exact."
	if VoiceReminder() != want {
		t.Errorf("reminder is not the plugin's ultra line:\n%s", VoiceReminder())
	}
	var plain, hook strings.Builder
	if err := RunVoiceReminder(nil, &plain); err != nil || plain.String() != want {
		t.Errorf("plain output = %q, %v", plain.String(), err)
	}
	if err := RunVoiceReminder([]string{"hook"}, &hook); err != nil {
		t.Fatal(err)
	}
	var out struct {
		Hook struct {
			Event   string `json:"hookEventName"`
			Context string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(hook.String()), &out); err != nil {
		t.Fatalf("hook output is not JSON: %v", err)
	}
	if out.Hook.Event != "UserPromptSubmit" || out.Hook.Context != want {
		t.Errorf("hook output = %+v", out.Hook)
	}
	var cfg struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(claudeStopGate), &cfg); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(claudeStopGate, `"awaySummaryEnabled":false`) {
		t.Error("claude settings must turn the away recap off")
	}
	if got := cfg.Hooks["UserPromptSubmit"]; len(got) != 1 || got[0].Hooks[0].Command != "el voice-reminder hook" {
		t.Errorf("claude settings must run the reminder hook on every prompt, got %+v", got)
	}
}

// The register covers replies and submit text, not the files an agent writes.
func TestRegisterLeavesWrittenArtifactsAlone(t *testing.T) {
	if !strings.Contains(voiceRegister, "write normal prose code, comments, commits, docs, issue/PR/MR/defect/ticket/bug-report text") {
		t.Error("the register must exempt code, docs, commit messages and tickets")
	}
}

// Submit is the whole answer: its description says to call it last and write
// nothing after it, and the reviewer types say the same, so the report is not
// restated in chat after it.
func TestSubmitIsTheWholeAnswer(t *testing.T) {
	for _, name := range []string{"worker", "reviewer-spec", "reviewer-standards", "scout"} {
		types := mustLoadAgentTypes(t.TempDir())
		at, ok := types[name]
		if !ok {
			t.Fatalf("no built-in agent type %q", name)
		}
		if !strings.Contains(submitDescription(at), "Submit is your whole answer") {
			t.Errorf("%s: submit description must say submit is the whole answer", name)
		}
	}
	for _, name := range []string{"reviewer-spec", "reviewer-standards"} {
		raw, err := os.ReadFile(filepath.Join("agenttypes", name+".md"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), "end the turn right after the submit call, with no text after it") {
			t.Errorf("%s body must tell the reviewer to stop after submit", name)
		}
	}
}

// Declined findings travel in the submit summary: the parent reads that and
// never a worker's closing message, so the brief must not ask for one.
func TestBriefPutsDeclinedFindingsInTheSubmitSummary(t *testing.T) {
	t.Setenv("EL_STATE", filepath.Join(t.TempDir(), "workers.json"))
	path, err := writeBrief("01-probe", "/repo/.scratch/p/issues/01-probe.md", "/repo/.scratch/p/spec.md", "spec/p", "pig")
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if strings.Contains(text, "final message") {
		t.Errorf("the brief must not ask for a final message:\n%s", text)
	}
	if !strings.Contains(text, "in your submit summary") {
		t.Errorf("the brief must send declined findings to the submit summary:\n%s", text)
	}
}

// A claude agent never writes the away recap: it is one more unrequested
// message, and its pane has no person to welcome back.
func TestClaudeLaunchDisablesTheAwayRecap(t *testing.T) {
	isolateState(t)
	cmd, err := claudeHarness{}.Command(Session{Role: "work", Cwd: t.TempDir(), ID: "recap"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range cmd.Env {
		if e == "CLAUDE_CODE_ENABLE_AWAY_SUMMARY=0" {
			found = true
		}
	}
	if !found {
		t.Errorf("claude env must switch the away recap off, got %v", cmd.Env)
	}
}
