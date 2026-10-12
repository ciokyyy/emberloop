package web

import (
	"sync"
	"time"
)

// mineFlight is the per-Session mining state. scan serializes scans of one
// Session (the cursor file is single-writer). running and again coalesce
// turn-end triggers: a trigger that lands while a scan runs sets again, and
// the running worker scans once more instead of stacking goroutines.
type mineFlight struct {
	scan    sync.Mutex
	running bool
	again   bool
}

// mineFlightFor returns the Session's flight state, creating it on first use.
func (d *Daemon) mineFlightFor(id string) *mineFlight {
	d.mineMu.Lock()
	defer d.mineMu.Unlock()
	if d.mineState == nil {
		d.mineState = make(map[string]*mineFlight)
	}
	fl, ok := d.mineState[id]
	if !ok {
		fl = &mineFlight{}
		d.mineState[id] = fl
	}
	return fl
}

// mineWaitDefault bounds Shutdown's wait for turn-end scans: a hung scan
// must wedge neither the restart nor the tests that call waitMining.
const mineWaitDefault = 30 * time.Second

// EnableLiveMining turns on the turn-end mine. The binary calls it once at
// startup; tests that exercise the trigger call it too. Off by default so
// unit tests do not race background scans against their temp dirs.
func (d *Daemon) EnableLiveMining() {
	d.mineMu.Lock()
	d.liveMine = true
	d.mineMu.Unlock()
}

// maybeMineLive is the turn-end ingestion trigger, the mnemopi
// `maybeRetainOnAgentEnd` shape: when a turn settles, mine only the bytes
// added to the Session log since the cursor. It never blocks the turn: the scan
// runs on its own goroutine, and a failing or missing engine is dropped
// silently (Memory never blocks a turn; the nightly catch-up retries from
// the same cursor). Only the heuristic pass and the configured confirming
// Decider run here. Distill, pairing and Profile proposals stay nightly.
func (d *Daemon) maybeMineLive(id string) {
	d.mineMu.Lock()
	on := d.liveMine
	d.mineMu.Unlock()
	if !on {
		return
	}
	fl := d.mineFlightFor(id)
	d.mineMu.Lock()
	if fl.running {
		fl.again = true
		d.mineMu.Unlock()
		return
	}
	fl.running = true
	d.mineMu.Unlock()
	d.mineWG.Add(1)
	d.mineMu.Lock()
	d.mineActive++
	d.mineMu.Unlock()
	go func() {
		defer d.mineWG.Done()
		defer func() {
			d.mineMu.Lock()
			d.mineActive--
			d.mineMu.Unlock()
		}()
		for {
			_, _ = d.mineSession(id, true)
			d.mineMu.Lock()
			if !fl.again {
				fl.running = false
				d.mineMu.Unlock()
				return
			}
			fl.again = false
			d.mineMu.Unlock()
		}
	}()
}

// waitMining blocks until every running turn-end scan finishes. Shutdown and
// tests use it.
func (d *Daemon) waitMining() { d.mineWG.Wait() }

// waitMiningTimeout waits up to timeout for the running turn-end scans and
// reports whether they all finished. Shutdown uses it so a hung scan cannot
// wedge the restart past the drain.
func (d *Daemon) waitMiningTimeout(timeout time.Duration) bool {
	done := make(chan struct{})
	go func() { d.mineWG.Wait(); close(done) }()
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case <-done:
		return true
	case <-t.C:
		return false
	}
}

// miningActive reports whether a turn-end scan runs. Callers of RestartState
// hold neither lock; mineMu guards the count.
func (d *Daemon) miningActive() bool {
	d.mineMu.Lock()
	defer d.mineMu.Unlock()
	return d.mineActive > 0
}

// mineWaitOrDefault is Shutdown's mining bound: the test seam mineWait, else
// mineWaitDefault.
func (d *Daemon) mineWaitOrDefault() time.Duration {
	d.mineMu.Lock()
	defer d.mineMu.Unlock()
	if d.mineWait > 0 {
		return d.mineWait
	}
	return mineWaitDefault
}
