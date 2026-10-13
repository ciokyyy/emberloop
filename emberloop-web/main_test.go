package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"example.com/emberloop-web/testsupport"
)

// sharedTempRoots holds process-shared template dirs (run repo templates)
// that TestMain removes after the run. Per-test dirs still use tempDir.
var (
	sharedTempMu    sync.Mutex
	sharedTempRoots []string
)

// registerSharedTemp remembers a process-shared temp dir for TestMain
// removal. It never fails the test: worst case the OS reaps /tmp.
func registerSharedTemp(dir string) {
	sharedTempMu.Lock()
	defer sharedTempMu.Unlock()
	sharedTempRoots = append(sharedTempRoots, dir)
}

// TestMain keeps session titles off for the whole package: title calls start
// extra adapter connections that would land in shared test stubs. The title
// tests switch them back on per test.
//
// It also isolates the package from the developer's machine: HOME and
// EL_SETTINGS point at an empty temp dir, so role settings (worker, scout,
// reviewers) never route test Sessions to real claude or pig harnesses, and
// the Daemon binding of a running Session never leaks in.
//
// Git identity and config isolation live here too, once per process, so
// tests stay parallel-safe: no test may t.Setenv git or HOME variables.
// PIG_TEST_FAUX is on for the whole package for the same reason; tests that
// need other pig scenario or key variables keep their own t.Setenv and stay
// serial.
// isHelperChild reports a re-executed test binary serving as an ACP adapter
// fake (see spawnHelperConn): it inherits the parent's HOME and EL_SETTINGS
// and makes no home dir of its own, so a child the parent kills leaks
// nothing. Env is left untouched: the parent arranged it per test.
func isHelperChild() bool {
	if os.Getenv("GO_TEST_ACP_ADAPTER") != "" {
		return true
	}
	for _, a := range os.Args {
		if strings.Contains(a, "TestHelperACPAdapter") {
			return true
		}
	}
	return false
}

func TestMain(m *testing.M) {
	if isHelperChild() {
		os.Exit(m.Run())
	}
	home, err := os.MkdirTemp("", "el-web-test-home-")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", home)
	os.Setenv("EL_SETTINGS", filepath.Join(home, "settings.json"))
	os.Setenv("EL_TITLES_DIR", filepath.Join(home, "el-titles"))
	os.Unsetenv("EL_MAX_WORKERS")
	os.Unsetenv("EL_MAX_TICKETS")
	os.Unsetenv("EL_WEB_PARENT")
	os.Unsetenv("EL_WEB_DAEMON_URL")
	os.Setenv("EL_SESSION_TITLES", "0")
	os.Setenv("PIG_TEST_FAUX", "1")
	// Every faux backend uses the parity-basic scenario; the SDK defaults to
	// it when unset, so fixing it here changes no behavior and lets pig
	// tests stay parallel-safe.
	os.Setenv("PIG_TEST_FAUX_SCENARIO", "parity-basic")
	os.Setenv("GIT_AUTHOR_NAME", "web-test")
	os.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	os.Setenv("GIT_COMMITTER_NAME", "web-test")
	os.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
	os.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	os.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	os.Setenv("GIT_TERMINAL_PROMPT", "0")
	// The SDK sometimes leaves a finished turn stream open; the stall
	// watchdog ends it. Real timings cost ~30s per hit, so shrink them.
	pigTailStallTimeout = 2 * time.Second
	pigStallTick = 200 * time.Millisecond
	code := m.Run()
	sharedTempMu.Lock()
	roots := sharedTempRoots
	sharedTempRoots = nil
	sharedTempMu.Unlock()
	for _, dir := range roots {
		testsupport.RemoveDir(dir)
	}
	left := testsupport.CleanupLeaks()
	ok, bad, _ := testsupport.Report()
	os.Stderr.WriteString(fmt.Sprintf("el-web: tempdir cleanups ok=%d failed=%d leaked=%d\n", ok, bad, len(left)))
	for _, dir := range left {
		os.Stderr.WriteString("el-web: leaked temp dir: " + dir + "\n")
	}
	testsupport.RemoveDir(home)
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
