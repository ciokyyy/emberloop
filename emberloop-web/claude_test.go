package web

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	el "example.com/emberloop"
)

// stubConn is the fake ACP adapter: canned updates, captured params, no
// subprocess. The seam under test is the ClaudeBackend, never internals.
type stubConn struct {
	mu          sync.Mutex
	newMeta     map[string]any
	newMCP      []acpMCPServer
	newCWD      string
	newMetas    []map[string]any
	newMCPs     [][]acpMCPServer
	newCWDs     []string
	loadMeta    map[string]any
	loadMCP     []acpMCPServer
	loadCWD     string
	loadID      string
	prompts     []string
	updates     []map[string]any
	permissions []stubPermission
	stopReason  string
	promptErr   error
	// promptErrs, when non-empty, serves one error per Prompt call head-first
	// (nil entries succeed), so retry tests script a failure then a success.
	promptErrs    []error
	newErr        error
	blockNew      chan struct{}
	promptStarted chan struct{}
	// blockPrompt holds a turn open until closed, so queue tests can
	// Steer mid-turn.
	blockPrompt chan struct{}
	// newCount counts NewSession calls so Handoff swaps rotate the token.
	// The first call still answers acp-session-1, keeping existing tests.
	newCount int
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
	s.newCWDs = append(s.newCWDs, cwd)
	s.newMCPs = append(s.newMCPs, mcp)
	s.newMetas = append(s.newMetas, meta)
	if s.newErr != nil {
		return "", s.newErr
	}
	s.newCount++
	return fmt.Sprintf("acp-session-%d", s.newCount), nil
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
	if len(s.promptErrs) > 0 {
		err = s.promptErrs[0]
		s.promptErrs = s.promptErrs[1:]
	}
	gate := s.blockPrompt
	s.mu.Unlock()
	if gate != nil {
		<-gate
	}
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
	t.Parallel()
	want := []string{"opus", "sonnet", "haiku"}
	if got := ClaudeModels(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ClaudeModels() = %v, want %v", got, want)
	}
}

