package wick

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// livePaneProcs answers a herdr pane process-info for a pane with a child
// running past its shell, the way herdr reports a live pane. A bare shell
// answers the same call with only the shell in the foreground, and a missing
// pane with pane_not_found; fakes use those shapes to say which panes hold
// an agent.
func livePaneProcs(pane string) Result {
	return Result{Stdout: `{"result":{"process_info":{"pane_id":"` + pane + `","shell_pid":100,"foreground_process_group_id":101,"foreground_processes":[{"pid":101}]}}}`}
}

// barePaneProcs answers a herdr pane process-info for a pane herdr knows but
// whose foreground holds only the shell: the pane still exists, but no agent
// process runs in it.
func barePaneProcs(pane string) Result {
	return Result{Stdout: `{"result":{"process_info":{"pane_id":"` + pane + `","shell_pid":100,"foreground_process_group_id":100,"foreground_processes":[{"pid":100}]}}}`}
}

// paneArg is the pane a herdr pane command names: --pane's value, or the
// bare id, whichever form the call uses.
func paneArg(args []string) string {
	if v := argValue(args, "--pane"); v != "" {
		return v
	}
	if len(args) > 2 {
		return args[2]
	}
	return ""
}

// liveDeliverReply answers every herdr call the notice path makes as a live
// pane: process-info reports a child past the shell, everything else is
// empty success. A test that kills a pane overrides its own reply.
func liveDeliverReply(cmd string, args []string) Result {
	if len(args) > 1 && args[0] == "pane" && args[1] == "process-info" {
		return livePaneProcs(paneArg(args))
	}
	return Result{Stdout: "{}"}
}

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
	// The code graph lifecycle stays unwired in tests: no Spawn, review or
	// finish test may spawn a real graph server. Graph tests wire the dial
	// themselves, per test.
	graphLifecycleDial = nil
	// The pause between a message and its Enter is real time; tests skip it.
	submitSettle = 0
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
