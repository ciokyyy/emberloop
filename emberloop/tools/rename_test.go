package tools

import (
	"context"
	"encoding/json"
	"testing"
)

// stubTool is a minimal Tool for the rename tests.
type stubTool struct {
	name     string
	readOnly bool
	ran      *string
}

func (s stubTool) Name() string           { return s.name }
func (s stubTool) Description() string    { return "stub " + s.name }
func (s stubTool) Schema() map[string]any { return map[string]any{"type": "object"} }
func (s stubTool) ReadOnly() bool         { return s.readOnly }
func (s stubTool) Execute(_ context.Context, _ string, _ json.RawMessage) (string, error) {
	if s.ran != nil {
		*s.ran = s.name
	}
	return "ran " + s.name, nil
}

// A renamed tool serves under its new name but runs the original.
func TestRenamedServesUnderItsNewName(t *testing.T) {
	var ran string
	r := Renamed(stubTool{name: "bash", ran: &ran}, "sh")
	if r.Name() != "sh" {
		t.Fatalf("renamed tool is %q, want sh", r.Name())
	}
	if got, _ := r.Execute(context.Background(), ".", nil); got != "ran bash" {
		t.Fatalf("renamed execute ran %q, want the original tool", got)
	}
	if ran != "bash" {
		t.Fatalf("renamed execute drove %q, want bash", ran)
	}
	if r.Description() != "stub bash" || len(r.Schema()) == 0 {
		t.Fatalf("renamed tool lost its description or schema")
	}
}

// Renaming must not drop the read-only mark: the MCP server advertises it so
// harnesses can batch the call.
func TestRenamedKeepsTheReadOnlyMark(t *testing.T) {
	r := Renamed(stubTool{name: "read", readOnly: true}, "rd")
	ro, ok := r.(ReadOnly)
	if !ok || !ro.ReadOnly() {
		t.Fatal("a renamed read-only tool must stay read-only")
	}
	plain := Renamed(stubTool{name: "bash"}, "sh")
	if ro, ok := plain.(ReadOnly); ok && ro.ReadOnly() {
		t.Fatal("a renamed writing tool must not gain the read-only mark")
	}
}
