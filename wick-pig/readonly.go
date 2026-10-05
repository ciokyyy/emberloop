package wick

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

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

	if res, err := run(herdrBin(), []string{"pane", "run", pane, launch.Line}, ExecOpts{Timeout: 15 * time.Second}); err != nil || res.Code != 0 {
		return "", fmt.Errorf("herdr pane run failed: %v %s", err, firstLine(res.Stderr, res.Stdout))
	}
	if err := DeliverLate(run, pane, launch.Late); err != nil {
		return "", err
	}
	return WaitReply(registry[harness], sess, readOnlyTimeout)
}

// Review has a fresh agent with no edit tools judge a worker's diff.
//
// The reviewer cannot run git, so the diff is written to a file for it to read.
// It gets the diff, not the ticket or the worker's own summary: reviewing the
// summary would review what the worker believes it did.
func Review(run Exec, id string) (string, error) {
	w, err := recall(id)
	if err != nil {
		return "", err
	}

	res, err := run("git", []string{"diff", w.Base + "..." + w.Branch}, ExecOpts{Cwd: w.Repo})
	if err != nil {
		return "", err
	}
	if res.Code != 0 {
		return "", fmt.Errorf("git diff for %s failed: %s", id, firstLine(res.Stderr, "unknown error"))
	}
	if strings.TrimSpace(res.Stdout) == "" {
		return "no changes: the branch has no commits beyond its base. Nothing to review.", nil
	}

	dir := filepath.Join(filepath.Dir(statePath), "reviews")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	diffPath := filepath.Join(dir, id+".diff")
	if err := os.WriteFile(diffPath, []byte(res.Stdout), 0o644); err != nil {
		return "", err
	}

	return ReadOnly(run, w.Worktree, w.Model, reviewPrompt(diffPath, w.Worktree))
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
