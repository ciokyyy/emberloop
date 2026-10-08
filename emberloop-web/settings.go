package web

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
)

// roleSetting is one role's harness and model, as stored under "roles" in
// the shared emberloop settings file. Unknown fields (effort) round-trip.
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

func (d *Daemon) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		writeJSON(w, http.StatusOK, map[string]any{"roles": readRoles()})
	case "PUT":
		var body struct {
			Roles map[string]roleSetting `json:"roles"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		for name, rs := range body.Roles {
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
		merged := readRoles()
		for name, rs := range body.Roles {
			merged[name] = rs
		}
		raw, _ := json.Marshal(merged)
		all["roles"] = raw
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
		writeJSON(w, http.StatusOK, map[string]any{"roles": merged})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
