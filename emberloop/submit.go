package el

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"example.com/emberloop/tools"
)

// AgentSubmit is how a spawned agent reports back: the status it chose, its
// report or summary, and the fact that it submitted this turn, which the stop
// gate reads. It is the agent's own state, kept beside the worker store and
// found by the agent id.
type AgentSubmit struct {
	AgentID string `json:"agent_id"`
	Type    string `json:"type"`
	Status  string `json:"status"`
	Report  string `json:"report,omitempty"`
	Summary string `json:"summary,omitempty"`
	// Completed lists the ticket checkboxes the submit completed, each named
	// by its line text. Workers set it; other types leave it out. A submit
	// without it still works.
	Completed []string `json:"completed,omitempty"`
	// Stopped marks a submit the stop gate wrote when it gave up on an agent
	// that never called submit: the agent's result is the give-up notice.
	Stopped  bool   `json:"stopped,omitempty"`
	At       string `json:"at"`
	ThisTurn bool   `json:"this_turn"`
}

// summaryLimit is the longest one-line summary a non-blocking submit accepts.
// A notice is typed into a pane, so it must stay one short line.
const summaryLimit = 300

// submitDir holds one file per spawned agent, its last submit.
func submitDir() string { return filepath.Join(filepath.Dir(statePath), "submits") }

func submitPath(agentID string) string { return filepath.Join(submitDir(), agentID+".json") }

// WriteAgentSubmit records a submit as the agent's last one and as submitted
// this turn. It writes a temp file and renames, so the parent polling for the
// submit never reads a half-written file.
func WriteAgentSubmit(s AgentSubmit) error {
	if err := os.MkdirAll(submitDir(), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := submitPath(s.AgentID) + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, submitPath(s.AgentID))
}

// ReadAgentSubmit returns an agent's last submit, and false when it has not
// submitted (or its file is unreadable).
func ReadAgentSubmit(agentID string) (AgentSubmit, bool, error) {
	raw, err := os.ReadFile(submitPath(agentID))
	if os.IsNotExist(err) {
		return AgentSubmit{}, false, nil
	}
	if err != nil {
		return AgentSubmit{}, false, err
	}
	var s AgentSubmit
	if err := json.Unmarshal(raw, &s); err != nil {
		return AgentSubmit{}, false, fmt.Errorf("submit file for %s is corrupt: %w", agentID, err)
	}
	return s, true, nil
}

// ClearAgentSubmitThisTurn marks an agent's last submit as belonging to an
// earlier turn, keeping its status and report. The stop gate calls it when a
// new turn starts, so "submitted this turn" means what it says.
func ClearAgentSubmitThisTurn(agentID string) error {
	s, ok, err := ReadAgentSubmit(agentID)
	if err != nil || !ok || !s.ThisTurn {
		return err
	}
	s.ThisTurn = false
	return WriteAgentSubmit(s)
}

// clearAgentSubmit forgets an agent's last submit, so a re-prompted reviewer's
// caller waits on the new one rather than reading the previous turn's.
func clearAgentSubmit(agentID string) {
	_ = os.Remove(submitPath(agentID))
}

// submitToolFor builds the submit tool of one spawned agent. A blocking type
// submits a free-text report; a non-blocking type submits a one-line summary.
func submitToolFor(t AgentType, agentID string) tools.Tool {
	properties := map[string]any{
		"status": map[string]any{
			"type":        "string",
			"enum":        stringEnum(t.Statuses),
			"description": "One of your type's statuses: " + strings.Join(t.Statuses, ", ") + ".",
		},
	}
	required := []string{"status"}
	properties["completed"] = map[string]any{
		"type":        "array",
		"items":       map[string]any{"type": "string"},
		"description": "Ticket checkboxes this submit completed, each named by its checkbox line text. Workers list them; every other type leaves this out.",
	}
	if t.Blocking {
		properties["report"] = map[string]any{
			"type":        "string",
			"description": "Your full report: findings, facts, or the result. No length limit.",
		}
		required = append(required, "report")
	} else {
		properties["summary"] = map[string]any{
			"type":        "string",
			"description": fmt.Sprintf("A one-line summary of at most %d characters.", summaryLimit),
		}
		required = append(required, "summary")
	}
	return fnTool{
		name:   "submit",
		desc:   submitDescription(t),
		schema: schema(properties, required...),
		run: func(_ string, params map[string]any) (string, error) {
			return submitRun(t, agentID, params)
		},
	}
}

// submitDescription is what a spawned agent reads: scouts learn their pane
// closes once the result is delivered (a follow-up means a new scout),
// every other type stays available for one.
func submitDescription(t AgentType) string {
	base := fmt.Sprintf("Report back to your parent. Set status from %s. "+
		"A blocking type submits a report; a non-blocking one a one-line summary. "+
		"Submit is your whole answer: call it as your last action with the full result inside it, and end the turn with no text after it. ",
		strings.Join(t.Statuses, ", "))
	if t.Checkout == "none" {
		return base + "Your pane closes once your result is delivered; a follow-up means a new scout."
	}
	if t.Name == "worker" {
		return base + "List the ticket checkboxes you completed in completed, each named by its checkbox line text; leave it out when you completed none. Stay available afterwards: your parent may send you a follow-up."
	}
	return base + "Stay available afterwards: your parent may send you a follow-up."
}

