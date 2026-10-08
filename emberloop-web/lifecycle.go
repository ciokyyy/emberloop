package web

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// defaultMaxRunning caps concurrent running Sessions when no setting exists.
// It matches the emberloop global default for max_workers.
const defaultMaxRunning = 3

// maxRunningFromSettings reads the existing global max_workers setting: the
// EMBERLOOP_MAX_WORKERS env wins, then the global settings file (or
// EMBERLOOP_SETTINGS when set, for tests and second profiles), then the old
// state-dir file, then the built-in default.
func maxRunningFromSettings() int {
	if w, err := strconv.Atoi(strings.TrimSpace(os.Getenv("EMBERLOOP_MAX_WORKERS"))); err == nil && w > 0 {
		return w
	}
	for _, p := range settingsPaths() {
		raw, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var s struct {
			MaxWorkers int `json:"max_workers"`
		}
		if err := json.Unmarshal(raw, &s); err == nil && s.MaxWorkers > 0 {
			return s.MaxWorkers
		}
	}
	return defaultMaxRunning
}

func settingsPaths() []string {
	if p := os.Getenv("EMBERLOOP_SETTINGS"); p != "" {
		return []string{p}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{
		filepath.Join(home, ".emberloop", "settings.json"),
		filepath.Join(home, ".pig", "state", "emberloop", "settings.json"),
	}
}

// SetMaxRunning overrides the max_workers setting, for tests.
func (d *Daemon) SetMaxRunning(n int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.maxRunning = n
}

// MaxRunning is the concurrency cap in force: the override, or the global
// max_workers setting.
func (d *Daemon) MaxRunning() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.maxRunningLocked()
}

func (d *Daemon) maxRunningLocked() int {
	if d.maxRunning > 0 {
		return d.maxRunning
	}
	return maxRunningFromSettings()
}

// liveCountLocked counts Sessions holding an agent process: live and not
// ended. Callers hold d.mu.
func (d *Daemon) liveCountLocked() int {
	n := 0
	for _, s := range d.sessions {
		if s.live && s.meta.Status != "ended" {
			n++
		}
	}
	return n + d.starting
}
