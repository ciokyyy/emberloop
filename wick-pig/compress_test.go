package wick

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"example.com/wick/tools"
)

// Seam: handoff sanity — the summary passes before any restart. Expected
// values come from the pinned prompt's own sections, never the summarizer.
func TestCheckHandoffAcceptsFullSummary(t *testing.T) {
	handoff := "## Task\nDo the thing\n\n## Context\nRepo wick.\n\n## Progress\nRead a.md.\n\n## Next steps\nEdit b.md.\n\n## Reference\n`wick-pig context`\n"
	if err := CheckHandoff(handoff); err != nil {
		t.Errorf("full handoff rejected: %v", err)
	}
}

func TestCheckHandoffRejectsEmpty(t *testing.T) {
	for _, h := range []string{"", "   \n  "} {
		if err := CheckHandoff(h); err == nil {
			t.Errorf("empty handoff %q accepted", h)
		}
	}
}

func TestCheckHandoffNamesMissingSections(t *testing.T) {
	err := CheckHandoff("## Task\nx\n\n## Context\ny\n")
	if err == nil {
		t.Fatal("partial handoff accepted")
	}
	for _, section := range []string{"## Progress", "## Next steps", "## Reference"} {
		if !strings.Contains(err.Error(), section) {
			t.Errorf("error lacks %q: %v", section, err)
		}
	}
}

func TestFinishHandoffContinuesOnlyAnInterruptedStep(t *testing.T) {
	base := "## Task\nx\n\n## Context\ny\n\n## Progress\nz\n\n## Next steps\nw\n\n## Reference\nv\n"
	with := FinishHandoff(base, true)
	if !strings.HasSuffix(strings.TrimSpace(with), ContinueLine) {
		t.Errorf("interrupted handoff lacks the continue line:\n%s", with)
	}
	without := FinishHandoff(base, false)
	if strings.Contains(without, ContinueLine) {
		t.Errorf("idle handoff must not resume a task:\n%s", without)
	}
	if !strings.HasSuffix(strings.TrimSpace(without), "v") {
		t.Errorf("idle handoff must be untouched, got:\n%s", without)
	}
}
func TestCompressPromptIsPinnedWithTheHandoffSections(t *testing.T) {
	for _, section := range []string{"## Task", "## Context", "## Progress", "## Next steps", "## Reference"} {
		if !strings.Contains(CompressPrompt, section) {
			t.Errorf("pinned prompt lacks %q", section)
		}
	}
	if strings.Contains(CompressPrompt, "wick://") {
		t.Error("pinned prompt must not read the skill library at runtime")
	}
}

// summarizeFake is the pig one-off behind the exec seam: it answers a canned
// handoff on stdout and records the invocation for the no-tools assertion.
// The prompt file is read at call time: the summary removes it on return.
type summarizeFake struct {
	calls   [][]string
	prompts []string
	handoff string
	code    int
}

func (f *summarizeFake) run(cmd string, args []string, _ ExecOpts) (Result, error) {
	f.calls = append(f.calls, append([]string{cmd}, args...))
	for _, arg := range args {
		if path, ok := strings.CutPrefix(arg, "@"); ok {
			raw, _ := os.ReadFile(path)
			f.prompts = append(f.prompts, string(raw))
		}
	}
	if f.code != 0 {
		return Result{Code: f.code, Stderr: "boom"}, nil
	}
	return Result{Stdout: f.handoff}, nil
}

const summarizeHandoff = "## Task\nDo the thing\n\n## Context\nRepo wick.\n\n## Progress\nRead a.md.\n\n## Next steps\nEdit b.md.\n\n## Reference\n`wick-pig context`\n"

// writeSummarizeTranscript writes a small pig transcript and returns its path.
func writeSummarizeTranscript(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sess.jsonl")
	writeLines(t, path,
		`{"type":"message","id":"u1","message":{"role":"user","content":[{"type":"text","text":"do the thing"}]}}`+"\n"+
			pigCallLine("s1", "a1", "call_1", "mcp__wick__read")+
			pigResultLine("r1", "call_1", "mcp__wick__read", "ok"))
	return path
}

