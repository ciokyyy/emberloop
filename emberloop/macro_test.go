package el

// Seam: the wrapped child. A blocking fake PTY under a real WrapChild with
// the input row on: type row text, assert the exact PTY bytes. The /compress
// trigger runs through the existing Exec seam: a fake exec answers the
// summary and the relaunch, like the compress request tests. No test touches
// live herdr, a live agent or the real home.

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"example.com/emberloop/tools"
)

// macroRowSettings is edge-to-edge settings with one prompt macro and cheap
// summarizers for pig and claude.
func macroRowSettings() tools.Settings {
	s := withNoPad(tools.ContextSettings{
		CompressModels: map[string]string{"pig": "pig-cheap", "claude": "claude-cheap"},
	})
	s.Macros = []tools.MacroSetting{
		{Name: "review", Kind: "prompt", Template: "please review {{args}}"},
	}
	return s
}

// A prompt template macro with arguments expands and delivers as a message:
// the row sends the expansion, never the raw /name text.
func TestWrapRowMacroExpandsAndDelivers(t *testing.T) {
	spawnTestState(t)
	oldPump, oldSettle := queuePumpEvery, submitSettle
	queuePumpEvery, submitSettle = 5*time.Millisecond, 0
	defer func() { queuePumpEvery, submitSettle = oldPump, oldSettle }()

	child := &blockingChild{release: make(chan struct{})}
	var stdout safeBuffer
	pr, pw := io.Pipe()
	cfg := wrapTestConfig(child, &stdout)
	cfg.InputRow = true
	cfg.Stdin = pr
	cfg.Poll = 5 * time.Millisecond
	cfg.Settings = macroRowSettings()

	wrapped := make(chan error, 1)
	go func() { wrapped <- WrapChild(cfg) }()

	waitForCond(t, "row ready", func() bool {
		return strings.Contains(stdout.String(), "❯")
	})
	_, _ = pw.Write([]byte("/review the auth flow\r"))
	waitForCond(t, "macro expansion delivered", func() bool {
		return strings.Contains(child.fakeChild.forwardedString(), "please review the auth flow\r")
	})
	if got := child.fakeChild.forwardedString(); strings.Contains(got, "/review") {
		t.Errorf("child got the raw macro call, want only the expansion: %q", got)
	}

	close(child.release)
	_ = pw.Close()
	<-wrapped
}

// Unknown slash text is sent as plain text, not dropped or rejected.
func TestWrapRowUnknownSlashSentAsText(t *testing.T) {
	spawnTestState(t)
	oldPump, oldSettle := queuePumpEvery, submitSettle
	queuePumpEvery, submitSettle = 5*time.Millisecond, 0
	defer func() { queuePumpEvery, submitSettle = oldPump, oldSettle }()

	child := &blockingChild{release: make(chan struct{})}
	var stdout safeBuffer
	pr, pw := io.Pipe()
	cfg := wrapTestConfig(child, &stdout)
	cfg.InputRow = true
	cfg.Stdin = pr
	cfg.Poll = 5 * time.Millisecond
	cfg.Settings = macroRowSettings()

	wrapped := make(chan error, 1)
	go func() { wrapped <- WrapChild(cfg) }()

	waitForCond(t, "row ready", func() bool {
		return strings.Contains(stdout.String(), "❯")
	})
	_, _ = pw.Write([]byte("/frobnicate the moon\r"))
	waitForCond(t, "unknown slash delivered as text", func() bool {
		return strings.Contains(child.fakeChild.forwardedString(), "/frobnicate the moon\r")
	})

	close(child.release)
	_ = pw.Close()
	<-wrapped
}

