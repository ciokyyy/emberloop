package mcp_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
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
func TestToolsListMarksExactlyReadFindAndScoutReadOnly(t *testing.T) {
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
	want := map[string]bool{"read": true, "find": true, "scout": true}
	if len(marked) != len(want) {
		t.Fatalf("readOnlyHint on %v, want exactly read, find, scout", marked)
	}
	for name := range want {
		if !marked[name] {
			t.Errorf("readOnlyHint missing on %s", name)
		}
	}
}

// slowTool answers after a delay, so the server's concurrency is observable.
type slowTool struct{ delay time.Duration }

func (s slowTool) Name() string           { return "slow" }
func (s slowTool) Description() string    { return "waits" }
func (s slowTool) Schema() map[string]any { return map[string]any{"type": "object"} }
func (s slowTool) Execute(context.Context, string, json.RawMessage) (string, error) {
	time.Sleep(s.delay)
	return "ok", nil
}

// Two calls asked at once must run at once: the server answers a slow call in
// its own goroutine so a second call does not queue behind it.
func TestTwoSlowCallsFinishInAboutOneCallsTime(t *testing.T) {
	const delay = 300 * time.Millisecond
	call := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"slow"}}`
	call2 := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"slow"}}`

	start := time.Now()
	serve(t, []tools.Tool{slowTool{delay}}, call, call2)
	elapsed := time.Since(start)

	if elapsed > 3*delay/2 {
		t.Errorf("two %s calls took %s: they ran in series", delay, elapsed)
	}
}
