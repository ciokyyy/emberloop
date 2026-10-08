//go:build live

package web

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestLiveClaudeBackend proves the ticket-04 checkboxes against the real
// pinned adapter. Never runs in the main check (it needs claude login,
// node and network):
//
//	EMBERLOOP_LIVE=claude go test -tags live -run TestLiveClaudeBackend .
//
// It uses the spike echo server as the MCP server, so no emberloop-mcp
// binary is needed.
func TestLiveClaudeBackend(t *testing.T) {
	if os.Getenv("EMBERLOOP_LIVE") != "claude" {
		t.Skip("set EMBERLOOP_LIVE=claude for the real-adapter probe")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not on PATH")
	}
	echo := filepath.Join("spike", "claude", "mcp-echo-server.mjs")
	if _, err := os.Stat(echo); err != nil {
		t.Fatalf("echo server missing: %v", err)
	}
	absEcho, _ := filepath.Abs(echo)
	cwd := t.TempDir()

	be := NewClaudeBackend()
	be.mcpName = "spike"
	be.mcpCommand = node
	be.mcpArgs = []string{absEcho}
	be.allowedTools = []string{"mcp__spike__spike_echo"}

	meta := SessionMeta{ID: "live-claude-1", Harness: "claude", Model: "haiku", WorkDir: cwd}
	if err := be.StartSession(meta); err != nil {
		t.Fatalf("start (MCP ready): %v", err)
	}

	var mu sync.Mutex
	var got []Event
	emit := func(kind, text, status string) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, Event{Kind: kind, Text: text, Status: status})
	}
	be.OnPrompt(meta.ID, "Call the spike_echo tool once with text alpha. Then reply with exactly: SPIKE-DONE", emit)

	native := []string{"Task", "Bash", "Read", "Write", "Edit", "Glob", "Grep", "WebFetch", "WebSearch"}
	var chunks, tools int
	var sawEcho, sawDone bool
	for _, e := range got {
		switch e.Kind {
		case "assistant_chunk":
			chunks++
			if strings.Contains(e.Text, "SPIKE-DONE") {
				sawDone = true
			}
		case "tool_call_started":
			tools++
			if strings.Contains(e.Text, "spike_echo") {
				sawEcho = true
			}
			for _, n := range native {
				if strings.Contains(e.Text, "["+n+"]") || strings.HasSuffix(e.Text, " "+n) {
					t.Errorf("native tool offered: %q", e.Text)
				}
			}
		case "permission":
			if !strings.Contains(e.Text, "allow-once") {
				t.Errorf("permission not allow-once: %q", e.Text)
			}
		}
	}
	if chunks == 0 {
		t.Error("no assistant chunks streamed")
	}
	if !sawEcho {
		t.Errorf("echo tool never called: %+v", got)
	}
	if !sawDone {
		t.Errorf("SPIKE-DONE missing: %+v", got)
	}

	// session/load repeats the stripping set and answers again.
	be.mu.Lock()
	acpID := be.sessions[meta.ID].acpID
	be.mu.Unlock()
	be2 := NewClaudeBackend()
	be2.mcpName = "spike"
	be2.mcpCommand = node
	be2.mcpArgs = []string{absEcho}
	be2.allowedTools = []string{"mcp__spike__spike_echo"}
	if err := be2.LoadSession("live-claude-2", acpID, "haiku", cwd); err != nil {
		t.Fatalf("load: %v", err)
	}
	var got2 []Event
	be2.OnPrompt("live-claude-2", "Reply with exactly: LOADED-OK", func(kind, text, status string) {
		got2 = append(got2, Event{Kind: kind, Text: text, Status: status})
	})
	found := false
	for _, e := range got2 {
		if e.Kind == "assistant_chunk" && strings.Contains(e.Text, "LOADED-OK") {
			found = true
		}
	}
	if !found {
		t.Errorf("loaded session did not answer: %+v", got2)
	}
}
