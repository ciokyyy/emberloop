package herdrreport

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentLogWritesOneLinePerEventAndShrinksBulk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "a.log")
	l := newAgentLog(func() string { return path })
	l.write("turn_end", map[string]any{
		"stopReason": "toolUse",
		"payload":    map[string]any{"big": strings.Repeat("x", 5000)},
		"errorText":  strings.Repeat("e", 1000),
	})
	l.write("agent_settled", nil)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines = %d, want 2: %s", len(lines), b)
	}
	if !strings.Contains(lines[0], `"stopReason":"toolUse"`) || !strings.Contains(lines[0], `"bytes"`) {
		t.Fatalf("record lost scalar or bulk size: %s", lines[0])
	}
	if len(lines[0]) > 1200 {
		t.Fatalf("record not shrunk: %d bytes", len(lines[0]))
	}
}

func TestAgentLogUnwritablePathIsSilent(t *testing.T) {
	l := newAgentLog(func() string { return "" })
	l.write("turn_end", map[string]any{"a": 1}) // must not panic
}

func TestRegisterLogSkipsHighRateEvents(t *testing.T) {
	r := &registrar{}
	registerLog(r, newAgentLog(func() string { return "" }))
	for _, ev := range r.events {
		if ev == "message_update" || ev == "provider_stream_event" {
			t.Fatalf("registered high-rate event %s", ev)
		}
	}
	if len(r.events) != len(logEvents) {
		t.Fatalf("registered %d, want %d", len(r.events), len(logEvents))
	}
}
