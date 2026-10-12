package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	web "example.com/emberloop-web"
)

// binStamp identifies one build of the Daemon binary on disk.
type binStamp struct {
	mod  time.Time
	size int64
	ino  uint64
}

func stampOf(path string) (binStamp, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return binStamp{}, err
	}
	st := binStamp{mod: fi.ModTime(), size: fi.Size()}
	if sys, ok := fi.Sys().(*syscall.Stat_t); ok {
		st.ino = sys.Ino
	}
	return st, nil
}

// watchForUpdate re-executes the Daemon in place once its binary on disk has
// changed and the Daemon is idle. Same pid, same address, so whatever started
// it (bun, a shell, systemd) keeps its handle. The change must hold for two
// ticks in a row, so a build still writing the file is never run half-written,
// and the Daemon must be idle on the tick that restarts. A busy Daemon just
// waits for the next tick. Exec failing leaves the running Daemon serving.
func watchForUpdate(d *web.Daemon, exe string, every time.Duration) {
	if every <= 0 {
		return
	}
	start, err := stampOf(exe)
	if err != nil {
		return
	}
	var seen binStamp
	for range time.Tick(every) {
		now, err := stampOf(exe)
		if err != nil || now == start {
			seen = binStamp{}
			continue
		}
		if now != seen {
			seen = now
			continue
		}
		// No quiet time: restart waits for work it would cut, not for quiet.
		if !d.Idle(0) {
			continue
		}
		d.LogRestart("binary changed and idle, restarting")
		d.Shutdown()
		if err := syscall.Exec(exe, os.Args, os.Environ()); err != nil {
			d.LogRestart("update restart exec failed: %v", err)
		}
	}
}

// watchForRestartRequest serves the graceful restart: once the Daemon
// asked for it (POST /api/restart or SIGUSR1) it waits until no Session is
// mid-turn, at most drain, then closes the agents and re-executes in place.
// The drain deadline forces the restart regardless of Idle: a Session that
// never settles delays the restart but cannot wedge it. Runs were paused
// and marked by RequestRestart, so the new Daemon resumes them. Exec failing
// cancels the request and resumes them here. Every step is logged to stderr
// and the state dir's restart.log, so a dead stdout never hides it.
func watchForRestartRequest(d *web.Daemon, exe string, drain time.Duration) {
	for range d.RestartCh() {
		// Re-establish the request without signaling: a re-arm consumed
		// after a failed exec's cancel is a fresh request, and a signal
		// here would queue another drain cycle behind itself forever.
		d.ReassertRestart()
		st := d.RestartState()
		d.LogRestart("requested, waiting for idle up to %s (sessions=%v groups=%v runs=%v starting=%d mining=%v)",
			drain, st.Sessions, st.Groups, st.Runs, st.Starting, st.Mining)
		if forced := waitRestartIdle(d, drain, 500*time.Millisecond); forced {
			d.LogRestart("drain %s expired, forcing the restart regardless of idle", drain)
		}
		d.LogRestart("shutting down agent processes")
		d.Shutdown()
		d.LogRestart("re-executing %s", exe)
		if err := syscall.Exec(exe, os.Args, os.Environ()); err != nil {
			d.LogRestart("exec failed: %v", err)
			d.CancelRestart()
		}
	}
}

// waitRestartIdle waits until the Daemon is idle or drain passes, polling
// every poll and logging the blockers every 30s. It reports whether the
// drain forced the return past idle: a Session that never settles delays the
// restart but cannot wedge it.
func waitRestartIdle(d *web.Daemon, drain, poll time.Duration) bool {
	deadline := time.Now().Add(drain)
	lastLog := time.Now()
	for {
		if d.Idle(0) {
			return false
		}
		if time.Now().After(deadline) {
			return true
		}
		if time.Since(lastLog) >= 30*time.Second {
			st := d.RestartState()
			d.LogRestart("still waiting on sessions=%v groups=%v runs=%v starting=%d mining=%v",
				st.Sessions, st.Groups, st.Runs, st.Starting, st.Mining)
			lastLog = time.Now()
		}
		time.Sleep(poll)
	}
}

// signalRestart turns SIGUSR1 (systemctl reload) into a restart request.
func signalRestart(d *web.Daemon) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGUSR1)
	for range ch {
		d.RequestRestart()
	}
}
