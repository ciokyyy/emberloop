package emberloop

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Seam: the memory sampler against a fake /proc tree. The tree carries known
// pids, parents, names and RSS; the sampler returns one row per process in
// the harness tree, skipping processes that vanish mid-walk.
func TestMemorySamplerFindsHarnessTreeInFakeProc(t *testing.T) {
	proc := t.TempDir()
	page := os.Getpagesize()
	// statLine builds a /proc/<pid>/stat line: pid (comm) state ppid then the
	// fields up to rss (the 22nd field after comm, in pages).
	statLine := func(pid int, comm string, ppid, rssPages int) string {
		return strconv.Itoa(pid) + " (" + comm + ") S " + strconv.Itoa(ppid) + " 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 12345 " + strconv.Itoa(rssPages) + "\n"
	}
	writeStat := func(pid int, comm string, ppid, rssPages int) {
		t.Helper()
		dir := filepath.Join(proc, strconv.Itoa(pid))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "stat"), []byte(statLine(pid, comm, ppid, rssPages)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// writeChildren records pid's direct children the way the kernel does:
	// /proc/<pid>/task/<tid>/children holds space-separated child pids.
	writeChildren := func(pid int, children ...int) {
		t.Helper()
		dir := filepath.Join(proc, strconv.Itoa(pid), "task", strconv.Itoa(pid))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		var b strings.Builder
		for _, c := range children {
			b.WriteString(strconv.Itoa(c))
			b.WriteByte(' ')
		}
		if err := os.WriteFile(filepath.Join(dir, "children"), []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeStat(100, "harness", 1, 100)
	writeStat(101, "emberloop-mcp", 100, 200)
	writeStat(102, "test-run", 101, 300)
	writeStat(104, "my worker", 100, 50)
	writeStat(200, "unrelated", 1, 999)
	writeChildren(100, 101, 104, 105)
	writeChildren(101, 102)
	writeChildren(102)
	writeChildren(104)
	writeChildren(200)
	// Pid 105 is named as a child but has no stat or task directory: a
	// process that vanished mid-walk. It is skipped, not an error.
	// Pid 200 roots a tree of its own, outside the harness: it is excluded.
	rows := SampleProcessTree(proc, 100, "01-x", "2026-10-05T00:00:00Z")
	sort.Slice(rows, func(i, j int) bool { return rows[i].PID < rows[j].PID })
	want := []MemorySample{
		{Agent: "01-x", At: "2026-10-05T00:00:00Z", PID: 100, Name: "harness", RSS: int64(100 * page)},
		{Agent: "01-x", At: "2026-10-05T00:00:00Z", PID: 101, Name: "emberloop-mcp", RSS: int64(200 * page)},
		{Agent: "01-x", At: "2026-10-05T00:00:00Z", PID: 102, Name: "test-run", RSS: int64(300 * page)},
		{Agent: "01-x", At: "2026-10-05T00:00:00Z", PID: 104, Name: "my worker", RSS: int64(50 * page)},
	}
	if len(rows) != len(want) {
		t.Fatalf("rows = %+v, want %+v", rows, want)
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, rows[i], want[i])
		}
	}
	// A harness that is already gone has no tree.
	if rows := SampleProcessTree(proc, 999, "01-x", "2026-10-05T00:00:00Z"); len(rows) != 0 {
		t.Errorf("missing harness gives %+v, want no rows", rows)
	}
}

// The sampler appends one JSON line per process per sample to the agent's
// memory log in the archive, so the TUI and stats can read it as plain files.
func TestMemorySamplesAppendToAgentLog(t *testing.T) {
	isolateState(t)
	rows := []MemorySample{
		{Agent: "01-x", At: "2026-10-05T00:00:00Z", PID: 100, Name: "harness", RSS: 409600},
		{Agent: "01-x", At: "2026-10-05T00:00:00Z", PID: 101, Name: "emberloop-mcp", RSS: 819200},
	}
	path := MemoryLogPath("run-records", "01-x")
	if err := AppendMemorySamples(path, rows); err != nil {
		t.Fatalf("AppendMemorySamples: %v", err)
	}
	if err := AppendMemorySamples(path, rows[:1]); err != nil {
		t.Fatalf("second append: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var lines []MemorySample
	for _, line := range splitLines(string(raw)) {
		var r MemorySample
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("line %q is not a memory sample: %v", line, err)
		}
		lines = append(lines, r)
	}
	if len(lines) != 3 {
		t.Fatalf("log holds %d rows, want 3 appended lines", len(lines))
	}
	if lines[0] != rows[0] || lines[1] != rows[1] || lines[2] != rows[0] {
		t.Errorf("log rows = %+v, want two samples then one", lines)
	}
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			if i > start {
				out = append(out, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

// The sampler ticks on its interval into the agent's log and stops with the
// session: rows carry the agent, and one of them is the harness (our parent
// here). A session with no agent id records nothing.
func TestMemorySamplerTicksAndStopsWithTheSession(t *testing.T) {
	isolateState(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		StartMemorySampler(ctx, "loop-1", "loop-run", 5*time.Millisecond)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if raw, err := os.ReadFile(MemoryLogPath("loop-run", "loop-1")); err == nil && len(raw) > 0 {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			<-done
			t.Fatal("sampler wrote no rows in 2s")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done
	raw, err := os.ReadFile(MemoryLogPath("loop-run", "loop-1"))
	if err != nil {
		t.Fatal(err)
	}
	ppid := os.Getppid()
	seenHarness := false
	for _, line := range splitLines(string(raw)) {
		var r MemorySample
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("line %q is not a memory sample: %v", line, err)
		}
		if r.Agent != "loop-1" || r.At == "" || r.PID <= 0 || r.Name == "" || r.RSS <= 0 {
			t.Errorf("row is not a full sample: %+v", r)
		}
		if r.PID == ppid {
			seenHarness = true
		}
	}
	if !seenHarness {
		t.Errorf("no row for the harness (parent pid %d) in %s", ppid, string(raw))
	}
}

func TestMemorySamplerWithoutAgentRecordsNothing(t *testing.T) {
	isolateState(t)
	ctx, cancel := context.WithCancel(context.Background())
	go StartMemorySampler(ctx, "", "loop-run", 5*time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	cancel()
	if _, err := os.Stat(filepath.Dir(MemoryLogPath("loop-run", "unknown"))); !os.IsNotExist(err) {
		t.Errorf("agentless sampler must not write the archive")
	}
}

func TestMemoryRunSlugFollowsTheSpecEnv(t *testing.T) {
	t.Setenv("EMBERLOOP_SPEC", "/repo/.scratch/run-records/spec.md")
	if got := MemoryRunSlug(); got != "run-records" {
		t.Errorf("run slug = %q, want run-records", got)
	}
	t.Setenv("EMBERLOOP_SPEC", "")
	if got := MemoryRunSlug(); got != "local" {
		t.Errorf("run slug without a spec = %q, want local", got)
	}
}
