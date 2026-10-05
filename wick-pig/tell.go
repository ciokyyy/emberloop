package wick

import (
	"fmt"
	"time"
)

// tellExec is the exec the tell tool drives. A var, like spawnExec, so a test
// can stand in for herdr.
var tellExec Exec = osExec

// renderSubmit is one agent's report as its caller sees it: its type, id,
// status and the body it gave, a summary for a non-blocking agent or a full
// report for a blocking one. It is shared by the spawn result and a tell that
// waited for a blocking agent's submit.
func renderSubmit(s AgentSubmit) string {
	body := s.Report
	if body == "" {
		body = s.Summary
	}
	return fmt.Sprintf("%s %s submitted (%s):\n%s", s.Type, s.AgentID, s.Status, body)
}

// tellAgent delivers text to any agent by id through the nudge path. A
// non-blocking agent's answer arrives later as a submit notice, so tell returns
// at once. A blocking agent's next submit is returned here; a tell that
// outlasts the tool budget auto-backgrounds the agent, so its result arrives as
// a notice and the caller ends its turn instead of waiting.
func tellAgent(run Exec, id, text string) (string, error) {
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
		return "", fmt.Errorf("could not deliver to %s: %w", id, err)
	}

	deadline := time.Now().Add(toolBudget)
	for {
		if s, ok, _ := ReadAgentSubmit(id); ok && s.ThisTurn {
			return renderSubmit(s), nil
		}
		if !time.Now().Before(deadline) {
			break
		}
		time.Sleep(pollEvery)
	}

	bg, err := backgroundToldAgent(id)
	if err != nil {
		return "", err
	}
	if !bg {
		// It submitted right at the deadline.
		if s, ok, _ := ReadAgentSubmit(id); ok && s.ThisTurn {
			return renderSubmit(s), nil
		}
	}
	return fmt.Sprintf("delivered to %s; it is still running. Its result will arrive as a system notice; end your turn.", id), nil
}
