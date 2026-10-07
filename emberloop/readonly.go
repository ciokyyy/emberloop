package emberloop

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ToolBudget is the longest any tool may block. A harness cuts an MCP call at
// its own timeout, and a tool that outlives it fails in the caller while the
// work goes on behind it. pig's default is 60s; mcp.json raises it per server
// with "timeout" (seconds), and emberloop's entry sets 600, so this stays well under.
const ToolBudget = 4 * time.Minute

// toolBudget is ToolBudget in use, a var only so a test can shrink it.
var toolBudget = ToolBudget

// readOnlyTimeout bounds one reviewer or scout run.
var readOnlyTimeout = 10 * time.Minute

var pollEvery = time.Second

func pigBin() string {
	if p := os.Getenv("EMBERLOOP_PIG"); p != "" {
		return p
	}
	return "pig"
}

// callerTab is the tab a read-only agent opens in: the tab emberloop itself runs
// in. herdr injects HERDR_TAB_ID; without it, the caller's own pane names the
// tab, so a session that knows only its pane still places the agent beside it
// rather than in whichever pane is focused.
func callerTab(run Exec) string {
	if tab := os.Getenv("HERDR_TAB_ID"); tab != "" {
		return tab
	}
	pane := os.Getenv("HERDR_PANE_ID")
	if pane == "" {
		return ""
	}
	reply, err := herdrJSON(run, []string{"pane", "list"}, 15*time.Second)
	if err != nil {
		return ""
	}
	for _, p := range reply.Result.Panes {
		if p.PaneID == pane {
			return p.TabID
		}
	}
	return ""
}

// ReadOnly runs one prompt in a read-only agent (role "review": read, find,
// execute, ctx_search) in its own herdr pane, so the human can watch it, and
// returns the agent's last message, read from its session transcript. The
// pane closes with the answer.
func ReadOnly(run Exec, cwd, model, prompt string) (string, error) {
	agent, err := startReadOnly(run, readOnlyPlace{Overflow: "scouts"}, cwd, model, prompt)
	if err != nil {
		return "", err
	}
	defer closeAgent(run, agent)
	return agent.reply(run, 0)
}

// readOnlyPlace is where a read-only agent opens. An empty Tab is the caller's
// own tab, which is where a scout goes. A Slot of 2 or more takes the slot a
// dead pane left instead of appending, so a replaced reviewer keeps the grid's
// shape; Overflow names the first tab a full grid spills into.
type readOnlyPlace struct {
	Tab      string
	Overflow string
	Slot     int
}

// readOnlyAgent is a launched read-only agent: the pane it runs in and the
// session its transcript is written under.
type readOnlyAgent struct {
	Pane    string
	Tab     string
	Slot    int
	Harness string
	Session Session
}

// closeAgent closes an agent's pane under its tab's grid lock, so a close
// never races a concurrent split of that tab, even across sessions. A lock
// setup failure never holds the close hostage: the pane is closed unlocked
// rather than leaked.
func closeAgent(run Exec, agent readOnlyAgent) {
	if err := withGridLock(agent.Tab, func() error {
		_, _ = run(herdrBin(), []string{"pane", "close", agent.Pane}, ExecOpts{Timeout: 15 * time.Second})
		return nil
	}); err != nil {
		_, _ = run(herdrBin(), []string{"pane", "close", agent.Pane}, ExecOpts{Timeout: 15 * time.Second})
	}
}

// closeDeliveredScout closes a scout's pane after its result reached the
// parent, and cleans up its launch file. Workers own worktrees (finish
// drops them) and reviewers stay open for follow-ups; only scouts close
// here. A missing pane is the wanted end state, not a failure, but the
// launch file is still removed so a vanished pane leaves nothing behind.
func closeDeliveredScout(run Exec, agentID string) {
	st, ok, err := ReadAgentState(agentID)
	if err != nil || !ok || !isScoutState(st) {
		return
	}
	if st.Pane != "" {
		closeAgent(run, readOnlyAgent{Pane: st.Pane, Tab: st.Tab})
	}
	CleanupSession(Session{ID: agentID})
}

// readOnlyDir holds read-only agents' prompts and sessions.
func readOnlyDir() string { return filepath.Join(filepath.Dir(statePath), "readonly") }

// readOnlySession is the session a read-only agent with this id writes, so a
// reviewer that outlives one tool call can be found again by its id. The agent
// id is the session id, so every reviewer and scout carries one.
func readOnlySession(id, cwd, model, effort string) Session {
	return Session{Role: "review", Cwd: cwd, Model: model, Effort: effort, ID: id, AgentID: id, Dir: filepath.Join(readOnlyDir(), "sessions"),
		// A read-only agent inside a run archives under it: the calling
		// session carries its spec in the environment.
		Spec: os.Getenv("EMBERLOOP_SPEC")}
}

// startReadOnly launches a read-only reviewer on prompt in a pane placed by
// the grid, and leaves it running: the caller decides when the pane closes.
func startReadOnly(run Exec, place readOnlyPlace, cwd, model, prompt string) (readOnlyAgent, error) {
	harness := os.Getenv("EMBERLOOP_READONLY_HARNESS")
	if harness == "" {
		harness = "pig"
	}
	sess := readOnlySession(newSessionID(), cwd, model, "")
	sess.Prompt = "Your last message is your answer."
	return launchPlacedAgent(run, place, harness, sess, prompt)
}

