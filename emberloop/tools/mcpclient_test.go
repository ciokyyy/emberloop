package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"example.com/emberloop/tools/mcpfake"
)

// killProbe signals a pid the way a supervisor checks liveness: signal 0
// performs error checking without delivering anything.
func killProbe(pid int, sig syscall.Signal) error { return syscall.Kill(pid, sig) }

// fakeRegistry builds a registry with one server, the fake, and closes it
// when the test ends, so no child outlives the test.
func fakeRegistry(t *testing.T, cfg mcpfake.Config) *Registry {
	t.Helper()
	command, args, env := mcpfake.Helper(t, cfg)
	reg := NewRegistry(map[string]MCPServerSetting{
		"fake": {Command: command, Args: args, Env: env},
	})
	t.Cleanup(reg.Close)
	return reg
}

func fakeToolNames(ts []Tool) []string {
	out := make([]string, 0, len(ts))
	for _, tl := range ts {
		out = append(out, tl.Name())
	}
	return out
}

// Listing a server's tools starts it, runs initialize plus
// notifications/initialized, and returns its tools with their descriptions,
// schemas and read-only marks.
func TestMCPRegistryListsToolsFromFakeServer(t *testing.T) {
	log := filepath.Join(t.TempDir(), "methods.log")
	reg := fakeRegistry(t, mcpfake.Config{
		Tools: []mcpfake.ToolDef{
			{Name: "search", Description: "search the web",
				Schema: map[string]any{"type": "object", "properties": map[string]any{
					"query": map[string]any{"type": "string"},
				}}},
			{Name: "fetch", Description: "fetch a page", ReadOnly: true},
		},
		LogPath: log,
	})
	got, err := reg.Tools(context.Background(), "fake")
	if err != nil {
		t.Fatal(err)
	}
	if names := fakeToolNames(got); len(names) != 2 || names[0] != "search" || names[1] != "fetch" {
		t.Fatalf("tools = %v, want [search fetch]", names)
	}
	for _, tl := range got {
		if tl.Name() == "search" {
			if tl.Description() != "search the web" {
				t.Errorf("search description = %q", tl.Description())
			}
			props, _ := tl.Schema()["properties"].(map[string]any)
			if _, ok := props["query"]; !ok {
				t.Errorf("search schema lost its query property: %v", tl.Schema())
			}
			if ro, ok := tl.(ReadOnly); !ok || ro.ReadOnly() {
				t.Errorf("search must not be marked read-only")
			}
		}
		if tl.Name() == "fetch" {
			ro, ok := tl.(ReadOnly)
			if !ok || !ro.ReadOnly() {
				t.Errorf("fetch must carry the server's readOnlyHint")
			}
		}
	}
	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"initialize", "notifications/initialized", "tools/list"} {
		if !strings.Contains(string(raw), want+"\n") {
			t.Errorf("server never saw %s: %q", want, raw)
		}
	}
}

// A configured server spawns no process until one of its tools is listed or
// called: building the registry alone starts nothing.
func TestMCPRegistryStartsServerLazily(t *testing.T) {
	command, args, env := mcpfake.Helper(t, mcpfake.Config{
		Tools: []mcpfake.ToolDef{{Name: "search"}},
	})
	reg := NewRegistry(map[string]MCPServerSetting{
		"fake": {Command: command, Args: args, Env: env},
	})
	t.Cleanup(reg.Close)
	reg.mu.Lock()
	c := reg.clients["fake"]
	reg.mu.Unlock()
	c.mu.Lock()
	started := c.cmd != nil
	c.mu.Unlock()
	if started {
		t.Fatal("registry construction started the server process")
	}
	if _, err := reg.Tools(context.Background(), "fake"); err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cmd == nil || c.cmd.Process == nil {
		t.Fatal("listing did not start the server process")
	}
}

// Calling a listed tool runs the server's tool with the caller's arguments
// and returns its text.
func TestMCPCallToolReturnsTextAndForwardsArgs(t *testing.T) {
	reg := fakeRegistry(t, mcpfake.Config{
		Tools: []mcpfake.ToolDef{{Name: "search"}},
	})
	got, err := reg.Tools(context.Background(), "fake")
	if err != nil {
		t.Fatal(err)
	}
	text, err := got[0].Execute(context.Background(), t.TempDir(), []byte(`{"query":"emberloop"}`))
	if err != nil {
		t.Fatal(err)
	}
	if text != `search({"query":"emberloop"})` {
		t.Fatalf("call text = %q, want the echo with arguments", text)
	}
}

