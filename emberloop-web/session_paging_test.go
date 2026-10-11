package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

// mkEvents builds a log of nTurns turns with two filler events each:
// prompt, assistant_chunk, status per turn.
func mkEvents(nTurns int) []Event {
	var out []Event
	seq := int64(1)
	for t := 1; t <= nTurns; t++ {
		for _, k := range []string{"prompt", "assistant_chunk", "status"} {
			out = append(out, Event{Seq: seq, Kind: k, Text: fmt.Sprintf("t%d %s", t, k), CreatedAt: time.Now()})
			seq++
		}
	}
	return out
}

func TestTailStartCutsAtPromptBoundaries(t *testing.T) {
	events := mkEvents(5) // prompts at idx 0,3,6,9,12
	cases := []struct {
		turns int
		want  int // start index
	}{
		{0, 0},  // degenerate: full
		{-1, 0}, // degenerate: full
		{1, 12}, // last turn
		{2, 9},  // last two turns
		{5, 0},  // exactly all
		{99, 0}, // more than all: full
	}
	for _, c := range cases {
		if got := tailStart(events, c.turns); got != c.want {
			t.Errorf("tailStart(turns=%d) = %d, want %d", c.turns, got, c.want)
		}
		if got := tailStart(events, c.turns); got > 0 && events[got].Kind != "prompt" {
			t.Errorf("tailStart(turns=%d) lands on %q, want a prompt boundary", c.turns, events[got].Kind)
		}
	}
}

func TestTailStartWithoutPrompts(t *testing.T) {
	events := []Event{{Seq: 1, Kind: "status"}, {Seq: 2, Kind: "assistant_chunk"}}
	if got := tailStart(events, 1); got != 0 {
		t.Fatalf("tailStart without prompts = %d, want 0", got)
	}
}

func TestBeforeWindowPagesEarlierTurns(t *testing.T) {
	events := mkEvents(5) // 15 events, seqs 1..15
	// Page the two turns before seq 10 (turn 4 starts at seq 10).
	if start, end := beforeWindow(events, 10, 2); start != 3 || end != 9 {
		t.Fatalf("beforeWindow(10, 2) = [%d, %d), want [3, 9)", start, end)
	}
	// Paging before the first event yields an empty window at 0.
	if start, end := beforeWindow(events, 1, 2); start != 0 || end != 0 {
		t.Fatalf("beforeWindow(1, 2) = [%d, %d), want [0, 0)", start, end)
	}
	// before past the end pages the tail.
	if start, end := beforeWindow(events, 1000, 1); start != 12 || end != 15 {
		t.Fatalf("beforeWindow(past-end, 1) = [%d, %d), want [12, 15)", start, end)
	}
}

func TestAtStartWidensToTargetTurn(t *testing.T) {
	events := mkEvents(5)
	tail := tailStart(events, 1) // 12
	if got := atStart(events, tail, 4); got != 3 {
		t.Fatalf("atStart(tail, at=4) = %d, want 3 (turn 2)", got)
	}
	// Inside the window, unknown, or empty target all keep start.
	for _, at := range []int64{0, 13, 999} {
		if got := atStart(events, tail, at); got != tail {
			t.Fatalf("atStart(tail, at=%d) = %d, want %d", at, got, tail)
		}
	}
	if got := atStart(events, 0, 4); got != 0 {
		t.Fatalf("atStart(full, at=4) = %d, want 0", got)
	}
}

func toolText(id, input, output string) string {
	t := ToolCall{ID: id, Name: "bash", Input: input, Output: output, Status: "ok"}
	b, _ := json.Marshal(t)
	return string(b)
}

