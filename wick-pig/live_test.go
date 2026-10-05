package wick

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLiveReadOnly runs a real reviewer pane. WICK_LIVE=pig|claude, inside herdr.
func TestLiveReadOnly(t *testing.T) {
	h := os.Getenv("WICK_LIVE")
	if h == "" {
		t.Skip("set WICK_LIVE=pig or claude, inside herdr")
	}
	t.Setenv("WICK_READONLY_HARNESS", h)
	statePath = filepath.Join(t.TempDir(), "workers.json")
	got, err := ReadOnly(osExec, mustAbs(t, ".."), "", "Use read on wick-pig/go.mod. Reply with only its first line.")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "module example.com/wick") {
		t.Errorf("reply = %q", got)
	}
}

func mustAbs(t *testing.T, p string) string {
	a, err := filepath.Abs(p)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// TestLiveModels lists a real harness's models through its CLI. WICK_LIVE=pig
// or claude. It proves the listing is a real run that answers and exits --
// claude's initialize spends no tokens, so this is safe to repeat.
func TestLiveModels(t *testing.T) {
	h := os.Getenv("WICK_LIVE")
	if h == "" {
		t.Skip("set WICK_LIVE=pig or claude")
	}
	isolateState(t)
	list, err := CachedModels(h, osExec, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) == 0 {
		t.Fatalf("%s listed no models", h)
	}
}

// liveClaudeVersion runs claude --version so a foreground failure records what
// version and setting is involved, as the ticket requires.
func liveClaudeVersion(t *testing.T) string {
	t.Helper()
	res, err := osExec("claude", []string{"--version"}, ExecOpts{Timeout: 15 * time.Second})
	if err != nil {
		t.Logf("claude --version: %v", err)
		return "unknown"
	}
	v := strings.TrimSpace(res.Stdout + " " + res.Stderr)
	if v == "" {
		v = "unknown"
	}
	t.Logf("claude version: %s", v)
	return v
}

// liveTranscript returns the raw claude transcript for sess, or "" when it is
// not on disk yet.
func liveTranscript(sess Session) string {
	p := findSession(Registered("claude").SessionRoot(sess), sess.ID)
	if p == "" {
		return ""
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return string(b)
}

// assertForeground fails when the transcript carries an auto-background
// marker. The system prompt names the background tool, so only the
// backgrounding shapes count, never the bare word. An empty transcript is a
// failure, not a pass: without it the foreground check would be vacuous.
func assertForeground(t *testing.T, transcript, version string) {
	t.Helper()
	if strings.TrimSpace(transcript) == "" {
		t.Errorf("claude %s: no transcript to check for backgrounding; the foreground check is vacuous", version)
		return
	}
	lowered := strings.ToLower(transcript)
	for _, marker := range []string{"backgrounded", "background task", "auto-background", "auto_background", "moved to the background", "running in the background"} {
		if strings.Contains(lowered, marker) {
			t.Errorf("claude %s backgrounded a running MCP call (%q): CLAUDE_CODE_DISABLE_BACKGROUND_TASKS=1 CLAUDE_CODE_MCP_AUTO_BACKGROUND_MS=0 did not hold; transcript:\n%s", version, marker, transcript)
			return
		}
	}
}

// waitForToolStart polls until the transcript shows the slow tool is in
// flight (the command text or a tool-use record), so a mid-call notice lands
// mid-call rather than before the call or after it returned. It also fails
// when the call already finished, for the same reason.
func waitForToolStart(t *testing.T, sess Session, command, doneMarker string) {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		tr := liveTranscript(sess)
		if strings.Contains(tr, doneMarker) {
			t.Fatalf("slow tool finished before the mid-call notice was delivered; transcript:\n%s", tr)
		}
		if strings.Contains(tr, command) || strings.Contains(strings.ToLower(tr), "tool_use") {
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("slow tool never started within 90s; transcript:\n%s", liveTranscript(sess))
}

// TestLiveClaudeSlowMCPStaysForeground is opt-in (WICK_LIVE=claude, inside
// herdr): a claude session calls a wick tool that runs longer than 120 s and
// the result arrives in the same turn, not as a background task.
func TestLiveClaudeSlowMCPStaysForeground(t *testing.T) {
	if os.Getenv("WICK_LIVE") != "claude" {
		t.Skip("set WICK_LIVE=claude, inside herdr")
	}
	version := liveClaudeVersion(t)
	statePath = filepath.Join(t.TempDir(), "workers.json")
	cwd := t.TempDir()
	sess := Session{Role: "work", Cwd: cwd, ID: newSessionID(), Prompt: "Your last message is your answer."}
	agent, err := launchPlacedAgent(osExec, readOnlyPlace{Overflow: "scouts"}, "claude", sess,
		`Use the bash tool with command "sleep 135; echo SLOW-TOOL-DONE" and timeout 300. When the tool returns, reply with only the tool output verbatim, prefixed with RESULT:. Do nothing else.`)
	if err != nil {
		t.Fatal(err)
	}
	defer closeAgent(osExec, agent)
	defer CleanupSession(agent.Session)
	got, err := agent.reply(osExec, 0)
	if err != nil {
		t.Fatalf("claude %s slow tool: %v", version, err)
	}
	if !strings.Contains(got, "SLOW-TOOL-DONE") {
		t.Errorf("claude %s: same-turn reply must carry the slow tool result, got %q", version, got)
	}
	assertForeground(t, liveTranscript(agent.Session), version)
}

// TestLiveClaudeNoticeDuringToolCallStaysForeground is opt-in
// (WICK_LIVE=claude, inside herdr): a notice delivered while a tool call runs
// does not detach the call; the notice is seen after the call returns.
func TestLiveClaudeNoticeDuringToolCallStaysForeground(t *testing.T) {
	if os.Getenv("WICK_LIVE") != "claude" {
		t.Skip("set WICK_LIVE=claude, inside herdr")
	}
	version := liveClaudeVersion(t)
	statePath = filepath.Join(t.TempDir(), "workers.json")
	cwd := t.TempDir()
	sess := Session{Role: "work", Cwd: cwd, ID: newSessionID(), Prompt: "Your last message is your answer."}
	agent, err := launchPlacedAgent(osExec, readOnlyPlace{Overflow: "scouts"}, "claude", sess,
		`Use the bash tool with command "sleep 30; echo CALL-DONE" and timeout 120. When the tool returns, reply with the tool output plus any notice text you received while it ran. Include the line NOTICE-SEEN:<the notice verbatim>. Do nothing else.`)
	if err != nil {
		t.Fatal(err)
	}
	defer closeAgent(osExec, agent)
	defer CleanupSession(agent.Session)
	// Wait until the tool call is in flight, then type a notice into the
	// pane the way notice delivery does. The wait proves the notice lands
	// mid-call; a fixed sleep could land it before the call or after it.
	waitForToolStart(t, agent.Session, "sleep 30", "CALL-DONE")
	if strings.Contains(liveTranscript(agent.Session), "CALL-DONE") {
		t.Fatalf("slow tool finished before the mid-call notice was delivered")
	}
	notice := "NOTICE-MARKER-FOREGROUND: acknowledge this after your tool returns."
	if err := Nudge(osExec, agent.Pane, notice); err != nil {
		t.Fatalf("deliver notice mid-call: %v", err)
	}
	got, err := agent.reply(osExec, 0)
	if err != nil {
		t.Fatalf("claude %s notice mid-call: %v", version, err)
	}
	if !strings.Contains(got, "CALL-DONE") {
		t.Errorf("claude %s: reply must carry the tool result after a mid-call notice, got %q", version, got)
	}
	// The queued notice may arrive in the same turn as the tool result or
	// as the next turn; either proves the call stayed foreground and the
	// notice was seen after it.
	if !strings.Contains(got, "NOTICE-MARKER-FOREGROUND") {
		after := agent.transcriptSize()
		second, err := WaitReply(Registered("claude"), agent.Session, after, 90*time.Second,
			func() error { return Nudge(osExec, agent.Pane, "continue") })
		if err != nil || !strings.Contains(second, "NOTICE-MARKER-FOREGROUND") {
			t.Errorf("claude %s: reply must show the mid-call notice was seen after the call, first %q second %q err %v", version, got, second, err)
		}
	}
	assertForeground(t, liveTranscript(agent.Session), version)
}
