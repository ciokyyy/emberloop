package web

import (
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// stubConn is the fake ACP adapter: canned updates, captured params, no
// subprocess. The seam under test is the ClaudeBackend, never internals.
type stubConn struct {
	mu            sync.Mutex
	newMeta       map[string]any
	newMCP        []acpMCPServer
	newCWD        string
	loadMeta      map[string]any
	loadMCP       []acpMCPServer
	loadCWD       string
	loadID        string
	prompts       []string
	updates       []map[string]any
	permissions   []stubPermission
	stopReason    string
	promptErr     error
	newErr        error
	blockNew      chan struct{}
	promptStarted chan struct{}
}

type stubPermission struct {
	toolName string
	options  []any
}

func (s *stubConn) Initialize() error { return nil }

func (s *stubConn) NewSession(cwd string, mcp []acpMCPServer, meta map[string]any) (string, error) {
	if s.blockNew != nil {
		if s.promptStarted != nil {
			close(s.promptStarted)
		}
		<-s.blockNew
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.newCWD, s.newMCP, s.newMeta = cwd, mcp, meta
	if s.newErr != nil {
		return "", s.newErr
	}
	return "acp-session-1", nil
}

func (s *stubConn) LoadSession(acpID, cwd string, mcp []acpMCPServer, meta map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loadID, s.loadCWD, s.loadMCP, s.loadMeta = acpID, cwd, mcp, meta
	return nil
}

func (s *stubConn) Prompt(acpID, text string, cb acpCallbacks) (string, error) {
	s.mu.Lock()
	s.prompts = append(s.prompts, text)
	updates := append([]map[string]any{}, s.updates...)
	perms := append([]stubPermission{}, s.permissions...)
	stop := s.stopReason
	err := s.promptErr
	s.mu.Unlock()
	if err != nil {
		return "", err
	}
	for _, p := range perms {
		id := pickAllowOption(p.options)
		if cb.OnPermission != nil && id != "" {
			cb.OnPermission(p.toolName, id)
		}
	}
	if cb.OnUpdate != nil {
		for _, u := range updates {
			cb.OnUpdate(u)
		}
	}
	if stop == "" {
		stop = "end_turn"
	}
	return stop, nil
}

func (s *stubConn) Close() error { return nil }

func chunk(text string) map[string]any {
	return map[string]any{
		"sessionUpdate": "agent_message_chunk",
		"content":       map[string]any{"type": "text", "text": text},
	}
}

// Slice 1: the claude Backend offers exactly opus, sonnet and haiku.
func TestClaudeModelsAreOpusSonnetHaiku(t *testing.T) {
	want := []string{"opus", "sonnet", "haiku"}
	if got := ClaudeModels(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ClaudeModels() = %v, want %v", got, want)
	}
}

// Slice 2: session/new and session/load carry the same full stripping set,
// with an absolute MCP command.
func TestNewAndLoadShareFullStrippingSet(t *testing.T) {
	stub := &stubConn{}
	be := newStubClaudeBackend(func() (acpConn, error) { return stub, nil })
	meta := SessionMeta{ID: "d1", Harness: "claude", Model: "sonnet", WorkDir: t.TempDir()}
	if err := be.StartSession(meta); err != nil {
		t.Fatal(err)
	}
	mustStripped(t, stub.newMeta, "sonnet")
	if len(stub.newMCP) != 1 || stub.newMCP[0].Name != "emberloop" {
		t.Fatalf("mcp servers = %+v, want one emberloop server", stub.newMCP)
	}
	if stub.newMCP[0].Command != "/abs/stub-mcp" {
		t.Fatalf("mcp command = %q, want absolute", stub.newMCP[0].Command)
	}

	be2 := newStubClaudeBackend(func() (acpConn, error) { return stub, nil })
	if err := be2.LoadSession("d2", "acp-session-1", "opus", meta.WorkDir); err != nil {
		t.Fatal(err)
	}
	mustStripped(t, stub.loadMeta, "opus")
	if stub.loadID != "acp-session-1" {
		t.Fatalf("load session id = %q", stub.loadID)
	}
}

func mustStripped(t *testing.T, meta map[string]any, model string) {
	t.Helper()
	cc, _ := meta["claudeCode"].(map[string]any)
	opts, _ := cc["options"].(map[string]any)
	if opts == nil {
		t.Fatalf("meta missing _meta.claudeCode.options: %v", meta)
	}
	if got, _ := opts["settingSources"].([]string); len(got) != 0 {
		t.Fatalf("settingSources = %v, want empty", got)
	}
	if opts["strictMcpConfig"] != true {
		t.Fatalf("strictMcpConfig = %v, want true", opts["strictMcpConfig"])
	}
	allowed, _ := opts["allowedTools"].([]string)
	if !reflect.DeepEqual(allowed, []string{"mcp__emberloop"}) {
		t.Fatalf("allowedTools = %v", allowed)
	}
	disallowed, ok := opts["disallowedTools"].([]string)
	if !ok {
		t.Fatalf("disallowedTools = %v, want []string", opts["disallowedTools"])
	}
	for _, banned := range []string{"Task", "Bash", "Read", "Write", "Edit", "Glob", "Grep", "WebFetch", "WebSearch"} {
		found := false
		for _, d := range disallowed {
			if d == banned {
				found = true
			}
		}
		if !found {
			t.Fatalf("disallowedTools = %v, missing %s", disallowed, banned)
		}
	}
	if opts["model"] != model {
		t.Fatalf("model = %v, want %s", opts["model"], model)
	}
}

// Slice 3: a prompt streams message chunks, thought, tool calls and diffs
// as normalized Events.
func TestPromptStreamsNormalizedEvents(t *testing.T) {
	stub := &stubConn{updates: []map[string]any{
		chunk("hello "),
		chunk("world"),
		{"sessionUpdate": "agent_thought_chunk", "content": map[string]any{"type": "text", "text": "thinking"}},
		{"sessionUpdate": "tool_call", "toolCallId": "t1", "title": "Spike Echo", "name": "mcp__emberloop__echo",
			"rawInput": map[string]any{"text": "alpha"}},
		{"sessionUpdate": "tool_call_update", "toolCallId": "t1", "status": "completed",
			"content": []any{map[string]any{"type": "content", "content": map[string]any{"type": "text", "text": "echo:alpha"}}}},
		{"sessionUpdate": "tool_call_update", "toolCallId": "t2", "status": "completed",
			"content": []any{map[string]any{"type": "diff", "path": "/w/f.txt", "newText": "new"}}},
	}}
	be := newStubClaudeBackend(func() (acpConn, error) { return stub, nil })
	meta := SessionMeta{ID: "d1", Harness: "claude", Model: "haiku", WorkDir: t.TempDir()}
	if err := be.StartSession(meta); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var got []struct{ kind, text, status string }
	be.OnPrompt("d1", "hi", func(kind, text, status string) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, struct{ kind, text, status string }{kind, text, status})
	})
	kinds := map[string][]string{}
	for _, e := range got {
		kinds[e.kind] = append(kinds[e.kind], e.text)
	}
	if len(kinds["assistant_chunk"]) != 2 || kinds["assistant_chunk"][0] != "hello " {
		t.Fatalf("assistant chunks = %v", kinds["assistant_chunk"])
	}
	if len(kinds["thinking_chunk"]) != 1 {
		t.Fatalf("thinking chunks = %v", kinds["thinking_chunk"])
	}
	if len(kinds["tool_call_started"]) != 1 || !strings.Contains(kinds["tool_call_started"][0], "mcp__emberloop__echo") {
		t.Fatalf("tool started = %v", kinds["tool_call_started"])
	}
	if len(kinds["tool_call_updated"]) != 2 {
		t.Fatalf("tool updated = %v", kinds["tool_call_updated"])
	}
	if len(kinds["diff"]) != 1 || !strings.Contains(kinds["diff"][0], "/w/f.txt") {
		t.Fatalf("diffs = %v", kinds["diff"])
	}
	if len(kinds["status"]) != 2 || kinds["status"][0] != "" {
		t.Fatalf("statuses = %v", got)
	}
}