// A scripted tool result answers verbatim.
func TestMCPCallToolReturnsScriptedText(t *testing.T) {
	reg := fakeRegistry(t, mcpfake.Config{
		Tools: []mcpfake.ToolDef{{Name: "search", Description: "s"}},
		Calls: map[string]mcpfake.Result{"search": {Text: "two hits"}},
	})
	got, err := reg.Tools(context.Background(), "fake")
	if err != nil {
		t.Fatal(err)
	}
	text, err := got[0].Execute(context.Background(), t.TempDir(), []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if text != "two hits" {
		t.Fatalf("call text = %q, want two hits", text)
	}
}

// The tools/list answer is cached for the session: two listings cost one
// tools/list round trip.
func TestMCPRegistryCachesTheToolList(t *testing.T) {
	log := filepath.Join(t.TempDir(), "methods.log")
	reg := fakeRegistry(t, mcpfake.Config{
		Tools:   []mcpfake.ToolDef{{Name: "search"}},
		LogPath: log,
	})
	ctx := context.Background()
	if _, err := reg.Tools(ctx, "fake"); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Tools(ctx, "fake"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(raw), "tools/list\n"); n != 1 {
		t.Fatalf("tools/list reached the server %d times, want once: %q", n, raw)
	}
}

// The server's include, exclude and rename shape the exposed set: globs
// filter over the original names, survivors serve under their renames, and a
// renamed tool still calls the server under its original name.
func TestMCPRegistryShapesToolsWithIncludeExcludeAndRename(t *testing.T) {
	command, args, env := mcpfake.Helper(t, mcpfake.Config{
		Tools: []mcpfake.ToolDef{{Name: "search_repos"}, {Name: "delete_repo"}, {Name: "get_repo"}},
	})
	reg := NewRegistry(map[string]MCPServerSetting{
		"fake": {
			Command: command, Args: args, Env: env,
			Include: []string{"search_*", "get_*"},
			Exclude: []string{"delete_*", "get_repo"},
			Rename:  map[string]string{"search_repos": "gh_search"},
		},
	})
	t.Cleanup(reg.Close)
	got, err := reg.Tools(context.Background(), "fake")
	if err != nil {
		t.Fatal(err)
	}
	if names := fakeToolNames(got); len(names) != 1 || names[0] != "gh_search" {
		t.Fatalf("tools = %v, want [gh_search]", names)
	}
	if orig, ok := got[0].(interface{ OriginalName() string }); !ok || orig.OriginalName() != "search_repos" {
		t.Fatalf("renamed tool lost its original name: %#v", got[0])
	}
	text, err := got[0].Execute(context.Background(), t.TempDir(), []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if text != "search_repos({})" {
		t.Fatalf("renamed tool called as %q, want the original name", text)
	}
}

// shortRegistry is a registry whose waits fail fast, so a hang would fail the
// test at the client timeout instead of the suite timeout.
func shortRegistry(t *testing.T, cfg mcpfake.Config) *Registry {
	t.Helper()
	reg := fakeRegistry(t, cfg)
	reg.SetTimeouts(300*time.Millisecond, 300*time.Millisecond)
	return reg
}

func mustMCPContain(t *testing.T, err error, wants ...string) string {
	t.Helper()
	if err == nil {
		t.Fatalf("want error containing %q, got nil", wants)
	}
	for _, want := range wants {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not name %q", err, want)
		}
	}
	return err.Error()
}

// A server that never answers initialize fails bounded with an error naming
// the server, and leaves no child behind.
func TestMCPInitTimeoutNamesTheServer(t *testing.T) {
	reg := shortRegistry(t, mcpfake.Config{
		Tools: []mcpfake.ToolDef{{Name: "search"}}, HangOn: "initialize",
	})
	mustMCPContain(t, mustToolsErr(t, reg), `mcp server "fake"`, "initialize")
	reg.mu.Lock()
	c := reg.clients["fake"]
	reg.mu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cmd != nil {
		t.Fatal("timed-out initialize left the child running")
	}
}

func mustToolsErr(t *testing.T, reg *Registry) error {
	t.Helper()
	_, err := reg.Tools(context.Background(), "fake")
	return err
}

// A server that never answers a call fails bounded with an error naming the
// server and the tool.
func TestMCPCallTimeoutNamesServerAndTool(t *testing.T) {
	reg := shortRegistry(t, mcpfake.Config{
		Tools: []mcpfake.ToolDef{{Name: "search"}}, HangOn: "call",
	})
	got, err := reg.Tools(context.Background(), "fake")
	if err != nil {
		t.Fatal(err)
	}
	_, err = got[0].Execute(context.Background(), t.TempDir(), []byte(`{}`))
	mustMCPContain(t, err, `mcp server "fake"`, "search", "timed out")
}

// A server that exits mid-call fails the call with an error naming the
// server instead of hanging on a dead pipe.
func TestMCPServerExitMidCallNamesTheServer(t *testing.T) {
	reg := shortRegistry(t, mcpfake.Config{
		Tools: []mcpfake.ToolDef{{Name: "search"}}, ExitOn: "call",
	})
	got, err := reg.Tools(context.Background(), "fake")
	if err != nil {
		t.Fatal(err)
	}
	_, err = got[0].Execute(context.Background(), t.TempDir(), []byte(`{}`))
	mustMCPContain(t, err, `mcp server "fake"`, "search", "exited")
}

// A server that exits at once fails listing with an error naming the server.
func TestMCPServerExitOnStartNamesTheServer(t *testing.T) {
	reg := shortRegistry(t, mcpfake.Config{ExitOn: "start", ExitCode: 3})
	mustMCPContain(t, mustToolsErr(t, reg), `mcp server "fake"`)
}

// A command that does not exist fails with an error naming the server.
func TestMCPServerFailedToStartNamesTheServer(t *testing.T) {
	reg := NewRegistry(map[string]MCPServerSetting{
		"fake": {Command: filepath.Join(t.TempDir(), "no-such-server")},
	})
	t.Cleanup(reg.Close)
	reg.SetTimeouts(300*time.Millisecond, 300*time.Millisecond)
	mustMCPContain(t, mustToolsErr(t, reg), `mcp server "fake"`, "failed to start")
}

// A JSON-RPC error reply is a tool error naming the server and the message.
func TestMCPProtocolErrorNamesTheServer(t *testing.T) {
	reg := shortRegistry(t, mcpfake.Config{
		Tools: []mcpfake.ToolDef{{Name: "search"}},
		Calls: map[string]mcpfake.Result{"search": {Message: "boom"}},
	})
	got, err := reg.Tools(context.Background(), "fake")
	if err != nil {
		t.Fatal(err)
	}
	_, err = got[0].Execute(context.Background(), t.TempDir(), []byte(`{}`))
	mustMCPContain(t, err, `mcp server "fake"`, "search", "boom")
}

// A tool that answers isError is a Go error carrying the tool's text.
func TestMCPRemoteToolErrorCarriesItsText(t *testing.T) {
	reg := shortRegistry(t, mcpfake.Config{
		Tools: []mcpfake.ToolDef{{Name: "search"}},
		Calls: map[string]mcpfake.Result{"search": {Text: "quota spent", IsError: true}},
	})
	got, err := reg.Tools(context.Background(), "fake")
	if err != nil {
		t.Fatal(err)
	}
	_, err = got[0].Execute(context.Background(), t.TempDir(), []byte(`{}`))
	mustMCPContain(t, err, `mcp server "fake"`, "quota spent")
}

// Listing an unconfigured server is an error, not a hang.
func TestMCPRegistryRejectsUnknownServers(t *testing.T) {
	reg := fakeRegistry(t, mcpfake.Config{Tools: []mcpfake.ToolDef{{Name: "search"}}})
	mustMCPContain(t, mustToolsErrFor(t, reg, "ghost"), `unknown mcp server "ghost"`)
}

func mustToolsErrFor(t *testing.T, reg *Registry, server string) error {
	t.Helper()
	_, err := reg.Tools(context.Background(), server)
	return err
}

// Closing the registry kills the server child: its pid is gone afterwards.
// The test closes the registry itself instead of through cleanup, so the
// assertion runs after Close.
func TestMCPRegistryCloseKillsTheChild(t *testing.T) {
	command, args, env := mcpfake.Helper(t, mcpfake.Config{
		Tools: []mcpfake.ToolDef{{Name: "search"}},
	})
	reg := NewRegistry(map[string]MCPServerSetting{
		"fake": {Command: command, Args: args, Env: env},
	})
	if _, err := reg.Tools(context.Background(), "fake"); err != nil {
		t.Fatal(err)
	}
	reg.mu.Lock()
	c := reg.clients["fake"]
	reg.mu.Unlock()
	c.mu.Lock()
	pid := c.cmd.Process.Pid
	c.mu.Unlock()
	if err := killProbe(pid, 0); err != nil {
		t.Fatalf("child %d not running before Close: %v", pid, err)
	}
	reg.Close()
	if err := killProbe(pid, 0); err == nil {
		t.Fatalf("child %d still running after Close", pid)
	}
}

// A snapshot lists every server and reports the broken ones: one bad server
// costs its own error, not the good server's tools.
func TestMCPRegistrySnapshotSkipsFailedServers(t *testing.T) {
	command, args, env := mcpfake.Helper(t, mcpfake.Config{
		Tools: []mcpfake.ToolDef{{Name: "search"}},
	})
	reg := NewRegistry(map[string]MCPServerSetting{
		"good": {Command: command, Args: args, Env: env},
		"bad":  {Command: filepath.Join(t.TempDir(), "no-such-server")},
	})
	t.Cleanup(reg.Close)
	reg.SetTimeouts(300*time.Millisecond, 300*time.Millisecond)
	listed, errs := reg.Snapshot(context.Background())
	if names := fakeToolNames(listed["good"]); len(names) != 1 || names[0] != "search" {
		t.Fatalf("good server tools = %v", listed["good"])
	}
	if _, ok := listed["bad"]; ok {
		t.Fatalf("bad server listed tools: %v", listed["bad"])
	}
	if len(errs) != 1 {
		t.Fatalf("errs = %v, want one naming the bad server", errs)
	}
	if !strings.Contains(errs[0].Error(), `mcp server "bad"`) {
		t.Fatalf("errs = %v, want the bad server named", errs)
	}
}

// A call the caller abandons fails without touching the server: nothing
// waits on the child, it stays running, and the client stays usable.
func TestMCPCallCancelledByCallerLeavesServerRunning(t *testing.T) {
	reg := shortRegistry(t, mcpfake.Config{
		Tools: []mcpfake.ToolDef{{Name: "search"}}, HangOn: "call",
	})
	got, err := reg.Tools(context.Background(), "fake")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := got[0].Execute(ctx, t.TempDir(), []byte(`{}`))
		done <- err
	}()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		mustMCPContain(t, err, `mcp server "fake"`, "search")
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled call never returned")
	}
	reg.mu.Lock()
	c := reg.clients["fake"]
	reg.mu.Unlock()
	c.mu.Lock()
	cmd, started := c.cmd, c.started
	c.mu.Unlock()
	if cmd == nil || cmd.Process == nil || !started {
		t.Fatal("cancelled call killed or unstarted the server")
	}
	if err := killProbe(cmd.Process.Pid, 0); err != nil {
		t.Fatalf("server child gone after cancel: %v", err)
	}
	if _, err := reg.Tools(context.Background(), "fake"); err != nil {
		t.Fatalf("client unusable after cancel: %v", err)
	}
}

