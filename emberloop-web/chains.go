package web

import (
	"encoding/json"
	"fmt"
)

// Agent chains store each role's harness, model and effort in one ordered
// list. Run chains keep fallback semantics: quota, credit, auth or session
// limit failures start a new Session on the next entry; any other failure
// surfaces as today.

// maxChainEntries caps one role's fallback chain.
const maxChainEntries = 3

// chainEntry is one role's harness, model and optional thinking effort.
type chainEntry struct {
	Backend string `json:"backend"`
	Model   string `json:"model"`
	Effort  string `json:"effort,omitempty"`
}

// UnmarshalJSON accepts `harness` as a legacy alias for the canonical
// `backend` field in stored and submitted chain entries.
func (e *chainEntry) UnmarshalJSON(raw []byte) error {
	var in struct {
		Backend string `json:"backend"`
		Harness string `json:"harness"`
		Model   string `json:"model"`
		Effort  string `json:"effort,omitempty"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return err
	}
	e.Backend = in.Backend
	if e.Backend == "" {
		e.Backend = in.Harness
	}
	e.Model = in.Model
	e.Effort = in.Effort
	return nil
}

// chainRoleGroups are the canonical run roles with their settings aliases,
// mirroring roleWorkerConfig and roleReviewerConfig in runs_exec.go: the
// worker slot is "worker" (older "implementer"), the reviewers are
// "reviewer-spec" ("spec-review") and "reviewer-standards" ("code-review").
var chainRoleGroups = [][]string{
	{"worker", "implementer"},
	{reviewerSpecType, "spec-review"},
	{reviewerStandardsType, "code-review"},
}

// chainNames lists the settings keys chainFor checks for a role: its alias
// group when it has one, else the role itself.
func chainNames(role string) []string {
	for _, group := range chainRoleGroups {
		for _, name := range group {
			if name == role {
				return group
			}
		}
	}
	return []string{role}
}

// agentChainRoles are the six slots shown on Settings > Agents. The legacy
// roles map remains available to the operator CLI; the browser migrates these
// model choices into chains on read and persists the merged map on save.
var agentChainRoles = []struct {
	key          string
	chainAliases []string
	roleAliases  []string
}{
	{key: "orchestrator", roleAliases: []string{"orchestrator"}},
	{key: "subagent", roleAliases: []string{"worker"}},
	{key: "scout", roleAliases: []string{"scout"}},
	{key: "worker", chainAliases: []string{"implementer"}, roleAliases: []string{"implementer", "worker"}},
	{key: reviewerSpecType, chainAliases: []string{"spec-review"}, roleAliases: []string{reviewerSpecType, "spec-review"}},
	{key: reviewerStandardsType, chainAliases: []string{"code-review"}, roleAliases: []string{reviewerStandardsType, "code-review"}},
}

const agentChainsVersionKey = "agents_chains_version"

func agentChainsView() map[string][]chainEntry {
	all, err := readSettingsMap()
	if err != nil {
		return map[string][]chainEntry{}
	}
	var version int
	if raw := all[agentChainsVersionKey]; json.Unmarshal(raw, &version) == nil && version >= 1 {
		return parseChains(all)
	}
	return mergedAgentChains(parseChains(all), parseRoles(all))
}

// mergedAgentChains returns the canonical Agents-page chains while retaining
// unrelated chain keys. Legacy role models become each Run role's primary;
// old Run Chain entries follow as fallbacks, with duplicate entries removed.
func mergedAgentChains(chains map[string][]chainEntry, roles map[string]roleSetting) map[string][]chainEntry {
	out := make(map[string][]chainEntry, len(chains)+len(agentChainRoles))
	known := make(map[string]bool, len(agentChainRoles)*2)
	for _, role := range agentChainRoles {
		known[role.key] = true
		for _, alias := range role.chainAliases {
			known[alias] = true
		}
	}
	for name, entries := range chains {
		if !known[name] {
			out[name] = append([]chainEntry(nil), entries...)
		}
	}
	for _, role := range agentChainRoles {
		var legacyEntries []chainEntry
		if entries, ok := chains[role.key]; ok {
			legacyEntries = entries
		} else {
			for _, alias := range role.chainAliases {
				if entries, ok := chains[alias]; ok {
					legacyEntries = entries
					break
				}
			}
		}
		entries := make([]chainEntry, 0, maxChainEntries)
		appendUnique := func(candidate chainEntry) {
			if candidate.Backend == "" || candidate.Model == "" || len(entries) >= maxChainEntries {
				return
			}
			for _, existing := range entries {
				if existing == candidate {
					return
				}
			}
			entries = append(entries, candidate)
		}
		for _, alias := range role.roleAliases {
			if setting, ok := roles[alias]; ok {
				appendUnique(chainEntry{Backend: setting.Harness, Model: setting.Model, Effort: setting.Effort})
				if len(entries) > 0 {
					break
				}
			}
		}
		for _, entry := range legacyEntries {
			appendUnique(entry)
		}
		out[role.key] = entries
	}
	return out
}

// canonicalAgentChains applies a partial write to the migrated chain view.
// Once the settings file has the version marker, its canonical chains are
// authoritative, including explicit empty chains.
func canonicalAgentChains(current, updates map[string][]chainEntry, roles map[string]roleSetting, alreadyCanonical bool) map[string][]chainEntry {
	var out map[string][]chainEntry
	if alreadyCanonical {
		out = make(map[string][]chainEntry, len(current)+len(agentChainRoles))
		for name, entries := range current {
			out[name] = append([]chainEntry(nil), entries...)
		}
		for _, role := range agentChainRoles {
			if _, ok := out[role.key]; !ok {
				out[role.key] = []chainEntry{}
			}
			for _, alias := range role.chainAliases {
				delete(out, alias)
			}
		}
	} else {
		out = mergedAgentChains(current, roles)
	}
	for _, role := range agentChainRoles {
		if entries, ok := updates[role.key]; ok {
			out[role.key] = append([]chainEntry(nil), entries...)
			continue
		}
		for _, alias := range role.chainAliases {
			if entries, ok := updates[alias]; ok {
				out[role.key] = append([]chainEntry(nil), entries...)
				break
			}
		}
	}
	for name, entries := range updates {
		canonical := false
		for _, role := range agentChainRoles {
			if name == role.key {
				canonical = true
				break
			}
			for _, alias := range role.chainAliases {
				if name == alias {
					canonical = true
					break
				}
			}
		}
		if !canonical {
			if len(entries) == 0 {
				delete(out, name)
			} else {
				out[name] = append([]chainEntry(nil), entries...)
			}
		}
	}
	return out
}

// readChains returns the stored per-role chains. A missing or unreadable
// settings file means no chains.
func readChains() map[string][]chainEntry {
	all, err := readSettingsMap()
	if err != nil {
		return map[string][]chainEntry{}
	}
	return parseChains(all)
}

// parseChains decodes the stored chains: shared by the host reader and the
// Daemon-scoped test seam so the two never diverge.
func parseChains(all map[string]json.RawMessage) map[string][]chainEntry {
	chains := map[string][]chainEntry{}
	if raw, ok := all["chains"]; ok {
		_ = json.Unmarshal(raw, &chains)
	}
	for name, entries := range chains {
		out := make([]chainEntry, 0, len(entries))
		removed := false
		for _, e := range entries {
			if e.Backend == "grok" {
				removed = true
				continue
			}
			out = append(out, e)
		}
		if removed && len(out) == 0 {
			delete(chains, name)
		} else if removed {
			chains[name] = out
		}
	}
	return chains
}

// settingsChains reads this Daemon's stored chains.
func (d *Daemon) settingsChains() map[string][]chainEntry {
	all, err := d.settingsMap()
	if err != nil {
		return map[string][]chainEntry{}
	}
	return parseChains(all)
}

// chainFor returns the active Run chain for a role, including its legacy
// aliases and the one-entry role setting used before Chains existed.
func chainFor(role string) []chainEntry {
	return lookupChain(readChains(), readRoles(), role)
}

// daemonChainFor returns this Daemon's active fallback chain for a role.
func (d *Daemon) daemonChainFor(role string) []chainEntry {
	return lookupChain(d.settingsChains(), d.settingsRoles(), role)
}

// lookupChain resolves a role against decoded chains and roles maps: shared
// by the host reader and the Daemon-scoped test seam. An explicit empty chain
// suppresses legacy role fallbacks.
func lookupChain(chains map[string][]chainEntry, roles map[string]roleSetting, role string) []chainEntry {
	names := chainNames(role)
	for _, name := range names {
		if c, ok := chains[name]; ok {
			valid := make([]chainEntry, 0, min(len(c), maxChainEntries))
			for i, entry := range c {
				if validateChainEntry(name, i, entry) == "" {
					valid = append(valid, entry)
					if len(valid) == maxChainEntries {
						break
					}
				}
			}
			return valid
		}
	}
	for _, name := range names {
		if r, ok := roles[name]; ok && r.Harness != "" && r.Model != "" {
			if r.Harness == "claude" && !validClaudeModel(r.Model) {
				continue
			}
			return []chainEntry{{Backend: r.Harness, Model: r.Model, Effort: r.Effort}}
		}
	}
	return nil
}

// chainAdvances reports whether a Worker or Reviewer failure moves the Run
// to the next chain entry: quota, credit, auth and session-limit failures
// do (failures.go), anything else surfaces as today. Ticket 14 runs the
// advance; this is its predicate.
func chainAdvances(err error) bool { return classifyFailure(err).advancesChain() }

// validateChains checks a PUT chains body: each role holds one to three
// complete entries. An empty list clears the role and is always valid. Role
// names stay open like "roles": any key is stored, so a future role needs no
// settings change and a typo surfaces at chainFor time as no chain.
func validateChains(chains map[string][]chainEntry) string {
	for name, entries := range chains {
		if len(entries) == 0 {
			continue
		}
		if len(entries) > maxChainEntries {
			return fmt.Sprintf("chains %s: at most %d entries", name, maxChainEntries)
		}
		for i, e := range entries {
			if msg := validateChainEntry(name, i, e); msg != "" {
				return msg
			}
		}
	}
	return ""
}

// validateChainEntry checks one entry, mirroring the "roles" rules in
// settings.go: a known harness, a model, an authed claude model, and the
// pig-only title role.
func validateChainEntry(role string, i int, e chainEntry) string {
	at := fmt.Sprintf("chains %s[%d]", role, i)
	if role == pigTitleRole && e.Backend != "pig" {
		return at + ": harness must be pig"
	}
	if e.Backend != "claude" && e.Backend != "pig" {
		return at + ": harness must be claude or pig"
	}
	if e.Backend == "claude" && !validClaudeModel(e.Model) {
		return at + ": unknown claude model"
	}
	if e.Model == "" {
		return at + ": model is required"
	}
	return ""
}
