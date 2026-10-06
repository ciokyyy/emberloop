package mcp_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"example.com/wick"
	"example.com/wick/mcp"
	"example.com/wick/tools"
)

// serve pipes requests through a live server and returns the replies in order.
func serve(t *testing.T, served []tools.Tool, requests ...string) []map[string]any {
	t.Helper()
	in, out := io.Pipe()
	var buf bytes.Buffer
	done := make(chan error, 1)
	go func() { done <- mcp.Serve(context.Background(), in, &buf, t.TempDir(), served) }()
	for _, r := range requests {
		if _, err := io.WriteString(out, r+"\n"); err != nil {
			t.Fatal(err)
		}
	}
	out.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	var replies []map[string]any
	sc := bufio.NewScanner(&buf)
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("reply is not JSON: %s", sc.Bytes())
		}
		replies = append(replies, m)
	}
	return replies
}

// toolsOf digs the tool entries out of a tools/list reply.
func toolsOf(t *testing.T, reply map[string]any) []map[string]any {
	t.Helper()
	result, _ := reply["result"].(map[string]any)
	list, _ := result["tools"].([]any)
	out := make([]map[string]any, 0, len(list))
	for _, e := range list {
		m, _ := e.(map[string]any)
		out = append(out, m)
	}
	return out
}

// The harness batches read-only calls only when the server says they are safe
// to batch, and it must never be told a call that writes or spawns is.
func TestToolsListMarksExactlyReadFindGrepCheckReadOnly(t *testing.T) {
	replies := serve(t, wick.AllTools(), `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if len(replies) != 1 {
		t.Fatalf("want one reply, got %d", len(replies))
	}
	marked := map[string]bool{}
	for _, tl := range toolsOf(t, replies[0]) {
		name, _ := tl["name"].(string)
		ann, _ := tl["annotations"].(map[string]any)
		if ro, _ := ann["readOnlyHint"].(bool); ro {
			marked[name] = true
		}
	}
	want := map[string]bool{"read": true, "find": true, "grep": true, "check": true}
	if len(marked) != len(want) {
		t.Fatalf("readOnlyHint on %v, want exactly read, find, grep and check", marked)
	}
	for name := range want {
		if !marked[name] {
			t.Errorf("readOnlyHint missing on %s", name)
		}
	}
}

// slowTool answers after a delay and records how many copies ran at once, so
// the server's concurrency is observable without trusting wall-clock alone.
type slowTool struct {
	delay      time.Duration
	live, peak *int32
}

func (s slowTool) Name() string           { return "slow" }
func (s slowTool) Description() string    { return "waits" }
func (s slowTool) Schema() map[string]any { return map[string]any{"type": "object"} }
func (s slowTool) Execute(context.Context, string, json.RawMessage) (string, error) {
	n := atomic.AddInt32(s.live, 1)
	for {
		p := atomic.LoadInt32(s.peak)
		if n <= p || atomic.CompareAndSwapInt32(s.peak, p, n) {
			break
		}
	}
	time.Sleep(s.delay)
	atomic.AddInt32(s.live, -1)
	return "ok", nil
}

// Two calls asked at once must run at once: the server answers a slow call in
// its own goroutine so a second call does not queue behind it.
func TestTwoSlowCallsFinishInAboutOneCallsTime(t *testing.T) {
	const delay = 300 * time.Millisecond
	var live, peak int32
	call := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"slow"}}`
	call2 := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"slow"}}`

	start := time.Now()
	serve(t, []tools.Tool{slowTool{delay: delay, live: &live, peak: &peak}}, call, call2)
	elapsed := time.Since(start)

	if peak != 2 {
		t.Errorf("peak concurrency was %d, want 2: the calls ran in series", peak)
	}
	if elapsed > 2*delay {
		t.Errorf("two %s calls took %s: they ran in series", delay, elapsed)
	}
}

// tools/list offers one spawn tool, described with only the caller's spawnable
// types, and the scout tool is gone.
func TestToolsListHasSpawnAndNoScout(t *testing.T) {
	isolatedSettings(t)
	served, err := wick.RoleTools("implement")
	if err != nil {
		t.Fatal(err)
	}
	replies := serve(t, served, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	var spawnDesc string
	found := false
	for _, tl := range toolsOf(t, replies[0]) {
		name, _ := tl["name"].(string)
		if name == "scout" {
			t.Fatal("tools/list still serves the removed scout tool")
		}
		if name == "spawn" {
			found = true
			spawnDesc, _ = tl["description"].(string)
		}
	}
	if !found {
		t.Fatal("tools/list does not serve spawn")
	}
	if !strings.Contains(spawnDesc, "scout") {
		t.Errorf("implement may spawn scout; the description must list it:\n%s", spawnDesc)
	}
}

// echoTool returns a fixed literal, so a test can tell exactly what the
// server added around the tool's own output.
type echoTool struct{ text string }

func (e echoTool) Name() string           { return "echo" }
func (e echoTool) Description() string    { return "returns its literal" }
func (e echoTool) Schema() map[string]any { return map[string]any{"type": "object"} }
func (e echoTool) Execute(context.Context, string, json.RawMessage) (string, error) {
	return e.text, nil
}

// callText digs the tool result text out of a tools/call reply.
func callText(t *testing.T, reply map[string]any) string {
	t.Helper()
	result, _ := reply["result"].(map[string]any)
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatalf("tools/call reply has no content: %v", reply)
	}
	entry, _ := content[0].(map[string]any)
	text, _ := entry["text"].(string)
	return text
}

// A server whose binary did not change leaves the tool's text alone.
func TestUnchangedBinaryLeavesReplyAlone(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "wick-mcp")
	if err := os.WriteFile(bin, []byte("binary-v1"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WICK_BINARY_PATH", bin)

	replies := serve(t, []tools.Tool{echoTool{text: "hello"}},
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"echo"}}`)
	if len(replies) != 1 {
		t.Fatalf("want one reply, got %d", len(replies))
	}
	if got := callText(t, replies[0]); got != "hello" {
		t.Errorf("an unchanged server must leave the reply alone, got %q", got)
	}
}

