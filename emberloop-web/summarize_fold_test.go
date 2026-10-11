package web

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"
)

// foldReply answers every chunk call of a fold test.
const foldReply = "PART NOTES: parser work kept"

// chunkCalls splits the summarizer calls into chunk calls and Handoff calls.
func chunkCalls(calls []summaryCall) (chunks, handoffs []summaryCall) {
	for _, c := range calls {
		if strings.HasPrefix(c.system, foldPrompt("")) {
			chunks = append(chunks, c)
		} else {
			handoffs = append(handoffs, c)
		}
	}
	return chunks, handoffs
}

// addPrompts logs n user prompts of size bytes each.
func addPrompts(d *Daemon, id string, n, size int) {
	for i := 0; i < n; i++ {
		head := fmt.Sprintf("P%d ", i)
		d.appendPromptEvent(id, head+strings.Repeat("x", size-len(head)), "", 0, nil)
	}
}

// stepTexts is the reason of each compress_step Event, in log order.
func stepTexts(evs []Event) []string {
	var out []string
	for _, e := range eventsOfKind(evs, "compress_step") {
		out = append(out, e.Text)
	}
	return out
}

// hasStep reports whether a compress_step Event names want.
func hasStep(evs []Event, want string) bool {
	for _, s := range stepTexts(evs) {
		if strings.Contains(s, want) {
			return true
		}
	}
	return false
}

// Input over the window is summarized in chunks. Each chunk call fits the
// window, and the Handoff call reads the chunk summaries.
func TestSummarizerOverWindowFoldsIntoChunks(t *testing.T) {
	d, sum, base, id := summaryDaemon(t, goodSummary)
	sum.window = 30000
	sum.partReply = foldReply
	addPrompts(d, id, 6, 5000)
	if code, body := compressReq(t, base, id, ""); code != http.StatusAccepted {
		t.Fatalf("compress = %d %v", code, body)
	}
	evs := waitIdleAfter(t, base, id, 1)

	chunks, handoffs := chunkCalls(sum.calls)
	if len(chunks) < 2 {
		t.Fatalf("chunk calls = %d, want at least 2", len(chunks))
	}
	for i, c := range chunks {
		if n := len(c.system) + len(c.user); n > 30000 {
			t.Fatalf("chunk call %d is %d chars, over the window of 30000", i, n)
		}
	}
	if len(handoffs) != 1 || !strings.Contains(handoffs[0].user, "PART 1 OF ") {
		t.Fatalf("handoff calls = %d, want one over the chunk summaries", len(handoffs))
	}
	if !hasStep(evs, "fold: input of") {
		t.Fatalf("no fold Event: %q", stepTexts(evs))
	}
	if text := handoffText(evs); !strings.Contains(text, "## Next steps\nrun the tests") {
		t.Fatalf("handoff prompt = %q", text)
	}
	if c := eventsOfKind(evs, "compressed")[0].Text; !strings.Contains(c, `"fallback":"summary"`) {
		t.Fatalf("compressed = %s", c)
	}
}

// A chunk the summarizer refuses is halved while it is over the fold floor.
// Each half that fits is summarized, and the Handoff call still gets a summary.
func TestSummarizerRefusedChunkHalvesToFloor(t *testing.T) {
	d, sum, base, id := summaryDaemon(t, goodSummary)
	sum.maxInput = 9000
	sum.partReply = foldReply
	addPrompts(d, id, 6, 5000)
	if code, body := compressReq(t, base, id, ""); code != http.StatusAccepted {
		t.Fatalf("compress = %d %v", code, body)
	}
	evs := waitIdleAfter(t, base, id, 1)

	if !hasStep(evs, "halved") {
		t.Fatalf("no halving Event: %q", stepTexts(evs))
	}
	if text := handoffText(evs); !strings.Contains(text, "## Next steps\nrun the tests") {
		t.Fatalf("handoff prompt = %q, want the summary after folding", text)
	}
	if c := eventsOfKind(evs, "compressed")[0].Text; !strings.Contains(c, `"fallback":"summary"`) {
		t.Fatalf("compressed = %s", c)
	}
}

// At the fold floor a tool result is cut to 500 characters of head. The chunk
// with that result is sent once more, and the summary goes on.
func TestSummarizerFloorCutsToolResultsToHead(t *testing.T) {
	d, sum, base, id := summaryDaemon(t, goodSummary)
	sum.maxInput = 1200
	sum.partReply = foldReply
	out := "HEADMARK" + strings.Repeat("q", 600) + "LATEMARK" + strings.Repeat("r", 20000)
	d.appendEvent(id, "tool_call_updated", ToolCall{ID: "t1", Name: "bash", Input: `{"command":"cat big"}`, Output: out, Status: "completed"}.Text(), "")
	if code, body := compressReq(t, base, id, ""); code != http.StatusAccepted {
		t.Fatalf("compress = %d %v", code, body)
	}
	evs := waitIdleAfter(t, base, id, 1)

	if !hasStep(evs, "tool results cut to 500 chars of head") {
		t.Fatalf("no floor Event: %q", stepTexts(evs))
	}
	chunks, _ := chunkCalls(sum.calls)
	var cut string
	for _, c := range chunks {
		if strings.Contains(c.user, "HEADMARK") {
			cut = c.user
		}
	}
	if cut == "" {
		t.Fatal("no chunk call carried the tool result")
	}
	if strings.Contains(cut, "LATEMARK") || !strings.Contains(cut, "\n[cut]") {
		t.Fatalf("floor chunk = %q, want the result cut to 500 chars of head", cut)
	}
	if text := handoffText(evs); !strings.Contains(text, "## Next steps\nrun the tests") {
		t.Fatalf("handoff prompt = %q", text)
	}
}