// launchPlacedAgent writes prompt to a file for the archive, launches the
// session with the full prompt as its first message in a pane placed by the
// grid, and leaves it running. The pane gets one shell-agnostic line,
// `emberloop exec <launch-file>`, which carries the prompt as a real argument
// instead of a "read this file" indirection. place names the tab and the
// overflow label; an empty Tab is the caller's own tab. beforeRun, when given,
// runs once the pane is placed but before it starts, so a reviewer can record
// its state ahead of the submit that state exists to catch.
func launchPlacedAgent(run Exec, place readOnlyPlace, harness string, sess Session, prompt string, beforeRun ...func(readOnlyAgent) error) (readOnlyAgent, error) {
	if err := os.MkdirAll(readOnlyDir(), 0o755); err != nil {
		return readOnlyAgent{}, err
	}
	promptPath := filepath.Join(readOnlyDir(), sess.ID+".prompt")
	if err := os.WriteFile(promptPath, []byte(prompt), 0o644); err != nil {
		return readOnlyAgent{}, err
	}
	if suffix := strings.TrimSpace(sess.Prompt); suffix != "" {
		sess.Prompt = strings.TrimSpace(prompt) + "\n\n" + suffix
	} else {
		sess.Prompt = prompt
	}
	launch, err := PrepareLaunch(harness, sess)
	if err != nil {
		return readOnlyAgent{}, err
	}
	// The agent reads its MCP config at startup, so the file is dead weight
	// once it is running. The launch file stays: `emberloop exec` in the pane
	// consumes it when the agent starts.
	defer cleanupMCP(sess)
	// A scout shares the caller's tab; a reviewer shares its ticket's tab, so
	// it sits beside the implementor. Either way the grid serializes the
	// splits in one tab and evens its sizes out, best effort; the agent then
	// runs and is waited on concurrently.
	tab := place.Tab
	if tab == "" {
		tab = callerTab(run)
	}
	overflow := place.Overflow
	if overflow == "" {
		overflow = "scouts"
	}
	var placed gridPane
	if place.Slot >= 2 {
		placed, err = replaceInGrid(run, tab, place.Slot, sess.Cwd)
	}
	if place.Slot < 2 || err != nil {
		// The dead pane's slot needs a pane before it or a row above it to
		// split. When that is gone too, a replacement appends to the grid
		// instead of failing the launch.
		placed, err = placeInGrid(run, tab, overflow, sess.Cwd)
	}
	if err != nil {
		CleanupSession(sess)
		return readOnlyAgent{}, err
	}
	pane := placed.Pane
	if pane == "" {
		CleanupSession(sess)
		return readOnlyAgent{}, fmt.Errorf("herdr split returned no pane id")
	}
	agent := readOnlyAgent{Pane: pane, Tab: placed.Tab, Slot: placed.Slot, Harness: harness, Session: sess}
	fail := func(err error) (readOnlyAgent, error) {
		CleanupSession(sess)
		closeAgent(run, agent)
		return readOnlyAgent{}, err
	}
	if len(beforeRun) > 0 && beforeRun[0] != nil {
		if err := beforeRun[0](agent); err != nil {
			return fail(err)
		}
	}

	if res, err := run(herdrBin(), []string{"pane", "run", pane, launch.Line}, ExecOpts{Timeout: 15 * time.Second}); err != nil || res.Code != 0 {
		return fail(fmt.Errorf("herdr pane run failed: %v %s", err, firstLine(res.Stderr, res.Stdout)))
	}
	// The pane carries the agent type's readable name; a session with no
	// type (a one-shot read-only run) names its pane scout. A rename that
	// fails is silent: the name is cosmetic and never fails the launch.
	renamePane(run, pane, placedPaneName(sess))
	if err := DeliverLate(run, pane, launch.Late); err != nil {
		return fail(err)
	}
	return agent, nil
}

// currentSessionID is the harness session the agent's transcript is
// written under now: the current id its agent state records, so a restart
// moves every lookup to the new transcript. No state (an agent that never
// restarted through emberloop) keeps the launch id.
func (a readOnlyAgent) currentSessionID() string {
	if a.Session.AgentID != "" {
		if s, ok, err := ReadAgentState(a.Session.AgentID); err == nil && ok && s.SessionID != "" {
			return s.SessionID
		}
	}
	return a.Session.ID
}

// reply waits for the agent's next finished turn after the transcript was
// after bytes long (0: its first turn) and returns that turn's last message.
func (a readOnlyAgent) reply(run Exec, after int64) (string, error) {
	sess := a.Session
	sess.ID = a.currentSessionID()
	return WaitReply(registry[a.Harness], sess, after, readOnlyTimeout,
		func() error { return Nudge(run, a.Pane, "continue") })
}

// transcriptSize is how long the agent's transcript is now; a reply to a
// message sent after this point is one written past it.
func (a readOnlyAgent) transcriptSize() int64 {
	p := findSession(registry[a.Harness].SessionRoot(a.Session), a.currentSessionID())
	if p == "" {
		return 0
	}
	info, err := os.Stat(p)
	if err != nil {
		return 0
	}
	return info.Size()
}

// tell sends the agent a follow-up and waits for its reply to it.
func (a readOnlyAgent) tell(run Exec, text string) (string, error) {
	size := a.transcriptSize()
	if err := Nudge(run, a.Pane, text); err != nil {
		return "", err
	}
	return a.reply(run, size)
}

// alive reports whether a process beyond the shell runs in the agent's
// pane, judged from herdr's per-pane process list the way the stop gate
// judges it. A bare shell, an unknown pane and a missing pane are all dead:
// pane existence alone never counts as alive. herdr's own agent detection
// lags and flaps, so the foreground process is the signal, not its name.
func (a readOnlyAgent) alive(run Exec) bool {
	if a.Pane == "" {
		return false
	}
	proc, err := paneProcesses(run, a.Pane)
	if err != nil {
		return false
	}
	return !proc.stopped()
}
