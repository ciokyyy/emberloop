package mcpfake_test

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example.com/emberloop-web/el/tools/mcpfake"
)

// TestMcpFakeHelperProcess serves the fake in this binary when re-executed
// with the helper environment.
func TestMcpFakeHelperProcess(t *testing.T) {
	mcpfake.RunIfHelper(t)
}

// frames runs the fake for cfg, feeds it requests, and returns its first
// want replies. It reads exactly want lines: after stdin closes the
// re-executed test binary prints its own PASS line, which is not a reply.
func frames(t *testing.T, cfg mcpfake.Config, want int, requests ...string) []map[string]any {
	t.Helper()
	cmd := mcpfake.CommandForTest(t, cfg)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Wait() })
	for _, r := range requests {
		if _, err := stdin.Write([]byte(r + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	out := make([]map[string]any, 0, want)
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 8<<20)
	for len(out) < want && sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("fake reply is not JSON: %s", sc.Bytes())
		}
		out = append(out, m)
	}
	_ = stdin.Close()
	return out
}

func resultOf(m map[string]any) map[string]any {
	res, _ := m["result"].(map[string]any)
	return res
}

// The fake speaks the handshake the client performs: initialize,
// notifications/initialized (logged, never answered), tools/list and a
// tools/call round trip.
func TestFakeServesInitializeListAndCall(t *testing.T) {
	log := filepath.Join(t.TempDir(), "methods.log")
	replies := frames(t, mcpfake.Config{
		Tools:   []mcpfake.ToolDef{{Name: "search", Description: "s"}},
		Calls:   map[string]mcpfake.Result{"search": {Text: "hits"}},
		LogPath: log,
	}, 3,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"search","arguments":{}}}`,
	)
	if len(replies) != 3 {
		t.Fatalf("replies = %d, want 3 (the notification is never answered): %v", len(replies), replies)
	}
	if _, ok := resultOf(replies[0])["protocolVersion"]; !ok {
		t.Errorf("initialize reply has no protocol version: %v", replies[0])
	}
	tools, _ := resultOf(replies[1])["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("listed tools = %v, want one", tools)
	}
	content, _ := resultOf(replies[2])["content"].([]any)
	if len(content) != 1 || content[0].(map[string]any)["text"] != "hits" {
		t.Errorf("call content = %v, want hits", content)
	}
	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"initialize", "notifications/initialized", "tools/list", "tools/call"} {
		if !strings.Contains(string(raw), want+"\n") {
			t.Errorf("log %q misses %s", raw, want)
		}
	}
}

// An unknown tool and a scripted message are JSON-RPC errors, and exiting on
// start ends the process with the configured code.
func TestFakeAnswersErrorsAndExits(t *testing.T) {
	replies := frames(t, mcpfake.Config{
		Tools: []mcpfake.ToolDef{{Name: "search"}},
		Calls: map[string]mcpfake.Result{"search": {Message: "boom"}},
	}, 2,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ghost"}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"search"}}`,
	)
	if len(replies) != 2 {
		t.Fatalf("replies = %v, want two errors", replies)
	}
	for i, want := range []string{`unknown tool "ghost"`, "boom"} {
		errObj, _ := replies[i]["error"].(map[string]any)
		msg, _ := errObj["message"].(string)
		if msg != want {
			t.Errorf("reply %d error = %q, want %q", i, msg, want)
		}
	}
	cmd := mcpfake.CommandForTest(t, mcpfake.Config{ExitOn: "start", ExitCode: 3})
	if err := cmd.Run(); err == nil {
		t.Fatal("exiting fake ran clean")
	} else if exit, ok := err.(interface{ ExitCode() int }); !ok || exit.ExitCode() != 3 {
		t.Fatalf("fake exit = %v, want code 3", err)
	}
}
