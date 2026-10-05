package antigravity

import "strings"

// Catalog is what the provider offers. It mirrors the public Antigravity model
// list of @cortexkit/antigravity-auth-core (image output excluded: pig's
// provider event protocol has no image-output stream events).
type catalogModel struct {
	ID        string
	Name      string
	Reasoning bool
	Context   int
	MaxTokens int
}

var catalog = []catalogModel{
	{"antigravity-gemini-3.8-flash", "Gemini 3.8 Flash (Antigravity)", true, 1048576, 65536},
	{"antigravity-gemini-3.7-flash", "Gemini 3.7 Flash (Antigravity)", true, 1048576, 65536},
	{"antigravity-gemini-3.6-flash", "Gemini 3.6 Flash (Antigravity)", true, 1048576, 65536},
	{"antigravity-gemini-3.5-flash", "Gemini 3.5 Flash (Antigravity)", true, 1048576, 65536},
	{"antigravity-gemini-3.1-pro", "Gemini 3.1 Pro (Antigravity)", true, 1048576, 65535},
	{"antigravity-claude-sonnet-4-6-thinking", "Claude Sonnet 4.6 Thinking (Antigravity)", true, 250000, 64000},
	{"antigravity-claude-opus-4-6-thinking", "Claude Opus 4.6 Thinking (Antigravity)", true, 250000, 64000},
	{"antigravity-gpt-oss-120b-medium", "GPT-OSS 120B Medium (Antigravity)", true, 131072, 32768},
}

func providerModels() []any {
	out := make([]any, 0, len(catalog))
	for _, m := range catalog {
		out = append(out, map[string]any{
			"id":            m.ID,
			"name":          m.Name,
			"reasoning":     m.Reasoning,
			"input":         []string{"text", "image"},
			"cost":          map[string]any{"input": 0, "output": 0, "cacheRead": 0, "cacheWrite": 0},
			"contextWindow": m.Context,
			"maxTokens":     m.MaxTokens,
		})
	}
	return out
}

// wireModel is the model name Antigravity receives plus the thinking budget
// that goes with it. Budget -1 means dynamic.
type wireModel struct {
	Name   string
	Budget int
}

// resolveWireModel maps a catalog id and a pig reasoning level to the wire
// model. Only Gemini 3 and Claude thinking models take a tier; every other
// model (GPT-OSS) ignores it. An unknown id is sent as-is, minus the
// "antigravity-" prefix, with no thinking config.
func resolveWireModel(id, reasoning string) (wireModel, bool) {
	tier := strings.ToLower(reasoning)
	switch tier {
	case "minimal":
		tier = "low"
	case "xhigh":
		tier = "high"
	}
	base := strings.TrimPrefix(strings.ToLower(id), "antigravity-")
	for _, t := range []string{"-minimal", "-low", "-medium", "-high", "-xhigh"} {
		if strings.HasPrefix(base, "gemini-3") && strings.HasSuffix(base, t) {
			base = strings.TrimSuffix(base, t)
		}
	}
	switch {
	case strings.HasPrefix(base, "gemini-3.5-flash"):
		switch tier {
		case "low":
			return wireModel{"gemini-3.5-flash-extra-low", 1000}, true
		case "high":
			return wireModel{"gemini-3-flash-agent", 10000}, true
		}
		return wireModel{"gemini-3.5-flash-low", 4000}, true
	case strings.HasPrefix(base, "gemini-3.6-flash"):
		return flashTier(base, tier, 10000), true
	case strings.HasPrefix(base, "gemini-3.7-flash"), strings.HasPrefix(base, "gemini-3.8-flash"):
		return flashTier(base, tier, -1), true
	case strings.HasPrefix(base, "gemini-3.1-pro"):
		if tier == "high" {
			return wireModel{"gemini-pro-agent", 10001}, true
		}
		return wireModel{"gemini-3.1-pro-low", 1001}, true
	case base == "claude-sonnet-4-6-thinking":
		return wireModel{"claude-sonnet-4-6", 1024}, true
	case strings.HasPrefix(base, "claude-") && strings.Contains(base, "thinking"):
		return wireModel{base, 1024}, true
	case base == "gpt-oss-120b-medium":
		return wireModel{base, 8192}, true
	}
	return wireModel{Name: base}, false
}

func flashTier(base, tier string, highBudget int) wireModel {
	switch tier {
	case "low":
		return wireModel{base + "-low", 1000}
	case "high":
		return wireModel{base + "-high", highBudget}
	}
	return wireModel{base + "-medium", 4000}
}

// agyModelEnum is the model_enum label agy attaches to a request.
var agyModelEnum = map[string]string{
	"gemini-3.5-flash-extra-low": "MODEL_PLACEHOLDER_M187",
	"gemini-3.5-flash-low":       "MODEL_PLACEHOLDER_M20",
	"gemini-3-flash-agent":       "MODEL_PLACEHOLDER_M84",
	"gemini-3.6-flash-low":       "MODEL_PLACEHOLDER_M73",
	"gemini-3.6-flash-medium":    "MODEL_PLACEHOLDER_M72",
	"gemini-3.6-flash-high":      "MODEL_PLACEHOLDER_M71",
	"gemini-3.7-flash-low":       "MODEL_PLACEHOLDER_M300",
	"gemini-3.7-flash-medium":    "MODEL_PLACEHOLDER_M299",
	"gemini-3.7-flash-high":      "MODEL_PLACEHOLDER_M298",
	"gemini-3.8-flash-low":       "MODEL_PLACEHOLDER_M320",
	"gemini-3.8-flash-medium":    "MODEL_PLACEHOLDER_M319",
	"gemini-3.8-flash-high":      "MODEL_PLACEHOLDER_M318",
	"gemini-3.1-pro-low":         "MODEL_PLACEHOLDER_M36",
	"gemini-pro-agent":           "MODEL_PLACEHOLDER_M16",
	"claude-sonnet-4-6":          "MODEL_PLACEHOLDER_M35",
	"claude-opus-4-6-thinking":   "MODEL_PLACEHOLDER_M26",
	"gpt-oss-120b-medium":        "MODEL_OPENAI_GPT_OSS_120B_MEDIUM",
}
