package tui

import (
	"encoding/json"
	"errors"
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

// paneEntry is one row of `herdr pane list`: the pane, the tab holding it,
// and its agent status ("" when the list carries none).
type paneEntry struct {
	paneID string
	tabID  string
	status string
}

// parsePaneList decodes a `herdr pane list` reply into its panes.
func parsePaneList(raw []byte) ([]paneEntry, error) {
	var reply struct {
		Result struct {
			Panes []struct {
				PaneID string `json:"pane_id"`
				TabID  string `json:"tab_id"`
				Status string `json:"agent_status"`
			} `json:"panes"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &reply); err != nil {
		return nil, fmt.Errorf("herdr pane list returned non-JSON: %.200s", strings.TrimSpace(string(raw)))
	}
	out := make([]paneEntry, 0, len(reply.Result.Panes))
	for _, p := range reply.Result.Panes {
		out = append(out, paneEntry{paneID: p.PaneID, tabID: p.TabID, status: p.Status})
	}
	return out, nil
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
		panes, err := parsePaneList(raw)
		if err != nil {
			return nil, err
		}
		out := map[string]string{}
		for _, p := range panes {
			out[p.paneID] = p.status
		}
		return out, nil
	}
}

// errPaneGone reports a jump whose pane herdr no longer lists: the agent
// is gone, so there is nothing to focus and the view shows a message.
var errPaneGone = errors.New("pane is gone")

// focusAgentPane lands the user in an agent's herdr tab and pane: it
// resolves the recorded pane to its tab through `herdr pane list`, then
// focuses the tab and the pane. The CLI's `pane focus` only steps to a
// neighbour by direction, so the pane leg goes through `agent focus`,
// which accepts a pane ID and focuses its tab and pane. Every call is
// focus-only: jumping never closes, moves or splits panes. A pane the
// list no longer names returns errPaneGone, before any focus call.
func focusAgentPane(run CommandRunner, pane string) error {
	if pane == "" {
		return errPaneGone
	}
	raw, err := run(herdrBin(), "pane", "list")
	if err != nil {
		return err
	}
	panes, err := parsePaneList(raw)
	if err != nil {
		return err
	}
	tab := ""
	for _, p := range panes {
		if p.paneID == pane {
			tab = p.tabID
		}
	}
	if tab == "" {
		return errPaneGone
	}
	if _, err := run(herdrBin(), "tab", "focus", tab); err != nil {
		return err
	}
	if _, err := run(herdrBin(), "agent", "focus", pane); err != nil {
		return err
	}
	return nil
}

// DefaultStateDir is el's state directory: the runs, history and worker
// stores the Snapshot read model reads. EL_STATE_DIR points it elsewhere,
// which is what scripts and tests use.
func DefaultStateDir() string {
	if dir := os.Getenv("EL_STATE_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(os.Getenv("HOME"), ".pig", "state", "emberloop")
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
