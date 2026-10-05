package wick

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"example.com/wick/tools"
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
		name: "submit",
		desc: fmt.Sprintf("Report back to your parent. Set status from %s. "+
			"A blocking type submits a report; a non-blocking one a one-line summary. "+
			"Stay available afterwards: your parent may send you a follow-up.",
			strings.Join(t.Statuses, ", ")),
		schema: schema(properties, required...),
		run: func(_ string, params map[string]any) (string, error) {
			return submitRun(t, agentID, params)
		},
	}
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
	// An agent whose submit goes to the parent as a notice delivers it now. The
	// decision is made under the child's own lock, so a spawn that
	// auto-backgrounds it at the same moment leaves exactly one of them owning
	// the result. A blocking agent still inside its spawn's budget has no
	// delivery: the waiting spawn returns it.
	if st, deliver, err := submitDelivers(agentID); err != nil {
		return "", err
	} else if deliver {
		if err := deliverSubmit(s, st); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("submitted %s; stay available for a follow-up.", status), nil
}

func stringEnum(values []string) []any {
	out := make([]any, 0, len(values))
	for _, v := range values {
		out = append(out, v)
	}
	return out
}
