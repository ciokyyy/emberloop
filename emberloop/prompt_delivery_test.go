package emberloop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A steering re-prompt sends the prompt text itself as the message: the agent
// spends no turn reading a file. The archive copy is still written.
func TestRepromptSendsPromptTextNotAPath(t *testing.T) {
	spawnTestState(t)
	root := t.TempDir()
	types := scoutTypeFile(t, root, "true")
	typ := types["scout"]

	s := AgentState{ID: "scout1", Type: "scout", Pane: "p1", Parent: "wP", ParentID: "p"}
	if err := WriteAgentState(s); err != nil {
		t.Fatal(err)
	}

	var sent []string
	run := func(cmd string, args []string, o ExecOpts) (Result, error) {
		if cmd == herdrBin() && len(args) > 2 && args[0] == "agent" && args[1] == "prompt" {
			sent = append(sent, args[3])
			return Result{}, nil
		}
		if cmd == herdrBin() && len(args) > 2 && args[0] == "pane" && args[1] == "send-text" {
			sent = append(sent, args[3])
			return Result{}, nil
		}
		if cmd == herdrBin() && len(args) > 2 && args[0] == "pane" && args[1] == "send-keys" {
			return Result{}, nil
		}
		return Result{Stdout: "{}"}, nil
	}

	got, err := repromptAgent(run, typ, s, spawnRequest{Type: "scout", Prompt: "second question"}, "g1")
	if err != nil {
		t.Fatalf("reprompt: %v", err)
	}
	if got.ID != "scout1" || !got.Reused {
		t.Fatalf("reprompt must reuse the live agent, got %+v", got)
	}
	if len(sent) != 1 {
		t.Fatalf("want exactly one delivered message, got %q", sent)
	}
	want := renderPrompt(typ, "second question")
	if sent[0] != want {
		t.Errorf("steering message must be the prompt text, got %q want %q", sent[0], want)
	}
	for _, bad := range []string{"Read ", "follow it exactly", ".prompt"} {
		if strings.Contains(sent[0], bad) {
			t.Errorf("steering message must never name a file, contains %q: %q", bad, sent[0])
		}
	}
	raw, err := os.ReadFile(filepath.Join(readOnlyDir(), "scout1.prompt"))
	if err != nil {
		t.Fatalf("archive copy must still be written: %v", err)
	}
	if string(raw) != want {
		t.Errorf("archive copy must hold the prompt text, got %q want %q", string(raw), want)
	}
}

// A reviewer follow-up sends the full follow-up text with the incremental
// diff inline as the message: no "read this file" instruction, no path, no
// size cap. The prompt and diff files stay as archive copies.
func TestFollowUpSendsDiffInlineNotAPath(t *testing.T) {
	_, tree := reviewRepo(t)
	f := newFakeReviewers(answers("pass", "changes"))
	if _, err := reviewSpawn(t, f, tree, bothReviewers()...); err != nil {
		t.Fatal(err)
	}
	changeTree(t, tree, "answer the spec findings")
	f.answer = answers("pass", "pass")
	if _, err := reviewSpawn(t, f, tree, withNote("B is out of scope: the ticket says A only")...); err != nil {
		t.Fatal(err)
	}
	spec := f.paneOf(t, reviewerSpec)
	if spec.turns != 2 {
		t.Fatalf("spec turns = %d, want 2", spec.turns)
	}
	msg := spec.prompts[1]
	if !strings.Contains(msg, "Follow-up") {
		t.Errorf("follow-up message must name itself, got:\n%s", msg)
	}
	if !strings.Contains(msg, "B is out of scope") {
		t.Errorf("follow-up message must carry the note, got:\n%s", msg)
	}
	if !strings.Contains(msg, "change-answer-the-spec-findings.go") {
		t.Errorf("follow-up message must carry the incremental diff inline, got:\n%s", msg)
	}
	for _, bad := range []string{"follow it", ".followup", ".since.diff", "Read /"} {
		if strings.Contains(msg, bad) {
			t.Errorf("follow-up message must never name a file, contains %q:\n%s", bad, msg)
		}
	}
	dir := filepath.Join(filepath.Dir(statePath), "reviews")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("archive copies must still be written: %v", err)
	}
	var havePrompt, haveDiff bool
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".followup") {
			havePrompt = true
		}
		if strings.HasSuffix(e.Name(), ".diff") {
			haveDiff = true
		}
	}
	if !havePrompt || !haveDiff {
		t.Errorf("prompt and diff archive copies must stay, got %v", entries)
	}
}

