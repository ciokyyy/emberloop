package web

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// defaultRestartDrain bounds the Session drain when the binary never set one.
const defaultRestartDrain = 5 * time.Minute

// restartLogName is the state dir file every restart step lands in, so an
// Exec failure stays visible after stdout went to a dead socket.
const restartLogName = "restart.log"

// Shutdown closes every live Session's agent process without ending the
// Session, so a restart leaves no adapter behind. History and resume tokens
// are already on disk; the next Daemon loads the Sessions as released. The
// wait for turn-end scans is bounded: a hung scan is logged and left behind,
// never wedging the restart past the drain.
func (d *Daemon) Shutdown() {
	if wait := d.mineWaitOrDefault(); !d.waitMiningTimeout(wait) {
		d.LogRestart("shutdown: mining still running after %s, continuing without it", wait)
	}
	type live struct {
		id string
		be Backend
	}
	var all []live
	d.mu.Lock()
	d.shuttingDown = true
	for id, s := range d.sessions {
		if s.live {
			d.markShutdownInterruptedLocked(id)
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

// SetRestartDrain tells the Daemon the drain the binary waits, so a repeat
// can tell a stuck request. The binary calls it once from -restart-drain.
func (d *Daemon) SetRestartDrain(drain time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.restartDrain = drain
}

// restartDrainOrDefaultLocked is the drain the binary waits. Callers hold d.mu.
func (d *Daemon) restartDrainOrDefaultLocked() time.Duration {
	if d.restartDrain > 0 {
		return d.restartDrain
	}
	return defaultRestartDrain
}

// RequestRestart asks for a graceful restart. Every running Run pauses and
// is marked ResumeOnRestart, so its driver starts nothing new while Worker
// turns finish, and the next Daemon resumes it on its own. Repeats are
// no-ops, except a repeat older than the drain plus a minute: the watcher
// may have died or lost the signal (an older binary), so the repeat
// re-arms it and reports true again. It reports whether this call set (or
// re-armed) the request. The last published driver rows reach the file
// first: the worker-start persist writes chain positions before the
// driver's next pass publishes, so without this the reloaded Run would
// show an empty or queued row for a running Worker.
func (d *Daemon) RequestRestart() bool {
	d.mu.Lock()
	if d.restartReq {
		if d.now().Sub(d.restartAt) > d.restartDrainOrDefaultLocked()+time.Minute {
			d.restartAt = d.now()
			d.mu.Unlock()
			d.LogRestart("restart requested again after the drain, re-arming the watcher")
			select {
			case d.restartCh <- struct{}{}:
			default:
			}
			return true
		}
		d.mu.Unlock()
		return false
	}
	d.restartReq = true
	d.restartAt = d.now()
	d.pauseRunsForRestartLocked()
	d.mu.Unlock()
	d.LogRestart("restart requested, pausing runs and waiting for idle")
	select {
	case d.restartCh <- struct{}{}:
	default:
	}
	return true
}

// ReassertRestart re-establishes a cleared request without signaling the
// watcher: a re-arm consumed after a failed exec's cancel must pause the
// resumed Runs again, not queue another drain cycle behind itself.
func (d *Daemon) ReassertRestart() {
	d.mu.Lock()
	if !d.restartReq {
		d.restartReq = true
		d.restartAt = d.now()
		d.pauseRunsForRestartLocked()
		d.mu.Unlock()
		d.LogRestart("restart re-asserted, pausing runs and waiting for idle")
		return
	}
	d.mu.Unlock()
}

// pauseRunsForRestartLocked pauses every running Run and marks it
// ResumeOnRestart, so its driver starts nothing new while Worker turns
// finish, and the next Daemon resumes it on its own. Callers hold d.mu.
func (d *Daemon) pauseRunsForRestartLocked() {
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
	d.armRestartStopsLocked()
}

// CancelRestart drops a request whose exec failed and resumes the Runs it
// paused, so the running Daemon keeps working.
func (d *Daemon) CancelRestart() {
	d.mu.Lock()
	d.restartReq = false
	d.shuttingDown = false
	d.stopRestart = nil
	d.mu.Unlock()
	d.LogRestart("restart cancelled after exec failure, resuming runs")
	d.resumeMarkedRuns()
}

// RestartRequested reports whether a graceful restart is pending.
func (d *Daemon) RestartRequested() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.restartReq
}

// RestartState is the GET /api/restart answer and the POST /api/restart
// detail: whether a graceful restart is pending, since when, and what still
// blocks the drain (busy Session ids, outstanding Group ids, running Run
// ids, the starting count, a running turn-end scan). el restart prints it.
type RestartState struct {
	Requested bool      `json:"requested"`
	Since     time.Time `json:"since,omitempty"`
	Drain     string    `json:"drain"`
	Sessions  []string  `json:"sessions,omitempty"`
	Groups    []string  `json:"groups,omitempty"`
	Runs      []string  `json:"runs,omitempty"`
	Starting  int       `json:"starting,omitempty"`
	Mining    bool      `json:"mining,omitempty"`
}

// RestartState snapshots the pending restart and its blockers: the Sessions
// failing Idle at zero quiet, the Groups still holding a compress, and the
// Runs still driving.
func (d *Daemon) RestartState() RestartState {
	mining := d.miningActive()
	d.mu.Lock()
	defer d.mu.Unlock()
	st := RestartState{
		Requested: d.restartReq,
		Drain:     d.restartDrainOrDefaultLocked().String(),
		Starting:  d.starting,
		Mining:    mining,
	}
	if d.restartReq {
		st.Since = d.restartAt
	}
	for _, rs := range d.runs {
		if rs.Status == runStatusRunning {
			st.Runs = append(st.Runs, rs.ID)
		}
	}
	now := d.now()
	for id, s := range d.sessions {
		if !s.Idle(now, 0, d.groupOutstandingLocked(id)) {
			st.Sessions = append(st.Sessions, id)
		}
	}
	for _, g := range d.groups {
		if d.groupOpenLocked(g) {
			st.Groups = append(st.Groups, g.ID)
		}
	}
	sort.Strings(st.Runs)
	sort.Strings(st.Sessions)
	sort.Strings(st.Groups)
	return st
}

// LogRestart writes one timestamped line to stderr and the state dir's
// restart.log, so the restart's steps survive a dead stdout and an Exec
// failure stays visible for the next el restart.
func (d *Daemon) LogRestart(format string, args ...any) {
	stamped := time.Now().Format(time.RFC3339) + " " + fmt.Sprintf(format, args...)
	fmt.Fprintln(os.Stderr, "el-web restart: "+stamped)
	dir := filepath.Join(d.stateDir, "enderbloom")
	_ = os.MkdirAll(dir, 0o755)
	f, err := os.OpenFile(filepath.Join(dir, restartLogName), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintln(f, stamped)
}

// ResumeAfterRestart resumes every paused Run saved with ResumeOnRestart.
// The binary calls it once its Backends are registered.
func (d *Daemon) ResumeAfterRestart() {
	if d.bootLoopTripped() {
		d.logBootLoop()
		return
	}
	d.resumeMarkedRuns()
}

// resumeMarkedRuns resumes every paused Run marked ResumeOnRestart. A boot
// goes through ResumeAfterRestart (the boot-loop guard); a cancelled restart
// in the same process calls this directly.
func (d *Daemon) resumeMarkedRuns() {
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
			continue
		}
	}
}

// handleRestart serves the graceful restart: POST asks for it and answers
// 202 at once (the restart happens once Sessions are idle), GET reports the
// pending request and what still blocks the drain.
func (d *Daemon) handleRestart(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		fresh := d.RequestRestart()
		writeJSON(w, http.StatusAccepted, map[string]any{"restart": "requested", "new": fresh, "state": d.RestartState()})
	case http.MethodGet:
		writeJSON(w, http.StatusOK, d.RestartState())
	default:
		http.Error(w, "GET or POST only", http.StatusMethodNotAllowed)
	}
}
