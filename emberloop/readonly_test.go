package el

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// recorder is an Exec that captures calls and answers from a script.
type recorder struct {
	calls [][]string
	reply func(cmd string, args []string) Result
}

func (r *recorder) run(cmd string, args []string, _ ExecOpts) (Result, error) {
	r.calls = append(r.calls, append([]string{cmd}, args...))
	return r.reply(cmd, args), nil
}

// fakeHerdr answers the herdr calls ReadOnly makes. On pane run it plays the
// agent: it reads the launch file the way `el exec` would and writes a
// pig transcript, at the dir and id the launch names, whose last message is
// the report.
func fakeHerdr(cmd string, args []string, report string, git Result) Result {
	if cmd == "git" {
		return git
	}
	switch args[1] {
	case "list":
		return Result{Stdout: `{"result":{"panes":[{"pane_id":"p1","tab_id":"` + os.Getenv("HERDR_TAB_ID") + `"}]}}`}
	case "split":
		return Result{Stdout: `{"result":{"pane":{"pane_id":"p9"}}}`}
	case "run":
		cmd := launchCmd(args[3])
		var dir, id string
		for i, a := range cmd.Args {
			if a == "--session-dir" && i+1 < len(cmd.Args) {
				dir = cmd.Args[i+1]
			}
			if a == "--session-id" && i+1 < len(cmd.Args) {
				id = cmd.Args[i+1]
			}
		}
		_ = os.MkdirAll(filepath.Join(dir, "--x--"), 0o755)
		rec, _ := json.Marshal(map[string]any{"type": "message", "message": map[string]any{
			"role": "assistant", "stopReason": "stop", "content": []any{map[string]any{"type": "text", "text": report}}}})
		mid := `{"type":"message","message":{"role":"assistant","stopReason":"toolUse","content":[{"type":"text","text":"working"}]}}` + "\n"
		_ = os.WriteFile(filepath.Join(dir, "--x--", "t_"+id+".jsonl"), []byte(mid+string(rec)+"\n"), 0o644)
	}
	return Result{Stdout: "{}"}
}

func TestReviewRoleServesOnlyLooking(t *testing.T) {
	hermeticSettings(t)
	served, _ := RoleTools("review")
	for _, tl := range served {
		// execute is the sandboxed script runner: read-only through the
		// sandbox, with grep, git and check reachable only inside it.
		if n := tl.Name(); n != "read" && n != "find" && n != "websearch" && n != "ctx_search" && n != "check" && n != "execute" {
			t.Errorf("review role serves %s", n)
		}
	}
}

func TestLastReplyIgnoresMidTurnMessages(t *testing.T) {
	dir := t.TempDir()
	pig := filepath.Join(dir, "p.jsonl")
	os.WriteFile(pig, []byte(`{"type":"message","message":{"role":"assistant","stopReason":"toolUse","content":[{"type":"text","text":"mid"}]}}`+"\n"), 0o644)
	if _, ok := (pigHarness{}).LastReply(pig); ok {
		t.Error("pig: a tool-use message is not a final reply")
	}
	cl := filepath.Join(dir, "c.jsonl")
	os.WriteFile(cl, []byte(`{"type":"assistant","message":{"stop_reason":"tool_use","content":[{"type":"text","text":"mid"}]}}`+"\n"+
		`{"type":"assistant","message":{"stop_reason":"end_turn","content":[{"type":"thinking","thinking":"x"},{"type":"text","text":"final"}]}}`+"\n"), 0o644)
	if got, ok := (claudeHarness{}).LastReply(cl); !ok || got != "final" {
		t.Errorf("claude reply = %q %v", got, ok)
	}
}

func TestReadOnlySurfacesFailure(t *testing.T) {
	statePath = filepath.Join(t.TempDir(), "workers.json")
	rec := &recorder{reply: func(string, []string) Result { return Result{Code: 1, Stderr: "no herdr\n"} }}
	if _, err := ReadOnly(rec.run, "", "", "q"); err == nil || !strings.Contains(err.Error(), "no herdr") {
		t.Errorf("err = %v", err)
	}
}

// A read-only agent opens beside the caller, through the grid: pane 2 of the
// caller's tab, split right, not a --current split downward.
func TestReadOnlyPlacesTheAgentInTheCallersTabGrid(t *testing.T) {
	statePath = filepath.Join(t.TempDir(), "workers.json")
	g := newFakeGrid()
	tab := g.tabs[0]
	t.Setenv("HERDR_TAB_ID", tab.tab)
	rec := &recorder{reply: func(cmd string, args []string) Result {
		if cmd == herdrBin() && len(args) > 1 && args[0] == "pane" && args[1] == "run" {
			return fakeHerdr(cmd, args, "no defects found", Result{})
		}
		res, _ := g.run(cmd, args, ExecOpts{})
		return res
	}}

	got, err := ReadOnly(rec.run, "/repo", "", "look at this")
	if err != nil {
		t.Fatal(err)
	}
	if got != "no defects found" {
		t.Errorf("verdict = %q", got)
	}
	split := ""
	for _, c := range g.calls {
		if strings.Contains(c, "pane split") {
			split = c
			break
		}
	}
	if split == "" {
		t.Fatal("the launch issued no pane split")
	}
	if !strings.Contains(split, "--pane "+tab.order[0]) {
		t.Errorf("the split must target the caller's pane: %s", split)
	}
	if !strings.Contains(split, "--direction right") {
		t.Errorf("a read-only agent must sit beside the caller, not below: %s", split)
	}
	if strings.Contains(split, "--current") {
		t.Errorf("read-only launches no longer split --current: %s", split)
	}
}

