package web

import (
	"sync"
	"time"
)

const runGitResultCacheTTL = 3 * time.Second

type runGitResultCacheKey struct {
	runID    string
	ticketID string
}

type runGitResultCacheEntry struct {
	value     any
	expiresAt time.Time
}

type runGitResultFlight struct {
	done        chan struct{}
	invalidated bool
}

// runGitResultCache holds git-derived Run and Ticket results briefly. A Run
// event invalidates every entry for that Run and prevents in-flight loads
// from repopulating the cache.
type runGitResultCache struct {
	mu        sync.Mutex
	entries   map[runGitResultCacheKey]runGitResultCacheEntry
	flights   map[runGitResultCacheKey]*runGitResultFlight
	nextSweep time.Time
	now       func() time.Time
}

func (c *runGitResultCache) get(runID, ticketID string, load func() any) any {
	key := runGitResultCacheKey{runID: runID, ticketID: ticketID}
	for {
		c.mu.Lock()
		now := c.cacheNow()
		if !now.Before(c.nextSweep) {
			for expiredKey, entry := range c.entries {
				if !now.Before(entry.expiresAt) {
					delete(c.entries, expiredKey)
				}
			}
			c.nextSweep = now.Add(runGitResultCacheTTL)
		}
		if entry, ok := c.entries[key]; ok && now.Before(entry.expiresAt) {
			c.mu.Unlock()
			return entry.value
		}
		delete(c.entries, key)
		if flight := c.flights[key]; flight != nil {
			done := flight.done
			c.mu.Unlock()
			<-done
			continue
		}
		if c.flights == nil {
			c.flights = make(map[runGitResultCacheKey]*runGitResultFlight)
		}
		flight := &runGitResultFlight{done: make(chan struct{})}
		c.flights[key] = flight
		c.mu.Unlock()
		return c.loadAndStore(key, flight, load)
	}
}

func (c *runGitResultCache) loadAndStore(
	key runGitResultCacheKey,
	flight *runGitResultFlight,
	load func() any,
) (value any) {
	defer func() {
		recovered := recover()
		c.mu.Lock()
		if recovered == nil && !flight.invalidated {
			if c.entries == nil {
				c.entries = make(map[runGitResultCacheKey]runGitResultCacheEntry)
			}
			c.entries[key] = runGitResultCacheEntry{value: value, expiresAt: c.cacheNow().Add(runGitResultCacheTTL)}
		}
		if c.flights[key] == flight {
			delete(c.flights, key)
		}
		close(flight.done)
		c.mu.Unlock()
		if recovered != nil {
			panic(recovered)
		}
	}()
	return load()
}

func (c *runGitResultCache) invalidateRun(runID string) {
	c.mu.Lock()
	for key := range c.entries {
		if key.runID == runID {
			delete(c.entries, key)
		}
	}
	for key, flight := range c.flights {
		if key.runID == runID {
			flight.invalidated = true
		}
	}
	c.mu.Unlock()
}

func (c *runGitResultCache) cacheNow() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}
