package emberloop

import (
	"os"
	"strings"
	"testing"
)

// Seam: transcript render — a pure function over the transcript fixtures the
// context core already uses. The summarizer reads this text, so it carries
// every user message, assistant text, tool call and result in full.
func TestRenderPigTranscriptCarriesEveryVoice(t *testing.T) {
	raw, err := os.ReadFile("testdata/usage/context-pig.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	got, err := RenderTranscript("pig", strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, want := range []string{
		"do the thing",
		"reading first",
		"mcp__emberloop__read",
		"call_1",
		"0123456789",
		"mcp__emberloop__bash",
		"sleep 60",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("render lacks %q:\n%s", want, got)
		}
	}
	userAt := strings.Index(got, "do the thing")
	readAt := strings.Index(got, "mcp__emberloop__read")
	if userAt < 0 || readAt < 0 || userAt > readAt {
		t.Errorf("render must keep transcript order:\n%s", got)
	}
}

func TestRenderClaudeTranscriptCarriesEveryVoice(t *testing.T) {
	raw, err := os.ReadFile("testdata/usage/context-claude.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	got, err := RenderTranscript("claude", strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, want := range []string{
		"do the thing",
		"Bash",
		"toolu_1",
		"0123456789012345678901234567890123456789",
		"Read",
		"a.md",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("render lacks %q:\n%s", want, got)
		}
	}
}

func TestRenderRefusesUnknownHarness(t *testing.T) {
	if _, err := RenderTranscript("nope", strings.NewReader("{}")); err == nil {
		t.Fatal("unknown harness must fail")
	}
}

func TestRenderKeepsLongResultsWhole(t *testing.T) {
	long := strings.Repeat("x", 20000)
	line := `{"type":"message","id":"r1","message":{"role":"toolResult","toolCallId":"call_9","toolName":"mcp__emberloop__bash","content":[{"type":"text","text":"` + long + `"}]}}` + "\n"
	got, err := RenderTranscript("pig", strings.NewReader(line))
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(got, long) {
		t.Errorf("render truncated a %d-byte result to %d bytes", len(long), len(got))
	}
}
