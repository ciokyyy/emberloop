package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CommandRunner runs one command and returns its stdout. The live program
// passes os/exec; tests pass a fake herdr or git.
type CommandRunner func(name string, args ...string) ([]byte, error)

// herdrBin is the multiplexer binary, honouring the core's override.
func herdrBin() string {
	if p := os.Getenv("HERDR_BIN_PATH"); p != "" {
		return p
	}
	return "herdr"
}

// HerdrPanes returns the liveness seam Snapshot reads: every herdr pane still
// alive, keyed by pane id with its agent status, from `herdr pane list`. A
// pane without an agent status reads as "" (alive, no state). A failure is
// returned — Snapshot keeps the last tree and retries — never an empty set
// that would read every run as ended.
func HerdrPanes(run CommandRunner) Herdr {
	return func() (map[string]string, error) {
		raw, err := run(herdrBin(), "pane", "list")
		if err != nil {
			return nil, err
		}
		var reply struct {
			Result struct {
				Panes []struct {
					PaneID string `json:"pane_id"`
					Status string `json:"agent_status"`
				} `json:"panes"`
			} `json:"result"`
		}
		if err := json.Unmarshal(raw, &reply); err != nil {
			return nil, fmt.Errorf("herdr pane list returned non-JSON: %.200s", strings.TrimSpace(string(raw)))
		}
		out := map[string]string{}
		for _, p := range reply.Result.Panes {
			out[p.PaneID] = p.Status
		}
		return out, nil
	}
}

// DefaultStateDir is wick's state directory: the runs, history and worker
// stores the Snapshot read model reads. WICK_STATE_DIR points it elsewhere,
// which is what scripts and tests use.
func DefaultStateDir() string {
	if dir := os.Getenv("WICK_STATE_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(os.Getenv("HOME"), ".pig", "state", "wick")
}

// CurrentRepo resolves the repo the dashboard runs in: the git top level, or
// the working directory outside a checkout. A non-repo still lists every run
// record's repo; only the current-first ordering falls back to the cwd. The
// git call is scoped with -C so the runner needs no working directory.
func CurrentRepo(run CommandRunner, cwd string) string {
	raw, err := run("git", "-C", cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return cwd
	}
	if top := strings.TrimSpace(string(raw)); top != "" {
		return top
	}
	return cwd
}
