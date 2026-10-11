package web

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestMain keeps session titles off for the whole package: title calls start
// extra adapter connections that would land in shared test stubs. The title
// tests switch them back on per test.
//
// It also isolates the package from the developer's machine: HOME and
// EL_SETTINGS point at an empty temp dir, so role settings (worker, scout,
// reviewers) never route test Sessions to real claude or pig harnesses, and
// the Daemon binding of a running Session never leaks in.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "el-web-test-home-")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", home)
	os.Setenv("EL_SETTINGS", filepath.Join(home, "settings.json"))
	os.Unsetenv("EL_MAX_WORKERS")
	os.Unsetenv("EL_WEB_PARENT")
	os.Unsetenv("EL_WEB_DAEMON_URL")
	os.Setenv("EL_SESSION_TITLES", "0")
	// The SDK sometimes leaves a finished turn stream open; the stall
	// watchdog ends it. Real timings cost ~30s per hit, so shrink them.
	pigTailStallTimeout = 2 * time.Second
	pigStallTick = 200 * time.Millisecond
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}

// getSessions lists every Session on the test Daemon.
func getSessions(t *testing.T, base string) []SessionMeta {
	t.Helper()
	resp, err := http.Get(base + "/api/sessions")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out []SessionMeta
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}
