package main

import (
	"os"
	"testing"
	"time"
)

func TestSampleTreeSelf(t *testing.T) {
	rows := sampleTree("/proc", os.Getpid())
	if len(rows) == 0 {
		t.Fatal("no rows for own pid")
	}
	var total int64
	for _, r := range rows {
		if r.rss <= 0 {
			t.Fatalf("pid %d rss %d", r.pid, r.rss)
		}
		total += r.rss
	}
	if total <= 0 {
		t.Fatal("non-positive total RSS")
	}
}

func TestSampleTreeBadPid(t *testing.T) {
	if rows := sampleTree("/proc", 1); rows != nil {
		t.Fatalf("pid 1 gave %d rows", len(rows))
	}
	if rows := sampleTree("/nonexistent-proc", os.Getpid()); len(rows) != 0 {
		t.Fatalf("bad proc root gave %d rows", len(rows))
	}
}

func TestMemWatcherTicks(t *testing.T) {
	w := newMemWatcher(10 * time.Millisecond)
	w.add(os.Getpid())
	stop := make(chan struct{})
	go w.run(stop)
	time.Sleep(60 * time.Millisecond)
	close(stop)
	if n := w.sampleCount(); n < 2 {
		t.Fatalf("only %d samples", n)
	}
	now := time.Now()
	st := w.statWindow(now.Add(-time.Hour), now)
	if st.Samples < 2 || st.MaxRSS <= 0 || st.LastRSS <= 0 || st.MaxProcs < 1 {
		t.Fatalf("bad window stat %+v", st)
	}
	empty := w.statWindow(now.Add(time.Hour), now.Add(2*time.Hour))
	if empty.Samples != 0 {
		t.Fatalf("future window has samples %+v", empty)
	}
}
