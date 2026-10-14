package web

import (
	"fmt"
	"testing"
	"time"
)

func TestRunGitResultCacheHitAndTTLExpiry(t *testing.T) {
	now := time.Date(2026, time.October, 14, 10, 0, 0, 0, time.UTC)
	cache := runGitResultCache{now: func() time.Time { return now }}
	loads := 0
	load := func() any {
		loads++
		return fmt.Sprintf("result-%d", loads)
	}

	if got := cache.get("run-1", "", load); got != "result-1" {
		t.Fatalf("first result = %v, want result-1", got)
	}
	if got := cache.get("run-1", "", load); got != "result-1" {
		t.Fatalf("cache hit = %v, want result-1", got)
	}
	if loads != 1 {
		t.Fatalf("loader calls before expiry = %d, want 1", loads)
	}

	now = now.Add(runGitResultCacheTTL)
	if got := cache.get("run-1", "", load); got != "result-2" {
		t.Fatalf("result after TTL = %v, want result-2", got)
	}
	if loads != 2 {
		t.Fatalf("loader calls after expiry = %d, want 2", loads)
	}
}

func TestRunGitResultCacheSeparatesRunsAndTickets(t *testing.T) {
	cache := runGitResultCache{}
	loads := 0
	load := func() any {
		loads++
		return loads
	}
	for _, key := range []runGitResultCacheKey{
		{runID: "run-1"},
		{runID: "run-1", ticketID: "01-first"},
		{runID: "run-1", ticketID: "02-second"},
		{runID: "run-2"},
	} {
		if got := cache.get(key.runID, key.ticketID, load); got != loads {
			t.Fatalf("result for %+v = %v, want distinct load %d", key, got, loads)
		}
	}
	if loads != 4 {
		t.Fatalf("loader calls = %d, want one per Run/Ticket key", loads)
	}
}

func TestRunEventInvalidatesRunAndTicketGitResults(t *testing.T) {
	d := &Daemon{live: newLiveHub()}
	rs := &runState{ID: "run-cache", notify: make(chan struct{})}
	loads := map[string]int{}
	load := func(key string) func() any {
		return func() any {
			loads[key]++
			return loads[key]
		}
	}
	readRun := func() any { return d.runGitCache.get(rs.ID, "", load("run")) }
	readTicket := func() any { return d.runGitCache.get(rs.ID, "01-first", load("ticket")) }

	if readRun() != 1 || readTicket() != 1 {
		t.Fatal("first Run and Ticket reads did not populate the cache")
	}
	if readRun() != 1 || readTicket() != 1 {
		t.Fatal("repeated reads did not hit the cache")
	}

	d.mu.Lock()
	d.broadcastRunLocked(rs)
	d.mu.Unlock()

	if readRun() != 2 || readTicket() != 2 {
		t.Fatalf("Run event did not invalidate both entries: loads=%v", loads)
	}
}

func TestRunEventInvalidatesInflightGitResult(t *testing.T) {
	cache := runGitResultCache{}
	loadStarted := make(chan struct{})
	finishLoad := make(chan struct{})
	firstResult := make(chan any, 1)
	go func() {
		firstResult <- cache.get("run-1", "01-first", func() any {
			close(loadStarted)
			<-finishLoad
			return "before-event"
		})
	}()
	<-loadStarted
	cache.invalidateRun("run-1")
	close(finishLoad)
	if got := <-firstResult; got != "before-event" {
		t.Fatalf("in-flight result = %v, want before-event", got)
	}
	if got := cache.get("run-1", "01-first", func() any { return "after-event" }); got != "after-event" {
		t.Fatalf("result after event = %v, want fresh result", got)
	}
}
