package web

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"example.com/emberloop-web/el/tools/systemone"
	"example.com/emberloop-web/memory"
)

// roleSetting is a legacy role's harness, model and effort in the shared el
// settings file. The Agents migration mirrors each configured chain primary.
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
	for name, rs := range roles {
		if rs.Harness == "grok" {
			log.Printf("emberloop: dropped grok role setting %s", name)
			delete(roles, name)
		}
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

// roleFor returns the configured harness and model for a legacy role key.
func roleFor(name string) (string, string) {
	return lookupRole(readRoles(), name)
}

// lookupRole resolves a legacy role setting, shared by the host reader and
// the Daemon-scoped test seam.
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

// lookupAgentRoleSetting reads one Session-agent primary from the merged
// chain form, with the pre-redesign role setting as a fallback for old files.
func lookupAgentRoleSetting(chains map[string][]chainEntry, roles map[string]roleSetting, name string) roleSetting {
	chainName := name
	if name == "worker" {
		chainName = "subagent"
	}
	if entries, ok := chains[chainName]; ok {
		if len(entries) == 0 {
			return roleSetting{}
		}
		entry := entries[0]
		candidate := roleSetting{Harness: entry.Backend, Model: entry.Model, Effort: entry.Effort}
		if _, model := lookupRole(map[string]roleSetting{name: candidate}, name); model == "" {
			return roleSetting{}
		}
		return candidate
	}
	return roles[name]
}

func (d *Daemon) daemonRoleSettingFor(name string) roleSetting {
	return lookupAgentRoleSetting(d.settingsChains(), d.settingsRoles(), name)
}

// daemonRoleFor resolves the configured harness and model for a Session role.
func (d *Daemon) daemonRoleFor(name string) (string, string) {
	r := d.daemonRoleSettingFor(name)
	if h, m := lookupRole(map[string]roleSetting{name: r}, name); h != "" {
		return h, m
	}
	return "", ""
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

// settingsView exposes host-global Memory controls only to admins.
func settingsView(memorySettings ...map[string]any) map[string]any {
	_, fromEnv := envMaxTickets()
	decider := readDeciderSettings()
	view := map[string]any{
		"roles": readRoles(), "run_skills": readRunSkillsRaw(), "chains": agentChainsView(),
		"max_tickets": maxTicketsFromSettings(), "max_tickets_from_env": fromEnv,
		"session_titles": sessionTitlesSetting(), "session_titles_from_env": titlesFromEnv(),
		"compress": readCompressSettings(), "signup_enabled": signupEnabledSetting(),
		"compress_note": compressNote(readCompressSettings().Model), "decider": decider,
		"decider_note": deciderNote(decider), "jev": readJevSettings(),
		"find":                 readFindSettings(),
		"disk_go_cache_cap_gb": diskGoCacheCapGB(), "oauth_redirect_base_url": oauthRedirectBaseURLSetting(),
	}
	if len(memorySettings) > 0 {
		for key, value := range memorySettings[0] {
			view[key] = value
		}
	}
	return view
}

func (d *Daemon) memorySettingsView() map[string]any {
	all, _ := d.settingsMap()
	model, effort, embedding := memory.DefaultExtractorModel, "low", memory.DefaultEmbeddingModel
	decodeStringSetting(all, "memory_extract_model", &model)
	decodeStringSetting(all, "memory_extract_effort", &effort)
	decodeStringSetting(all, "memory_embedding_model", &embedding)
	weights := map[string]float64{"vector": 0.5, "fts": 0.3, "importance": 0.2}
	if raw, ok := all["memory_recall_weights"]; ok {
		_ = json.Unmarshal(raw, &weights)
	}
	auto := systemone.APIKey(context.Background()) != ""
	if raw, ok := all["memory_auto_extract"]; ok {
		_ = json.Unmarshal(raw, &auto)
	}
	return map[string]any{"memory_extract_model": model, "memory_extract_effort": effort, "memory_embedding_model": embedding, "memory_recall_weights": weights, "memory_auto_extract": auto}
}

func decodeStringSetting(all map[string]json.RawMessage, key string, value *string) {
	if raw, ok := all[key]; ok {
		_ = json.Unmarshal(raw, value)
	}
}

func (d *Daemon) settingsViewFor(r *http.Request) map[string]any {
	if who, ok := d.requesterOf(r); ok && who.user != nil && who.user.Admin {
		return settingsView(d.memorySettingsView())
	}
	return settingsView()
}

func validateMemoryRecallWeights(raw json.RawMessage) string {
	var weights struct {
		Vector     *float64 `json:"vector"`
		FTS        *float64 `json:"fts"`
		Importance *float64 `json:"importance"`
	}
	if err := json.Unmarshal(raw, &weights); err != nil || weights.Vector == nil || weights.FTS == nil || weights.Importance == nil {
		return "memory_recall_weights must contain vector, fts and importance numbers"
	}
	values := []float64{*weights.Vector, *weights.FTS, *weights.Importance}
	total := 0.0
	for _, value := range values {
		if value < 0 || value > 1 {
			return "memory_recall_weights must be between 0 and 1"
		}
		total += value
	}
	if total == 0 {
		return "memory_recall_weights must sum to more than 0"
	}
	return ""
}

func (d *Daemon) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		writeJSON(w, http.StatusOK, d.settingsViewFor(r))
	case "PUT":
		defer d.live.publish("settings")
		var body struct {
			OAuthRedirectBaseURL *string                 `json:"oauth_redirect_base_url"`
			Roles                map[string]roleSetting  `json:"roles"`
			RunSkills            json.RawMessage         `json:"run_skills"`
			Chains               map[string][]chainEntry `json:"chains"`
			MaxTickets           *int                    `json:"max_tickets"`
			MaxWorkers           *int                    `json:"max_workers"`
			SessionTitles        *bool                   `json:"session_titles"`
			SignupEnabled        *bool                   `json:"signup_enabled"`
			DiskGoCacheCapGB     *int64                  `json:"disk_go_cache_cap_gb"`
			Compress             *compressPatch          `json:"compress"`
			Decider              *deciderPatch           `json:"decider"`
			Jev                  *jevPatch               `json:"jev"`
			MemoryExtractModel   *string                 `json:"memory_extract_model"`
			MemoryExtractEffort  *string                 `json:"memory_extract_effort"`
			MemoryEmbeddingModel *string                 `json:"memory_embedding_model"`
			MemoryRecallWeights  json.RawMessage         `json:"memory_recall_weights"`
			MemoryAutoExtract    *bool                   `json:"memory_auto_extract"`
			Find                 *findPatch              `json:"find"`
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
		// Users' Runs, so like signup, decider, jev and the disk cap it
		// writes admin-only.
		// The redirect base is a host-global OAuth setting, so it follows the
		// same admin-only write rule as other host-global settings.
		memorySettingsSet := body.MemoryExtractModel != nil || body.MemoryExtractEffort != nil || body.MemoryEmbeddingModel != nil || body.MemoryRecallWeights != nil || body.MemoryAutoExtract != nil
		if memorySettingsSet {
			if who, ok := d.requesterOf(r); !ok || who.user == nil || !who.user.Admin {
				http.Error(w, "only the admin can change Memory settings", http.StatusForbidden)
				return
			}
		}
		if body.MemoryExtractEffort != nil && *body.MemoryExtractEffort != "off" && *body.MemoryExtractEffort != "low" && *body.MemoryExtractEffort != "medium" {
			http.Error(w, "memory_extract_effort must be off, low or medium", http.StatusBadRequest)
			return
		}
		if body.MemoryRecallWeights != nil {
			if msg := validateMemoryRecallWeights(body.MemoryRecallWeights); msg != "" {
				http.Error(w, msg, http.StatusBadRequest)
				return
			}
		}
		if body.SignupEnabled != nil || body.Decider != nil || body.Jev != nil || body.Find != nil || runSkillsSet || body.DiskGoCacheCapGB != nil || body.OAuthRedirectBaseURL != nil {
			if who, ok := d.requesterOf(r); !ok || who.user == nil || !who.user.Admin {
				http.Error(w, "only the admin can change signup, decider, jev, find, run_skills, disk_go_cache_cap_gb and oauth_redirect_base_url settings", http.StatusForbidden)
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
		if msg := body.Find.validate(); msg != "" {
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
		if body.DiskGoCacheCapGB != nil && *body.DiskGoCacheCapGB < 0 {
			http.Error(w, "disk_go_cache_cap_gb must be zero or above", http.StatusBadRequest)
			return
		}
		if body.OAuthRedirectBaseURL != nil {
			normalized, err := normalizeOAuthRedirectBaseURL(*body.OAuthRedirectBaseURL)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			*body.OAuthRedirectBaseURL = normalized
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
		if body.Chains != nil {
			var version int
			alreadyCanonical := json.Unmarshal(all[agentChainsVersionKey], &version) == nil && version >= 1
			merged := canonicalAgentChains(readChains(), body.Chains, readRoles(), alreadyCanonical)
			raw, _ := json.Marshal(merged)
			all["chains"] = raw
			all[agentChainsVersionKey] = json.RawMessage("1")
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
		if body.DiskGoCacheCapGB != nil {
			raw, _ := json.Marshal(*body.DiskGoCacheCapGB)
			all["disk_go_cache_cap_gb"] = raw
		}
		if body.OAuthRedirectBaseURL != nil {
			raw, _ := json.Marshal(*body.OAuthRedirectBaseURL)
			all[oauthRedirectBaseURLSettingKey] = raw
		}
		if body.MemoryExtractModel != nil {
			raw, _ := json.Marshal(strings.TrimSpace(*body.MemoryExtractModel))
			all["memory_extract_model"] = raw
		}
		if body.MemoryExtractEffort != nil {
			raw, _ := json.Marshal(*body.MemoryExtractEffort)
			all["memory_extract_effort"] = raw
		}
		if body.MemoryEmbeddingModel != nil {
			raw, _ := json.Marshal(strings.TrimSpace(*body.MemoryEmbeddingModel))
			all["memory_embedding_model"] = raw
		}
		if body.MemoryRecallWeights != nil {
			all["memory_recall_weights"] = body.MemoryRecallWeights
		}
		if body.MemoryAutoExtract != nil {
			raw, _ := json.Marshal(*body.MemoryAutoExtract)
			all["memory_auto_extract"] = raw
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
		body.Find.apply(all)
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
		if memorySettingsSet {
			d.reconfigureMemoryEmbedding()
		}
		// The live cap reads the file on every check, so the saved value is
		// already in force; the view reports the cap in force, which the
		// environment still wins.
		writeJSON(w, http.StatusOK, d.settingsViewFor(r))
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

// oauthRedirectBaseURLSetting reads the validated optional base shared by the
// Settings view and any future browser OAuth flow.
func oauthRedirectBaseURLSetting() string {
	all, err := readSettingsMap()
	if err != nil {
		return ""
	}
	return oauthRedirectBaseFromSettings(all)
}
