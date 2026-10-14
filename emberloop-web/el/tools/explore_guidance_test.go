package tools

import (
	"strings"
	"testing"
)

// Explore guidance (ticket 08): tool descriptions steer exploration to find
// and execute before repeated bash grep. The served execute text is the
// per-role bound description (ExecuteDescription); the unbound placeholder
// carries no guidance of its own.
func TestExploreGuidanceInDescriptions(t *testing.T) {
	findDesc := findTool{}.Description()
	for _, want := range []string{"Prefer find", "bash grep", "execute"} {
		if !strings.Contains(findDesc, want) {
			t.Errorf("find description lacks %q: %q", want, findDesc)
		}
	}
	bound := ExecuteDescription([]string{"grep"}, []string{"bash"}, nil)
	for _, want := range []string{"find for locating code", "execute for multi-step exploration", "before repeated bash grep"} {
		if !strings.Contains(bound, want) {
			t.Errorf("bound execute description lacks %q: %q", want, bound)
		}
	}
	bashDesc := bashTool{}.Description()
	for _, want := range []string{"Prefer find", "execute for multi-step exploration", "before repeated bash grep"} {
		if !strings.Contains(bashDesc, want) {
			t.Errorf("bash description lacks %q: %q", want, bashDesc)
		}
	}
	grepDesc := grepTool{}.Description()
	for _, want := range []string{"prefer find", "use execute"} {
		if !strings.Contains(strings.ToLower(grepDesc), want) {
			t.Errorf("grep description lacks %q: %q", want, grepDesc)
		}
	}
}
