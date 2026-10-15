package el

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Memory sampling lives in the session's own el process (the el-mcp
// attached to the harness), so samples are written whether or not the TUI is
// open, and stop when the session ends with the process.
//
// Archive layout, for the TUI and the stats indexer: the agent's memory log
// is append-only JSONL at
//
//	<state>/archive/<run>/<agent>/memory.jsonl
//
// one line per process per sample. run is the spec slug from EL_SPEC, or
// "local" for a session outside a run; agent is EL_AGENT_ID.

// MemorySample is one process in one tick: who it belongs to, when it was
// taken, and the process's RSS in bytes.
type MemorySample struct {
	Agent string `json:"agent"`
	At    string `json:"at"`
	PID   int    `json:"pid"`
	Name  string `json:"name"`
	RSS   int64  `json:"rss"`
}

// MemorySampleInterval is how often each session samples its harness tree.
const MemorySampleInterval = 5 * time.Second

// SampleProcessTree returns one row per process in the harness tree rooted at
// harnessPid: the harness itself, el-mcp, and anything they started. The
// walk follows /proc/<pid>/task/*/children links, so a tick reads only the
// tree: one children file per thread plus one stat file per process, a
// handful of file reads. A process that vanishes mid-walk is skipped, never
// an error. A harness at or below pid 1 is not a session tree and gives no
// rows.
func SampleProcessTree(procRoot string, harnessPid int, agent, at string) []MemorySample {
	if harnessPid <= 1 {
		return nil
	}
	page := int64(os.Getpagesize())
	var out []MemorySample
	seen := map[int]bool{harnessPid: true}
	queue := []int{harnessPid}
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		name, rss, ok := readProcStat(procRoot, pid, page)
		if !ok {
			continue // vanished mid-walk: skip, keep walking the rest
		}
		out = append(out, MemorySample{Agent: agent, At: at, PID: pid, Name: name, RSS: rss})
		for _, c := range procChildren(procRoot, pid) {
			if !seen[c] {
				seen[c] = true
				queue = append(queue, c)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PID < out[j].PID })
	return out
}

// procChildren lists the direct children of pid through its per-thread
// children files, unioned across threads. An unreadable thread or task
// directory is a process on its way out and contributes nothing.
func procChildren(procRoot string, pid int) []int {
	threads, err := os.ReadDir(filepath.Join(procRoot, strconv.Itoa(pid), "task"))
	if err != nil {
		return nil
	}
	var out []int
	for _, th := range threads {
		raw, err := os.ReadFile(filepath.Join(procRoot, strconv.Itoa(pid), "task", th.Name(), "children"))
		if err != nil {
			continue
		}
		for _, f := range strings.Fields(string(raw)) {
			if c, err := strconv.Atoi(f); err == nil && c > 1 {
				out = append(out, c)
			}
		}
	}
	return out
}

// readProcStat reads /proc/<pid>/stat for the comm name and the RSS (field
// 24, in pages). It reports false when the process vanished or the line is
// malformed.
func readProcStat(procRoot string, pid int, page int64) (name string, rss int64, ok bool) {
	raw, err := os.ReadFile(filepath.Join(procRoot, strconv.Itoa(pid), "stat"))
	if err != nil {
		return "", 0, false
	}
	line := string(raw)
	open := strings.IndexByte(line, '(')
	close := strings.LastIndexByte(line, ')')
	if open < 0 || close < open+2 || close+2 > len(line) {
		return "", 0, false
	}
	fields := strings.Fields(line[close+2:])
	// After comm: state ppid pgrp session tty tpgid flags minflt cminflt
	// majflt cmajflt utime stime cutime cstime priority nice threads
	// itreal starttime vsize rss — rss is the 22nd.
	if len(fields) < 22 {
		return "", 0, false
	}
	pages, err := strconv.ParseInt(fields[21], 10, 64)
	if err != nil || pages < 0 {
		return "", 0, false
	}
	return line[open+1 : close], pages * page, true
}

// MemoryLogPath is the agent's memory log in the archive.
func MemoryLogPath(run, agent string) string {
	if run == "" {
		run = "local"
	}
	if agent == "" {
		agent = "unknown"
	}
	run = strings.ReplaceAll(run, "/", "-")
	run = strings.ReplaceAll(run, "\\", "-")
	agent = strings.ReplaceAll(agent, "/", "-")
	agent = strings.ReplaceAll(agent, "\\", "-")
	return filepath.Join(filepath.Dir(statePath), "archive", run, agent, "memory.jsonl")
}

// AppendMemorySamples appends one JSON line per row to the agent's log,
// creating the archive directory. An empty sample appends nothing.
func AppendMemorySamples(path string, rows []MemorySample) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, r := range rows {
		if err := enc.Encode(r); err != nil {
			return err
		}
	}
	return nil
}

// MemoryRunSlug is the archive run for this process: the spec slug from
// EL_SPEC, or "local" for a session outside a run.
func MemoryRunSlug() string {
	if spec := os.Getenv("EL_SPEC"); spec != "" {
		return specSlug(spec)
	}
	return "local"
}

// SampleAndAppend takes one sample of the harness tree and appends it to the
// agent's memory log. It is one tick of the sampler.
func SampleAndAppend(procRoot string, harnessPid int, agent, run, at string) error {
	return AppendMemorySamples(MemoryLogPath(run, agent), SampleProcessTree(procRoot, harnessPid, agent, at))
}

// StartMemorySampler samples the harness tree every interval and appends each
// sample to the agent's memory log, starting with an immediate sample so
// short sessions still record one. The harness is this process's parent,
// resolved once up front: if the harness is already gone (parent 1), there
// is no session tree to record. Sampling is best-effort: a failed tick is
// skipped and retried next interval, never fatal to the MCP server. It stops
// when ctx ends with the session. A session with no agent id records nothing.
func StartMemorySampler(ctx context.Context, agent, run string, interval time.Duration) {
	startMemorySampler(ctx, agent, run, interval, "/proc", os.Getppid())
}

// startMemorySampler lets tests supply a stable process-tree root and proc tree.
func startMemorySampler(ctx context.Context, agent, run string, interval time.Duration, procRoot string, harness int) {
	if agent == "" || harness <= 1 {
		return
	}
	if run == "" {
		run = "local"
	}
	tick := func() {
		_ = SampleAndAppend(procRoot, harness, agent, run, time.Now().UTC().Format(time.RFC3339))
	}
	tick()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			tick()
		}
	}
}
