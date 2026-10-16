package web

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"

	"example.com/emberloop-web/el/tools/systemone"
)

// findDecisionModelKey is the shared el settings key the find tool reads as
// Settings.DecisionModel (el/tools/settings.go). The browser writes the same
// key `el settings decision-model` writes.
const findDecisionModelKey = "decision_model"

// findModelSuggestions are decisions-model ids offered as quick picks. Any id
// OpenRouter serves under output_modalities=decisions is accepted; the list
// only saves typing.
var findModelSuggestions = []string{
	systemone.JevModelPinned,
	"typesafe/jev-1.14",
	"perplexity/pplx-decider-v1.1-27b",
	"liquid/d1",
	"inception/mercury-decide",
	"respan/span-01",
	"cloudflare/clef-flash",
}

var findModelID = regexp.MustCompile(`^~?[A-Za-z0-9][A-Za-z0-9._:/-]*$`)

// findSettings is the GET shape for the find tool's decision model. Model is
// the saved file value, empty meaning the pinned default. FromEnv locks the
// field while EL_DECISION_MODEL is set.
type findSettings struct {
	DecisionModel string   `json:"decision_model"`
	Default       string   `json:"default"`
	FromEnv       bool     `json:"from_env"`
	Suggestions   []string `json:"suggestions"`
}

func parseFindSettings(all map[string]json.RawMessage) findSettings {
	s := findSettings{
		Default:     systemone.JevModelPinned,
		FromEnv:     strings.TrimSpace(os.Getenv("EL_DECISION_MODEL")) != "",
		Suggestions: findModelSuggestions,
	}
	if raw, ok := all[findDecisionModelKey]; ok {
		var str string
		if json.Unmarshal(raw, &str) == nil {
			s.DecisionModel = strings.TrimSpace(str)
		}
	}
	return s
}

func readFindSettings() findSettings {
	all, err := readSettingsMap()
	if err != nil {
		all = map[string]json.RawMessage{}
	}
	return parseFindSettings(all)
}

// findPatch is the PUT shape. An empty decision_model clears the key, so find
// returns to the pinned default.
type findPatch struct {
	DecisionModel *string `json:"decision_model"`
}

func (p *findPatch) validate() string {
	if p == nil || p.DecisionModel == nil {
		return ""
	}
	m := strings.TrimSpace(*p.DecisionModel)
	if m == "" {
		return ""
	}
	if len(m) > 128 {
		return "find decision_model is limited to 128 characters"
	}
	if !findModelID.MatchString(m) {
		return "find decision_model must be one OpenRouter model id"
	}
	return ""
}

func (p *findPatch) apply(all map[string]json.RawMessage) {
	if p == nil || p.DecisionModel == nil {
		return
	}
	m := strings.TrimSpace(*p.DecisionModel)
	if m == "" {
		delete(all, findDecisionModelKey)
		return
	}
	raw, _ := json.Marshal(systemone.CanonicalModel(m))
	all[findDecisionModelKey] = raw
}
