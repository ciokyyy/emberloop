package emberloop

import (
	"context"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// herdrAgentSource names the wrapper as the reporter of a claude pane. It
// must not start with "herdr:"; the pig extension reports under its own
// source, and the two are never active for one pane.
const herdrAgentSource = "emberloop"

// agentReporter tells herdr that a wrapped child is an agent and what it is
// doing. A wrapped claude is no longer the pane's foreground process, so
// herdr's process detection never marks the pane an agent and the spawn's
// readiness wait fails. Pig reports itself through its extension, so only
// the claude harness is reported here. Every call is best effort: a herdr
// that is gone or refuses a report never disturbs the child.
type agentReporter struct {
	run       func(args []string)
	pane      string
	label     string
	sessionID string
	mu        sync.Mutex
	seq       int64
	last      string
	released  bool
	// ready turns true once the child shows its input prompt, or once
	// readyAfter has passed. Until then the pane reports working: herdr's
	// own wait for idle must not return while the child is still starting,
	// or the first prompt is typed into a screen that is not listening.
	ready      bool
	readyAfter time.Time
	tail       string
}

// claudeReadyAfter is the longest a claude child may take to show its
// prompt before the pane is reported idle anyway, so a UI change in claude
// slows a spawn down instead of hanging it.
const claudeReadyAfter = 25 * time.Second

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(\x07|\x1b\\)|\x1b[()][0-9A-B]`)

// readyMarks are the footer texts claude draws only once its input box is
// up, with and without a permission mode showing.
var readyMarks = []string{"?forshortcuts", "shift+tabtocycle", "esctointerrupt", "bypasspermissionson"}

// newAgentReporter builds the reporter for a wrapped child, or nil when
// none applies: not claude, or no herdr pane around the process.
func newAgentReporter(cfg WrapConfig) *agentReporter {
	if cfg.Source.Harness != "claude" {
		return nil
	}
	pane := cfg.PaneID
	if pane == "" && os.Getenv("HERDR_ENV") == "1" {
		pane = os.Getenv("HERDR_PANE_ID")
	}
	if pane == "" {
		return nil
	}
	run := cfg.Report
	if run == nil {
		run = runHerdr
	}
	// A typed agent reports under its type's readable name. A top-level
	// session stays "claude": its strip chip names its role, but herdr
	// recognizes the agent by the harness name.
	label := cfg.Source.AgentName
	if label == "" || cfg.Source.AgentType == "" {
		label = "claude"
	}
	return &agentReporter{
		run: run, pane: pane, label: label, sessionID: cfg.Source.SessionID,
		seq: time.Now().UnixMilli() * 1000, readyAfter: time.Now().Add(claudeReadyAfter),
	}
}

// observe watches the child's output for the input prompt. When it shows,
// or the wait limit passes, the pane is reported idle.
func (r *agentReporter) observe(out []byte) {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.ready || r.released {
		r.mu.Unlock()
		return
	}
	r.tail += ansiRe.ReplaceAllString(string(out), "")
	if len(r.tail) > 8192 {
		r.tail = r.tail[len(r.tail)-8192:]
	}
	ready := time.Now().After(r.readyAfter)
	// Claude spaces words with cursor moves, not blanks, so compare with all
	// whitespace removed.
	low := strings.Join(strings.Fields(strings.ToLower(r.tail)), "")
	for _, m := range readyMarks {
		if strings.Contains(low, m) {
			ready = true
		}
	}
	if ready {
		r.ready = true
		r.tail = ""
	}
	r.mu.Unlock()
	if ready {
		r.report(HeaderIdle)
	}
}

// report sends a state to herdr when it changed. The strip's working and
// compressing states both mean busy; anything else is idle.
func (r *agentReporter) report(state HeaderState) {
	if r == nil {
		return
	}
	herdrState := "idle"
	r.mu.Lock()
	if !r.ready && time.Now().After(r.readyAfter) {
		r.ready = true
	}
	if state == HeaderWorking || state == HeaderCompressing || !r.ready {
		herdrState = "working"
	}
	if r.released || r.last == herdrState {
		r.mu.Unlock()
		return
	}
	r.last = herdrState
	r.seq++
	args := []string{"pane", "report-agent", r.pane,
		"--source", herdrAgentSource, "--agent", r.label,
		"--state", herdrState, "--seq", strconv.FormatInt(r.seq, 10)}
	if r.sessionID != "" {
		args = append(args, "--agent-session-id", r.sessionID)
	}
	r.mu.Unlock()
	r.run(args)
}

// release hands authority back so herdr stops treating the pane as an
// agent after the child ends.
func (r *agentReporter) release() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.released {
		r.mu.Unlock()
		return
	}
	r.released = true
	r.seq++
	args := []string{"pane", "release-agent", r.pane,
		"--source", herdrAgentSource, "--agent", r.label,
		"--seq", strconv.FormatInt(r.seq, 10)}
	r.mu.Unlock()
	r.run(args)
}

// runHerdr runs one herdr command with a short limit and ignores the
// result.
func runHerdr(args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = exec.CommandContext(ctx, herdrBin(), args...).Run()
}