// Input that still does not fit after folding gives the deterministic Handoff,
// and the Event log names the reason.
func TestSummarizerStillOversizeFallsBackDeterministic(t *testing.T) {
	d, sum, base, id := summaryDaemon(t, goodSummary)
	sum.maxInput = 300
	sum.partReply = foldReply
	addPrompts(d, id, 3, 4000)
	if code, body := compressReq(t, base, id, ""); code != http.StatusAccepted {
		t.Fatalf("compress = %d %v", code, body)
	}
	evs := waitIdleAfter(t, base, id, 1)

	if !hasStep(evs, "does not fit the summarizer window") {
		t.Fatalf("no fit Event: %q", stepTexts(evs))
	}
	text := handoffText(evs)
	if !strings.Contains(text, "## Task\nbuild the thing") || strings.Contains(text, "## Next steps") {
		t.Fatalf("handoff prompt = %q, want the deterministic Handoff", text)
	}
	if c := eventsOfKind(evs, "compressed")[0].Text; !strings.Contains(c, `"fallback":"deterministic"`) || strings.Contains(c, `"error"`) {
		t.Fatalf("compressed = %s", c)
	}
}

// Each repair and fallback step appends a compress_step Event with its reason.
func TestSummarizerFallbackStepsRecordReason(t *testing.T) {
	t.Run("missed sections", func(t *testing.T) {
		_, _, base, id := summaryDaemon(t, "plain prose about the parser work")
		if code, body := compressReq(t, base, id, ""); code != http.StatusAccepted {
			t.Fatalf("compress = %d %v", code, body)
		}
		evs := waitIdleAfter(t, base, id, 1)
		for _, want := range []string{
			"repair 1 of 2: reply missed the sections",
			"repair 2 of 2: reply missed the sections",
			"lenient: no usable reply after 2 repairs",
		} {
			if !hasStep(evs, want) {
				t.Fatalf("no %q Event: %q", want, stepTexts(evs))
			}
		}
	})
	t.Run("empty", func(t *testing.T) {
		_, _, base, id := summaryDaemon(t, "", "", "")
		if code, body := compressReq(t, base, id, ""); code != http.StatusAccepted {
			t.Fatalf("compress = %d %v", code, body)
		}
		evs := waitIdleAfter(t, base, id, 1)
		for _, want := range []string{
			"repair 1 of 2: reply empty",
			"deterministic: no usable reply after 2 repairs",
		} {
			if !hasStep(evs, want) {
				t.Fatalf("no %q Event: %q", want, stepTexts(evs))
			}
		}
	})
}

// A provider's overflow wording maps to errSummaryOverflow. Other failures
// pass through unchanged.
func TestOverflowErrorIsClassified(t *testing.T) {
	if err := overflowOr(errString("prompt is too long: 250000 tokens > 200000 maximum"), 0); !errors.Is(err, errSummaryOverflow) {
		t.Fatalf("overflow wording = %v, want errSummaryOverflow", err)
	}
	if err := overflowOr(errString("credit exhausted"), 0); err == nil || errors.Is(err, errSummaryOverflow) {
		t.Fatalf("credit error = %v, want it passed through", err)
	}
}

// capHeadTail keeps the head and the tail within about n bytes and never
// splits a character, even where the cut falls inside one.
func TestCapHeadTailKeepsEndsWholeCharacters(t *testing.T) {
	s := "héllo" + strings.Repeat("é", 3000) + "END"
	out := capHeadTail(s, 2000)
	if !utf8.ValidString(out) {
		t.Fatalf("cut split a character: %q", out)
	}
	if !strings.HasPrefix(out, "héllo") || !strings.HasSuffix(out, "END") || len(out) > 2100 {
		t.Fatalf("capHeadTail = %d bytes %q", len(out), out)
	}
	if got := capHeadTail("short", 2000); got != "short" {
		t.Fatalf("short text = %q", got)
	}
}

// A chunk call that returns no text ends the fold at the deterministic Handoff.
// The compressed Event carries the reason as its error text.
func TestSummarizerEmptyChunkSummaryFallsBackDeterministic(t *testing.T) {
	d, sum, base, id := summaryDaemon(t, goodSummary)
	sum.window = 30000
	sum.partReply = " "
	addPrompts(d, id, 6, 5000)
	if code, body := compressReq(t, base, id, ""); code != http.StatusAccepted {
		t.Fatalf("compress = %d %v", code, body)
	}
	evs := waitIdleAfter(t, base, id, 1)

	if !hasStep(evs, "chunk summary came back empty") {
		t.Fatalf("no empty-chunk Event: %q", stepTexts(evs))
	}
	c := eventsOfKind(evs, "compressed")[0].Text
	if !strings.Contains(c, `"fallback":"deterministic"`) || !strings.Contains(c, `"error":"chunk summary came back empty"`) {
		t.Fatalf("compressed = %s", c)
	}
}
