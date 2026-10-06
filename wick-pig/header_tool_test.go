package wick

import (
	"strings"
	"testing"

	"example.com/wick/tools"
)

func TestToolLabel(t *testing.T) {
	for _, c := range []struct {
		call    ToolCall
		running bool
		want    string
	}{
		{ToolCall{Name: "mcp__wick__read", Arg: "/home/dev/wick-pig/header.go"}, true, "▤ reading wick-pig/header.go"},
		{ToolCall{Name: "mcp__wick__spawn", Arg: "scout, reviewer-spec"}, true, "⑂ spawning scout, reviewer-spec"},
		{ToolCall{Name: "mcp__wick__edit", Arg: "a/b/c.go"}, false, "✎ edited a/b/c.go"},
		{ToolCall{Name: "mcp__wick__bash", Arg: "go test ./...\nsecond line"}, true, "❯ running go test ./..."},
		{ToolCall{Name: "Read", Arg: "x.go"}, true, "▤ reading x.go"},
		{ToolCall{Name: "mcp__other__thing"}, true, "▸ thing"},
	} {
		icon, verb, arg := toolLabel(c.call, c.running, 60)
		got := strings.TrimSpace(icon + " " + verb + " " + arg)
		if got != c.want {
			t.Errorf("toolLabel(%+v, %v) = %q, want %q", c.call, c.running, got, c.want)
		}
	}
}

func TestShortArgCuts(t *testing.T) {
	if got := shortArg("abcdefghij", 5); got != "abcd…" {
		t.Errorf("shortArg = %q", got)
	}
}

// Every tool wick serves needs a strip label: a glyph and a verb pair in
// toolStyles. A new tool without one would show as a bare name.
func TestEveryServedToolHasAStripLabel(t *testing.T) {
	names := []string{"spawn", "submit"}
	for _, tl := range tools.All() {
		names = append(names, tl.Name())
	}
	for _, n := range names {
		if _, ok := toolStyles[n]; !ok {
			t.Errorf("tool %q has no entry in toolStyles (header_tool.go)", n)
		}
	}
}

func TestSummarizeContextTracksRunningAndLastTool(t *testing.T) {
	transcript := strings.Join([]string{
		`{"type":"assistant","message":{"role":"assistant","model":"m","content":[{"type":"tool_use","id":"a","name":"mcp__wick__read","input":{"path":"/x/y/z.go"}}]}}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"a","content":"hello"}]}}`,
		`{"type":"assistant","message":{"role":"assistant","model":"m","content":[{"type":"tool_use","id":"b","name":"mcp__wick__spawn","input":{"agents":[{"type":"scout"},{"type":"reviewer-spec"}]}}]}}`,
	}, "\n")
	snap, err := SummarizeContext("claude", strings.NewReader(transcript))
	if err != nil {
		t.Fatal(err)
	}
	if snap.Running.Name != "mcp__wick__spawn" || snap.Running.Arg != "scout, reviewer-spec" {
		t.Errorf("running = %+v", snap.Running)
	}
	if snap.Last.Name != "mcp__wick__read" || snap.Last.Arg != "/x/y/z.go" || snap.Last.Bytes != 5 {
		t.Errorf("last = %+v", snap.Last)
	}
}
