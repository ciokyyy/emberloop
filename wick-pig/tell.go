package wick

import (
	"context"
	"fmt"
	"os"
	"time"
)

// tellExec is the exec the tell tool drives. A var, like spawnExec, so a test
// can stand in for herdr.
var tellExec Exec = osExec

// renderSubmit is one agent's report as its caller sees it: its type, id,
// status and the body it gave, a summary for a non-blocking agent or a full
// report for a blocking one. A give-up is not a status the agent chose but the
// stop gate's, so it reads as what happened. It is shared by the spawn result
// and a tell that waited for a blocking agent's submit.
func renderSubmit(s AgentSubmit) string {
	if s.Stopped {
		return fmt.Sprintf("%s %s %s", s.Type, s.AgentID, s.Report)
	}
	body := s.Report
	if body == "" {
		body = s.Summary
	}
	return fmt.Sprintf("%s %s submitted (%s):\n%s", s.Type, s.AgentID, s.Status, body)
}

// answersTell reports whether a submit is the answer to a waiting tell: the
// turn's own submit, or the stop gate's give-up, which is returned the way a
// submit would be.
func answersTell(s AgentSubmit) bool { return s.ThisTurn || s.Stopped }

// tellAgent delivers text to any agent by id through the nudge path. A
// non-blocking agent's answer arrives later as a submit notice, so tell returns
// at once. A blocking agent's next submit is returned here; a tell that
// outlasts the tool budget auto-backgrounds the agent, so its result arrives as
// a notice and the caller ends its turn instead of waiting.
func tellAgent(run Exec, id, text string) (string, error) {
	return tellAgentCtx(context.Background(), run, id, text)
}

// tellAgentCtx is tellAgent for a call the harness can cancel: a cancelled wait
// hands the answer to the notice path like a wait that ran out of budget.
func tellAgentCtx(ctx context.Context, run Exec, id, text string) (string, error) {
	// Each parent action sweeps its stranded groups first, in the
	// background: a vanished member with no further submit is finalized
	// after this call ends instead of waiting forever.
	sweepOpenGroups(os.Getenv("HERDR_PANE_ID"), os.Getenv("WICK_AGENT_ID"))
	st, ok, err := ReadAgentState(id)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("unknown agent %q", id)
	}
	t, ok := mustLoadAgentTypes(cwd())[st.Type]
	if !ok {
		return "", fmt.Errorf("agent %s has unknown type %q", id, st.Type)
	}
	if st.Pane == "" {
		return "", fmt.Errorf("agent %s has no pane to deliver to", id)
	}
	if !t.Blocking {
		if err := Nudge(run, st.Pane, text); err != nil {
			// The pane may be gone: dispatch the agent's group in the
			// background, so a vanished member is reported as lost after
			// this call ends instead of cancelling it.
			if st.GroupID != "" {
				dispatchGroupDelivery(st.GroupID)
			}
			return "", fmt.Errorf("could not deliver to %s: %w", id, err)
		}
		return fmt.Sprintf("delivered to %s. Its result will arrive as a system notice; end your turn.", id), nil
	}

	// A blocking agent's next submit is the answer to this message: claim it,
	// so it is returned here rather than delivered as a notice. If the message
	// does not reach the agent, put the child back so its result still lands
	// somewhere.
	undo, err := claimNextSubmit(id)
	if err != nil {
		return "", err
	}
	if err := Nudge(run, st.Pane, text); err != nil {
		_ = undo()
		// The pane may be gone: dispatch the agent's group in the
		// background, so a vanished member is reported as lost after this
		// call ends instead of cancelling it.
		if st.GroupID != "" {
			dispatchGroupDelivery(st.GroupID)
		}
		return "", fmt.Errorf("could not deliver to %s: %w", id, err)
	}

	deadline := time.Now().Add(toolBudget)
	for {
		if s, ok, _ := ReadAgentSubmit(id); ok && answersTell(s) {
			return renderSubmit(s), nil
		}
		if !time.Now().Before(deadline) || ctx.Err() != nil {
			break
		}
		select {
		case <-ctx.Done():
		case <-time.After(pollEvery):
		}
	}
	bg, err := backgroundToldAgent(id)
	if err != nil {
		return "", err
	}
	if !bg {
		// It submitted right at the deadline.
		if s, ok, _ := ReadAgentSubmit(id); ok && answersTell(s) {
			return renderSubmit(s), nil
		}
	}
	return fmt.Sprintf("delivered to %s; it is still running. Its result will arrive as a system notice; end your turn.", id), nil
}
