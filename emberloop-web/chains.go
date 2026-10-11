package web

import (
	"encoding/json"
	"fmt"
)

// Run fallback chains: per role, an ordered list of up to three
// {backend, model} entries stored under "chains" in the shared settings file.
// A Worker or Reviewer failure classified (failures.go) as quota, credit,
// auth or session limit starts a new Session on the next entry; any other
// failure surfaces as today. Ticket 14 runs the advance; this file stores,
// validates and returns the chains.

// maxChainEntries caps one role's fallback chain.
const maxChainEntries = 3

// chainEntry is one fallback step: the Backend (harness) and model to start,
// with the worker effort riding along when set.
type chainEntry struct {
	Backend string `json:"backend"`
	Model   string `json:"model"`
	Effort  string `json:"effort,omitempty"`
}

// UnmarshalJSON reads one entry, accepting "harness" as an alias of
// "backend": the settings page wrote "harness" before chains existed, and the
// spec names the field "backend".
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

// readChains returns the stored per-role chains. A missing or unreadable
// settings file means no chains.
func readChains() map[string][]chainEntry {
	chains := map[string][]chainEntry{}
	all, err := readSettingsMap()
	if err != nil {
		return chains
	}
	if raw, ok := all["chains"]; ok {
		_ = json.Unmarshal(raw, &chains)
	}
	return chains
}

// chainFor returns the active fallback chain for a role: the stored chain
// under the role or one of its aliases, else the single "roles" entry as a
// chain of one, so a role without a chain still runs once. Empty when the
// role is nowhere configured.
func chainFor(role string) []chainEntry {
	chains := readChains()
	names := chainNames(role)
	for _, name := range names {
		if c, ok := chains[name]; ok && len(c) > 0 {
			return c
		}
	}
	roles := readRoles()
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
	if e.Backend != "claude" && e.Backend != "pig" && e.Backend != "grok" {
		return at + ": harness must be claude, pig or grok"
	}
	if e.Backend == "claude" && !validClaudeModel(e.Model) {
		return at + ": unknown claude model"
	}
	if e.Model == "" {
		return at + ": model is required"
	}
	return ""
}
