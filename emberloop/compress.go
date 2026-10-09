package el

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"example.com/emberloop/tools"
)

// summarizeTimeout bounds one pig one-off summary: a long transcript reads
// slow, but a hung summarizer must not hold the compress forever. A var so
// tests run it fast.
var summarizeTimeout = 5 * time.Minute

// SummarizeSession runs the compression summarizer over one session's
// transcript: a pig one-off with no tools and no extra system prompt, fed
// the pinned combined prompt plus the transcript rendered in full. The
// prompt travels in a temp file, not argv: one argument caps at 128KB and a
// long session is exactly when compress runs. The answer passes the handoff
// sanity check before it returns; a failed or empty summary reports and the
// old session stays untouched.
//
// An unset model or input larger than the compress window refuses before any
// process starts.
func SummarizeSession(run Exec, harness, transcriptPath, model string, window int64) (string, error) {
	if strings.TrimSpace(model) == "" {
		return "", fmt.Errorf("no compressModel for harness %q: set one with `el settings compress-model %s <model>`; compression refused", harness, harness)
	}
	raw, err := os.ReadFile(transcriptPath)
	if err != nil {
		return "", fmt.Errorf("open transcript for summary: %w", err)
	}
	rendered, err := RenderTranscript(harness, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	snap, err := SummarizeContext(harness, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	if estimate := summarizeTokens(snap, rendered); estimate > window {
		return "", fmt.Errorf("transcript is ~%d tokens, past the %q window %d: compression refused", estimate, model, window)
	}
	promptFile, err := writePromptFile(CompressPrompt + "\nSession transcript:\n" + rendered)
	if err != nil {
		return "", err
	}
	defer os.Remove(promptFile)
	res, err := run(pigBin(), []string{"-p", "--no-session", "--no-tools", "--no-extensions", "--no-skills", "--no-context-files", "--model", model, "--", "@" + promptFile}, ExecOpts{Timeout: summarizeTimeout})
	if err != nil {
		return "", fmt.Errorf("summarizer: %w", err)
	}
	if res.Code != 0 {
		return "", fmt.Errorf("summarizer failed: %s", firstLine(res.Stderr, res.Stdout))
	}
	handoff := strings.TrimSpace(res.Stdout)
	if err := CheckHandoff(handoff); err != nil {
		return "", err
	}
	return handoff, nil
}

// writePromptFile files the summarizer prompt: pig reads @files into the
// initial message, which carries a long transcript past the argv cap.
func writePromptFile(prompt string) (string, error) {
	f, err := os.CreateTemp("", "el-compress-*.md")
	if err != nil {
		return "", fmt.Errorf("prompt file: %w", err)
	}
	if _, err := f.WriteString(prompt); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", fmt.Errorf("prompt file: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return "", fmt.Errorf("prompt file: %w", err)
	}
	return f.Name(), nil
}

// summarizeTokens estimates the summarizer input in tokens: the context
// core's token count, or a quarter of the rendered bytes, whichever is
// larger. The rendered full text is what the model actually reads.
func summarizeTokens(snap ContextSnapshot, rendered string) int64 {
	if estimate := int64(len(rendered)) / 4; estimate > snap.ContextTokens {
		return estimate
	}
	return snap.ContextTokens
}

// compressAgentDir holds one compress's audit trail: the handoff copy and
// the old transcript beside it, for inspection and rollback.
func compressAgentDir(agentID string) string {
	return filepath.Join(filepath.Dir(statePath), "compress", agentID)
}

// CompressPane compresses the worker in pane by hand: it summarizes the
// session, keeps the handoff copy and the old transcript, and hands the
// finished handoff to the supervisor for a same-pane restart. A refusal or
// a failed summary returns before anything stops: the old session stays
// untouched. A press during a running tool call queues to the next boundary
// with the queue mark set, so the header shows it; see RequestCompress for
// the key's signal path and CompressPaneWithForce to force past the call.
func CompressPane(run Exec, pane string, s tools.Settings) (string, error) {
	return CompressPaneWithForce(run, pane, s, false)
}

// workerLaunchFor rebuilds the worker's launch for the fresh session id
// carrying the handoff, the way spawn built it: the agent role in its
// worktree with its model and effort.
func workerLaunchFor(w Worker, handoff string) func(newSessionID string) (Session, Launch, error) {
	return func(next string) (Session, Launch, error) {
		sess := Session{
			ID:        next,
			Role:      "agent",
			Cwd:       w.Worktree,
			Model:     w.Model,
			Effort:    w.Effort,
			Label:     w.ID,
			AgentType: w.AgentType,
			AgentID:   w.ID,
			Prompt:    handoff,
		}
		launch, err := PrepareLaunch(w.Harness, sess)
		return sess, launch, err
	}
}

// keepCompressCopies files the handoff copy and the old transcript under
// the agent's compress dir, named for the session they replace.
func keepCompressCopies(agentID, sessionID, transcriptPath, handoff string) error {
	dir := compressAgentDir(agentID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, sessionID+"-handoff.md"), []byte(handoff), 0o644); err != nil {
		return err
	}
	raw, err := os.ReadFile(transcriptPath)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, sessionID+"-transcript.jsonl"), raw, 0o644)
}
