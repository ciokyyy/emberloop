package wick

import "testing"

// herdr recognizes a top-level claude by the harness name, whatever role chip
// the strip shows; a typed agent reports under its type's readable name.
func TestAgentReporterLabel(t *testing.T) {
	for _, c := range []struct {
		src  HeaderSource
		want string
	}{
		{HeaderSource{Harness: "claude", AgentName: "plan"}, "claude"},
		{HeaderSource{Harness: "claude", AgentName: "orchestrator"}, "claude"},
		{HeaderSource{Harness: "claude"}, "claude"},
		{HeaderSource{Harness: "claude", AgentType: "reviewer-spec", AgentName: "spec-reviewer"}, "spec-reviewer"},
	} {
		r := newAgentReporter(WrapConfig{Source: c.src, PaneID: "p1", Report: func([]string) {}})
		if r == nil || r.label != c.want {
			t.Errorf("source %+v reports as %v, want %q", c.src, r, c.want)
		}
	}
}