// Late-prompt delivery sends through the same shared prompt path: the full
// text arrives as one agent-prompt message with newlines intact, never typed
// line by line.
func TestLateDeliveryUsesTheSharedPromptPath(t *testing.T) {
	var prompted []string
	var typed int
	run := func(cmd string, args []string, o ExecOpts) (Result, error) {
		if cmd == herdrBin() && len(args) > 1 && args[0] == "agent" && args[1] == "prompt" {
			prompted = append(prompted, args[3])
			return Result{Stdout: "{}"}, nil
		}
		if cmd == herdrBin() && len(args) > 1 && args[0] == "agent" && args[1] == "wait" {
			return Result{Stdout: "{}"}, nil
		}
		if cmd == herdrBin() && len(args) > 1 && args[0] == "pane" {
			typed++
		}
		return Result{Stdout: "{}"}, nil
	}
	text := "line one\nline two"
	if err := DeliverLate(run, "p9", text); err != nil {
		t.Fatalf("DeliverLate: %v", err)
	}
	if typed != 0 {
		t.Errorf("late delivery must not fall back to typing, pane calls: %d", typed)
	}
	if len(prompted) != 1 {
		t.Fatalf("late delivery must prompt the pane exactly once, got %q", prompted)
	}
	if prompted[0] != text {
		t.Errorf("late delivery must carry the full text with newlines intact, got %q want %q", prompted[0], text)
	}
}

// The herdr fallback (a pig-reported pane, which herdr refuses to prompt)
// types the message as one flattened line and presses Enter. The words and
// their order survive, so the full prompt arrives complete, without newlines.
func TestDeliverPromptFallbackTypesOneCompleteLine(t *testing.T) {
	var texts []string
	var keys []string
	run := func(cmd string, args []string, o ExecOpts) (Result, error) {
		if cmd == herdrBin() && len(args) > 1 && args[0] == "agent" && args[1] == "prompt" {
			return Result{Code: 1}, nil
		}
		if cmd == herdrBin() && len(args) > 2 && args[0] == "pane" && args[1] == "send-text" {
			texts = append(texts, args[3])
			return Result{}, nil
		}
		if cmd == herdrBin() && len(args) > 2 && args[0] == "pane" && args[1] == "send-keys" {
			keys = append(keys, args[3])
			return Result{}, nil
		}
		return Result{}, nil
	}
	text := "Follow-up review. What changed:\n\ndiff --git a/a.go b/a.go\n+new line\n"
	if err := DeliverPrompt(run, "p1", text); err != nil {
		t.Fatalf("DeliverPrompt: %v", err)
	}
	if len(texts) != 1 {
		t.Fatalf("fallback must type exactly one line, got %q", texts)
	}
	if strings.Contains(texts[0], "\n") {
		t.Errorf("typed fallback must hold no newline, got %q", texts[0])
	}
	for _, want := range []string{"Follow-up review.", "diff --git", "a/a.go", "new line"} {
		if !strings.Contains(texts[0], want) {
			t.Errorf("typed fallback must keep %q, got %q", want, texts[0])
		}
	}
	if len(keys) != 1 || keys[0] != "Enter" {
		t.Errorf("fallback must press Enter once after typing, got %q", keys)
	}
}
