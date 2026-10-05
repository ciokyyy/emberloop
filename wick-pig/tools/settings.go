package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
)

// Settings are the user's choices that tools read at call time. They live
// beside the worker store so pig and the MCP server see the same file.
type Settings struct {
	// DecisionModel is the OpenRouter decisions model find asks. Empty means
	// the pinned Jev.
	DecisionModel string `json:"decision_model,omitempty"`
	// Threshold is find's passage cutoff. Zero means 0.20.
	// ponytail: 0.20 is the value old wick measured against jev-1.13; a different
	// decisions model may need its own, so re-measure before trusting it.
	Threshold float64 `json:"threshold,omitempty"`
}

// SettingsPath is where Settings are stored. WICK_SETTINGS overrides it for
// tests and for a second profile.
func SettingsPath() string {
	if p := os.Getenv("WICK_SETTINGS"); p != "" {
		return p
	}
	return filepath.Join(os.Getenv("HOME"), ".pig", "state", "wick", "settings.json")
}

// LoadSettings reads the file; a missing or unreadable file is the defaults.
// WICK_DECISION_MODEL and WICK_FIND_THRESHOLD win over the file.
func LoadSettings() Settings {
	var s Settings
	if raw, err := os.ReadFile(SettingsPath()); err == nil {
		_ = json.Unmarshal(raw, &s)
	}
	if m := os.Getenv("WICK_DECISION_MODEL"); m != "" {
		s.DecisionModel = m
	}
	if t, err := strconv.ParseFloat(os.Getenv("WICK_FIND_THRESHOLD"), 64); err == nil && t > 0 {
		s.Threshold = t
	}
	return s
}

// SaveSettings writes the file, creating its directory.
func SaveSettings(s Settings) error {
	if err := os.MkdirAll(filepath.Dir(SettingsPath()), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(SettingsPath(), append(raw, '\n'), 0o644)
}
