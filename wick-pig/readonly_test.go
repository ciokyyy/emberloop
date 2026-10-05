package wick

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
// agent: it writes a pig transcript, at the dir and id the command line names,
// whose last message is the report.
func fakeHerdr(cmd string, args []string, report string, git Result) Result {
	if cmd == "git" {
		return git
	}
	switch args[1] {
	case "split":
		return Result{Stdout: `{"result":{"pane":{"pane_id":"p9"}}}`}
	case "run":
		w := strings.Fields(args[3])
		var dir, id string
		for i, a := range w {
			if a == "--session-dir" {
				dir = w[i+1]
			}
			if a == "--session-id" {
				id = w[i+1]
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
	served, _ := RoleTools("review")
	for _, tl := range served {
		if n := tl.Name(); n != "read" && n != "find" {
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

func TestReviewHandsTheDiffToAReadOnlyAgent(t *testing.T) {
	statePath = filepath.Join(t.TempDir(), "workers.json")
	if err := save(map[string]Worker{"01-x": {
		ID: "01-x", Repo: "/repo", Base: "main", Branch: "implement/01-x", Worktree: "/repo/wt", Model: "m1",
	}}); err != nil {
		t.Fatal(err)
	}

	rec := &recorder{reply: func(cmd string, args []string) Result {
		return fakeHerdr(cmd, args, "f:1 major: bug\n", Result{Stdout: "diff --git a/f b/f\n+bug\n"})
	}}
	got, err := Review(rec.run, "01-x")
	if err != nil {
		t.Fatal(err)
	}
	if got != "f:1 major: bug" {
		t.Errorf("verdict = %q", got)
	}
	if strings.Join(rec.calls[0], " ") != "git diff main...implement/01-x" {
		t.Errorf("diff call = %v", rec.calls[0])
	}
	pig := strings.Join(rec.calls[2], " ")
	if !strings.Contains(pig, "WICK_ROLE=review") || !strings.Contains(pig, "--model m1") {
		t.Errorf("reviewer launch = %s", pig)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(statePath), "reviews", "01-x.diff")); err != nil {
		t.Errorf("diff file not written: %v", err)
	}
}

func TestReviewOfAnEmptyBranchSkipsTheModel(t *testing.T) {
	statePath = filepath.Join(t.TempDir(), "workers.json")
	_ = save(map[string]Worker{"01-x": {ID: "01-x", Base: "main", Branch: "b"}})
	rec := &recorder{reply: func(string, []string) Result { return Result{} }}
	if _, err := Review(rec.run, "01-x"); err != nil {
		t.Fatal(err)
	}
	if len(rec.calls) != 1 {
		t.Errorf("only git should run for an empty diff, got %v", rec.calls)
	}
}

func TestReadOnlySurfacesFailure(t *testing.T) {
	statePath = filepath.Join(t.TempDir(), "workers.json")
	rec := &recorder{reply: func(string, []string) Result { return Result{Code: 1, Stderr: "no herdr\n"} }}
	if _, err := ReadOnly(rec.run, "", "", "q"); err == nil || !strings.Contains(err.Error(), "no herdr") {
		t.Errorf("err = %v", err)
	}
}

// ReadOnly tiles the tab it split into, which it only knows from the tab id
// herdr puts in the split reply.
func TestReadOnlyTilesTheTabItSplitInto(t *testing.T) {
	statePath = filepath.Join(t.TempDir(), "workers.json")
	tab := newFakeTab("down", []float64{0.3})
	rec := &recorder{reply: func(cmd string, args []string) Result {
		if cmd == herdrBin() {
			switch args[1] {
			case "split":
				return Result{Stdout: `{"result":{"pane":{"pane_id":"p1","tab_id":"` + tab.tab + `"}}}`}
			case "list", "layout", "resize":
				res, _ := tab.run(cmd, args, ExecOpts{})
				return res
			}
		}
		return fakeHerdr(cmd, args, "no defects found", Result{})
	}}

	got, err := ReadOnly(rec.run, t.TempDir(), "", "look at this")
	if err != nil {
		t.Fatal(err)
	}
	if got != "no defects found" {
		t.Errorf("verdict = %q", got)
	}
	if strings.Join(tab.calls, ", ") != "p0 down 0.2000" {
		t.Errorf("resize calls = %v, want the tab evened out", tab.calls)
	}
}

func TestSkillCommandsPointAtTheSkillFile(t *testing.T) {
	t.Setenv("WICK_SKILLS_DIR", "/skills")
	got := skillPrompt("grilling", "the auth plan", true)
	for _, want := range []string{"/skills/matt-pocock/grilling/SKILL.md", "Input: the auth plan", "scout("} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(skillPrompt("prototype", "", false), "Input:") {
		t.Error("empty args must not add an Input line")
	}
}
