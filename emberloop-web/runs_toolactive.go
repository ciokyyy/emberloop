package web

import (
	"encoding/json"
	"time"
)

// runToolActiveCap is how long one in-flight tool call counts as worker
// activity for the Run driver's idle alert. A worker running a long
// foreground command (go test runs 8+ minutes) emits no commits, verdicts
// or submits while the tool runs, so without this the ten-minute idle alert
// fires on a healthy worker. Past the cap the ticket alerts normally: a
// tool wedged forever must not silence the driver.
const runToolActiveCap = 30 * time.Minute

// parseToolCallIDStatus reads the id and status of one ToolCall Event text.
// The pig stall watchdog already treats tools in flight as activity at the
// Backend level; this is the Run driver's equivalent. ok is false when the
// text is not a ToolCall payload, so foreign event text never tracks.
func parseToolCallIDStatus(text string) (id, status string, ok bool) {
	var tc ToolCall
	if err := json.Unmarshal([]byte(text), &tc); err != nil {
		return "", "", false
	}
	if tc.ID == "" {
		return "", "", false
	}
	return tc.ID, tc.Status, true
}

// trackSessionToolEvent maintains the session's open tool calls on the
// Daemon clock: a tool_call_started opens one, a tool_call_updated carrying
// a finished status (completed or failed, the same terminal set toolCallDone
// uses) closes it. Partial updates (running, in_progress) leave the call
// open. Callers hold d.mu.
func trackSessionToolEvent(s *session, kind, text string, now time.Time) {
	switch kind {
	case "tool_call_started":
		id, _, ok := parseToolCallIDStatus(text)
		if !ok {
			return
		}
		if s.openTools == nil {
			s.openTools = map[string]time.Time{}
		}
		s.openTools[id] = now
		// Drop calls the close never reached past the cap, so a Backend
		// that drops a completion never pins the ticket silent forever.
		for open, since := range s.openTools {
			if now.Sub(since) > runToolActiveCap {
				delete(s.openTools, open)
			}
		}
	case "tool_call_updated":
		id, status, ok := parseToolCallIDStatus(text)
		if !ok {
			return
		}
		if status == "completed" || status == "failed" {
			delete(s.openTools, id)
		}
	}
}

// sessionToolActiveLocked reports whether the session has a tool call in
// flight that started within the cap. The Run driver's Working check reads
// it beside the working status, so a long foreground tool call dates
// activity even when no commit, verdict or submit lands. Callers hold d.mu.
func sessionToolActiveLocked(s *session, now time.Time) bool {
	if s == nil {
		return false
	}
	for _, since := range s.openTools {
		if now.Sub(since) <= runToolActiveCap {
			return true
		}
	}
	return false
}
