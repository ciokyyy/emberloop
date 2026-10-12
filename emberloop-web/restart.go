package web

import (
	"fmt"
	"net/http"
	"os"
)

// Shutdown closes every live Session's agent process without ending the
// Session, so a restart leaves no adapter behind. History and resume tokens
// are already on disk; the next Daemon loads the Sessions as released.
func (d *Daemon) Shutdown() {
	d.waitMining()
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

// RestartCh fires once after RequestRestart; the binary waits on it, drains
// and re-executes.
func (d *Daemon) RestartCh() <-chan struct{} { return d.restartCh }

// RequestRestart asks for a graceful restart. Every running Run pauses and
// is marked ResumeOnRestart, so its driver starts nothing new while Worker
// turns finish, and the next Daemon resumes it on its own. Repeats are
// no-ops. It reports whether this call set the request. The last published
// driver rows reach the file first: the worker-start persist writes chain
// positions before the driver's next pass publishes, so without this the
// reloaded Run would show an empty or queued row for a running Worker.
func (d *Daemon) RequestRestart() bool {
	d.mu.Lock()
	if d.restartReq {
		d.mu.Unlock()
		return false
	}
	d.restartReq = true
	for _, rs := range d.runs {
		if rs.Status != runStatusRunning {
			continue
		}
		rs.Status = runStatusPaused
		rs.ResumeOnRestart = true
		d.snapshotRowsLocked(rs)
		d.persistRun(rs)
		d.broadcastRunLocked(rs)
	}
	d.mu.Unlock()
	select {
	case d.restartCh <- struct{}{}:
	default:
	}
	return true
}

// CancelRestart drops a request whose exec failed and resumes the Runs it
// paused, so the running Daemon keeps working.
func (d *Daemon) CancelRestart() {
	d.mu.Lock()
	d.restartReq = false
	d.mu.Unlock()
	d.ResumeAfterRestart()
}

// RestartRequested reports whether a graceful restart is pending.
func (d *Daemon) RestartRequested() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.restartReq
}

// ResumeAfterRestart resumes every paused Run saved with ResumeOnRestart.
// The binary calls it once its Backends are registered.
func (d *Daemon) ResumeAfterRestart() {
	d.mu.Lock()
	var ids []string
	for id, rs := range d.runs {
		if rs.Status == runStatusPaused && rs.ResumeOnRestart {
			ids = append(ids, id)
		}
	}
	d.mu.Unlock()
	for _, id := range ids {
		if _, code, msg := d.resumeRunCore(id); code != 0 {
			fmt.Fprintf(os.Stderr, "el-web: resume run %s after restart: %d %s\n", id, code, msg)
		}
	}
}

// handleRestart serves POST /api/restart: ask for the graceful restart. It
// answers 202 at once; the restart happens once Sessions are idle.
func (d *Daemon) handleRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	fresh := d.RequestRestart()
	writeJSON(w, http.StatusAccepted, map[string]any{"restart": "requested", "new": fresh})
}
