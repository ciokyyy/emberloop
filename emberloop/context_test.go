package el

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Seam: context core as a pure function over transcript fixtures. One
// transcript in, one snapshot out. Fixtures live in testdata/usage beside
// the usage logs; expected numbers are worked from the fixture lines by
// hand, never from the implementation.

// readContextFixture loads a whole fixture transcript.
func readContextFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "usage", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// The shared formula: the latest request's input plus cache read plus
// cache write plus that message's output. Both harnesses feed it.
func TestContextFormulaUsesLatestRequest(t *testing.T) {
	// Pig fixture: latest assistant usage is input 2000, output 400,
	// cacheRead 6000, cacheWrite 1000.
	snap, err := SummarizeContext("pig", strings.NewReader(readContextFixture(t, "context-pig.jsonl")))
	if err != nil {
		t.Fatal(err)
	}
	if snap.ContextTokens != 2000+400+6000+1000 {
		t.Errorf("pig context = %d, want 9400", snap.ContextTokens)
	}
	if snap.Model != "muse-spark-test" {
		t.Errorf("pig model = %q, want muse-spark-test", snap.Model)
	}
	// Claude fixture: latest assistant usage is input 20, output 80,
	// cache_read 2000, cache_creation 200.
	snap, err = SummarizeContext("claude", strings.NewReader(readContextFixture(t, "context-claude.jsonl")))
	if err != nil {
		t.Fatal(err)
	}
	if snap.ContextTokens != 20+80+2000+200 {
		t.Errorf("claude context = %d, want 2300", snap.ContextTokens)
	}
	if snap.Model != "claude-test-model" {
		t.Errorf("claude model = %q, want claude-test-model", snap.Model)
	}
}

func TestEmptyTranscriptIsZero(t *testing.T) {
	for _, harness := range []string{"pig", "claude"} {
		snap, err := SummarizeContext(harness, strings.NewReader(""))
		if err != nil {
			t.Fatal(err)
		}
		if snap.ContextTokens != 0 || snap.Model != "" || snap.Outstanding != 0 || len(snap.TopTools) != 0 {
			t.Errorf("%s empty = %+v, want zeros", harness, snap)
		}
	}
}

// A call without its result counts as outstanding; a completed one does
// not. Worked from the fixtures: the pig transcript leaves call_3 (bash)
// open, the claude transcript leaves toolu_3 (Bash) open.
func TestOutstandingCountsCallsWithoutResults(t *testing.T) {
	snap, err := SummarizeContext("pig", strings.NewReader(readContextFixture(t, "context-pig.jsonl")))
	if err != nil {
		t.Fatal(err)
	}
	if snap.Outstanding != 1 {
		t.Errorf("pig outstanding = %d, want 1", snap.Outstanding)
	}
	snap, err = SummarizeContext("claude", strings.NewReader(readContextFixture(t, "context-claude.jsonl")))
	if err != nil {
		t.Fatal(err)
	}
	if snap.Outstanding != 1 {
		t.Errorf("claude outstanding = %d, want 1", snap.Outstanding)
	}
	// A quiet transcript with no calls at all has none outstanding.
	snap, err = SummarizeContext("pig", strings.NewReader(
		"{\"type\":\"message\",\"id\":\"a1\",\"timestamp\":\"2026-10-06T10:00:01Z\","+
			"\"message\":{\"role\":\"assistant\",\"provider\":\"p\",\"model\":\"m\","+
			"\"usage\":{\"input\":5,\"output\":5,\"cacheRead\":0,\"cacheWrite\":0},\"stopReason\":\"stop\","+
			"\"content\":[{\"type\":\"text\",\"text\":\"done\"}]}}"))
	if err != nil {
		t.Fatal(err)
	}
	if snap.Outstanding != 0 {
		t.Errorf("quiet outstanding = %d, want 0", snap.Outstanding)
	}
}

