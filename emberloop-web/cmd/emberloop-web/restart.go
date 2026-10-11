package main

import (
	"fmt"
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
		fmt.Println("el-web: binary changed and idle, restarting")
		d.Shutdown()
		if err := syscall.Exec(exe, os.Args, os.Environ()); err != nil {
			fmt.Fprintln(os.Stderr, "restart:", err)
		}
	}
}

// watchForRestartRequest serves the graceful restart: once the Daemon
// asked for it (POST /api/restart or SIGUSR1) it waits until no Session is
// mid-turn, at most drain, then closes the agents and re-executes in place.
// Runs were paused and marked by RequestRestart, so the new Daemon resumes
// them. Exec failing cancels the request and resumes them here.
func watchForRestartRequest(d *web.Daemon, exe string, drain time.Duration) {
	for range d.RestartCh() {
		fmt.Println("el-web: restart requested, waiting for idle")
		deadline := time.Now().Add(drain)
		for !d.Idle(0) && time.Now().Before(deadline) {
			time.Sleep(500 * time.Millisecond)
		}
		fmt.Println("el-web: restarting")
		d.Shutdown()
		if err := syscall.Exec(exe, os.Args, os.Environ()); err != nil {
			fmt.Fprintln(os.Stderr, "restart:", err)
			d.CancelRestart()
		}
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