// The summarizer is a pig one-off with no tools and no extra system prompt:
// the pinned prompt plus the rendered transcript travel in a file (one argv
// element caps at 128KB; a long session must not fail with E2BIG), answered
// on stdout.
func TestSummarizeRunsPigOneOffWithNoTools(t *testing.T) {
	isolateState(t)
	fake := &summarizeFake{handoff: summarizeHandoff}
	got, err := SummarizeSession(fake.run, "pig", writeSummarizeTranscript(t), "pig-cheap", 200000)
	if err != nil {
		t.Fatalf("summarize: %v", err)
	}
	if got != strings.TrimSpace(summarizeHandoff) {
		t.Errorf("handoff = %q, want the summarizer's answer trimmed", got)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("want one pig call, got %d", len(fake.calls))
	}
	call := strings.Join(fake.calls[0], " ")
	for _, want := range []string{"-p", "--no-session", "--no-tools", "--no-extensions", "--no-skills", "--no-context-files", "pig-cheap"} {
		if !strings.Contains(call, want) {
			t.Errorf("pig call lacks %q: %s", want, call)
		}
	}
	for _, banned := range []string{"--append-system-prompt", "--no-builtin-tools", "-e "} {
		if strings.Contains(call, banned) {
			t.Errorf("pig call must not carry %q: %s", banned, call)
		}
	}
	prompt := readPromptFile(t, fake)
	for _, want := range []string{"## Task", "## Reference", "do the thing"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt file lacks %q", want)
		}
	}
	for _, arg := range fake.calls[0] {
		if strings.Contains(arg, "## Task") {
			t.Errorf("prompt must travel in the file, not argv: %.100q...", arg)
		}
	}
}

// readPromptFile returns the @file prompt the fake read at call time.
func readPromptFile(t *testing.T, fake *summarizeFake) string {
	t.Helper()
	if len(fake.prompts) != 1 {
		t.Fatalf("pig call carries %d @prompt files, want 1", len(fake.prompts))
	}
	return fake.prompts[0]
}

// A transcript past the 128KB argv cap still summarizes: the prompt file
// carries it whole.
func TestSummarizeCarriesLongTranscriptsWhole(t *testing.T) {
	isolateState(t)
	long := strings.Repeat("y", 200000)
	path := filepath.Join(t.TempDir(), "sess.jsonl")
	writeLines(t, path, pigCallLine("s", "a1", "call_1", "mcp__wick__bash")+pigResultLine("r1", "call_1", "mcp__wick__bash", long))
	fake := &summarizeFake{handoff: summarizeHandoff}
	if _, err := SummarizeSession(fake.run, "pig", path, "pig-cheap", 2000000); err != nil {
		t.Fatalf("summarize: %v", err)
	}
	for _, arg := range fake.calls[0] {
		if len(arg) > 128*1024 {
			t.Fatalf("argv element is %d bytes, past the 128KB cap", len(arg))
		}
	}
	if prompt := readPromptFile(t, fake); !strings.Contains(prompt, long) {
		t.Error("prompt file truncated the long result")
	}
}

// No compress model means refusal before any process starts.
func TestSummarizeRefusesWithoutModel(t *testing.T) {
	isolateState(t)
	fake := &summarizeFake{handoff: summarizeHandoff}
	if _, err := SummarizeSession(fake.run, "pig", writeSummarizeTranscript(t), "", 200000); err == nil {
		t.Fatal("unset model must refuse")
	} else if !strings.Contains(err.Error(), "compressModel") {
		t.Errorf("error must name compressModel, got: %v", err)
	}
	if len(fake.calls) != 0 {
		t.Errorf("refusal must start nothing, got %d calls", len(fake.calls))
	}
}

// Input larger than the compress window refuses before any process starts.
func TestSummarizeRefusesOversizeInput(t *testing.T) {
	isolateState(t)
	fake := &summarizeFake{handoff: summarizeHandoff}
	if _, err := SummarizeSession(fake.run, "pig", writeSummarizeTranscript(t), "pig-cheap", 10); err == nil {
		t.Fatal("oversize input must refuse")
	}
	if len(fake.calls) != 0 {
		t.Errorf("refusal must start nothing, got %d calls", len(fake.calls))
	}
}

