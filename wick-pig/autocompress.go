package wick

import (
	"sort"
	"sync"
	"time"

	"example.com/wick/tools"
)

// autoFailCooldown bounds paid retries after a failed auto-compress: a pane
// whose summary or restart just failed sits out this long. A var so tests
// run it fast.
var autoFailCooldown = 10 * time.Minute

// autoFailedAt holds the last auto-compress failure per pane, so one bad
// pane does not spend a model call on every sweep. Guarded by autoFailMu:
// sweeps run panes concurrently.
var (
	autoFailMu   sync.Mutex
	autoFailedAt = map[string]time.Time{}
)

// autoFailedRecently says whether pane failed inside the cooldown. Old
// entries prune on read, so dead panes do not grow the map.
func autoFailedRecently(pane string) bool {
	autoFailMu.Lock()
	defer autoFailMu.Unlock()
	for p, at := range autoFailedAt {
		if time.Since(at) >= autoFailCooldown {
			delete(autoFailedAt, p)
		}
	}
	at, ok := autoFailedAt[pane]
	return ok && time.Since(at) < autoFailCooldown
}

// recordAutoFailure parks pane for the cooldown; clearAutoFailure parks
// nothing: a success compresses again on the next pass past the threshold.
func recordAutoFailure(pane string) {
	autoFailMu.Lock()
	defer autoFailMu.Unlock()
	autoFailedAt[pane] = time.Now()
}

func clearAutoFailure(pane string) {
	autoFailMu.Lock()
	defer autoFailMu.Unlock()
	delete(autoFailedAt, pane)
}

// AutoDecision is what the automatic trigger says about one session: whether
// to compress now, whether the trigger waits for a tool-call boundary first,
// and why. A session compresses only at a boundary or at idle, never during
// a running tool call.
type AutoDecision struct {
	Compress bool
	Wait     bool
	Reason   string
}

// DecideAutoCompress says whether the session in snap compresses on its own:
// over the threshold, the switch on, a model set, past the cooldown, and at
// a tool-call boundary (or idle). compressed counts the handoffs so far: the
// cooldown only binds a session that already went through one, never the
// first run.
func DecideAutoCompress(snap ContextSnapshot, s tools.Settings, harness string, compressed int) AutoDecision {
	if !s.EffectiveAutoCompress() {
		return AutoDecision{Reason: "auto-compress is off"}
	}
	if snap.ContextTokens < s.EffectiveCompressThreshold(harness) {
		return AutoDecision{Reason: "below threshold"}
	}
	if s.EffectiveCompressModel(harness) == "" {
		return AutoDecision{Reason: "no compressModel"}
	}
	if compressed > 0 && snap.CompletedCalls < s.EffectiveCompressCooldown() {
		return AutoDecision{Reason: "cooldown"}
	}
	if snap.Outstanding > 0 {
		return AutoDecision{Wait: true, Reason: "waiting for tool boundary"}
	}
	return AutoDecision{Compress: true, Reason: "over threshold at boundary"}
}

