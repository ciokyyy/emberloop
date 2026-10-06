package wick

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestMain keeps the whole package's tests away from the herdr session that
// runs them. The runner sits in a herdr pane, so its HERDR_PANE_ID would make
// a test's grouped notice a real message typed into the running session, and
// a test that reaches the herdr binary would act on real panes. The pane
// variables are cleared, and HERDR_BIN_PATH names a stand-in that answers every
// call as herdr answers a pane it does not know. A test that needs herdr
// injects its own Exec.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "wick-test-herdr")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	stub := filepath.Join(dir, "herdr")
	script := "#!/bin/sh\necho '{\"error\":{\"code\":\"pane_not_found\"}}' >&2\nexit 1\n"
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	for _, k := range []string{"HERDR_ENV", "HERDR_PANE_ID", "HERDR_TAB_ID", "HERDR_WORKSPACE_ID", "WICK_AGENT_ID", "WICK_AGENT_TYPE"} {
		os.Unsetenv(k)
	}
	os.Setenv("HERDR_BIN_PATH", stub)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