// When the server dies mid-session the next use respawns it: the reader's
// cleanup retires the old spawn and the call goes out on a new child.
func TestMCPRespawnsAfterServerExit(t *testing.T) {
	reg := shortRegistry(t, mcpfake.Config{
		Tools: []mcpfake.ToolDef{{Name: "search"}},
	})
	got, err := reg.Tools(context.Background(), "fake")
	if err != nil {
		t.Fatal(err)
	}
	reg.mu.Lock()
	c := reg.clients["fake"]
	reg.mu.Unlock()
	c.mu.Lock()
	old := c.cmd.Process.Pid
	c.mu.Unlock()
	if err := syscall.Kill(old, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		c.mu.Lock()
		started := c.started
		c.mu.Unlock()
		if !started {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("dead server was never retired")
		}
		time.Sleep(10 * time.Millisecond)
	}
	text, err := got[0].Execute(context.Background(), t.TempDir(), []byte(`{}`))
	if err != nil {
		t.Fatalf("call after respawn: %v", err)
	}
	if text != "search({})" {
		t.Fatalf("call after respawn = %q", text)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cmd == nil || c.cmd.Process.Pid == old {
		t.Fatal("call after exit did not spawn a new child")
	}
}

// Calls after Close fail instead of respawning: the registry stays dead.
func TestMCPUseAfterCloseFails(t *testing.T) {
	command, args, env := mcpfake.Helper(t, mcpfake.Config{
		Tools: []mcpfake.ToolDef{{Name: "search"}},
	})
	reg := NewRegistry(map[string]MCPServerSetting{
		"fake": {Command: command, Args: args, Env: env},
	})
	got, err := reg.Tools(context.Background(), "fake")
	if err != nil {
		t.Fatal(err)
	}
	reg.Close()
	mustMCPContain(t, mustToolsErr(t, reg), `mcp server "fake"`, "closed")
	_, err = got[0].Execute(context.Background(), t.TempDir(), []byte(`{}`))
	mustMCPContain(t, err, `mcp server "fake"`, "closed")
	reg.mu.Lock()
	c := reg.clients["fake"]
	reg.mu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cmd != nil || c.started {
		t.Fatal("use after Close respawned the server")
	}
}