// MaybeAutoCompress compresses the worker in pane when the automatic trigger
// says so, and reports whether it did. Below the threshold, switched off, in
// cooldown, or with no model set, it returns without touching the pane: the
// header already warns about the missing model. A session past the threshold
// with a tool call running waits for the boundary first, then compresses.
// The claim keeps one pane on one compress: a second trigger (manual or
// sweep) while the mark is live returns without doing anything. The handoff
// always ends with the continue line, so the new session resumes
// the interrupted step without waiting for the user. A failed summary or
// restart parks the pane for the failure cooldown instead of spending a
// model call on every sweep.
func MaybeAutoCompress(run Exec, pane string, s tools.Settings) (string, bool, error) {
	agentID, err := resolvePaneToAgent(pane, run)
	if err != nil {
		return "", false, err
	}
	tgt, err := resolveCompressTarget(agentID, pane)
	if err != nil {
		return "", false, err
	}
	sessionID, path := tgt.sessionID, tgt.path
	snap, err := summarizePath(tgt.harness, path)
	if err != nil {
		return "", false, err
	}
	compressed := 0
	if st, ok, serr := ReadAgentState(agentID); serr != nil {
		return "", false, serr
	} else if ok {
		compressed = st.Compressed
	}
	if d := DecideAutoCompress(snap, s, tgt.harness, compressed); !d.Compress && !d.Wait {
		return "", false, nil
	}
	// The boundary waits before the summary: a result landing mid-wait
	// belongs in the handoff, not after it.
	if err := WaitForToolBoundary(tgt.harness, path); err != nil {
		return "", false, err
	}
	snap, err = summarizePath(tgt.harness, path)
	if err != nil {
		return "", false, err
	}
	if d := DecideAutoCompress(snap, s, tgt.harness, compressed); !d.Compress {
		return "", false, nil
	}
	model := s.EffectiveCompressModel(tgt.harness)
	claimed, err := ClaimCompressing(agentID)
	if err != nil {
		return "", false, err
	}
	if !claimed {
		return "", false, nil
	}
	defer SetAgentCompressing(agentID, false) //nolint:errcheck
	// The decision ran before the claim: a winner that finished in between
	// rotated the session, so this snapshot is stale. A rotated session
	// compresses once; the fresh session decides on its own transcript.
	// The re-read runs under our live claim, so no later winner slips
	// between this check and the summary.
	if st, ok, serr := ReadAgentState(agentID); serr != nil {
		return "", false, serr
	} else if ok && st.SessionID != "" && st.SessionID != sessionID {
		return "", false, nil
	}
	handoff, err := SummarizeSession(run, tgt.harness, path, model, ModelWindow(model, s.ContextWindows()))
	if err != nil {
		recordAutoFailure(pane)
		return "", false, err
	}
	handoff = FinishHandoff(handoff, true)
	if err := keepCompressCopies(agentID, sessionID, path, handoff); err != nil {
		recordAutoFailure(pane)
		return "", false, err
	}
	next, err := RestartPane(run, PaneRestart{
		Harness:      tgt.harness,
		Pane:         pane,
		AgentID:      agentID,
		OldSessionID: sessionID,
		Handoff:      handoff,
	}, tgt.launchFor(handoff))
	if err != nil {
		recordAutoFailure(pane)
		return "", false, err
	}
	clearAutoFailure(pane)
	return next, true, nil
}

// SweepAutoCompress runs one automatic pass over every worker pane and
// every top-level session pane: each pane
// past its threshold compresses through MaybeAutoCompress, and the panes
// that did are returned. Panes compress concurrently, so one pane's boundary
// wait never holds the rest; the claim keeps two passes off one pane, and a
// pane that refuses or fails is skipped, never fatal: the header shows its
// state and the next sweep retries it past the failure cooldown.
func SweepAutoCompress(run Exec, s tools.Settings) []string {
	store, err := load()
	if err != nil {
		return nil
	}
	panes := make([]string, 0, len(store))
	seen := map[string]bool{}
	for _, w := range store {
		if w.Pane != "" && !seen[w.Pane] && !autoFailedRecently(w.Pane) {
			seen[w.Pane] = true
			panes = append(panes, w.Pane)
		}
	}
	// Top-level sessions are not workers but compress like them: each one
	// recorded its pane, harness and session at startup.
	for _, pane := range topLevelPanes() {
		if !seen[pane] && !autoFailedRecently(pane) {
			seen[pane] = true
			panes = append(panes, pane)
		}
	}
	var mu sync.Mutex
	var done []string
	var wg sync.WaitGroup
	for _, pane := range panes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, did, err := MaybeAutoCompress(run, pane, s); err == nil && did {
				mu.Lock()
				done = append(done, pane)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	sort.Strings(done)
	return done
}

// autoPollEvery bounds one supervisor sweep pass. A var so tests run it fast.
var autoPollEvery = 10 * time.Second

// SuperviseAutoCompress watches every worker pane until stop closes: each
// pass compresses the panes past their threshold, at a tool-call boundary
// or at idle, never during a running tool call. Settings reload every pass,
// so the disable switch and the cooldown take effect without a restart.
func SuperviseAutoCompress(run Exec, loadSettings func() tools.Settings, stop <-chan struct{}) {
	for {
		select {
		case <-stop:
			return
		case <-time.After(autoPollEvery):
		}
		SweepAutoCompress(run, loadSettings())
	}
}