// A 228k transcript passes the 1M window check for deepseek-v4 and reaches
// fake exec.
func TestSummarize228kTranscriptWithOpenRouterDeepSeek(t *testing.T) {
	isolateState(t)
	long := strings.Repeat("x", 912000) // ~228k tokens rendered
	path := filepath.Join(t.TempDir(), "sess.jsonl")
	writeLines(t, path, pigCallLine("s", "a1", "call_1", "mcp__wick__bash")+pigResultLine("r1", "call_1", "mcp__wick__bash", long))
	fake := &summarizeFake{handoff: summarizeHandoff}
	model := "openrouter/deepseek/deepseek-v4-flash-0731"
	window := ModelWindow(model, nil)
	if window != 1000000 {
		t.Fatalf("ModelWindow(%q) = %d, want 1000000", model, window)
	}
	handoff, err := SummarizeSession(fake.run, "pig", path, model, window)
	if err != nil {
		t.Fatalf("SummarizeSession failed: %v", err)
	}
	if handoff != strings.TrimSpace(summarizeHandoff) {
		t.Errorf("got handoff %q, want %q", handoff, strings.TrimSpace(summarizeHandoff))
	}
	if len(fake.calls) != 1 {
		t.Fatalf("expected fake exec to be called once, got %d", len(fake.calls))
	}
}

