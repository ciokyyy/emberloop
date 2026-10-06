package wick

import "testing"

// The strip source for a launched agent comes from the launch command's own
// environment: the identity the harness command already carries. The chip
// shows the herdr pane name for the type, from the one mapping both read.
func TestLaunchStripSource(t *testing.T) {
	cmd := Command{
		Bin:  "pig",
		Args: []string{"--session-id", "s1", "--session-dir", "/tmp/sess"},
		Env: []string{
			"WICK_AGENT_ID=w1",
			"WICK_AGENT_TYPE=worker",
			"WICK_AGENT_LABEL=ticket-1",
			"WICK_SESSION_ID=s1",
			"WICK_HARNESS=pig",
		},
	}
	src := LaunchStripSource(cmd)
	if src.AgentID != "w1" || src.AgentType != "worker" || src.AgentName != "implementer" {
		t.Errorf("identity = %+v, want w1/worker/implementer", src)
	}
	if src.Harness != "pig" || src.SessionID != "s1" || src.SessionDir != "/tmp/sess" {
		t.Errorf("session = %+v, want pig s1 /tmp/sess", src)
	}
}

// A bare session has no agent identity: the chip falls back to the
// orchestrator in the render.
func TestLaunchStripSourceBare(t *testing.T) {
	src := LaunchStripSource(Command{Bin: "pig", Env: []string{"WICK_HARNESS=pig"}})
	if src.AgentName != "" || src.Harness != "pig" {
		t.Errorf("bare source = %+v, want empty name with pig", src)
	}
}
