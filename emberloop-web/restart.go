package web

// Idle reports whether the Daemon holds no work a restart would cut short: no
// Session mid-turn, starting, resuming or releasing, and no Run still driving.
// Live Sessions that wait for a prompt do not count: they hold history and a
// resume token, and the next prompt reloads them.
func (d *Daemon) Idle() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.starting > 0 {
		return false
	}
	for _, s := range d.sessions {
		if s.busy || s.active > 0 || s.resuming || s.releasing {
			return false
		}
	}
	for _, rs := range d.runs {
		if rs.Status == runStatusRunning {
			return false
		}
	}
	return true
}

// Shutdown closes every live Session's agent process without ending the
// Session, so a restart leaves no adapter behind. History and resume tokens
// are already on disk; the next Daemon loads the Sessions as released.
func (d *Daemon) Shutdown() {
	type live struct {
		id string
		be Backend
	}
	var all []live
	d.mu.Lock()
	for id, s := range d.sessions {
		if s.live {
			s.live = false
			all = append(all, live{id, d.backendForLocked(s.meta.Harness)})
		}
	}
	d.mu.Unlock()
	for _, l := range all {
		if l.be != nil {
			l.be.CloseSession(l.id)
		}
	}
}