// A failed or empty summary never reaches the swap: the error reports and
// the old session stays untouched.
func TestSummarizeFailureKeepsOldSession(t *testing.T) {
	isolateState(t)
	path := writeSummarizeTranscript(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	fake := &summarizeFake{code: 1}
	if _, err := SummarizeSession(fake.run, "pig", path, "pig-cheap", 200000); err == nil {
		t.Fatal("failed summarizer must report")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Error("failed summary must leave the transcript untouched")
	}
	if _, err := SummarizeSession((&summarizeFake{handoff: "no sections here"}).run, "pig", path, "pig-cheap", 200000); err == nil {
		t.Fatal("section-less summary must report")
	}
	if _, err := SummarizeSession((&summarizeFake{handoff: ""}).run, "pig", path, "pig-cheap", 200000); err == nil {
		t.Fatal("empty summary must report")
	}
}

// compressFake is the full compress flow behind the exec seam: the pig
// one-off answers the canned handoff, the herdr pane answers like the
// supervisor's fake child.
type compressFake struct {
	t       *testing.T
	alive   bool
	group   int
	pig     int
	handoff string
	ran     int
	closed  int
}

func (f *compressFake) run(cmd string, args []string, _ ExecOpts) (Result, error) {
	if cmd == "kill" {
		f.alive = false
		return Result{}, nil
	}
	if cmd == "pig" {
		f.pig++
		return Result{Stdout: f.handoff}, nil
	}
	if cmd != herdrBin() {
		return Result{Stdout: "{}"}, nil
	}
	if len(args) < 2 {
		return Result{Stdout: "{}"}, nil
	}
	switch args[0] + " " + args[1] {
	case "pane process-info":
		fg := `[]`
		group := 0
		if f.alive {
			fg = `[{"pid":` + itoa(f.group) + `}]`
			group = f.group
		}
		return Result{Stdout: `{"result":{"process_info":{"shell_pid":111,"foreground_process_group_id":` + itoa(group) + `,"foreground_processes":` + fg + `}}}`}, nil
	case "pane run":
		f.ran++
		f.alive = true
		return Result{Stdout: "{}"}, nil
	case "agent prompt":
		return Result{Stdout: "{}"}, nil
	case "pane close", "tab close":
		f.closed++
		return Result{Stdout: "{}"}, nil
	case "pane send-text":
		return Result{Stdout: "{}"}, nil
	case "pane send-keys":
		return Result{Stdout: "{}"}, nil
	}
	return Result{Stdout: "{}"}, nil
}

// compressSettings names cheap summarizers for pig and claude.
func compressSettings() tools.Settings {
	return tools.Settings{Context: &tools.ContextSettings{CompressModels: map[string]string{"pig": "pig-cheap", "claude": "claude-cheap"}}}
}

// writeCompressWorker records worker w1 on pane-1 with a transcript under
// the harness session root, and returns the old session id.
func writeCompressWorker(t *testing.T, transcript string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	oldID := "sess-compress-old"
	root := filepath.Join(home, ".pig", "agent", "sessions")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	writeLines(t, filepath.Join(root, oldID+".jsonl"), transcript)
	if err := WriteAgentState(AgentState{ID: "w1", Type: "worker", Pane: "pane-1", SessionID: oldID}); err != nil {
		t.Fatal(err)
	}
	if err := save(map[string]Worker{"w1": {ID: "w1", Pane: "pane-1", Harness: "pig", Worktree: t.TempDir(), SessionID: oldID, AgentType: "worker"}}); err != nil {
		t.Fatal(err)
	}
	return oldID
}

// A manual compress summarizes, files the handoff copy and the old
// transcript, and restarts the same pane and agent on a fresh session.
func TestCompressPaneSummarizesAndRestarts(t *testing.T) {
	isolateState(t)
	oldPoll, oldWait := pollEvery, paneStopWait
	pollEvery, paneStopWait = 5*time.Millisecond, 50*time.Millisecond
	defer func() { pollEvery, paneStopWait = oldPoll, oldWait }()
	oldID := writeCompressWorker(t,
		`{"type":"message","id":"u1","message":{"role":"user","content":[{"type":"text","text":"do the thing"}]}}`+"\n"+
			pigCallLine("s1", "a1", "call_1", "mcp__wick__read")+
			pigResultLine("r1", "call_1", "mcp__wick__read", "ok"))
	fake := &compressFake{t: t, alive: true, group: 4242, handoff: summarizeHandoff}
	next, err := CompressPane(fake.run, "pane-1", compressSettings())
	if err != nil {
		t.Fatalf("compress: %v", err)
	}
	if next == "" || next == oldID {
		t.Fatalf("new session = %q, want a fresh id", next)
	}
	if fake.pig != 1 || fake.ran != 1 || fake.closed != 0 {
		t.Errorf("one summary, one relaunch, no close: pig=%d ran=%d closed=%d", fake.pig, fake.ran, fake.closed)
	}
	s, ok, err := ReadAgentState("w1")
	if err != nil || !ok {
		t.Fatalf("ReadAgentState: %v %v", ok, err)
	}
	if s.SessionID != next || s.Compressed != 1 {
		t.Errorf("state = %+v, want session %q compressed 1", s, next)
	}
	kept := filepath.Join(filepath.Dir(statePath), "compress", "w1")
	handoff, err := os.ReadFile(filepath.Join(kept, oldID+"-handoff.md"))
	if err != nil {
		t.Fatalf("handoff copy: %v", err)
	}
	if !strings.Contains(string(handoff), "## Next steps") {
		t.Errorf("handoff copy lacks the summary:\n%s", handoff)
	}
	if strings.Contains(string(handoff), ContinueLine) {
		t.Errorf("idle handoff must not carry the continue line:\n%s", handoff)
	}
	transcript, err := os.ReadFile(filepath.Join(kept, oldID+"-transcript.jsonl"))
	if err != nil {
		t.Fatalf("transcript copy: %v", err)
	}
	if !strings.Contains(string(transcript), "do the thing") {
		t.Errorf("transcript copy lacks the session:\n%s", transcript)
	}
}

// An interrupted step resumes: the filed handoff ends with the continue line.
func TestCompressPaneContinuesAnInterruptedStep(t *testing.T) {
	isolateState(t)
	oldPoll, oldWait, oldTimeout := pollEvery, paneStopWait, restartBoundaryTimeout
	pollEvery, paneStopWait, restartBoundaryTimeout = 5*time.Millisecond, 50*time.Millisecond, 2*time.Second
	defer func() { pollEvery, paneStopWait, restartBoundaryTimeout = oldPoll, oldWait, oldTimeout }()
	oldID := writeCompressWorker(t, pigCallLine("s1", "a1", "call_1", "mcp__wick__bash"))
	// The open tool call lands while the restart waits at the boundary.
	go func() {
		time.Sleep(40 * time.Millisecond)
		root := filepath.Join(os.Getenv("HOME"), ".pig", "agent", "sessions")
		f, err := os.OpenFile(filepath.Join(root, oldID+".jsonl"), os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return
		}
		defer f.Close()
		_, _ = f.WriteString(pigResultLine("r1", "call_1", "mcp__wick__bash", "ok"))
	}()
	fake := &compressFake{t: t, alive: true, group: 4242, handoff: summarizeHandoff}
	if _, err := CompressPane(fake.run, "pane-1", compressSettings()); err != nil {
		t.Fatalf("compress: %v", err)
	}
	handoff, err := os.ReadFile(filepath.Join(filepath.Dir(statePath), "compress", "w1", oldID+"-handoff.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(strings.TrimSpace(string(handoff)), ContinueLine) {
		t.Errorf("interrupted handoff lacks the continue line:\n%s", handoff)
	}
}

// Without a compress model the pane is never touched: no summary, no stop,
// no relaunch, identity unchanged.
func TestCompressPaneRefusesWithoutModel(t *testing.T) {
	isolateState(t)
	writeCompressWorker(t, pigCallLine("s1", "a1", "call_1", "mcp__wick__read")+pigResultLine("r1", "call_1", "mcp__wick__read", "ok"))
	fake := &compressFake{t: t, alive: true, group: 4242, handoff: summarizeHandoff}
	if _, err := CompressPane(fake.run, "pane-1", tools.Settings{}); err == nil {
		t.Fatal("unset model must refuse")
	}
	if fake.pig != 0 || fake.ran != 0 || fake.closed != 0 {
		t.Errorf("refusal must touch nothing: pig=%d ran=%d closed=%d", fake.pig, fake.ran, fake.closed)
	}
	if !fake.alive {
		t.Error("refusal must not stop the child")
	}
	s, _, _ := ReadAgentState("w1")
	if s.Compressed != 0 {
		t.Errorf("refusal must leave identity alone, got %+v", s)
	}
}

// A failed summary never reaches the swap: the transcript stays live and
// the pane keeps its session.
func TestCompressPaneFailureKeepsOldSession(t *testing.T) {
	isolateState(t)
	oldPoll, oldWait := pollEvery, paneStopWait
	pollEvery, paneStopWait = 5*time.Millisecond, 50*time.Millisecond
	defer func() { pollEvery, paneStopWait = oldPoll, oldWait }()
	oldID := writeCompressWorker(t, pigCallLine("s1", "a1", "call_1", "mcp__wick__read")+pigResultLine("r1", "call_1", "mcp__wick__read", "ok"))
	fake := &compressFake{t: t, alive: true, group: 4242, handoff: "no sections here"}
	if _, err := CompressPane(fake.run, "pane-1", compressSettings()); err == nil {
		t.Fatal("bad summary must report")
	}
	if fake.ran != 0 || fake.closed != 0 {
		t.Errorf("bad summary must stop nothing: ran=%d closed=%d", fake.ran, fake.closed)
	}
	s, _, _ := ReadAgentState("w1")
	if s.SessionID != oldID || s.Compressed != 0 {
		t.Errorf("bad summary must leave identity alone, got %+v", s)
	}
}

// A pane no agent runs in, and an agent with no worker record, are clear
// errors that start nothing.
func TestCompressPaneUnknownPaneFails(t *testing.T) {
	isolateState(t)
	fake := &compressFake{t: t, handoff: summarizeHandoff}
	if _, err := CompressPane(fake.run, "pane-ghost", compressSettings()); err == nil {
		t.Fatal("unknown pane must fail")
	}
	if err := WriteAgentState(AgentState{ID: "s1", Type: "scout", Pane: "pane-s", SessionID: "sess-s", Harness: "pig"}); err != nil {
		t.Fatal(err)
	}
	if _, err := CompressPane(fake.run, "pane-s", compressSettings()); err == nil {
		t.Fatal("non-worker agent must fail")
	}
	if fake.pig != 0 || fake.ran != 0 {
		t.Errorf("a refused compress must start nothing: pig=%d ran=%d", fake.pig, fake.ran)
	}
}
