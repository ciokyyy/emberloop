// Package herdrreport teaches herdr that a PiG pane is an agent, and gives it a
// name it can be addressed by.
//
// herdr's agent kinds are compiled into its binary and contain no entry for
// pig, so `herdr agent list` shows nothing for a running pig. But detection is
// not the only path: an agent that reports its own state overrides
// screen-scraping entirely. This extension does that and nothing else — no
// tools, no schema cost — so a worker can load it without carrying the merger's
// delegation tools.
//
// What this buys, all of it verified against herdr 0.9.1:
//
//   - the pane appears in `herdr agent list` at all. Its agent kinds are
//     compiled into its binary with no entry for pig, so without a report a
//     running pig is just a pane.
//   - the state follows the session: idle at session start, working at agent
//     start, idle at agent settled, and a release at shutdown so no stale
//     entry outlives the session.
//   - EL_AGENT_LABEL separates agents in that list. It does NOT make them
//     addressable: `herdr agent get` and `herdr agent prompt` both answer
//     agent_not_found for the reported name, and passing the session identity
//     does not change it. Addressability comes only from `herdr agent rename`,
//     which el issues once the worker has registered. A worker is always
//     targeted by its recorded pane id.
package herdrreport

import (
	"os"
	"strconv"
	"sync/atomic"
	"time"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
)

const (
	source = "pig"
	// defaultLabel is the name a plain `pig` session reports under.
	defaultLabel = "pig"
)

func Extension() *sdk.Extension {
	e := sdk.New("herdr-report")
	name := label()

	// herdr orders reports by seq. Milliseconds since the epoch keep them
	// monotonic across a long session without a lock on the happy path.
	var seq atomic.Int64
	seq.Store(time.Now().UnixMilli() * 1000)

	var sessionID, sessionPath string
	capture := func(ctx sdk.Context) {
		if id, err := ctx.GetSessionID(); err == nil {
			sessionID = id
		}
		if file, err := ctx.GetSessionFile(); err == nil && file != nil {
			sessionPath = *file
		}
	}

	report := func(ctx sdk.Context, state string) {
		pane, ok := paneID()
		if !ok {
			return
		}
		// The session file does not exist until the first turn has run, so
		// session_start is too early to ask for it. Keep asking until it
		// answers: without it herdr knows the agent exists but not which
		// session it is, and cannot target the agent by name.
		if sessionPath == "" {
			capture(ctx)
		}
		// Best effort: a failed report must never disturb the session.
		_, _ = ctx.Exec(herdrBin(), reportArgs(pane, name, state, seq.Add(1), sessionID, sessionPath))
	}

	e.OnSessionStart(func(ctx sdk.Context, _ map[string]any) (any, error) {
		capture(ctx)
		report(ctx, "idle")
		return nil, nil
	})
	e.OnEvent(sdk.EventAgentStart, func(ctx sdk.Context, _ map[string]any) (any, error) {
		report(ctx, "working")
		return nil, nil
	})
	e.OnEvent(sdk.EventAgentSettled, func(ctx sdk.Context, _ map[string]any) (any, error) {
		report(ctx, "idle")
		return nil, nil
	})

	// Every lifecycle event, one JSON line each, so a stalled agent leaves a
	// trail. Registered after the session_start handler above, so the session
	// id is captured before the first write names the file.
	registerLog(e, newAgentLog(func() string { return logPath(name, sessionID) }))

	// The stop gate: the same decision claude asks through its Stop hook,
	// enforced here the way only a pig extension can — by continuing the agent
	// with the reminder, and by pinning the submit tool on the pinned attempt.
	gate := newGate(execGate)
	gate.register(e)

	// The register's per-turn reminder, as the caveman plugin sends it.
	registerVoice(e, execVoice)

	// Hand authority back so herdr does not keep treating this pane as an agent
	// after the session ends, instead of leaving a stale entry behind.
	e.OnSessionShutdown(func(ctx sdk.Context, _ map[string]any) (any, error) {
		pane, ok := paneID()
		if !ok {
			return nil, nil
		}
		_, _ = ctx.Exec(herdrBin(), releaseArgs(pane, name, seq.Add(1)))
		return nil, nil
	})

	return e
}

// label is the name herdr displays for this agent. It is not a lookup key:
// get and prompt reject it unless `herdr agent rename` has set one.
func label() string {
	if name := os.Getenv("EL_AGENT_LABEL"); name != "" {
		return name
	}
	return defaultLabel
}

func reportArgs(pane, name, state string, seq int64, sessionID, sessionPath string) []string {
	args := []string{
		"pane", "report-agent", pane,
		"--source", source,
		"--agent", name,
		"--state", state,
		"--seq", strconv.FormatInt(seq, 10),
	}
	if sessionID != "" {
		args = append(args, "--agent-session-id", sessionID)
	}
	if sessionPath != "" {
		args = append(args, "--agent-session-path", sessionPath)
	}
	return args
}

func releaseArgs(pane, name string, seq int64) []string {
	return []string{
		"pane", "release-agent", pane,
		"--source", source,
		"--agent", name,
		"--seq", strconv.FormatInt(seq, 10),
	}
}

// paneID is the gate herdr itself documents: HERDR_ENV=1 marks a pane herdr
// manages, and the pane id names it.
func paneID() (string, bool) {
	if os.Getenv("HERDR_ENV") != "1" {
		return "", false
	}
	pane := os.Getenv("HERDR_PANE_ID")
	return pane, pane != ""
}

func herdrBin() string {
	if bin := os.Getenv("HERDR_BIN_PATH"); bin != "" {
		return bin
	}
	return "herdr"
}
