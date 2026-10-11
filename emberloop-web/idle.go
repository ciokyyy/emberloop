package web

import "time"

// now reads the Daemon clock: the test seam when set, else time.Now.
func (d *Daemon) now() time.Time {
	if d.clock != nil {
		return d.clock()
	}
	return time.Now()
}

// Idle is the Daemon-wide idle check, the one definition every idle caller
// uses. The Daemon is idle when no Session is starting, no Run is running and
// every Session is idle (see session.Idle) at quiet. Background work passes
// its settle window; restart passes zero, since it only needs to avoid cutting
// live work.
func (d *Daemon) Idle(quiet time.Duration) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.starting > 0 {
		return false
	}
	for _, rs := range d.runs {
		if rs.Status == runStatusRunning {
			return false
		}
	}
	now := d.now()
	open := d.openParentsLocked()
	for id, s := range d.sessions {
		if !s.Idle(now, quiet, open[id]) {
			return false
		}
	}
	return true
}

// Idle is the per-Session half of the one definition: no turn running, starting
// or queued, no resume or release in flight, no open Group led, and at least
// quiet since the Session's last Event on the Daemon clock. A live Session that
// waits for a prompt is idle: its history and resume token persist, and the next
// prompt reloads it. Callers hold d.mu.
func (s *session) Idle(now time.Time, quiet time.Duration, open bool) bool {
	if s.hasWork() || s.resuming || s.releasing || open {
		return false
	}
	return now.Sub(s.last) >= quiet
}

// hasWork reports whether a turn runs or starts, or input waits in the Daemon
// queue. Callers hold d.mu.
func (s *session) hasWork() bool {
	return s.busy || s.active > 0 || len(s.pending) > 0
}

// openParentsLocked returns the Sessions that lead a Group with a member still
// working. A Group closes once its members settle, even when its result never
// delivered (a restart cut it), so a stale Group never blocks idle. Callers hold
// d.mu.
func (d *Daemon) openParentsLocked() map[string]bool {
	open := map[string]bool{}
	for _, g := range d.groups {
		for _, m := range g.Members {
			if ms, ok := d.sessions[m]; ok && ms.hasWork() {
				open[g.Parent] = true
				break
			}
		}
	}
	return open
}
