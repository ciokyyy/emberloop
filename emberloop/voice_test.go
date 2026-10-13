package el

import (
	"encoding/json"
	"strings"
	"testing"
)

// Every el session reads the register as part of its system prompt, so no
// role is left out and no skill body can outrank it.
func TestSystemPromptCarriesTheRegister(t *testing.T) {
	if !strings.Contains(systemPrompt, voiceRegister) {
		t.Fatal("the system prompt must carry the register in full")
	}
	if !strings.Contains(SystemPrompt(), voiceRegister) {
		t.Fatal("the served system prompt must carry the register in full")
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

// The per-turn reminder is the plugin's ultra line, served plain or as the
// claude UserPromptSubmit hook's JSON.
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
}

// The register covers replies and submit text, not the files an agent writes.
func TestRegisterLeavesWrittenArtifactsAlone(t *testing.T) {
	if !strings.Contains(voiceRegister, "write normal prose code, comments, commits, docs, issue/PR/MR/defect/ticket/bug-report text") {
		t.Error("the register must exempt code, docs, commit messages and tickets")
	}
}