// submitRun validates one submit and records it.
func submitRun(t AgentType, agentID string, params map[string]any) (string, error) {
	if agentID == "" {
		return "", fmt.Errorf("this agent has no id; submit belongs to a spawned agent")
	}
	status := str(params, "status")
	if !contains(t.Statuses, status) {
		return "", fmt.Errorf("status %q is not one of %s", status, strings.Join(t.Statuses, ", "))
	}
	s := AgentSubmit{
		AgentID:  agentID,
		Type:     t.Name,
		Status:   status,
		At:       time.Now().UTC().Format(time.RFC3339),
		ThisTurn: true,
	}
	if raw, present := params["completed"]; present && raw != nil {
		boxes, err := submitCompleted(raw)
		if err != nil {
			return "", err
		}
		s.Completed = boxes
	}
	if t.Blocking {
		s.Report = str(params, "report")
		if strings.TrimSpace(s.Report) == "" {
			return "", fmt.Errorf("report is required")
		}
	} else {
		s.Summary = str(params, "summary")
		if strings.TrimSpace(s.Summary) == "" {
			return "", fmt.Errorf("summary is required")
		}
		if strings.ContainsAny(s.Summary, "\r\n") {
			return "", fmt.Errorf("summary must be one line, not %d lines", strings.Count(s.Summary, "\n")+1)
		}
		if n := len([]rune(s.Summary)); n > summaryLimit {
			return "", fmt.Errorf("summary is %d characters; the limit is %d", n, summaryLimit)
		}
	}
	if err := WriteAgentSubmit(s); err != nil {
		return "", err
	}
	// A reviewer's submit is its verdict: fold it into the record for the
	// worktree it judged, so finish's gate sees the pass at that commit.
	if st, ok, err := ReadAgentState(agentID); err != nil {
		return "", err
	} else if ok && st.Review != nil {
		if err := mergeReviewerVerdict(st.Review.Tree, st.Review.Type, status, st.Review.At, st.Review.Head, st.Review.TreeID, st.Review.PatchID, s.Report); err != nil {
			return "", err
		}
	}
	// A spawn member's submit never delivers alone while its group is still
	// open: when the last member has submitted, one notice carrying every
	// result goes to the parent pane. A tell takes over a member's next
	// submit by clearing Deliver (and leaving the group, so the group does
	// not wait on it), so a claimed member's result returns to the waiting
	// tell instead of joining the group. Once the group has delivered, a
	// later submit (a worker sent back after finish, a stop-gate give-up) is
	// no longer part of it and is delivered alone, the way an agent outside
	// any group is.
	if st, ok, err := ReadAgentState(agentID); err != nil {
		return "", err
	} else if ok && st.GroupID != "" {
		if delivered, err := spawnGroupDelivered(st.GroupID); err != nil {
			return "", err
		} else if !delivered {
			if st.Deliver {
				if err := maybeDeliverGroup(agentID); err != nil {
					return "", err
				}
			}
			return submitAck(t, status), nil
		}
	}
	// An agent outside any open spawn group whose submit goes to the parent
	// as a notice delivers it now. The decision is made under the child's
	// own lock, so a tell that claims it at the same moment leaves exactly
	// one of them owning the result.
	if st, deliver, err := submitDelivers(agentID); err != nil {
		return "", err
	} else if deliver {
		if err := deliverSubmit(s, st); err != nil {
			return "", err
		}
		closeDeliveredScout(deliverExec, agentID)
	}
	return submitAck(t, status), nil
}

// submitAck is what a submit returns: scouts learn their pane closes,
// every other type stays available for a follow-up.
func submitAck(t AgentType, status string) string {
	if t.Checkout == "none" {
		return fmt.Sprintf("submitted %s. Nobody reads your chat, only this submit. Your next and last message is exactly \".\" (one period, nothing else, no tool call); your pane closes once delivered.", status)
	}
	return fmt.Sprintf("submitted %s. Nobody reads your chat, only this submit. Your next and last message is exactly \".\" (one period, nothing else, no tool call); a follow-up arrives as a new message.", status)
}

func stringEnum(values []string) []any {
	out := make([]any, 0, len(values))
	for _, v := range values {
		out = append(out, v)
	}
	return out
}

// submitCompleted validates the optional completed checkbox list: an array
// of non-empty strings, each naming a ticket checkbox by its line text.
// Absent stays nil, so a submit without the list still works.
func submitCompleted(raw any) ([]string, error) {
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("completed must be an array of checkbox line texts")
	}
	boxes := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return nil, fmt.Errorf("completed must be an array of checkbox line texts")
		}
		boxes = append(boxes, text)
	}
	return boxes, nil
}