// Slice 2: session/new and session/load carry the same full stripping set,
// with an absolute MCP command.
func TestNewAndLoadShareFullStrippingSet(t *testing.T) {
	t.Parallel()
	stub := &stubConn{}
	be := newStubClaudeBackend(func() (acpConn, error) { return stub, nil })
	meta := SessionMeta{ID: "d1", Harness: "claude", Model: "sonnet", WorkDir: tempDir(t)}
	if err := be.StartSession(meta); err != nil {
		t.Fatal(err)
	}
	mustStripped(t, stub.newMeta, "sonnet")
	if len(stub.newMCP) != 1 || stub.newMCP[0].Name != "el" {
		t.Fatalf("mcp servers = %+v, want one el server", stub.newMCP)
	}
	if stub.newMCP[0].Command != "/abs/stub-mcp" {
		t.Fatalf("mcp command = %q, want absolute", stub.newMCP[0].Command)
	}

	be2 := newStubClaudeBackend(func() (acpConn, error) { return stub, nil })
	if err := be2.LoadSession("d2", "acp-session-1", "opus", meta.WorkDir, "web", ownerDirs{}); err != nil {
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
	if opts["permissionMode"] != "bypassPermissions" {
		t.Fatalf("permissionMode = %v, want bypassPermissions", opts["permissionMode"])
	}
	if opts["strictMcpConfig"] != true {
		t.Fatalf("strictMcpConfig = %v, want true", opts["strictMcpConfig"])
	}
	allowed, _ := opts["allowedTools"].([]string)
	if !reflect.DeepEqual(allowed, []string{"mcp__el"}) {
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
	t.Parallel()
	stub := &stubConn{updates: []map[string]any{
		chunk("hello "),
		chunk("world"),
		{"sessionUpdate": "agent_thought_chunk", "content": map[string]any{"type": "text", "text": "thinking"}},
		{"sessionUpdate": "tool_call", "toolCallId": "t1", "title": "Spike Echo", "name": "mcp__el__echo",
			"rawInput": map[string]any{"text": "alpha"}},
		{"sessionUpdate": "tool_call_update", "toolCallId": "t1", "status": "completed",
			"content": []any{map[string]any{"type": "content", "content": map[string]any{"type": "text", "text": "echo:alpha"}}}},
		{"sessionUpdate": "tool_call_update", "toolCallId": "t2", "status": "completed",
			"content": []any{map[string]any{"type": "diff", "path": "/w/f.txt", "newText": "new"}}},
	}}
	be := newStubClaudeBackend(func() (acpConn, error) { return stub, nil })
	meta := SessionMeta{ID: "d1", Harness: "claude", Model: "haiku", WorkDir: tempDir(t)}
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
	if len(kinds["tool_call_started"]) != 1 || !strings.Contains(kinds["tool_call_started"][0], "Spike Echo") {
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
	t.Parallel()
	stub := &stubConn{permissions: []stubPermission{
		{
			toolName: "mcp__el__echo",
			options: []any{
				map[string]any{"optionId": "allow-once", "name": "Yes", "kind": "allow_once"},
				map[string]any{"optionId": "reject", "name": "No", "kind": "reject_once"},
			},
		},
		{
			toolName: "mcp__el__other",
			options: []any{
				map[string]any{"optionId": "reject", "name": "No", "kind": "reject_once"},
				map[string]any{"optionId": "allow-always", "name": "Always", "kind": "allow_always"},
			},
		},
	}}
	be := newStubClaudeBackend(func() (acpConn, error) { return stub, nil })
	meta := SessionMeta{ID: "d1", Harness: "claude", Model: "opus", WorkDir: tempDir(t)}
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
	t.Parallel()
	block := make(chan struct{})
	started := make(chan struct{})
	stub := &stubConn{blockNew: block, promptStarted: started, updates: []map[string]any{chunk("late")}}
	be := newStubClaudeBackend(func() (acpConn, error) { return stub, nil })
	meta := SessionMeta{ID: "d1", Harness: "claude", Model: "sonnet", WorkDir: tempDir(t)}
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
	t.Parallel()
	block := make(chan struct{})
	started := make(chan struct{})
	stub := &stubConn{blockNew: block, promptStarted: started, newErr: errNoAdapter}
	be := newStubClaudeBackend(func() (acpConn, error) { return stub, nil })
	meta := SessionMeta{ID: "d1", Harness: "claude", Model: "sonnet", WorkDir: tempDir(t)}
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

// A DropSession racing a blocked start still wakes a waiting prompt:
// the Once signals ready even though the later failure-drop finds no entry.
func TestPromptAfterDropDuringStartReportsFailed(t *testing.T) {
	t.Parallel()
	block := make(chan struct{})
	started := make(chan struct{})
	stub := &stubConn{blockNew: block, promptStarted: started, newErr: errNoAdapter}
	be := newStubClaudeBackend(func() (acpConn, error) { return stub, nil })
	meta := SessionMeta{ID: "d1", Harness: "claude", Model: "sonnet", WorkDir: tempDir(t)}
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
	be.DropSession("d1")
	close(block)
	select {
	case <-startDone:
	case <-time.After(5 * time.Second):
		t.Fatal("start never finished")
	}
	var got []emit
	select {
	case got = <-promptDone:
	case <-time.After(5 * time.Second):
		t.Fatal("prompt hung after drop during start")
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

// Every claude Session carries the caveman ultra per-turn reminder as an
// inline UserPromptSubmit hook, and the system prompt holds the ultra layer.
func TestClaudeSessionsCarryVoiceReminder(t *testing.T) {
	dir := tempDir(t)
	bin := filepath.Join(dir, "el")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	meta := newStubClaudeBackend(nil).strippingMeta("sonnet")
	opts := meta["claudeCode"].(map[string]any)["options"].(map[string]any)
	raw, _ := json.Marshal(opts["settings"])
	if !strings.Contains(string(raw), "UserPromptSubmit") || !strings.Contains(string(raw), bin+" voice-reminder hook") {
		t.Fatalf("settings = %s, want the voice-reminder hook", raw)
	}
	if sp, _ := meta["systemPrompt"].(string); !strings.Contains(sp, "level: ultra") {
		t.Fatalf("systemPrompt lacks the ultra register: %q", sp)
	}
	t.Setenv("PATH", tempDir(t))
	opts = newStubClaudeBackend(nil).strippingMeta("sonnet")["claudeCode"].(map[string]any)["options"].(map[string]any)
	if _, ok := opts["settings"]; ok {
		t.Fatal("settings set without an el binary")
	}
}

// Claude Sessions get one bound el server: the Session's kind picks the role
// (top-level web, spawned group child web-worker, scout read-only), and the
// binding carries the Session the Daemon tools and sudo approvals act under.
// Scouts and children never see a second server.
func TestClaudeMCPServesSingleBoundServer(t *testing.T) {
	t.Parallel()
	b := newStubClaudeBackend(nil)
	b.SetDaemonAddr("http://127.0.0.1:9")
	for _, tc := range []struct {
		meta SessionMeta
		role string
	}{
		{SessionMeta{ID: "top"}, "web"},
		{SessionMeta{ID: "kid", ParentID: "top", GroupID: "g"}, "web-worker"},
		{SessionMeta{ID: "look", ParentID: "top", GroupID: "g", Scout: true}, "scout"},
		{SessionMeta{ID: "rw", ParentID: "run", RunWorker: true}, "run-worker"},
	} {
		if got := elRoleFor(tc.meta); got != tc.role {
			t.Errorf("elRoleFor(%+v) = %q, want %q", tc.meta, got, tc.role)
		}
		servers, err := b.mcpServersFor(tc.meta.ID, elRoleFor(tc.meta))
		if err != nil {
			t.Fatal(err)
		}
		if len(servers) != 1 || servers[0].Name != "el" {
			t.Fatalf("servers = %+v, want one el server", servers)
		}
		if len(servers[0].Args) != 2 || servers[0].Args[0] != "--role" || servers[0].Args[1] != tc.role {
			t.Errorf("server args = %v, want --role %s", servers[0].Args, tc.role)
		}
		env := map[string]string{}
		for _, kv := range servers[0].Env {
			env[kv[0]] = kv[1]
		}
		if env["EL_WEB_PARENT"] != tc.meta.ID || env["EL_WEB_DAEMON_URL"] != "http://127.0.0.1:9" {
			t.Errorf("server env = %v, want the Session plus the Daemon URL", env)
		}
	}
	// Unbound, the listing is still the bare base server.
	bare := newStubClaudeBackend(nil)
	servers, err := bare.mcpServersFor("sess-1", "web")
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 1 || len(servers[0].Env) != 0 {
		t.Fatalf("unbound servers = %+v, want one bare server", servers)
	}
}

// The claude system prompt rewords the shared submit-settlement lines while
// the flag is off: delivery on settle, last message as the result. With the
// flag on the shared wording stays verbatim.
func TestClaudeSystemPromptFollowsSubmitFlag(t *testing.T) {
	if el.WebSubmitEnabled {
		t.Fatal("WebSubmitEnabled must default false")
	}
	off := claudeSystemPrompt()
	if strings.Contains(off, "yields right after submit") {
		t.Fatalf("flag-off prompt still tells children to submit: %q", off)
	}
	if !strings.Contains(off, "arrives when the last agent of that call settles.") {
		t.Fatalf("flag-off prompt missing settle delivery: %q", off)
	}
	if !strings.Contains(off, "its last message is delivered as its result.") {
		t.Fatalf("flag-off prompt missing last-message result: %q", off)
	}
	el.WebSubmitEnabled = true
	t.Cleanup(func() { el.WebSubmitEnabled = false })
	on := claudeSystemPrompt()
	if !strings.Contains(on, "has submitted.") || !strings.Contains(on, "yields right after submit") {
		t.Fatalf("flag-on prompt must keep the shared submit wording: %q", on)
	}
}

// The OAuth refresh race maps to one retry plus an actionable session line.
// Serial: it shortens the package-wide retry pause.
func TestClaudeLoginRaceRetriesOnceThenAdvises(t *testing.T) {
	oldRetry := claudeLoginRetry
	claudeLoginRetry = time.Millisecond
	t.Cleanup(func() { claudeLoginRetry = oldRetry })
	race := fmt.Errorf("acp: Internal error: Failed to refresh OAuth token: another Claude Code process is refreshing it or exited mid-refresh")

	// A transient race succeeds on retry with no error Event.
	stub := &stubConn{promptErrs: []error{race, nil}}
	be := newStubClaudeBackend(func() (acpConn, error) { return stub, nil })
	be.tokenFile = "/tmp/fake-state/enderbloom/claude-oauth-token"
	meta := SessionMeta{ID: "d1", Harness: "claude", Model: "haiku", WorkDir: tempDir(t)}
	if err := be.StartSession(meta); err != nil {
		t.Fatal(err)
	}
	var kinds []string
	be.OnPrompt("d1", "hi", func(kind, text, status string) { kinds = append(kinds, kind) })
	stub.mu.Lock()
	prompts := len(stub.prompts)
	stub.mu.Unlock()
	if prompts != 2 {
		t.Fatalf("prompts after a transient race = %d, want 2 (one retry)", prompts)
	}
	for _, k := range kinds {
		if k == "error" {
			t.Fatalf("error Event after a recovered race: %v", kinds)
		}
	}

	// A persistent race fails with the setup-token advice naming the file,
	// and no token value anywhere in the transcript.
	stub2 := &stubConn{promptErrs: []error{race, race}}
	be2 := newStubClaudeBackend(func() (acpConn, error) { return stub2, nil })
	be2.tokenFile = "/tmp/fake-state/enderbloom/claude-oauth-token"
	meta2 := SessionMeta{ID: "d2", Harness: "claude", Model: "haiku", WorkDir: tempDir(t)}
	if err := be2.StartSession(meta2); err != nil {
		t.Fatal(err)
	}
	var errs, statuses []string
	be2.OnPrompt("d2", "hi", func(kind, text, status string) {
		if kind == "error" {
			errs = append(errs, text)
		}
		if kind == "status" {
			statuses = append(statuses, status)
		}
	})
	if len(errs) != 1 || !strings.Contains(errs[0], "Claude login race") || !strings.Contains(errs[0], "claude setup-token") {
		t.Fatalf("race error Events = %v, want one actionable line", errs)
	}
	if !strings.Contains(errs[0], "/tmp/fake-state/enderbloom/claude-oauth-token") {
		t.Fatalf("race advice = %q, want the token file path", errs[0])
	}
	if len(statuses) == 0 || statuses[len(statuses)-1] != "failed" {
		t.Fatalf("statuses = %v, want a terminal failed", statuses)
	}

	// A dead adapter is not a plain failure: it respawns and retries the
	// same prompt (see acpretry_test.go). A non-race, non-close prompt
	// error still neither retries nor advises.
	stub3 := &stubConn{promptErr: fmt.Errorf("acp: session/prompt: model overloaded")}
	be3 := newStubClaudeBackend(func() (acpConn, error) { return stub3, nil })
	meta3 := SessionMeta{ID: "d3", Harness: "claude", Model: "haiku", WorkDir: tempDir(t)}
	if err := be3.StartSession(meta3); err != nil {
		t.Fatal(err)
	}
	errs = nil
	be3.OnPrompt("d3", "hi", func(kind, text, status string) {
		if kind == "error" {
			errs = append(errs, text)
		}
	})
	stub3.mu.Lock()
	plain := len(stub3.prompts)
	stub3.mu.Unlock()
	if plain != 1 {
		t.Fatalf("prompts after a plain failure = %d, want 1 (no retry)", plain)
	}
	if len(errs) != 1 || strings.Contains(errs[0], "Claude login race") {
		t.Fatalf("plain error Events = %v, want the raw error only", errs)
	}
}
