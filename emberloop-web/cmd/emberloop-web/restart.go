package main

import (
	"fmt"
	"os"
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
		if !d.Idle() {
			continue
		}
		fmt.Println("el-web: binary changed and idle, restarting")
		d.Shutdown()
		if err := syscall.Exec(exe, os.Args, os.Environ()); err != nil {
			fmt.Fprintln(os.Stderr, "restart:", err)
		}
	}
}
