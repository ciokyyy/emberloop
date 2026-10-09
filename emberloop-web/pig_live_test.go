package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
)

// liveEchoTool is the probe's stand-in for the el MCP tool set: the
// only tool the stripped live Session may offer.
type liveEchoTool struct{ calls *atomic.Int64 }

func (t liveEchoTool) Name() string  { return "mcp__el__live_echo" }
func (t liveEchoTool) Label() string { return "live echo" }
func (t liveEchoTool) Schema() ai.ToolSchema {
	return ai.ToolSchema{
		Name:        t.Name(),
		Description: "Echo back the given text. For the pig Backend live probe only.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{"text": map[string]any{"type": "string"}},
		},
	}
}

func (t liveEchoTool) Execute(_ context.Context, _ string, params json.RawMessage, _ agent.ToolUpdateCallback) (agent.AgentToolResult, error) {
	t.calls.Add(1)
	return agent.AgentToolResult{
		Content: []ai.ToolResultMessageContent{ai.TextContent{Text: "echo:" + string(params)}},
	}, nil
}

func (t liveEchoTool) ExecutionMode() agent.ToolExecutionMode { return agent.ToolModeSequential }

// TestLivePigBackend proves the ticket with a real pig: a stripped Session
// answers, executes a tool, takes a follow-up after the turn, and closes
// cleanly. Not in the main check: set EL_LIVE_PIG=1 and PIG_LIVE_MODEL
// to a healthy provider/model (the ticket-01 spike's deepseek model stalls
// tool-call turns, so it cannot prove execution).
func TestLivePigBackend(t *testing.T) {
	if os.Getenv("EL_LIVE_PIG") == "" {
		t.Skip("set EL_LIVE_PIG=1 and PIG_LIVE_MODEL for the real-pig probe")
	}
	model := os.Getenv("PIG_LIVE_MODEL")
	if model == "" {
		t.Skip("set PIG_LIVE_MODEL to a healthy provider/model")
	}

	var calls atomic.Int64
	pb := NewPigBackend(liveEchoTool{calls: &calls})
	d := NewDaemon(t.TempDir(), NewFakeBackend())
	d.RegisterBackend("pig", pb)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	id := createSession(t, srv.URL, "pig", model, t.TempDir())
	if names := pb.ToolNames(id); len(names) != 1 || names[0] != "mcp__el__live_echo" {
		t.Fatalf("live session tools = %q, want only the echo tool", names)
	}

	sendPrompt(t, srv.URL, id, "Call the live_echo tool once with text alpha. Then reply with exactly: LIVE-DONE")
	first := pollEvents(t, srv.URL, id, 6*time.Minute, func(es []Event) bool {
		return hasKindWith(es, "assistant_chunk", "LIVE-DONE") && hasKind(es, "tool_call_started")
	})
	if calls.Load() == 0 {
		t.Fatal("echo tool never executed")
	}
	if !statusSeen(first, "idle") {
		t.Fatalf("no idle after tool turn in %+v", first)
	}

	pb.FollowUp(id, "Reply with exactly: FOLLOWUP-OK and nothing else.")
	second := pollEvents(t, srv.URL, id, 4*time.Minute, func(es []Event) bool {
		return hasKindWith(es, "assistant_chunk", "FOLLOWUP-OK")
	})
	_ = second

	req, _ := http.NewRequest("DELETE", srv.URL+"/api/sessions/"+id, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete status = %d", resp.StatusCode)
	}
	if names := pb.ToolNames(id); names != nil {
		t.Fatalf("closed live session still held: %q", names)
	}
}

func pollEvents(t *testing.T, base, id string, timeout time.Duration, done func([]Event) bool) []Event {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		get, err := http.Get(base + "/api/sessions/" + id)
		if err != nil {
			t.Fatal(err)
		}
		var out struct {
			Events []Event `json:"events"`
		}
		if err := json.NewDecoder(get.Body).Decode(&out); err != nil {
			get.Body.Close()
			t.Fatal(err)
		}
		get.Body.Close()
		if done(out.Events) {
			return out.Events
		}
		if time.Now().After(deadline) {
			t.Fatalf("live probe timed out with events %+v", out.Events)
		}
		time.Sleep(2 * time.Second)
	}
}

func hasKind(es []Event, kind string) bool {
	for _, e := range es {
		if e.Kind == kind {
			return true
		}
	}
	return false
}

func hasKindWith(es []Event, kind, sub string) bool {
	for _, e := range es {
		if e.Kind == kind && strings.Contains(e.Text, sub) {
			return true
		}
	}
	return false
}

func statusSeen(es []Event, status string) bool {
	for _, e := range es {
		if e.Kind == "status" && e.Status == status {
			return true
		}
	}
	return false
}