func TestCompactToolUpdatesKeepsLastPerID(t *testing.T) {
	events := []Event{
		{Seq: 1, Kind: "prompt", Text: "hi"},
		{Seq: 2, Kind: "tool_call_started", Text: toolText("a", "echo", "")},
		{Seq: 3, Kind: "tool_call_updated", Text: toolText("a", "echo h", "")},
		{Seq: 4, Kind: "tool_call_updated", Text: toolText("a", "echo hi", "hi")},
		{Seq: 5, Kind: "tool_call_updated", Text: toolText("b", "ls", "x")},
		{Seq: 6, Kind: "assistant_chunk", Text: "done"},
		{Seq: 7, Kind: "tool_call_updated", Text: "not json at all"},
	}
	got := compactToolUpdates(events)
	var seqs []int64
	for _, e := range got {
		seqs = append(seqs, e.Seq)
	}
	want := []int64{1, 2, 4, 5, 6, 7} // seq 3 folded away, id-less kept
	if !reflect.DeepEqual(seqs, want) {
		t.Fatalf("compact seqs = %v, want %v", seqs, want)
	}
}

// TestCompactFoldEquivalent proves the compaction lossless for display: the
// per-tool fold (last non-empty field wins, as the transcript merges) is
// identical before and after.
func TestCompactFoldEquivalent(t *testing.T) {
	events := []Event{
		{Seq: 1, Kind: "prompt", Text: "go"},
		{Seq: 2, Kind: "tool_call_started", Text: toolText("a", "ec", "")},
		{Seq: 3, Kind: "tool_call_updated", Text: toolText("a", "echo ", "")},
		{Seq: 4, Kind: "tool_call_updated", Text: toolText("a", "echo hello", "")},
		{Seq: 5, Kind: "tool_call_updated", Text: toolText("a", "echo hello", "hello")},
		{Seq: 6, Kind: "tool_call_started", Text: toolText("b", "ls", "")},
		{Seq: 7, Kind: "tool_call_updated", Text: toolText("b", "ls -la", "total 0")},
		{Seq: 8, Kind: "assistant_chunk", Text: "done"},
	}
	before := foldToolCalls(events)
	after := foldToolCalls(compactToolUpdates(events))
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("fold differs:\nbefore=%v\nafter=%v", before, after)
	}
	if after["a"].Input != "echo hello" || after["a"].Output != "hello" {
		t.Fatalf("folded tool a = %+v, want full input/output", after["a"])
	}
}

// seedDirectSession creates a Session with the given events without running
// a Backend turn.
func seedDirectSession(d *Daemon, id string, events []Event) {
	d.mu.Lock()
	defer d.mu.Unlock()
	s := &session{meta: SessionMeta{ID: id, Harness: "fake", Model: "m", Status: "idle"}, notify: make(chan struct{})}
	for i, e := range events {
		e.Seq = int64(i + 1)
		e.SessionID = id
		s.events = append(s.events, e)
	}
	d.sessions[id] = s
}