// Without HERDR_TAB_ID, the caller's own pane names the tab, so a session that
// knows only its pane still opens the agent beside it rather than in the
// focused pane.
func TestReadOnlyFindsTheCallersTabFromItsPane(t *testing.T) {
	statePath = filepath.Join(t.TempDir(), "workers.json")
	g := newFakeGrid()
	caller := g.tabs[0].order[0]
	t.Setenv("HERDR_TAB_ID", "")
	t.Setenv("HERDR_PANE_ID", caller)
	rec := &recorder{reply: func(cmd string, args []string) Result {
		if cmd == herdrBin() && len(args) > 1 && args[0] == "pane" && args[1] == "run" {
			return fakeHerdr(cmd, args, "no defects found", Result{})
		}
		res, _ := g.run(cmd, args, ExecOpts{})
		return res
	}}

	if _, err := ReadOnly(rec.run, "/repo", "", "look"); err != nil {
		t.Fatal(err)
	}
	split := ""
	for _, c := range g.calls {
		if strings.Contains(c, "pane split") {
			split = c
			break
		}
	}
	if !strings.Contains(split, "--pane "+caller) {
		t.Errorf("the split must target the caller's pane named by HERDR_PANE_ID: %s", split)
	}
}

func TestSkillCommandsPointAtTheSkillFile(t *testing.T) {
	got := skillPrompt("grilling", "the auth plan", true)
	for _, want := range []string{"el://matt-pocock/grilling/SKILL.md", "Input: the auth plan", "scout", "spawn"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(skillPrompt("prototype", "", false), "Input:") {
		t.Error("empty args must not add an Input line")
	}
}

// Every session is pointed at the el:// address the read tool serves, since
// no harness loads the library itself any more.
func TestSkillCommandNamesNoAbsolutePath(t *testing.T) {
	t.Setenv("EL_SKILLS_DIR", "/skills")
	got := skillPrompt("grilling", "", true)
	if !strings.Contains(got, "el://matt-pocock/grilling/SKILL.md") {
		t.Errorf("a skill command must name the el:// address, got:\n%s", got)
	}
	if strings.Contains(got, "/skills/matt-pocock") {
		t.Errorf("a skill command must name no absolute library path:\n%s", got)
	}
}

// read opens the prompt and diff by absolute path on its own, so a reviewer
// launch must not widen read's access any further.
func TestReadOnlyDoesNotWidenReadAccess(t *testing.T) {
	statePath = filepath.Join(t.TempDir(), "workers.json")
	rec := &recorder{reply: func(cmd string, args []string) Result {
		return fakeHerdr(cmd, args, "no defects found", Result{})
	}}
	if _, err := ReadOnly(rec.run, t.TempDir(), "", "look"); err != nil {
		t.Fatal(err)
	}
	for _, call := range rec.calls {
		if strings.Contains(strings.Join(call, " "), "EL_READ_ALLOW") {
			t.Fatalf("reviewer launch still sets EL_READ_ALLOW: %v", call)
		}
	}
}

// A read-only agent on a non-pig harness keeps the model it was given: a model
// set for a read-only slot is meant for that harness, so the rule that dropped
// it for a non-pig harness is gone.
func TestReadOnlyKeepsTheModelOnANonPigHarness(t *testing.T) {
	statePath = filepath.Join(t.TempDir(), "workers.json")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("EL_READONLY_HARNESS", "claude")

	rec := &recorder{reply: claudeHerdr(home, "looks fine")}
	if _, err := ReadOnly(rec.run, "/repo", "claude-sonnet", "look"); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range rec.calls {
		joined := strings.Join(c, " ")
		if !strings.Contains(joined, "el exec ") {
			continue
		}
		if flagIn(joined[strings.Index(joined, "el exec "):], "--model") == "claude-sonnet" {
			found = true
		}
	}
	if !found {
		t.Errorf("a claude read-only agent must keep the model it was given; calls:\n%v", rec.calls)
	}
}

// concExec is a fake herdr that records whether read-only launches overlap.
// The split handler sleeps, so an unserialized split shows up as two at once.
// Its pane list grows with each split, the way the grid expects to find the
// panes it has already placed.
type concExec struct {
	mu           sync.Mutex
	panes        []string
	next         int
	splitActive  int
	maxSplit     int
	inFlight     int
	maxInFlight  int
	launches     int
	pending      [][2]string
	bothLaunched chan struct{}
}

func (c *concExec) run(cmd string, args []string, _ ExecOpts) (Result, error) {
	if cmd != herdrBin() || len(args) < 2 {
		return Result{}, nil
	}
	switch args[1] {
	case "list":
		tab := os.Getenv("HERDR_TAB_ID")
		c.mu.Lock()
		defer c.mu.Unlock()
		panes := []any{}
		for _, id := range c.panes {
			panes = append(panes, map[string]any{"pane_id": id, "tab_id": tab})
		}
		return jsonReply(map[string]any{"panes": panes}), nil
	case "layout":
		return Result{Stdout: `{"result":{"layout":{"area":{"width":100,"height":100},"panes":[],"splits":[]}}}`}, nil
	case "split":
		c.mu.Lock()
		c.splitActive++
		if c.splitActive > c.maxSplit {
			c.maxSplit = c.splitActive
		}
		c.mu.Unlock()
		time.Sleep(30 * time.Millisecond)
		c.mu.Lock()
		c.splitActive--
		c.inFlight++
		if c.inFlight > c.maxInFlight {
			c.maxInFlight = c.inFlight
		}
		c.next++
		pane := fmt.Sprintf("p%d", c.next)
		c.panes = append(c.panes, pane)
		c.mu.Unlock()
		return Result{Stdout: `{"result":{"pane":{"pane_id":"` + pane + `"}}}`}, nil
	case "run":
		cmd := launchCmd(args[3])
		var dir, id string
		for i, a := range cmd.Args {
			if a == "--session-dir" && i+1 < len(cmd.Args) {
				dir = cmd.Args[i+1]
			}
			if a == "--session-id" && i+1 < len(cmd.Args) {
				id = cmd.Args[i+1]
			}
		}
		c.mu.Lock()
		c.pending = append(c.pending, [2]string{dir, id})
		c.launches++
		if c.launches == 2 {
			close(c.bothLaunched)
		}
		c.mu.Unlock()
	case "close":
		c.mu.Lock()
		c.inFlight--
		c.mu.Unlock()
	}
	return Result{Stdout: "{}"}, nil
}

// replyAll writes each launched session's final transcript, the event both
// waits are blocked on.
func (c *concExec) replyAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, p := range c.pending {
		_ = os.MkdirAll(filepath.Join(p[0], "--x--"), 0o755)
		rec, _ := json.Marshal(map[string]any{"type": "message", "message": map[string]any{
			"role": "assistant", "stopReason": "stop", "content": []any{map[string]any{"type": "text", "text": "fact"}}}})
		_ = os.WriteFile(filepath.Join(p[0], "--x--", "t_"+p[1]+".jsonl"), append(rec, '\n'), 0o644)
	}
}

