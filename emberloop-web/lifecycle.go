package web

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// defaultMaxTickets caps concurrent Run tickets when no setting exists.
// It matches the el global default for max_tickets.
const defaultMaxTickets = 3

// maxTicketsFromSettings reads the existing global max_tickets setting: the
// EL_MAX_TICKETS env wins, then the global settings file (or
// EL_SETTINGS when set, for tests and second profiles), then the old
// state-dir file, then the built-in default. The pre-rename max_workers key
// and EL_MAX_WORKERS env still work as fallbacks when the new ones are
// absent, so existing settings keep working; they are never written.
func maxTicketsFromSettings() int {
	if w, ok := envMaxTickets(); ok {
		return w
	}
	for _, p := range settingsPaths() {
		raw, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var s struct {
			MaxTickets int `json:"max_tickets"`
			MaxWorkers int `json:"max_workers"`
		}
		if err := json.Unmarshal(raw, &s); err == nil {
			if s.MaxTickets > 0 {
				return s.MaxTickets
			}
			if s.MaxWorkers > 0 {
				return s.MaxWorkers
			}
		}
	}
	return defaultMaxTickets
}

func settingsPaths() []string {
	if p := os.Getenv("EL_SETTINGS"); p != "" {
		return []string{p}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{
		filepath.Join(home, ".el", "settings.json"),
		filepath.Join(home, ".pig", "state", "emberloop", "settings.json"),
	}
}

// SetMaxTickets overrides the max_tickets setting, for tests.
func (d *Daemon) SetMaxTickets(n int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.maxTickets = n
}

// SetPollInterval caps this Daemon's Run driver sleep, for tests. Call it
// before serving: the write happens before the server's goroutines start,
// so no lock is needed. Zero keeps the production default.
func (d *Daemon) SetPollInterval(v time.Duration) {
	d.pollInterval = v
}

// pollIntervalOrDefault reports the driver sleep cap in force: the test
// override, or the production default.
func (d *Daemon) pollIntervalOrDefault() time.Duration {
	if d != nil && d.pollInterval > 0 {
		return d.pollInterval
	}
	return runPollInterval
}

// MaxTickets is the Run ticket concurrency in force: the override, or the
// global max_tickets setting. Sessions themselves are uncapped; only
// concurrent Run tickets are limited.
func (d *Daemon) MaxTickets() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.maxTicketsLocked()
}

func (d *Daemon) maxTicketsLocked() int {
	if d.maxTickets > 0 {
		return d.maxTickets
	}
	return maxTicketsFromSettings()
}