// Slice 4: permission requests auto-approve the allow-once option and log.
func TestPermissionAutoApprovedAndLogged(t *testing.T) {
	stub := &stubConn{permissions: []stubPermission{
		{
			toolName: "mcp__emberloop__echo",
			options: []any{
				map[string]any{"optionId": "allow-once", "name": "Yes", "kind": "allow_once"},
				map[string]any{"optionId": "reject", "name": "No", "kind": "reject_once"},
			},
		},
		{
			toolName: "mcp__emberloop__other",
			options: []any{
				map[string]any{"optionId": "reject", "name": "No", "kind": "reject_once"},
				map[string]any{"optionId": "allow-always", "name": "Always", "kind": "allow_always"},
			},
		},
	}}
	be := newStubClaudeBackend(func() (acpConn, error) { return stub, nil })
	meta := SessionMeta{ID: "d1", Harness: "claude", Model: "opus", WorkDir: t.TempDir()}
	if err := be.StartSession(meta); err != nil {
		t.Fatal(err)
	}
	var perms []string
	be.OnPrompt("d1", "hi", func(kind, text, status string) {
		if kind == "permission" {
			perms = append(perms, text+"|"+status)
		}
	})
	if len(perms) != 2 {
		t.Fatalf("permission events = %v", perms)
	}
	if !strings.Contains(perms[0], "allow-once") || !strings.Contains(perms[1], "allow-always") {
		t.Fatalf("permission events = %v", perms)
	}
	for _, p := range perms {
		if !strings.HasSuffix(p, "|auto-approved") {
			t.Fatalf("permission not logged auto-approved: %v", perms)
		}
	}
}

