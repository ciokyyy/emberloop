package emberloop

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// deliverExec is the exec the notice path drives. A var, like spawnExec, so a
// test can watch the herdr calls a submit makes.
var deliverExec Exec = osExec

// noticeText is the one-line system notice a submit is delivered as. A
// non-blocking agent contributes its summary; a blocking agent that was
// auto-backgrounded contributes its report, flattened, because a typed notice
// must stay one line.
func noticeText(s AgentSubmit) string {
	body := s.Summary
	if body == "" {
		body = strings.Join(strings.Fields(s.Report), " ")
	}
	return fmt.Sprintf("<system-notice> %s %s submitted (%s). Resume your work using the result below. ── %s ── %s</system-notice>",
		s.Type, s.AgentID, s.Status, s.AgentID, body)
}

// deliverSubmit sends one agent's submit into its parent pane. It does nothing
// when the agent has no parent pane, so a session started outside herdr never
// fails a submit.
func deliverSubmit(s AgentSubmit, st AgentState) error {
	return deliverToParent(st, noticeText(s))
}

// deliverToParent types one notice into the parent pane, serialized with every
// other delivery to that pane. It does nothing when the agent has no parent
// pane, so a session started outside herdr never fails on delivery.
func deliverToParent(st AgentState, text string) error {
	if st.Parent == "" {
		return nil
	}
	return withPaneLock(st.Parent, func() error {
		return deliverNotice(deliverExec, st.Parent, text)
	})
}

// deliverNotice nudges the parent pane through herdr's prompt first, falling
// back to typing one line and pressing Enter. A pane herdr no longer knows is
// dropped without error: nothing is queued for a session that is gone.
func deliverNotice(run Exec, pane, text string) error {
	res, err := run(herdrBin(), []string{"agent", "prompt", pane, text}, ExecOpts{Timeout: 30 * time.Second})
	if err == nil && res.Code == 0 {
		return nil
	}
	if err == nil && paneNotFound(res) {
		return nil
	}
	line := strings.Join(strings.Fields(text), " ")
	res, err = run(herdrBin(), []string{"pane", "send-text", pane, line}, ExecOpts{Timeout: 15 * time.Second})
	if err != nil {
		return err
	}
	if paneNotFound(res) {
		return nil
	}
	if res.Code != 0 {
		return fmt.Errorf("herdr pane send-text %s failed: %s", pane, firstLine(res.Stderr, res.Stdout))
	}
	res, err = run(herdrBin(), []string{"pane", "send-keys", pane, "Enter"}, ExecOpts{Timeout: 15 * time.Second})
	if err != nil {
		return err
	}
	if paneNotFound(res) {
		return nil
	}
	if res.Code != 0 {
		return fmt.Errorf("herdr pane send-keys %s failed: %s", pane, firstLine(res.Stderr, res.Stdout))
	}
	return nil
}

// paneNotFound reports that herdr answered about a pane it no longer has.
// Only the explicit code counts: herdr also says "not found" while an agent is
// still booting, and a notice for a pig pane must fall back to typing, not be
// dropped.
func paneNotFound(res Result) bool {
	return strings.Contains(res.Stdout+res.Stderr, "pane_not_found")
}

// withPaneLock runs fn while holding a lock for one parent pane, so two agents
// submitting at the same moment never type over each other.
func withPaneLock(pane string, fn func() error) error {
	return withFileLock("pane-"+pane, fn)
}

// withFileLock runs fn while holding a file lock named key. emberloop's agents run
// in separate emberloop-mcp processes, so an in-process mutex cannot serialize what
// they do to a shared file; a file lock holds across all of them.
func withFileLock(key string, fn func() error) error {
	dir := filepath.Join(filepath.Dir(statePath), "locks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, lockName(key)+".lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return fn()
}

// lockName turns a key into a file name safe on any filesystem.
func lockName(key string) string {
	var b strings.Builder
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	if b.Len() == 0 {
		return "lock"
	}
	return b.String()
}
