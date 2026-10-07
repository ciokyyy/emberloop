package emberloop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example.com/emberloop/tools"
)

// Seam: the context report — the numbers the `emberloop context` command
// prints for one transcript. Expected strings are worked from the
// fixture, never the implementation.
func TestContextReportPrintsNumbers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sess-test.jsonl")
	raw, err := os.ReadFile(filepath.Join("testdata", "usage", "context-pig.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	if err := WriteContextReport(&b, "pig", "sess-test", path, tools.Settings{}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{
		"model: muse-spark-test",
		"context: 9400 tokens",
		"4% of 200000",
		"outstanding tool calls: 1",
		"mcp__emberloop__bash 30",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report %q lacks %q", out, want)
		}
	}
}

func TestContextReportRefusesUnknownHarness(t *testing.T) {
	var b strings.Builder
	if err := WriteContextReport(&b, "nope", "s", "/nonexistent", tools.Settings{}); err == nil {
		t.Error("unknown harness must fail, not print zeros")
	}
}
