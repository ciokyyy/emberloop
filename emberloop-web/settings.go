package web

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// roleSetting is one role's harness and model, as stored under "roles" in
// the shared el settings file. Unknown fields (effort) round-trip.
type roleSetting struct {
	Harness string `json:"harness"`
	Model   string `json:"model"`
	Effort  string `json:"effort,omitempty"`
}

// settingsFile is the first existing settings path, else the first path.
func settingsFile() string {
	paths := settingsPaths()
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if len(paths) > 0 {
		return paths[0]
	}
	return ""
}

func readSettingsMap() (map[string]json.RawMessage, error) {
	all := map[string]json.RawMessage{}
	p := settingsFile()
	if p == "" {
		return all, nil
	}
	raw, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return all, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &all); err != nil {
		return nil, err
	}
	return all, nil
}

func readRoles() map[string]roleSetting {
	roles := map[string]roleSetting{}
	all, err := readSettingsMap()
	if err != nil {
		return roles
	}
	if raw, ok := all["roles"]; ok {
		_ = json.Unmarshal(raw, &roles)
	}
	return roles
}

// roleFor returns the configured harness and model for a role, or empty.
func roleFor(name string) (string, string) {
	r, ok := readRoles()[name]
	if !ok || r.Harness == "" || r.Model == "" {
		return "", ""
	}
	if r.Harness == "claude" && !validClaudeModel(r.Model) {
		return "", ""
	}
	return r.Harness, r.Model
}

// envMaxWorkers reports the EL_MAX_WORKERS override when it sets a
// positive cap. The environment wins over the settings file.
func envMaxWorkers() (int, bool) {
	w, err := strconv.Atoi(strings.TrimSpace(os.Getenv("EL_MAX_WORKERS")))
	if err != nil || w <= 0 {
		return 0, false
	}
	return w, true
}

// settingsView is the GET and PUT shape: roles next to the live cap and
// whether the environment overrides the file, so the UI can lock the field.
func settingsView() map[string]any {
	_, fromEnv := envMaxWorkers()
	return map[string]any{
		"roles":                   readRoles(),
		"max_workers":             maxRunningFromSettings(),
		"max_workers_from_env":    fromEnv,
		"session_titles":          sessionTitlesSetting(),
		"session_titles_from_env": titlesFromEnv(),
		"compress":                readCompressSettings(),
	}
}

func (d *Daemon) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		writeJSON(w, http.StatusOK, settingsView())
	case "PUT":
		defer d.live.publish("settings")
		var body struct {
			Roles         map[string]roleSetting `json:"roles"`
			MaxWorkers    *int                   `json:"max_workers"`
			SessionTitles *bool                  `json:"session_titles"`
			Compress      *compressPatch         `json:"compress"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if msg := body.Compress.validate(); msg != "" {
			http.Error(w, msg, http.StatusBadRequest)
			return
		}
		if body.MaxWorkers != nil && *body.MaxWorkers <= 0 {
			http.Error(w, "max_workers must be a whole number above zero", http.StatusBadRequest)
			return
		}
		for name, rs := range body.Roles {
			// An empty role clears it (pig Sessions then stay unnamed).
			if rs.Harness == "" && rs.Model == "" {
				continue
			}
			if name == pigTitleRole && rs.Harness != "pig" {
				http.Error(w, "role "+name+": harness must be pig", http.StatusBadRequest)
				return
			}
			if rs.Harness != "claude" && rs.Harness != "pig" {
				http.Error(w, "role "+name+": harness must be claude or pig", http.StatusBadRequest)
				return
			}
			if rs.Harness == "claude" && !validClaudeModel(rs.Model) {
				http.Error(w, "role "+name+": unknown claude model", http.StatusBadRequest)
				return
			}
			if rs.Model == "" {
				http.Error(w, "role "+name+": model is required", http.StatusBadRequest)
				return
			}
		}
		all, err := readSettingsMap()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if body.Roles != nil {
			merged := readRoles()
			for name, rs := range body.Roles {
				if rs.Harness == "" && rs.Model == "" {
					delete(merged, name)
					continue
				}
				merged[name] = rs
			}
			raw, _ := json.Marshal(merged)
			all["roles"] = raw
		}
		if body.MaxWorkers != nil {
			raw, _ := json.Marshal(*body.MaxWorkers)
			all["max_workers"] = raw
		}
		if body.SessionTitles != nil {
			raw, _ := json.Marshal(*body.SessionTitles)
			all["session_titles"] = raw
		}
		body.Compress.apply(all)
		out, _ := json.MarshalIndent(all, "", "  ")
		p := settingsFile()
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := os.WriteFile(p, append(out, '\n'), 0o600); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// The live cap reads the file on every check, so the saved value is
		// already in force; the view reports the cap in force, which the
		// environment still wins.
		writeJSON(w, http.StatusOK, settingsView())
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
