package wick

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every wick session reads the register as part of its system prompt, under
// both harnesses, so no role or agent type is left out and no skill body can
// outrank it. There is no switch and no reminder.
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
	if strings.Contains(strings.ToLower(voiceRegister), "caveman mode") || strings.Contains(voiceRegister, "/caveman") {
		t.Error("the register has no switch to name")
	}
}

// The register covers replies and submit text, not the files an agent writes.
func TestRegisterLeavesWrittenArtifactsAlone(t *testing.T) {
	if !strings.Contains(voiceRegister, "code, comments, docs, commit messages, tickets and specs in normal clear prose") {
		t.Error("the register must exempt code, docs, commit messages, tickets and specs")
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
	t.Setenv("WICK_STATE", filepath.Join(t.TempDir(), "workers.json"))
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