// A server that could not read its binary's identity at start never marks a
// reply stale: without both identities there is nothing to compare, so an
// odd launch environment can never produce a false notice.
func TestUnknownBinaryLeavesReplyAlone(t *testing.T) {
	t.Setenv("WICK_BINARY_PATH", filepath.Join(t.TempDir(), "no-such-binary"))

	replies := serve(t, []tools.Tool{echoTool{text: "hello"}},
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"echo"}}`)
	if len(replies) != 1 {
		t.Fatalf("want one reply, got %d", len(replies))
	}
	if got := callText(t, replies[0]); got != "hello" {
		t.Errorf("a server with unknown identity must leave the reply alone, got %q", got)
	}
}

// gateTool blocks inside Execute so the test can replace the binary
// mid-call: the entered signal proves the server snapshotted its identity at
// start before the replacement lands.
type gateTool struct {
	entered chan struct{}
	release chan struct{}
}

func (g gateTool) Name() string           { return "gated" }
func (g gateTool) Description() string    { return "blocks until released" }
func (g gateTool) Schema() map[string]any { return map[string]any{"type": "object"} }
func (g gateTool) Execute(context.Context, string, json.RawMessage) (string, error) {
	close(g.entered)
	<-g.release
	return "hello", nil
}

// A server started before its binary changed says so in its next reply,
// keeping the tool's own output and adding the stale line.
func TestStaleBinaryAddsNoticeToNextReply(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "wick-mcp")
	if err := os.WriteFile(bin, []byte("binary-v1"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WICK_BINARY_PATH", bin)

	tool := gateTool{entered: make(chan struct{}), release: make(chan struct{})}
	in, out := io.Pipe()
	var buf bytes.Buffer
	done := make(chan error, 1)
	go func() { done <- mcp.Serve(context.Background(), in, &buf, t.TempDir(), []tools.Tool{tool}) }()
	if _, err := io.WriteString(out, "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"gated\"}}\n"); err != nil {
		t.Fatal(err)
	}
	<-tool.entered
	if err := os.WriteFile(bin, []byte("binary-v2"), 0o755); err != nil {
		t.Fatal(err)
	}
	close(tool.release)
	out.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	var replies []map[string]any
	sc := bufio.NewScanner(&buf)
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("reply is not JSON: %s", sc.Bytes())
		}
		replies = append(replies, m)
	}
	if len(replies) != 1 {
		t.Fatalf("want one reply, got %d", len(replies))
	}
	got := callText(t, replies[0])
	if !strings.Contains(got, "hello") {
		t.Errorf("the stale reply must keep the tool's output, got %q", got)
	}
	if !strings.Contains(strings.ToLower(got), "stale") || !strings.Contains(got, "binary") {
		t.Errorf("the stale reply must say the binary changed since start, got %q", got)
	}
	lines := strings.Split(strings.TrimSpace(got), "\n")
	if len(lines) < 2 || lines[0] != "hello" {
		t.Errorf("the stale line must be added after the tool's output, got %q", got)
	}
}

// waitTool blocks until its call is cancelled.
type waitTool struct{}

func (waitTool) Name() string           { return "wait" }
func (waitTool) Description() string    { return "waits for cancellation" }
func (waitTool) Schema() map[string]any { return map[string]any{"type": "object"} }
func (waitTool) Execute(ctx context.Context, _ string, _ json.RawMessage) (string, error) {
	select {
	case <-ctx.Done():
		return "cancelled", nil
	case <-time.After(10 * time.Second):
		return "never cancelled", nil
	}
}

// A cancellation notification ends the call it names, so a tool waiting on an
// agent stops waiting when the harness gives up on the call.
func TestCancelledNotificationEndsTheNamedCall(t *testing.T) {
	call := `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"wait"}}`
	cancel := `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":7}}`
	start := time.Now()
	replies := serve(t, []tools.Tool{waitTool{}}, call, cancel)
	if time.Since(start) > 5*time.Second {
		t.Fatal("the call kept waiting after it was cancelled")
	}
	body, _ := json.Marshal(replies)
	if !strings.Contains(string(body), "cancelled") {
		t.Errorf("want the tool to see its context end, got %s", body)
	}
}