// Two read-only launches at once must not race on the herdr layout: the split
// is serialized. Everything after it, the pane run and the wait for the reply,
// stays concurrent, so both scouts get going before either answers.
func TestConcurrentReadOnlyLaunchesSerializeOnlyTheSplit(t *testing.T) {
	statePath = filepath.Join(t.TempDir(), "workers.json")
	oldPoll, oldTimeout := pollEvery, readOnlyTimeout
	pollEvery, readOnlyTimeout = 5*time.Millisecond, 5*time.Second
	t.Cleanup(func() { pollEvery, readOnlyTimeout = oldPoll, oldTimeout })

	ex := &concExec{panes: []string{"p1"}, next: 1, bothLaunched: make(chan struct{})}
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := ReadOnly(ex.run, t.TempDir(), "", "q"); err != nil {
				t.Errorf("read-only launch: %v", err)
			}
		}()
	}

	select {
	case <-ex.bothLaunched:
	case <-time.After(3 * time.Second):
		t.Fatal("two launches never reached their wait together: the lock covers more than the split")
	}
	// Both are waiting now; the replies release them.
	ex.replyAll()
	wg.Wait()

	if ex.maxSplit != 1 {
		t.Errorf("split calls overlapped (%d at once): the layout can race", ex.maxSplit)
	}
	if ex.maxInFlight < 2 {
		t.Errorf("only %d launch(es) in flight at once: the waits did not overlap", ex.maxInFlight)
	}
}

// Ticket 18: skill prompts (the plan role's prompt) carry one positive
// execute direction: search and process content with execute.
func TestSkillPromptDirectsExecute(t *testing.T) {
	const line = "Search and process content with `execute`"
	for _, scout := range []bool{true, false} {
		if got := strings.Count(skillPrompt("research", "the auth question", scout), line); got != 1 {
			t.Errorf("skillPrompt(scout=%v) must carry the execute direction exactly once, found %d", scout, got)
		}
	}
}
