package wick

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strings"
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

// readOnlyOnce runs ReadOnly at most once per key. A caller waits up to
// toolBudget; if the reviewer is still going it gets "still running" and the
// reviewer carries on, and the next call with the same key rejoins it.
func readOnlyOnce(run Exec, key, cwd, model, prompt string) (string, error) {
	jobsMu.Lock()
	ch, running := jobs[key]
	if !running {
		ch = make(chan jobResult, 1)
		jobs[key] = ch
		go func() {
			text, err := ReadOnly(run, cwd, model, prompt)
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

// ReadOnly runs one prompt in a read-only agent (role "review": read and
// find) in its own herdr pane, so the human can watch it, and returns the
// agent's last message, read from its session transcript. The prompt goes
// through a file: pane run types its argv as one line.
// WICK_READONLY_HARNESS picks the harness (default pig); a model goes only to
// pig, since worker models are pig model names.
func ReadOnly(run Exec, cwd, model, prompt string) (string, error) {
	harness := os.Getenv("WICK_READONLY_HARNESS")
	if harness == "" {
		harness = "pig"
	}
	if harness != "pig" {
		model = ""
	}
	dir := filepath.Join(filepath.Dir(statePath), "readonly")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	sess := Session{Role: "review", Cwd: cwd, Model: model, ID: newSessionID(), Dir: filepath.Join(dir, "sessions"),
		ReadAllow: []string{filepath.Dir(statePath)}}
	promptPath := filepath.Join(dir, sess.ID+".prompt")
	if err := os.WriteFile(promptPath, []byte(prompt), 0o644); err != nil {
		return "", err
	}
	sess.Prompt = fmt.Sprintf("Read %s and follow it exactly. Your last message is your answer.", promptPath)
	launch, err := PrepareLaunch(harness, sess)
	if err != nil {
		return "", err
	}
	// The reviewer is done with the MCP config the moment it has launched.
	defer CleanupSession(sess)
	// --ratio is the share the pane being split keeps, not the share the new
	// pane gets: measured on a scratch pane, a 229-column split at 0.3 left
	// the original 69 columns and gave the new pane 160. So the reviewer gets
	// 0.7 here, and tilePanes below evens the tab back out.
	split, err := herdrJSON(run, []string{"pane", "split", "--current", "--direction", "down",
		"--ratio", "0.3", "--cwd", cwd, "--no-focus"}, 15*time.Second)
	if err != nil {
		return "", err
	}
	pane := split.Result.Pane.PaneID
	if pane == "" {
		return "", fmt.Errorf("herdr split returned no pane id")
	}
	defer run(herdrBin(), []string{"pane", "close", pane}, ExecOpts{Timeout: 15 * time.Second})

	// Every review splits the caller's pane, so a few of them leave the tab at
	// 1/2, 1/4, 1/8. Tiling is cosmetic: never fail a review over an uneven tab.
	_ = tilePanes(run, split.Result.Pane.TabID)

	if res, err := run(herdrBin(), []string{"pane", "run", pane, launch.Line}, ExecOpts{Timeout: 15 * time.Second}); err != nil || res.Code != 0 {
		return "", fmt.Errorf("herdr pane run failed: %v %s", err, firstLine(res.Stderr, res.Stdout))
	}
	if err := DeliverLate(run, pane, launch.Late); err != nil {
		return "", err
	}
	return WaitReply(registry[harness], sess, readOnlyTimeout, func() error { return Nudge(run, pane, "continue") })
}

// Review has a fresh agent with no edit tools judge a worker's diff.
func Review(run Exec, id string) (string, error) {
	w, err := recall(id)
	if err != nil {
		return "", err
	}
	return diffReview(run, diffRequest{
		runDir: w.Repo, tree: w.Worktree, model: w.Model, name: id, key: "review " + id,
		args: []string{"diff", w.Base + "..." + w.Branch},
	})
}

// SelfReview judges the work in the tree the caller is standing in, so a
// worker can review its own before it hands the branch back. It takes no
// arguments: the tree is wherever the tool runs, and the base is the commit
// that tree was created from.
func SelfReview(run Exec, tree string) (string, error) {
	return diffReview(run, diffRequest{
		runDir: tree, tree: tree, name: filepath.Base(tree), key: "code_review " + filepath.Base(tree),
		// A ref against the *working tree*, not a three-dot ref against a
		// branch: a worker reviews mid-branch, and only this form covers
		// commits and uncommitted work in one diff. The base is a SHA
		// (resolveRef), so it still means "since the worktree branched".
		args: []string{"diff", baseFor(tree)},
	})
}

// baseFor is the commit a worktree branched from, as recorded when it was
// spawned. Outside a worktree wick created there is no such record, and the
// default branch name is the only fallback available; the diff it produces is
// then measured from there rather than from the worker's real base.
func baseFor(tree string) string {
	if store, err := load(); err == nil {
		for _, w := range store {
			if w.Base != "" && filepath.Clean(w.Worktree) == filepath.Clean(tree) {
				return w.Base
			}
		}
	}
	return "main"
}

// diffReview is the one path both reviewers take: run git, write the diff
// somewhere the reviewer can read it, hand it to ReadOnly. The reviewer
// cannot run git itself, and it gets the diff rather than the worker's own
// account of the work: reviewing that would review what the worker believes
// it did.
type diffRequest struct {
	runDir, tree, model, name, key string
	args                           []string
}

func diffReview(run Exec, in diffRequest) (string, error) {
	res, err := run("git", in.args, ExecOpts{Cwd: in.runDir})
	if err != nil {
		return "", err
	}
	if res.Code != 0 {
		return "", fmt.Errorf("git diff for %s failed: %s", in.name, firstLine(res.Stderr, "unknown error"))
	}
	if strings.TrimSpace(res.Stdout) == "" {
		return "no changes: nothing differs from the base. Nothing to review.", nil
	}

	dir := filepath.Join(filepath.Dir(statePath), "reviews")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	diffPath := filepath.Join(dir, in.name+".diff")
	if err := os.WriteFile(diffPath, []byte(res.Stdout), 0o644); err != nil {
		return "", err
	}

	return readOnlyOnce(run, in.key, in.tree, in.model, reviewPrompt(diffPath, in.tree))
}

func reviewPrompt(diffPath, tree string) string {
	return fmt.Sprintf(`Review the change in %s (a git diff). The working directory, %s, holds the result; read files there for context.

You can only read and find. List real defects: correctness bugs, missed edge cases, callers the change broke, new logic with no test. Skip style.

One line per defect: file:line severity: problem. Severity is blocker, major or minor.
If there are none, reply exactly "no defects found". Do not praise and do not suggest rewrites.`, diffPath, tree)
}

func scoutPrompt(question string) string {
	return fmt.Sprintf(`Answer from this repository only: %s

You can only read and find. Cite file:line for every claim. If you cannot find it, say "not found" rather than guess. Report facts; do not recommend or decide.`, question)
}
