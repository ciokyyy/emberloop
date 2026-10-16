// RAM sampling: VmRSS summed over the whole process tree.
//
// The walk mirrors emberloop-web/el/memory.go: follow
// /proc/<pid>/task/*/children links from each root pid, read RSS from
// /proc/<pid>/stat field 24, skip processes that vanish mid-walk.
package main

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// memSample is one tick: the summed RSS of every watched agent tree.
type memSample struct {
	At       time.Time `json:"at"`
	RSSBytes int64     `json:"rssBytes"`
	Procs    int       `json:"procs"`
}

// memWatcher samples the union of the process trees rooted at its pids
// every interval until stopped.
type memWatcher struct {
	interval time.Duration

	mu      sync.Mutex
	roots   map[int]bool
	samples []memSample
}

func newMemWatcher(interval time.Duration) *memWatcher {
	return &memWatcher{interval: interval, roots: map[int]bool{}}
}

func (w *memWatcher) add(pid int) {
	if pid <= 1 {
		return
	}
	w.mu.Lock()
	w.roots[pid] = true
	w.mu.Unlock()
}

func (w *memWatcher) run(stop <-chan struct{}) {
	t := time.NewTicker(w.interval)
	defer t.Stop()
	w.tick()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			w.tick()
		}
	}
}

func (w *memWatcher) tick() {
	w.mu.Lock()
	roots := make([]int, 0, len(w.roots))
	for pid := range w.roots {
		roots = append(roots, pid)
	}
	w.mu.Unlock()
	var total int64
	seen := map[int]bool{}
	for _, r := range roots {
		for _, s := range sampleTree("/proc", r) {
			if !seen[s.pid] {
				seen[s.pid] = true
				total += s.rss
			}
		}
	}
	w.mu.Lock()
	w.samples = append(w.samples, memSample{At: time.Now(), RSSBytes: total, Procs: len(seen)})
	w.mu.Unlock()
}

type procRow struct {
	pid int
	rss int64
}

func sampleTree(procRoot string, root int) []procRow {
	if root <= 1 {
		return nil
	}
	page := int64(os.Getpagesize())
	var out []procRow
	seen := map[int]bool{root: true}
	queue := []int{root}
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		rss, ok := readStatRSS(procRoot, pid, page)
		if !ok {
			continue
		}
		out = append(out, procRow{pid: pid, rss: rss})
		for _, c := range procChildren(procRoot, pid) {
			if !seen[c] {
				seen[c] = true
				queue = append(queue, c)
			}
		}
	}
	return out
}

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

func readStatRSS(procRoot string, pid int, page int64) (int64, bool) {
	raw, err := os.ReadFile(filepath.Join(procRoot, strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0, false
	}
	line := string(raw)
	open := strings.IndexByte(line, '(')
	close := strings.LastIndexByte(line, ')')
	if open < 0 || close < open+2 || close+2 > len(line) {
		return 0, false
	}
	fields := strings.Fields(line[close+2:])
	if len(fields) < 22 {
		return 0, false
	}
	pages, err := strconv.ParseInt(fields[21], 10, 64)
	if err != nil || pages < 0 {
		return 0, false
	}
	return pages * page, true
}

// phaseStat summarizes the samples inside one phase window.
type phaseStat struct {
	Samples  int   `json:"samples"`
	LastRSS  int64 `json:"lastRssBytes"`
	MaxRSS   int64 `json:"maxRssBytes"`
	MaxProcs int   `json:"maxProcs"`
}

func (w *memWatcher) statWindow(start, end time.Time) phaseStat {
	w.mu.Lock()
	defer w.mu.Unlock()
	var st phaseStat
	for _, s := range w.samples {
		if s.At.Before(start) || s.At.After(end) {
			continue
		}
		st.Samples++
		st.LastRSS = s.RSSBytes
		if s.RSSBytes > st.MaxRSS {
			st.MaxRSS = s.RSSBytes
		}
		if s.Procs > st.MaxProcs {
			st.MaxProcs = s.Procs
		}
	}
	return st
}

func (w *memWatcher) sampleCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.samples)
}

// percentile returns the p-th percentile of sorted values (0..100).
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	s := append([]float64(nil), sorted...)
	sort.Float64s(s)
	idx := int((p / 100) * float64(len(s)-1))
	return s[idx]
}
