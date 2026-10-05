package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
)

// Role is the default harness, model and effort for one launch slot. An empty
// field means "nothing chosen": the resolver falls back to the built-in
// default.
type Role struct {
	Harness string `json:"harness,omitempty"`
	Model   string `json:"model,omitempty"`
	Effort  string `json:"effort,omitempty"`
}

// RoleSlots are the launch slots a settings file may set, in print order.
var RoleSlots = []string{"orchestrator", "implementer", "code-review", "spec-review", "scout"}

// IsRoleSlot reports whether name names a launch slot.
func IsRoleSlot(name string) bool {
	for _, s := range RoleSlots {
		if s == name {
			return true
		}
	}
	return false
}

// Settings are the user's choices that tools read at call time. They live in
// the user's wick-pig directory, beside the skills, so pig, the MCP server and
// the TUI all read the same file.
type Settings struct {
	// DecisionModel is the OpenRouter decisions model find asks. Empty means
	// the pinned Jev.
	DecisionModel string `json:"decision_model,omitempty"`
	// Threshold is find's passage cutoff. Zero means 0.20.
	// ponytail: 0.20 is the value old wick measured against jev-1.13; a different
	// decisions model may need its own, so re-measure before trusting it.
	Threshold float64 `json:"threshold,omitempty"`
	// Roles holds one default per launch slot, keyed by slot name.
	Roles map[string]Role `json:"roles,omitempty"`

	// extra holds top-level keys wick does not know yet, so a save writes them
	// back and the file can grow without a version bump. Never written from the
	// struct itself; SaveSettings merges it in.
	extra map[string]json.RawMessage
	// roleExtra holds keys inside a role object that wick does not know yet,
	// keyed by slot then field. SaveSettings merges it into Roles.
	roleExtra map[string]map[string]json.RawMessage
	// src names where a top-level value came from: "file", "env" or "default".
	// Print uses it to show a value's origin.
	src map[string]string
}

// SourceOf names where a top-level value came from: "file", "env" or
// "default".
func (s Settings) SourceOf(key string) string {
	if where, ok := s.src[key]; ok {
		return where
	}
	return "default"
}

// SettingsPath is where settings are saved. WICK_SETTINGS overrides it for
// tests and for a second profile.
func SettingsPath() string {
	if p := os.Getenv("WICK_SETTINGS"); p != "" {
		return p
	}
	return filepath.Join(os.Getenv("HOME"), ".wick-pig", "settings.json")
}

// oldSettingsPath is the state-dir file settings lived in before the move
// beside the skills. It is read, never written, so an upgrade loses nothing.
func oldSettingsPath() string {
	return filepath.Join(os.Getenv("HOME"), ".pig", "state", "wick", "settings.json")
}

// LoadSettings reads the file; a missing or unreadable file is the defaults.
// When WICK_SETTINGS is unset and the new file is absent, the old state-dir
// file is read instead. WICK_DECISION_MODEL and WICK_FIND_THRESHOLD win over
// the file.
func LoadSettings() Settings {
	var s Settings
	if raw, err := os.ReadFile(SettingsPath()); err == nil {
		s = decodeSettings(raw)
	} else if os.Getenv("WICK_SETTINGS") == "" {
		// An explicit override is the whole world: it must not quietly read the
		// machine's old file too, or a hermetic test reads the developer's data.
		if raw, err := os.ReadFile(oldSettingsPath()); err == nil {
			s = decodeSettings(raw)
		}
	}
	if m := os.Getenv("WICK_DECISION_MODEL"); m != "" {
		s.DecisionModel = m
		s.setSource("decision_model", "env")
	}
	if t, err := strconv.ParseFloat(os.Getenv("WICK_FIND_THRESHOLD"), 64); err == nil && t > 0 {
		s.Threshold = t
		s.setSource("threshold", "env")
	}
	return s
}

// decodeSettings parses one settings file. Garbage is the defaults; the known
// keys fill the struct and the rest are kept for the next save.
func decodeSettings(raw []byte) Settings {
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return Settings{}
	}
	var s Settings
	_ = json.Unmarshal(raw, &s)
	s.src = map[string]string{}
	s.extra = map[string]json.RawMessage{}
	for k, v := range fields {
		switch k {
		case "decision_model", "threshold", "roles":
		default:
			s.extra[k] = v
		}
	}
	// A value that failed to parse or is empty stays the default, and the
	// source has to say so: the value and the name it prints must agree.
	if s.DecisionModel != "" {
		s.src["decision_model"] = "file"
	}
	if s.Threshold != 0 {
		s.src["threshold"] = "file"
	}
	s.roleExtra = unknownRoleFields(fields["roles"])
	return s
}

// unknownRoleFields collects, per slot, the keys inside a role object that are
// not harness, model or effort, so a save writes them back.
func unknownRoleFields(rawRoles json.RawMessage) map[string]map[string]json.RawMessage {
	var perSlot map[string]map[string]json.RawMessage
	if err := json.Unmarshal(rawRoles, &perSlot); err != nil {
		return nil
	}
	var out map[string]map[string]json.RawMessage
	for slot, roleFields := range perSlot {
		for k, v := range roleFields {
			switch k {
			case "harness", "model", "effort":
			default:
				if out == nil {
					out = map[string]map[string]json.RawMessage{}
				}
				if out[slot] == nil {
					out[slot] = map[string]json.RawMessage{}
				}
				out[slot][k] = v
			}
		}
	}
	return out
}

func (s *Settings) setSource(key, where string) {
	if s.src == nil {
		s.src = map[string]string{}
	}
	s.src[key] = where
}

// SaveSettings writes the file, creating its directory. Keys wick does not
// know survive the round trip, at the top level and inside a role object.
func SaveSettings(s Settings) error {
	known, err := json.Marshal(s)
	if err != nil {
		return err
	}
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(known, &fields); err != nil {
		return err
	}
	for k, v := range s.extra {
		if _, ok := fields[k]; !ok {
			fields[k] = v
		}
	}
	if err := mergeRoleExtras(fields, s.roleExtra); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(SettingsPath()), 0o755); err != nil {
		return err
	}
	return os.WriteFile(SettingsPath(), append(raw, '\n'), 0o644)
}

// mergeRoleExtras writes the unknown fields of each role back into the roles
// object already in fields.
func mergeRoleExtras(fields map[string]json.RawMessage, roleExtra map[string]map[string]json.RawMessage) error {
	if len(roleExtra) == 0 {
		return nil
	}
	roles := map[string]map[string]json.RawMessage{}
	if raw, ok := fields["roles"]; ok {
		if err := json.Unmarshal(raw, &roles); err != nil {
			return err
		}
	}
	for slot, extras := range roleExtra {
		if roles[slot] == nil {
			roles[slot] = map[string]json.RawMessage{}
		}
		for k, v := range extras {
			if _, ok := roles[slot][k]; !ok {
				roles[slot][k] = v
			}
		}
	}
	raw, err := json.Marshal(roles)
	if err != nil {
		return err
	}
	fields["roles"] = raw
	return nil
}
