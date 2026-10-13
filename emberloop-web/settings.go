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
	return readSettingsFileMap(p)
}

// readSettingsFileMap reads one settings path: a missing file means empty
// settings, a broken one errors.
func readSettingsFileMap(p string) (map[string]json.RawMessage, error) {
	all := map[string]json.RawMessage{}
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

// SetSettingsPath points this Daemon at a settings file for tests; empty
// restores the host file. Call it before serving: the write happens before
// the server's goroutines start, so no lock is needed.
func (d *Daemon) SetSettingsPath(p string) {
	d.settingsPath = p
}

// settingsMap reads this Daemon's settings: the override file when set, else
// the host file. Production leaves the override empty, so this matches
// readSettingsMap exactly.
func (d *Daemon) settingsMap() (map[string]json.RawMessage, error) {
	if d != nil && d.settingsPath != "" {
		return readSettingsFileMap(d.settingsPath)
	}
	return readSettingsMap()
}

func readRoles() map[string]roleSetting {
	all, err := readSettingsMap()
	if err != nil {
		return map[string]roleSetting{}
	}
	return parseRoles(all)
}

// parseRoles decodes the roles map: shared by the host reader and the
// Daemon-scoped test seam so the two never diverge.
func parseRoles(all map[string]json.RawMessage) map[string]roleSetting {
	roles := map[string]roleSetting{}
	if raw, ok := all["roles"]; ok {
		_ = json.Unmarshal(raw, &roles)
	}
	return roles
}

// settingsRoles reads this Daemon's roles: the override file when set, else
// the host file.
func (d *Daemon) settingsRoles() map[string]roleSetting {
	all, err := d.settingsMap()
	if err != nil {
		return map[string]roleSetting{}
	}
	return parseRoles(all)
}

// roleFor returns the configured harness and model for a role, or empty.
func roleFor(name string) (string, string) {
	return lookupRole(readRoles(), name)
}

// lookupRole resolves one role from a decoded roles map: shared by the host
// reader and the Daemon-scoped test seam so the two never diverge.
func lookupRole(roles map[string]roleSetting, name string) (string, string) {
	r, ok := roles[name]
	if !ok || r.Harness == "" || r.Model == "" {
		return "", ""
	}
	if r.Harness == "claude" && !validClaudeModel(r.Model) {
		return "", ""
	}
	return r.Harness, r.Model
}

// daemonRoleFor resolves one role from this Daemon's settings.
func (d *Daemon) daemonRoleFor(name string) (string, string) {
	return lookupRole(d.settingsRoles(), name)
}

// envMaxTickets reports the EL_MAX_TICKETS override when it sets a
// positive cap. The environment wins over the settings file. EL_MAX_WORKERS
// still works as a fallback when EL_MAX_TICKETS is unset, so old
// environments keep working; it is never written anywhere.
func envMaxTickets() (int, bool) {
	if w, err := strconv.Atoi(strings.TrimSpace(os.Getenv("EL_MAX_TICKETS"))); err == nil && w > 0 {
		return w, true
	}
	if w, err := strconv.Atoi(strings.TrimSpace(os.Getenv("EL_MAX_WORKERS"))); err == nil && w > 0 {
		return w, true
	}
	return 0, false
}

// firstSetTicketCap prefers the renamed key: max_tickets wins when both
// arrive, max_workers is only the deprecated alias. Nil when neither does.
func firstSetTicketCap(primary, alias *int) *int {
	if primary != nil {
		return primary
	}
	return alias
}

// settingsView is the GET and PUT shape: roles, per-role Run skills and
// per-role chains next to the live cap and whether the environment overrides
// the file, so the UI can lock the field.
func settingsView() map[string]any {
	_, fromEnv := envMaxTickets()
	decider := readDeciderSettings()
	return map[string]any{
		"roles":                   readRoles(),
		"run_skills":              readRunSkillsRaw(),
		"chains":                  readChains(),
		"max_tickets":             maxTicketsFromSettings(),
		"max_tickets_from_env":    fromEnv,
		"session_titles":          sessionTitlesSetting(),
		"session_titles_from_env": titlesFromEnv(),
		"compress":                readCompressSettings(),
		"signup_enabled":          signupEnabledSetting(),
		"compress_note":           compressNote(readCompressSettings().Model),
		"decider":                 decider,
		"decider_note":            deciderNote(decider),
		"jev":                     readJevSettings(),
	}
}

func (d *Daemon) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		writeJSON(w, http.StatusOK, settingsView())
	case "PUT":
		defer d.live.publish("settings")
		var body struct {
			Roles         map[string]roleSetting  `json:"roles"`
			RunSkills     json.RawMessage         `json:"run_skills"`
			Chains        map[string][]chainEntry `json:"chains"`
			MaxTickets    *int                    `json:"max_tickets"`
			MaxWorkers    *int                    `json:"max_workers"`
			SessionTitles *bool                   `json:"session_titles"`
			SignupEnabled *bool                   `json:"signup_enabled"`
			Compress      *compressPatch          `json:"compress"`
			Decider       *deciderPatch           `json:"decider"`
			Jev           *jevPatch               `json:"jev"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		// An explicit run_skills key (including null/empty) clears or merges;
		// an absent key leaves custom skills untouched. RawMessage is nil
		// when absent, so one decode detects presence without a second struct.
		var runSkills map[string][]string
		runSkillsSet := body.RunSkills != nil
		if runSkillsSet {
			trimmed := strings.TrimSpace(string(body.RunSkills))
			if trimmed != "" && trimmed != "null" {
				if err := json.Unmarshal(body.RunSkills, &runSkills); err != nil {
					http.Error(w, "run_skills must be an object of string arrays", http.StatusBadRequest)
					return
				}
			} else {
				runSkills = map[string][]string{}
			}
		}
		// run_skills is host-global prompt text: it is injected verbatim into
		// every Run's Worker and Reviewer first prompts, including other
		// Users' Runs, so like signup, decider and jev it writes admin-only.
		if body.SignupEnabled != nil || body.Decider != nil || body.Jev != nil || runSkillsSet {
			if who, ok := d.requesterOf(r); !ok || who.user == nil || !who.user.Admin {
				http.Error(w, "only the admin can change signup, decider, jev and run_skills settings", http.StatusForbidden)
				return
			}
		}
		if msg := body.Compress.validate(); msg != "" {
			http.Error(w, msg, http.StatusBadRequest)
			return
		}
		if msg := body.Decider.validate(); msg != "" {
			http.Error(w, msg, http.StatusBadRequest)
			return
		}
		if msg := body.Jev.validate(); msg != "" {
			http.Error(w, msg, http.StatusBadRequest)
			return
		}
		if body.MaxTickets != nil && *body.MaxTickets <= 0 {
			http.Error(w, "max_tickets must be a whole number above zero", http.StatusBadRequest)
			return
		}
		if body.MaxWorkers != nil && *body.MaxWorkers <= 0 {
			http.Error(w, "max_tickets must be a whole number above zero", http.StatusBadRequest)
			return
		}
		if runSkillsSet {
			if msg := validateRunSkills(runSkills); msg != "" {
				http.Error(w, msg, http.StatusBadRequest)
				return
			}
		}
		if msg := validateChains(body.Chains); msg != "" {
			http.Error(w, msg, http.StatusBadRequest)
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
			if rs.Harness != "claude" && rs.Harness != "pig" && rs.Harness != "grok" {
				http.Error(w, "role "+name+": harness must be claude, pig or grok", http.StatusBadRequest)
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
		if body.Chains != nil {
			merged := readChains()
			for name, entries := range body.Chains {
				if len(entries) == 0 {
					delete(merged, name)
					continue
				}
				merged[name] = entries
			}
			raw, _ := json.Marshal(merged)
			all["chains"] = raw
		}
		// max_tickets wins when both arrive; the old key is only a
		// deprecated alias and is never written back.
		if tickets := firstSetTicketCap(body.MaxTickets, body.MaxWorkers); tickets != nil {
			raw, _ := json.Marshal(*tickets)
			all["max_tickets"] = raw
		}
		if body.SessionTitles != nil {
			raw, _ := json.Marshal(*body.SessionTitles)
			all["session_titles"] = raw
		}
		if body.SignupEnabled != nil {
			raw, _ := json.Marshal(*body.SignupEnabled)
			all["signup_enabled"] = raw
		}
		if runSkillsSet {
			merged := readRunSkillsRaw()
			for role, list := range runSkills {
				kept := cleanSkillList(list)
				if len(kept) == 0 {
					delete(merged, role)
					continue
				}
				merged[role] = kept
			}
			// An explicit empty object clears all custom skills.
			if len(runSkills) == 0 {
				merged = map[string][]string{}
			}
			raw, _ := json.Marshal(merged)
			all["run_skills"] = raw
		}
		body.Compress.apply(all)
		body.Decider.apply(all)
		body.Jev.apply(all)
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

// signupEnabledSetting reads signup_enabled. Unset means on. An unreadable
// settings file means off, so signup fails closed.
func signupEnabledSetting() bool {
	all, err := readSettingsMap()
	if err != nil {
		return false
	}
	raw, ok := all["signup_enabled"]
	if !ok {
		return true
	}
	var on bool
	if err := json.Unmarshal(raw, &on); err != nil {
		return false
	}
	return on
}