// Slice 5: the first prompt waits until session/new answers (MCP ready).
func TestFirstPromptWaitsForMCPReady(t *testing.T) {
	block := make(chan struct{})
	started := make(chan struct{})
	stub := &stubConn{blockNew: block, promptStarted: started, updates: []map[string]any{chunk("late")}}
	be := newStubClaudeBackend(func() (acpConn, error) { return stub, nil })
	meta := SessionMeta{ID: "d1", Harness: "claude", Model: "sonnet", WorkDir: t.TempDir()}
	done := make(chan error, 1)
	go func() { done <- be.StartSession(meta) }()
	<-started
	promptDone := make(chan struct{})
	go func() {
		defer close(promptDone)
		be.OnPrompt("d1", "hi", func(kind, text, status string) {})
	}()
	// No sleep: the prompt cannot reach the adapter before ready closes,
	// which needs close(block). An empty prompt list here is conclusive.
	stub.mu.Lock()
	early := len(stub.prompts)
	stub.mu.Unlock()
	if early != 0 {
		t.Fatalf("prompts before MCP ready = %d, want 0", early)
	}
	close(block)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("start never finished")
	}
	select {
	case <-promptDone:
	case <-time.After(5 * time.Second):
		t.Fatal("prompt never ran after MCP ready")
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.prompts) != 1 || stub.prompts[0] != "hi" {
		t.Fatalf("prompts = %v", stub.prompts)
	}
}

// A prompt racing a failed start reports failed instead of hanging: drop
// closes ready, and OnPrompt rechecks the entry after it wakes.
func TestPromptAfterFailedStartReportsFailed(t *testing.T) {
	block := make(chan struct{})
	started := make(chan struct{})
	stub := &stubConn{blockNew: block, promptStarted: started, newErr: errNoAdapter}
	be := newStubClaudeBackend(func() (acpConn, error) { return stub, nil })
	meta := SessionMeta{ID: "d1", Harness: "claude", Model: "sonnet", WorkDir: t.TempDir()}
	startDone := make(chan error, 1)
	go func() { startDone <- be.StartSession(meta) }()
	<-started
	type emit struct{ kind, text, status string }
	promptDone := make(chan []emit, 1)
	go func() {
		var got []emit
		be.OnPrompt("d1", "hi", func(kind, text, status string) {
			got = append(got, emit{kind, text, status})
		})
		promptDone <- got
	}()
	close(block)
	select {
	case err := <-startDone:
		if err == nil {
			t.Fatal("start succeeded, want failure")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("start never finished")
	}
	var got []emit
	select {
	case got = <-promptDone:
	case <-time.After(5 * time.Second):
		t.Fatal("prompt hung after failed start")
	}
	seen := map[string]string{}
	for _, e := range got {
		seen[e.kind] = e.text + "|" + e.status
	}
	if seen["error"] != "claude session not started|" {
		t.Fatalf("error events = %v", got)
	}
	if seen["status"] != "|failed" {
		t.Fatalf("status events = %v", got)
	}
}