// The top three tools by result bytes, largest first. Fixture sizes are
// exact text lengths: pig bash 30, grep 20, read 10; claude Bash 40,
// Read 10.
func TestTopThreeToolsByResultSize(t *testing.T) {
	snap, err := SummarizeContext("pig", strings.NewReader(readContextFixture(t, "context-pig.jsonl")))
	if err != nil {
		t.Fatal(err)
	}
	want := []ToolSize{{"mcp__el__bash", 30}, {"mcp__el__grep", 20}, {"mcp__el__read", 10}}
	if len(snap.TopTools) != len(want) {
		t.Fatalf("pig top = %+v, want %+v", snap.TopTools, want)
	}
	for i := range want {
		if snap.TopTools[i] != want[i] {
			t.Errorf("pig top[%d] = %+v, want %+v", i, snap.TopTools[i], want[i])
		}
	}
	snap, err = SummarizeContext("claude", strings.NewReader(readContextFixture(t, "context-claude.jsonl")))
	if err != nil {
		t.Fatal(err)
	}
	want = []ToolSize{{"Bash", 40}, {"Read", 10}}
	if len(snap.TopTools) != len(want) {
		t.Fatalf("claude top = %+v, want %+v", snap.TopTools, want)
	}
	for i := range want {
		if snap.TopTools[i] != want[i] {
			t.Errorf("claude top[%d] = %+v, want %+v", i, snap.TopTools[i], want[i])
		}
	}
}

// A new harness plugs in with one parser registration: usage, model,
// calls and results all flow through it, never a second switch.
type fakeContextParser struct{}

func (fakeContextParser) ParseLine(line string) ContextEvents {
	if line == "CALL" {
		return ContextEvents{Calls: []ContextCall{{ID: "1", Name: "Fake"}}}
	}
	if line == "RESULT" {
		return ContextEvents{Results: []ContextResult{{ID: "1", Bytes: 7}},
			Usage: &RequestUsage{Model: "fake-model", Input: 1, Output: 2, CacheRead: 3, CacheWrite: 4}}
	}
	return ContextEvents{}
}

func TestNewHarnessNeedsOneParser(t *testing.T) {
	RegisterContextParser("fake-harness", fakeContextParser{})
	defer delete(contextParsers, "fake-harness")
	snap, err := SummarizeContext("fake-harness", strings.NewReader("CALL\nRESULT\n"))
	if err != nil {
		t.Fatal(err)
	}
	if snap.Model != "fake-model" || snap.ContextTokens != 1+2+3+4 {
		t.Errorf("fake snapshot = %+v, want model fake-model context 10", snap)
	}
	if snap.Outstanding != 0 {
		t.Errorf("fake outstanding = %d, want 0", snap.Outstanding)
	}
	if len(snap.TopTools) != 1 || snap.TopTools[0] != (ToolSize{"Fake", 7}) {
		t.Errorf("fake top = %+v, want [{Fake 7}]", snap.TopTools)
	}
}

func TestUnknownHarnessFails(t *testing.T) {
	if _, err := SummarizeContext("nope", strings.NewReader("")); err == nil {
		t.Error("an unknown harness must fail, not print zeros")
	}
}

// CompletedCalls counts finished tool calls (one per result), the cooldown's
// input: the pig fixture answers three calls, the claude fixture two.
func TestCompletedCallsCountsResults(t *testing.T) {
	snap, err := SummarizeContext("pig", strings.NewReader(readContextFixture(t, "context-pig.jsonl")))
	if err != nil {
		t.Fatal(err)
	}
	if snap.CompletedCalls != 3 {
		t.Errorf("pig completed = %d, want 3", snap.CompletedCalls)
	}
	snap, err = SummarizeContext("claude", strings.NewReader(readContextFixture(t, "context-claude.jsonl")))
	if err != nil {
		t.Fatal(err)
	}
	if snap.CompletedCalls != 2 {
		t.Errorf("claude completed = %d, want 2", snap.CompletedCalls)
	}
	snap, err = SummarizeContext("pig", strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	if snap.CompletedCalls != 0 {
		t.Errorf("empty completed = %d, want 0", snap.CompletedCalls)
	}
}