func getSnapshot(t *testing.T, base, id, query string) (events []Event, hasMore bool, firstSeq int64) {
	t.Helper()
	resp, err := http.Get(base + "/api/sessions/" + id + query)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status = %d", query, resp.StatusCode)
	}
	var body struct {
		Events   []Event `json:"events"`
		HasMore  bool    `json:"hasMore"`
		FirstSeq int64   `json:"firstSeq"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body.Events, body.HasMore, body.FirstSeq
}

func TestSessionTailBeforePagingOverHTTP(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	seedDirectSession(d, "s1", mkEvents(4)) // prompts at seqs 1,4,7,10
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	// Full for old clients: everything, no more.
	events, hasMore, first := getSnapshot(t, srv.URL, "s1", "")
	if len(events) != 12 || hasMore || first != 1 {
		t.Fatalf("full: %d events hasMore=%v first=%d", len(events), hasMore, first)
	}
	// Tail of 2 turns: seqs 7..12, cut at a prompt.
	events, hasMore, first = getSnapshot(t, srv.URL, "s1", "?tail=2")
	if len(events) != 6 || !hasMore || first != 7 || events[0].Kind != "prompt" {
		t.Fatalf("tail=2: %d events hasMore=%v first=%d firstKind=%q", len(events), hasMore, first, events[0].Kind)
	}
	// Walk the log one turn per page: each page starts where the last
	// ended, every cut lands on a prompt, the last page stops.
	for _, want := range []struct {
		query    string
		first    int64
		more     bool
		firstSeq int64
	}{
		{"?before=7&turns=1", 4, true, 4},
		{"?before=4&turns=1", 1, false, 1},
	} {
		events, hasMore, first = getSnapshot(t, srv.URL, "s1", want.query)
		if len(events) != 3 || hasMore != want.more || first != want.firstSeq || events[0].Kind != "prompt" {
			t.Fatalf("%s: %d events hasMore=%v first=%d", want.query, len(events), hasMore, first)
		}
		if events[0].Seq != want.first {
			t.Fatalf("%s: first event seq = %d, want %d", want.query, events[0].Seq, want.first)
		}
	}
	// Walking before=firstSeq pages to the start, then stops.
	events, hasMore, first = getSnapshot(t, srv.URL, "s1", "?before=1&turns=2")
	if len(events) != 0 || hasMore || first != 0 {
		t.Fatalf("before=1: %d events hasMore=%v first=%d", len(events), hasMore, first)
	}
	// at= widens the tail to include the target turn (here back to the
	// start, so nothing more remains).
	events, hasMore, first = getSnapshot(t, srv.URL, "s1", "?tail=1&at=2")
	if first != 1 || hasMore || events[0].Seq != 1 || len(events) != 12 {
		t.Fatalf("tail=1&at=2: %d events hasMore=%v first=%d", len(events), hasMore, first)
	}
}

func TestSessionSnapshotCompactsOverHTTP(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	seedDirectSession(d, "s1", []Event{
		{Kind: "prompt", Text: "hi"},
		{Kind: "tool_call_started", Text: toolText("a", "ec", "")},
		{Kind: "tool_call_updated", Text: toolText("a", "echo", "")},
		{Kind: "tool_call_updated", Text: toolText("a", "echo hi", "hi")},
		{Kind: "assistant_chunk", Text: "done"},
	})
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	events, _, _ := getSnapshot(t, srv.URL, "s1", "")
	if len(events) != 4 {
		t.Fatalf("snapshot events = %d, want 4 after compaction", len(events))
	}
	// The live SSE tail still replays everything uncompacted.
	resp, err := http.Get(srv.URL + "/api/sessions/s1/events?wait=0")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var n int
	for _, line := range strings.Split(readAll(t, resp), "\n") {
		if strings.HasPrefix(line, "data: ") {
			n++
		}
	}
	if n != 5 {
		t.Fatalf("SSE replay events = %d, want 5 uncompacted", n)
	}
}

func readAll(t *testing.T, resp *http.Response) string {
	t.Helper()
	buf := new(strings.Builder)
	tmp := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(tmp)
		buf.Write(tmp[:n])
		if err != nil {
			break
		}
	}
	return buf.String()
}

func TestSPAFallback(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	get := func(path string) (int, string) {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		tmp := make([]byte, 4096)
		n, _ := resp.Body.Read(tmp)
		return resp.StatusCode, string(tmp[:n])
	}

	for _, p := range []string{"/s/abc123", "/runs", "/runs/r1", "/runs/r1/t1", "/settings", "/"} {
		code, body := get(p)
		if code != http.StatusOK || !strings.Contains(body, "Emberloop") {
			t.Fatalf("GET %s = %d, want 200 with the app shell", p, code)
		}
	}
	for _, p := range []string{"/assets/missing.js", "/favicon-missing.png", "/nope", "/api/nope"} {
		if code, _ := get(p); code != http.StatusNotFound {
			t.Fatalf("GET %s = %d, want 404", p, code)
		}
	}
}

func TestIsSPARoute(t *testing.T) {
	for _, p := range []string{"/s/x", "/s/abc/", "/runs", "/runs/", "/runs/r1", "/runs/r1/t2", "/settings"} {
		if !isSPARoute(p) {
			t.Errorf("isSPARoute(%q) = false, want true", p)
		}
	}
	for _, p := range []string{"/", "/s", "/s/", "/api/sessions", "/assets/a.js", "/favicon.png", "/unknown/deep"} {
		if isSPARoute(p) {
			t.Errorf("isSPARoute(%q) = true, want false", p)
		}
	}
}
