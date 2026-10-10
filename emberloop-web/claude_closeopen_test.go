package web

import "testing"

func TestToolTrackerCloseOpenEndsOnlyUnfinishedCalls(t *testing.T) {
	tr := newToolTracker()
	var got []string
	emit := func(kind, text, status string) { got = append(got, kind+":"+status) }
	mapACPUpdate(map[string]any{"sessionUpdate": "tool_call", "toolCallId": "a", "title": "bash"}, emit, tr)
	mapACPUpdate(map[string]any{"sessionUpdate": "tool_call", "toolCallId": "b", "title": "read"}, emit, tr)
	mapACPUpdate(map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": "b", "status": "completed"}, emit, tr)
	got = nil
	tr.closeOpen(emit)
	if len(got) != 1 || got[0] != "tool_call_updated:failed" {
		t.Fatalf("closeOpen events = %v, want one failed update", got)
	}
	got = nil
	tr.closeOpen(emit)
	if len(got) != 0 {
		t.Fatalf("second closeOpen emitted %v", got)
	}
}
