package wick

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ToolBudget is the longest any tool may block. A harness cuts an MCP call at
// its own timeout, and a tool that outlives it fails in the caller while the
// work goes on behind it. pig's default is 60s; mcp.json raises it per server
// with "timeout" (seconds), and wick's entry sets 600, so this stays well under.
const ToolBudget = 4 * time.Minute

// toolBudget is ToolBudget in use, a var only so a test can shrink it.
var toolBudget = ToolBudget

type jobResult struct {
	text string
	err  error
}

var (
	jobsMu sync.Mutex
	jobs   = map[string]chan jobResult{}
)

// readOnlyOnce runs ReadOnly at most once per key; see once.
func readOnlyOnce(run Exec, key, cwd, model, prompt string) (string, error) {
	return once(key, func() (string, error) { return ReadOnly(run, cwd, model, prompt) })
}

// once runs fn at most once per key. A caller waits up to toolBudget; if fn is
// still going it gets "still running" and fn carries on, and the next call
// with the same key rejoins it.
func once(key string, fn func() (string, error)) (string, error) {
	jobsMu.Lock()
	ch, running := jobs[key]
	if !running {
		ch = make(chan jobResult, 1)
		jobs[key] = ch
		go func() {
			text, err := fn()
			ch <- jobResult{text, err}
		}()
	}
	jobsMu.Unlock()

	select {
	case r := <-ch:
		jobsMu.Lock()
		delete(jobs, key)
		jobsMu.Unlock()
		return r.text, r.err
	case <-time.After(toolBudget):
		return fmt.Sprintf("still running: %s. Call the same tool again with the same arguments to get its answer; do not start anything else.", key), nil
	}
}

func keyOf(kind, text string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(text))
	return fmt.Sprintf("%s-%08x", kind, h.Sum32())
}

// readOnlyTimeout bounds one reviewer or scout run.
var readOnlyTimeout = 10 * time.Minute

var pollEvery = time.Second

func pigBin() string {
	if p := os.Getenv("WICK_PIG"); p != "" {
		return p
	}
	return "pig"
}

// callerTab is the tab a read-only agent opens in: the tab wick itself runs
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

// ReadOnly runs one prompt in a read-only agent (role "review": read and
// find) in its own herdr pane, so the human can watch it, and returns the
// agent's last message, read from its session transcript. The pane closes
// with the answer.
func ReadOnly(run Exec, cwd, model, prompt string) (string, error) {
	agent, err := startReadOnly(run, cwd, model, prompt)
	if err != nil {
		return "", err
	}
	defer closeAgent(run, agent)
	return agent.reply(run, 0)
}

// readOnlyAgent is a launched read-only agent: the pane it runs in and the
// session its transcript is written under.
type readOnlyAgent struct {
	Pane    string
	Tab     string
	Harness string
	Session Session
}

// closeAgent closes an agent's pane under its tab's grid lock, so a close
// never races a concurrent split of that tab.
func closeAgent(run Exec, agent readOnlyAgent) {
	lock := gridLock(agent.Tab)
	lock.Lock()
	defer lock.Unlock()
	_, _ = run(herdrBin(), []string{"pane", "close", agent.Pane}, ExecOpts{Timeout: 15 * time.Second})
}

// readOnlyDir holds read-only agents' prompts and sessions.
func readOnlyDir() string { return filepath.Join(filepath.Dir(statePath), "readonly") }

// readOnlySession is the session a read-only agent with this id writes, so a
// reviewer that outlives one tool call can be found again by its id.
func readOnlySession(id, cwd, model string) Session {
	return Session{Role: "review", Cwd: cwd, Model: model, ID: id, Dir: filepath.Join(readOnlyDir(), "sessions")}
}

// startReadOnly launches a read-only agent on prompt in a pane of the caller's
// tab, placed by the grid, and leaves it running: the caller decides when the
// pane closes. The prompt goes through a file: pane run types its argv as one
// line. WICK_READONLY_HARNESS picks the harness (default pig); a model goes
// only to pig, since worker models are pig model names.
func startReadOnly(run Exec, cwd, model, prompt string) (readOnlyAgent, error) {
	harness := os.Getenv("WICK_READONLY_HARNESS")
	if harness == "" {
		harness = "pig"
	}
	if harness != "pig" {
		model = ""
	}
	if err := os.MkdirAll(readOnlyDir(), 0o755); err != nil {
		return readOnlyAgent{}, err
	}
	sess := readOnlySession(newSessionID(), cwd, model)
	promptPath := filepath.Join(readOnlyDir(), sess.ID+".prompt")
	if err := os.WriteFile(promptPath, []byte(prompt), 0o644); err != nil {
		return readOnlyAgent{}, err
	}
	sess.Prompt = fmt.Sprintf("Read %s and follow it exactly. Your last message is your answer.", promptPath)
	launch, err := PrepareLaunch(harness, sess)
	if err != nil {
		return readOnlyAgent{}, err
	}
	// The agent is done with the MCP config the moment it has launched.
	defer CleanupSession(sess)
	// Read-only agents share the caller's tab, side by side in the grid; the
	// ninth caller overflow opens a `scouts` tab. The grid serializes the
	// splits in one tab and evens its sizes out, best effort; the agent then
	// runs and is waited on concurrently.
	placed, err := placeInGrid(run, callerTab(run), "scouts", cwd)
	if err != nil {
		return readOnlyAgent{}, err
	}
	pane := placed.Pane
	if pane == "" {
		return readOnlyAgent{}, fmt.Errorf("herdr split returned no pane id")
	}
	agent := readOnlyAgent{Pane: pane, Tab: placed.Tab, Harness: harness, Session: sess}
	fail := func(err error) (readOnlyAgent, error) {
		closeAgent(run, agent)
		return readOnlyAgent{}, err
	}

	if res, err := run(herdrBin(), []string{"pane", "run", pane, launch.Line}, ExecOpts{Timeout: 15 * time.Second}); err != nil || res.Code != 0 {
		return fail(fmt.Errorf("herdr pane run failed: %v %s", err, firstLine(res.Stderr, res.Stdout)))
	}
	if err := DeliverLate(run, pane, launch.Late); err != nil {
		return fail(err)
	}
	return agent, nil
}

// reply waits for the agent's next finished turn after the transcript was
// after bytes long (0: its first turn) and returns that turn's last message.
func (a readOnlyAgent) reply(run Exec, after int64) (string, error) {
	return WaitReply(registry[a.Harness], a.Session, after, readOnlyTimeout,
		func() error { return Nudge(run, a.Pane, "continue") })
}

// transcriptSize is how long the agent's transcript is now; a reply to a
// message sent after this point is one written past it.
func (a readOnlyAgent) transcriptSize() int64 {
	p := findSession(registry[a.Harness].SessionRoot(a.Session), a.Session.ID)
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

// alive reports whether the agent's pane still exists.
func (a readOnlyAgent) alive(run Exec) bool {
	if a.Pane == "" {
		return false
	}
	res, err := run(herdrBin(), []string{"pane", "get", a.Pane}, ExecOpts{Timeout: 15 * time.Second})
	return err == nil && res.Code == 0
}

func scoutPrompt(question string) string {
	return fmt.Sprintf(`Answer from this repository only: %s

You can only read and find. Cite file:line for every claim. If you cannot find it, say "not found" rather than guess. Report facts; do not recommend or decide.`, question)
}