// isolatedSettings points both settings layers at missing temp files, so a
// test asserting an exact tool list never reads the developer's own tools
// configuration.
func isolatedSettings(t *testing.T) {
	t.Helper()
	t.Setenv("WICK_SETTINGS", filepath.Join(t.TempDir(), "missing.json"))
	t.Setenv("WICK_PROJECT_SETTINGS", filepath.Join(t.TempDir(), "missing.json"))
	t.Setenv("WICK_DECISION_MODEL", "")
	t.Setenv("WICK_FIND_THRESHOLD", "")
	t.Setenv("WICK_MAX_WORKERS", "")
}

// listNames digs the tool names out of a tools/list reply.
func listNames(t *testing.T, reply map[string]any) []string {
	t.Helper()
	var out []string
	for _, tl := range toolsOf(t, reply) {
		name, _ := tl["name"].(string)
		out = append(out, name)
	}
	return out
}

// tools/list serves the resolved main list: a disabled tool is absent and a
// rename takes effect, matching the resolution the execute stub generator
// will read for the same settings.
func TestToolsListServesTheResolvedMainList(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(p, []byte(`{"tools":{"bash":{"enabled":false},"read":{"rename":"rd"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WICK_SETTINGS", p)
	t.Setenv("WICK_PROJECT_SETTINGS", filepath.Join(t.TempDir(), "missing.json"))
	t.Setenv("WICK_DECISION_MODEL", "")
	t.Setenv("WICK_FIND_THRESHOLD", "")
	t.Setenv("WICK_MAX_WORKERS", "")
	served, err := wick.RoleTools("work")
	if err != nil {
		t.Fatal(err)
	}
	replies := serve(t, served, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if len(replies) != 1 {
		t.Fatalf("want one reply, got %d", len(replies))
	}
	names := listNames(t, replies[0])
	for _, n := range names {
		if n == "bash" || n == "read" {
			t.Fatalf("tools/list serves a disabled or unrenamed tool: %v", names)
		}
	}
	found := false
	for _, n := range names {
		if n == "rd" {
			found = true
		}
	}
	if !found {
		t.Fatalf("tools/list does not serve the renamed read: %v", names)
	}
	main, execute, err := wick.ResolvedTools("work")
	if err != nil {
		t.Fatal(err)
	}
	var mainNames, executeNames []string
	for _, tl := range main {
		mainNames = append(mainNames, tl.Name())
	}
	for _, tl := range execute {
		executeNames = append(executeNames, tl.Name())
	}
	if len(names) != len(mainNames) {
		t.Fatalf("tools/list serves %v but resolution lists %v", names, mainNames)
	}
	for i := range names {
		if names[i] != mainNames[i] {
			t.Fatalf("tools/list serves %v but resolution lists %v", names, mainNames)
		}
	}
	if len(executeNames) != 1 || executeNames[0] != "check" {
		t.Fatalf("only check hides behind execute here, got %v", executeNames)
	}
}