// Macro text goes through the same queue and hold rules as typed text: a
// pending injection waits, and the expanded macro goes first.
func TestWrapRowMacroBeatsPendingInjection(t *testing.T) {
	spawnTestState(t)
	oldPump, oldSettle := queuePumpEvery, submitSettle
	queuePumpEvery, submitSettle = 5*time.Millisecond, 0
	defer func() { queuePumpEvery, submitSettle = oldPump, oldSettle }()

	dir := t.TempDir()
	transcript := filepath.Join(dir, "s1.jsonl")
	if err := os.WriteFile(transcript, []byte(pigAssistantRunning()), 0o644); err != nil {
		t.Fatal(err)
	}

	child := &blockingChild{release: make(chan struct{})}
	var stdout safeBuffer
	pr, pw := io.Pipe()
	cfg := wrapTestConfig(child, &stdout)
	cfg.InputRow = true
	cfg.ServeCtrl = true
	cfg.Stdin = pr
	cfg.Poll = 5 * time.Millisecond
	cfg.Settings = macroRowSettings()
	cfg.Source.TranscriptPath = transcript

	wrapped := make(chan error, 1)
	go func() { wrapped <- WrapChild(cfg) }()

	waitForCond(t, "control socket", func() bool {
		_, err := os.Stat(ctrlSocketPath("w1"))
		return err == nil
	})

	_, _ = pw.Write([]byte("/review the auth flow"))
	waitForCond(t, "draft rendered", func() bool {
		return strings.Contains(stdout.String(), "/review the auth flow")
	})
	if _, err := ctrlEnqueue("w1", QueueKindSteer, "injection text", "tell"); err != nil {
		t.Fatalf("ctrlEnqueue: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	if got := child.fakeChild.forwardedString(); got != "" {
		t.Fatalf("child got text while draft active: %q", got)
	}

	_, _ = pw.Write([]byte("\r"))
	waitForCond(t, "macro expansion delivered first", func() bool {
		return strings.Contains(child.fakeChild.forwardedString(), "please review the auth flow\r")
	})
	appendLine(t, transcript, pigUserTurn(t, "please review the auth flow"))
	waitForCond(t, "injection delivered after macro", func() bool {
		return strings.Contains(child.fakeChild.forwardedString(), "injection text\r")
	})
	if got, want := child.fakeChild.forwardedString(), "please review the auth flow\rinjection text\r"; got != want {
		t.Fatalf("forwarded bytes = %q, want %q", got, want)
	}

	close(child.release)
	_ = pw.Close()
	<-wrapped
}

// /compress in the row triggers compress for pig through the exec seam: one
// summary and one relaunch, with no plugin or key, and nothing enqueued.
func TestWrapRowCompressTriggersCompress(t *testing.T) {
	spawnTestState(t)
	oldPoll, oldWait, oldPump, oldSettle := pollEvery, paneStopWait, queuePumpEvery, submitSettle
	pollEvery, paneStopWait = 5*time.Millisecond, 50*time.Millisecond
	queuePumpEvery, submitSettle = 5*time.Millisecond, 0
	defer func() { pollEvery, paneStopWait, queuePumpEvery, submitSettle = oldPoll, oldWait, oldPump, oldSettle }()

	writeCompressWorker(t,
		`{"type":"message","id":"u1","message":{"role":"user","content":[{"type":"text","text":"do the thing"}]}}`+"\n"+
			pigCallLine("s1", "a1", "call_1", "mcp__el__read")+
			pigResultLine("r1", "call_1", "mcp__el__read", "ok"))
	fake := &compressFake{t: t, alive: true, group: 4242, handoff: summarizeHandoff}

	child := &blockingChild{release: make(chan struct{})}
	var stdout safeBuffer
	pr, pw := io.Pipe()
	cfg := wrapTestConfig(child, &stdout)
	cfg.InputRow = true
	cfg.Stdin = pr
	cfg.Poll = 5 * time.Millisecond
	cfg.Settings = macroRowSettings()
	cfg.CompressRun = fake.run

	wrapped := make(chan error, 1)
	go func() { wrapped <- WrapChild(cfg) }()

	waitForCond(t, "row ready", func() bool {
		return strings.Contains(stdout.String(), "❯")
	})
	_, _ = pw.Write([]byte("/compress\r"))
	waitForCond(t, "compress relaunched", func() bool {
		return fake.ran == 1
	})
	if fake.pig != 1 {
		t.Errorf("one summary: pig=%d", fake.pig)
	}
	if got := child.fakeChild.forwardedString(); got != "" {
		t.Errorf("/compress must trigger compress, not enqueue text: %q", got)
	}

	close(child.release)
	_ = pw.Close()
	<-wrapped
}

// /compress in the row triggers compress for claude too: same signal path,
// same restart, no per-harness extension.
func TestWrapRowCompressCoversClaude(t *testing.T) {
	spawnTestState(t)
	oldPoll, oldWait, oldPump, oldSettle := pollEvery, paneStopWait, queuePumpEvery, submitSettle
	pollEvery, paneStopWait = 5*time.Millisecond, 50*time.Millisecond
	queuePumpEvery, submitSettle = 5*time.Millisecond, 0
	defer func() { pollEvery, paneStopWait, queuePumpEvery, submitSettle = oldPoll, oldWait, oldPump, oldSettle }()

	home := t.TempDir()
	t.Setenv("HOME", home)
	oldID := "sess-claude-old"
	root := filepath.Join(home, ".claude", "projects")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	writeLines(t, filepath.Join(root, oldID+".jsonl"),
		`{"type":"user","uuid":"u1","timestamp":"2026-10-06T10:00:00Z","sessionId":"s1","message":{"role":"user","content":"do the thing"}}`+"\n"+
			`{"type":"assistant","uuid":"c1","timestamp":"2026-10-06T10:00:01Z","sessionId":"s1","message":{"id":"msg_1","model":"claude-test-model","type":"message","role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"Bash","input":{"command":"ls"}}],"stop_reason":"tool_use","usage":{"input_tokens":10,"output_tokens":50,"cache_read_input_tokens":1000,"cache_creation_input_tokens":100}}}`+"\n"+
			`{"type":"user","uuid":"c2","timestamp":"2026-10-06T10:00:02Z","sessionId":"s1","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"ok"}]}}`+"\n")
	if err := WriteAgentState(AgentState{ID: "c1", Type: "worker", Pane: "pane-c", Harness: "claude", SessionID: oldID}); err != nil {
		t.Fatal(err)
	}
	if err := save(map[string]Worker{"c1": {ID: "c1", Pane: "pane-c", Harness: "claude", Worktree: t.TempDir(), SessionID: oldID, AgentType: "worker"}}); err != nil {
		t.Fatal(err)
	}
	fake := &compressFake{t: t, alive: true, group: 4242, handoff: summarizeHandoff}

	child := &blockingChild{release: make(chan struct{})}
	var stdout safeBuffer
	pr, pw := io.Pipe()
	cfg := wrapTestConfig(child, &stdout)
	cfg.InputRow = true
	cfg.Stdin = pr
	cfg.Poll = 5 * time.Millisecond
	cfg.Settings = macroRowSettings()
	cfg.Source.AgentID = "c1"
	cfg.Source.Harness = "claude"
	cfg.CompressRun = fake.run

	wrapped := make(chan error, 1)
	go func() { wrapped <- WrapChild(cfg) }()

	waitForCond(t, "row ready", func() bool {
		return strings.Contains(stdout.String(), "❯")
	})
	_, _ = pw.Write([]byte("/compress\r"))
	waitForCond(t, "claude compress relaunched", func() bool {
		return fake.ran == 1
	})
	if fake.pig != 1 {
		t.Errorf("one summary: pig=%d", fake.pig)
	}
	if got := child.fakeChild.forwardedString(); got != "" {
		t.Errorf("/compress must trigger compress, not enqueue text: %q", got)
	}

	close(child.release)
	_ = pw.Close()
	<-wrapped
}

// /compress with no recorded pane warns instead of running: outside herdr
// there is no pane to compress in, and the row stays usable.
func TestWrapRowCompressWithoutPaneWarns(t *testing.T) {
	spawnTestState(t)
	t.Setenv("HOME", t.TempDir())
	oldPump, oldSettle := queuePumpEvery, submitSettle
	queuePumpEvery, submitSettle = 5*time.Millisecond, 0
	defer func() { queuePumpEvery, submitSettle = oldPump, oldSettle }()

	child := &blockingChild{release: make(chan struct{})}
	var stdout, wrapperLog safeBuffer
	pr, pw := io.Pipe()
	cfg := wrapTestConfig(child, &stdout)
	cfg.InputRow = true
	cfg.Stdin = pr
	cfg.Poll = 5 * time.Millisecond
	cfg.Settings = macroRowSettings()
	cfg.Stderr = &wrapperLog
	cfg.Source.AgentID = "ghost-1"
	cfg.CompressRun = func(cmd string, args []string, opts ExecOpts) (Result, error) {
		t.Error("no pane means no compress run")
		return Result{}, nil
	}

	wrapped := make(chan error, 1)
	go func() { wrapped <- WrapChild(cfg) }()

	waitForCond(t, "row ready", func() bool {
		return strings.Contains(stdout.String(), "❯")
	})
	_, _ = pw.Write([]byte("/compress\r"))
	waitForCond(t, "compress warning", func() bool {
		return strings.Contains(wrapperLog.String(), "/compress")
	})
	if got := child.fakeChild.forwardedString(); got != "" {
		t.Errorf("a failed /compress must not reach the child: %q", got)
	}

	close(child.release)
	_ = pw.Close()
	<-wrapped
}

// A macro shadowing a built-in is reported when the row starts: the warning
// names the collision.
func TestWrapRowMacroShadowWarnsAtStartup(t *testing.T) {
	spawnTestState(t)
	child := &blockingChild{release: make(chan struct{})}
	var stdout, wrapperLog safeBuffer
	pr, pw := io.Pipe()
	cfg := wrapTestConfig(child, &stdout)
	cfg.InputRow = true
	cfg.Stdin = pr
	cfg.Stderr = &wrapperLog
	settings := macroRowSettings()
	settings.Macros = append(settings.Macros, tools.MacroSetting{Name: "compress", Kind: "prompt", Template: "summarize {{args}}"})
	cfg.Settings = settings

	wrapped := make(chan error, 1)
	go func() { wrapped <- WrapChild(cfg) }()
	close(child.release)
	_ = pw.Close()
	<-wrapped

	if got := wrapperLog.String(); !strings.Contains(got, "compress") {
		t.Errorf("startup must report the builtin shadow, got %q", got)
	}
}

// A macro expanding to nothing sends the raw text: an empty message never
// reaches the child.
func TestWrapRowMacroEmptyExpansionSendsRawText(t *testing.T) {
	spawnTestState(t)
	oldPump, oldSettle := queuePumpEvery, submitSettle
	queuePumpEvery, submitSettle = 5*time.Millisecond, 0
	defer func() { queuePumpEvery, submitSettle = oldPump, oldSettle }()

	child := &blockingChild{release: make(chan struct{})}
	var stdout safeBuffer
	pr, pw := io.Pipe()
	cfg := wrapTestConfig(child, &stdout)
	cfg.InputRow = true
	cfg.Stdin = pr
	cfg.Poll = 5 * time.Millisecond
	settings := macroRowSettings()
	settings.Macros = append(settings.Macros, tools.MacroSetting{Name: "blank", Kind: "prompt", Template: "{{args}}"})
	cfg.Settings = settings

	wrapped := make(chan error, 1)
	go func() { wrapped <- WrapChild(cfg) }()

	waitForCond(t, "row ready", func() bool {
		return strings.Contains(stdout.String(), "❯")
	})
	_, _ = pw.Write([]byte("/blank\r"))
	waitForCond(t, "raw macro text delivered", func() bool {
		return strings.Contains(child.fakeChild.forwardedString(), "/blank\r")
	})

	close(child.release)
	_ = pw.Close()
	<-wrapped
}

// A duplicate macro name still serves the row: the first entry wins, and the
// reject is reported at startup.
func TestWrapRowMacroDuplicateKeepsFirst(t *testing.T) {
	spawnTestState(t)
	oldPump, oldSettle := queuePumpEvery, submitSettle
	queuePumpEvery, submitSettle = 5*time.Millisecond, 0
	defer func() { queuePumpEvery, submitSettle = oldPump, oldSettle }()

	child := &blockingChild{release: make(chan struct{})}
	var stdout, wrapperLog safeBuffer
	pr, pw := io.Pipe()
	cfg := wrapTestConfig(child, &stdout)
	cfg.InputRow = true
	cfg.Stdin = pr
	cfg.Poll = 5 * time.Millisecond
	cfg.Stderr = &wrapperLog
	settings := macroRowSettings()
	settings.Macros = append(settings.Macros, tools.MacroSetting{Name: "review", Kind: "prompt", Template: "second {{args}}"})
	cfg.Settings = settings

	wrapped := make(chan error, 1)
	go func() { wrapped <- WrapChild(cfg) }()

	waitForCond(t, "row ready", func() bool {
		return strings.Contains(stdout.String(), "❯")
	})
	_, _ = pw.Write([]byte("/review the auth flow\r"))
	waitForCond(t, "first macro entry delivered", func() bool {
		return strings.Contains(child.fakeChild.forwardedString(), "please review the auth flow\r")
	})

	close(child.release)
	_ = pw.Close()
	<-wrapped

	if got := wrapperLog.String(); !strings.Contains(got, "duplicate") {
		t.Errorf("startup must report the duplicate, got %q", got)
	}
}

// Two quick /compress submits start one compress request: the second warns
// instead of doubling the summary and relaunch.
func TestWrapRowCompressDoubleSubmitSingleFlights(t *testing.T) {
	spawnTestState(t)
	oldPoll, oldWait, oldPump, oldSettle := pollEvery, paneStopWait, queuePumpEvery, submitSettle
	pollEvery, paneStopWait = 5*time.Millisecond, 50*time.Millisecond
	queuePumpEvery, submitSettle = 5*time.Millisecond, 0
	defer func() { pollEvery, paneStopWait, queuePumpEvery, submitSettle = oldPoll, oldWait, oldPump, oldSettle }()

	writeCompressWorker(t,
		`{"type":"message","id":"u1","message":{"role":"user","content":[{"type":"text","text":"do the thing"}]}}`+"\n"+
			pigCallLine("s1", "a1", "call_1", "mcp__el__read")+
			pigResultLine("r1", "call_1", "mcp__el__read", "ok"))
	fake := &compressFake{t: t, alive: true, group: 4242, handoff: summarizeHandoff}
	var probes atomic.Int32
	unblock := make(chan struct{})
	run := func(cmd string, args []string, opts ExecOpts) (Result, error) {
		if cmd == herdrBin() && len(args) >= 2 && args[0] == "pane" && args[1] == "process-info" {
			if probes.Add(1) == 1 {
				<-unblock
			}
		}
		return fake.run(cmd, args, opts)
	}

	child := &blockingChild{release: make(chan struct{})}
	var stdout, wrapperLog safeBuffer
	pr, pw := io.Pipe()
	cfg := wrapTestConfig(child, &stdout)
	cfg.InputRow = true
	cfg.Stdin = pr
	cfg.Poll = 5 * time.Millisecond
	cfg.Settings = macroRowSettings()
	cfg.Stderr = &wrapperLog
	cfg.CompressRun = run

	wrapped := make(chan error, 1)
	go func() { wrapped <- WrapChild(cfg) }()

	waitForCond(t, "row ready", func() bool {
		return strings.Contains(stdout.String(), "❯")
	})
	_, _ = pw.Write([]byte("/compress\r"))
	waitForCond(t, "first compress started", func() bool {
		return probes.Load() == 1
	})
	_, _ = pw.Write([]byte("/compress\r"))
	waitForCond(t, "second compress warned", func() bool {
		return strings.Contains(wrapperLog.String(), "already running")
	})
	close(unblock)
	waitForCond(t, "compress relaunched once", func() bool {
		return fake.ran == 1
	})
	if fake.pig != 1 {
		t.Errorf("one summary: pig=%d", fake.pig)
	}

	close(child.release)
	_ = pw.Close()
	<-wrapped
}
