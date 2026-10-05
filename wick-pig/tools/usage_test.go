package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"example.com/wick/tools/systemone"
)

func TestLogFindCallWritesOneLinePerCall(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WICK_STATE_DIR", dir)

	rec := FindUsage{
		Agent:      "agent-1",
		Model:      systemone.JevModelPinned,
		FindCallID: "find-abc",
		Questions:  16,
		Generation: "gen-dec-1",
		Provider:   "TypeSafe",
		Usage:      &systemone.Usage{InputTokens: 278, OutputTokens: 21, Cost: 0.000011676},
	}
	if err := LogFindCall(rec); err != nil {
		t.Fatalf("LogFindCall: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "usage", "agent-1.jsonl"))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	var line map[string]any
	if err := json.Unmarshal(raw, &line); err != nil {
		t.Fatalf("decode log line %q: %v", raw, err)
	}
	if line["agent"] != "agent-1" || line["kind"] != "find" {
		t.Errorf("line identity = %v", line)
	}
	if line["model"] != systemone.JevModelPinned || line["find_call_id"] != "find-abc" {
		t.Errorf("line attribution = %v", line)
	}
	if line["questions"] != float64(16) {
		t.Errorf("line questions = %v, want 16", line["questions"])
	}
	usage, _ := line["usage"].(map[string]any)
	if usage["input_tokens"] != float64(278) || usage["output_tokens"] != float64(21) {
		t.Errorf("line usage = %v", line["usage"])
	}
	if usage["cost"] != 0.000011676 {
		t.Errorf("line cost = %v", usage["cost"])
	}
	if line["generation"] != "gen-dec-1" || line["provider"] != "TypeSafe" {
		t.Errorf("line provenance = %v", line)
	}
	if line["at"] == nil || line["at"] == "" {
		t.Errorf("line has no timestamp: %v", line)
	}
}

func TestLogFindCallWithoutUsageStillLogsTheCall(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WICK_STATE_DIR", dir)

	if err := LogFindCall(FindUsage{Agent: "agent-2", Model: "m", FindCallID: "f", Questions: 3}); err != nil {
		t.Fatalf("LogFindCall: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "usage", "agent-2.jsonl"))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	var line map[string]any
	if err := json.Unmarshal(raw, &line); err != nil {
		t.Fatalf("decode %q: %v", raw, err)
	}
	if _, ok := line["usage"]; ok {
		t.Errorf("line carries usage = %v, want none", line["usage"])
	}
	if line["agent"] != "agent-2" || line["kind"] != "find" {
		t.Errorf("line = %v", line)
	}
}

func TestLogFindCallWithoutAgentWritesNothing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WICK_STATE_DIR", dir)

	if err := LogFindCall(FindUsage{Model: "m", FindCallID: "f", Questions: 1}); err != nil {
		t.Fatalf("LogFindCall: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err == nil && len(entries) > 0 {
		t.Errorf("wrote %v without an agent id", entries)
	}
}
