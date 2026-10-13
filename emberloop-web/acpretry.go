package web

import (
	"errors"
	"strings"
	"time"
)

// maxACPPromptAttempts bounds one prompt's sends after the adapter dies
// mid-turn: the first send plus up to two respawned retries.
const maxACPPromptAttempts = 3

// acpRespawnBackoff waits before each respawned retry: index 0 before the
// second send, index 1 before the third. The table carries one wait per
// possible retry; tests shrink it.
var acpRespawnBackoff = []time.Duration{time.Second, 3 * time.Second, 8 * time.Second}

// isAdapterClosedErr reports the read-loop orphan error: the adapter
// process's stdout closed, so every pending request fails. A deliberate
// close (CloseSession, DropSession, restart Shutdown) and an unexpected
// adapter death both read this way; the backend map tells them apart.
func isAdapterClosedErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "adapter closed")
}

// acpRespawnBackoffFor is the wait before one respawned retry, 0 past the
// table: with three attempts only the first two waits ever run.
func acpRespawnBackoffFor(i int) time.Duration {
	if i < 0 || i >= len(acpRespawnBackoff) {
		return 0
	}
	return acpRespawnBackoff[i]
}

// errAdapterSessionClosed is the internal respawn stop: the Session closed
// deliberately while its dead adapter respawned, so the orphaned prompt
// result drops quietly instead of surfacing. Never stored in the log.
var errAdapterSessionClosed = errors.New("adapter session closed")

// promptHandleClosed reports whether a prompt's handle died deliberately:
// the Session entry is gone (CloseSession, DropSession, restart Shutdown)
// or holds a newer handle (Handoff swap, resume). Either way the in-flight
// result is orphaned and drops quietly: no error Event, no failed status,
// no retry. An entry still holding this handle means the adapter died on
// its own, and the prompt retries through respawnAdapter.
func (b *ClaudeBackend) promptHandleClosed(sessionID string, conn acpConn) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	st, ok := b.sessions[sessionID]
	return !ok || st.conn != conn
}

// respawnAdapter restarts a dead adapter around the saved ACP session id
// through the existing resume path (role, WorkDir and owner dirs come from
// the stored record), so the retried prompt continues the same adapter
// session. Pig-acp and grok respawn through their outer ResumeSession,
// which also restages the steer socket and the effort; plain claude
// respawns through ResumeSession directly. A missing entry means a
// deliberate close won the race: errAdapterSessionClosed, drop quietly.
// Any other failure is terminal: the prompt fails with one actionable
// error naming the attempts. There is no Handoff fallback: a Handoff needs
// a live handle to write it, and the adapter is dead.
func (b *ClaudeBackend) respawnAdapter(sessionID string) error {
	b.mu.Lock()
	st, ok := b.sessions[sessionID]
	b.mu.Unlock()
	if !ok {
		return errAdapterSessionClosed
	}
	meta := st.meta
	meta.ID = sessionID
	meta.ResumeToken = st.acpID
	resumer := b.resumer
	if resumer == nil {
		resumer = b.ResumeSession
	}
	return resumer(meta, func(kind, text, status string) {})
}
