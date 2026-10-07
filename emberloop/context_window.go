package emberloop

import (
	"strings"

	"example.com/emberloop/tools"
)

// DefaultContextWindow is the window for a model neither the settings
// overrides nor the built-in table names: 200k, the same default the
// compress threshold uses.
const DefaultContextWindow = 200000

// modelWindowFamilies are the built-in windows by model prefix, longest
// prefix wins. Values are the harnesses' documented context sizes,
// approximate by nature: a settings window override always wins. A new
// harness family with a different window adds one line here.
var modelWindowFamilies = []struct {
	prefix string
	window int64
}{
	{"deepseek-v4", 1000000},
	{"gemini-", 1000000},
	{"gpt-", 400000},
	{"claude-opus-", 200000},
	{"claude-sonnet-", 200000},
	{"claude-haiku-", 200000},
	{"claude-", 200000},
	{"sonnet", 200000},
	{"opus", 200000},
	{"haiku", 200000},
}

// cleanModelName strips leading provider paths (any number of segments) and
// harness prefixes such as antigravity-, lower-casing the remainder for
// table matching.
func cleanModelName(model string) string {
	lower := strings.ToLower(model)
	if idx := strings.LastIndex(lower, "/"); idx != -1 {
		lower = lower[idx+1:]
	}
	lower = strings.TrimPrefix(lower, "antigravity-")
	return lower
}

// ModelWindow resolves a model's context window in tokens: the settings
// override for the exact model first, then the built-in family table,
// else the default. Matching is case-insensitive; an empty model is the
// default.
func ModelWindow(model string, overrides map[string]int64) int64 {
	for name, window := range overrides {
		if window > 0 && strings.EqualFold(name, model) {
			return window
		}
	}
	cleaned := cleanModelName(model)
	best := -1
	for i, fam := range modelWindowFamilies {
		if strings.HasPrefix(cleaned, fam.prefix) && (best == -1 || len(fam.prefix) > len(modelWindowFamilies[best].prefix)) {
			best = i
		}
	}
	if best != -1 {
		return modelWindowFamilies[best].window
	}
	return DefaultContextWindow
}

// SnapshotWindow is the window for a snapshot's model under settings.
func SnapshotWindow(snap ContextSnapshot, s tools.Settings) int64 {
	return ModelWindow(snap.Model, s.ContextWindows())
}
