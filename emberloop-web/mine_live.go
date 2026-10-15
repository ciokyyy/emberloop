package web

import (
	"sync"
	"time"
)

// mineFlight is the per-Session turn-end ingestion state.
type mineFlight struct {
	scan       sync.Mutex
	running    bool
	again      bool
	queued     bool
	generation uint64
	timer      *time.Timer
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

// maybeMineLive debounces idle notifications, then scans off the turn path.
// A burst during a scan schedules exactly one follow-up scan.
func (d *Daemon) maybeMineLive(id string) {
	d.mineMu.Lock()
	if !d.liveMine {
		d.mineMu.Unlock()
		return
	}
	if d.mineState == nil {
		d.mineState = make(map[string]*mineFlight)
	}
	fl := d.mineState[id]
	if fl == nil {
		fl = &mineFlight{}
		d.mineState[id] = fl
	}
	if fl.running {
		fl.again = true
		d.mineMu.Unlock()
		return
	}
	if fl.queued {
		fl.timer.Stop()
	} else {
		fl.queued = true
		d.mineWG.Add(1)
		d.mineActive++
	}
	fl.generation++
	generation := fl.generation
	fl.timer = time.AfterFunc(ingestDebounce, func() { d.runTurnIngest(id, fl, generation) })
	d.mineMu.Unlock()
}

func (d *Daemon) runTurnIngest(id string, fl *mineFlight, generation uint64) {
	fl.scan.Lock()
	defer fl.scan.Unlock()
	for {
		d.mineMu.Lock()
		if generation != fl.generation {
			d.mineMu.Unlock()
			return
		}
		fl.queued = false
		fl.running = true
		fl.again = false
		d.mineMu.Unlock()
		_ = d.ingestSession(id)
		d.mineMu.Lock()
		if !fl.again {
			fl.running = false
			d.mineActive--
			d.mineMu.Unlock()
			d.mineWG.Done()
			return
		}
		fl.again = false
		d.mineMu.Unlock()
		time.Sleep(ingestDebounce)
	}
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
