package web

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Optional-section markers, matched case-insensitively on the original text:
// lowercasing first can change byte length for some non-ASCII runes, so the
// offsets would no longer line up with the original for slicing.
var (
	optionalStartRe   = regexp.MustCompile(`(?i)<!-- optional:start -->`)
	optionalEndRe     = regexp.MustCompile(`(?i)<!-- optional:end -->`)
	optionalHeadingRe = regexp.MustCompile(`(?i)\(optional\)`)
)

// Run prompt injection (ticket 04): the driver builds each Worker and
// Reviewer first prompt from the Ticket file, the spec, the AGENTS.md chain
// for the changed paths (root chain when unknown) and the role's trimmed
// skills. The skill list per role lives in Run settings under "run_skills";
// each entry is skill text with sections marked optional dropped. The
// existing standards and spec briefs are the first entries.

// canonicalSkillRole maps settings aliases to the canonical skill role: the
// worker slot (implementer, worker) and the two reviewer slots
// (reviewer-spec/spec-review, reviewer-standards/code-review).
func canonicalSkillRole(role string) string {
	switch role {
	case "worker", "implementer":
		return "worker"
	case reviewerSpecType, "spec-review":
		return reviewerSpecType
	case reviewerStandardsType, "code-review":
		return reviewerStandardsType
	default:
		return role
	}
}

// skillAliases lists every settings key that feeds one canonical role, so a
// User configuring either alias still injects.
func skillAliases(canonical string) []string {
	switch canonical {
	case "worker":
		return []string{"worker", "implementer"}
	case reviewerSpecType:
		return []string{reviewerSpecType, "spec-review"}
	case reviewerStandardsType:
		return []string{reviewerStandardsType, "code-review"}
	default:
		return []string{canonical}
	}
}

// defaultRunSkills returns the built-in first entries per role: both briefs
// for the Worker (it must meet both axes), one brief for each Reviewer.
// Reviewer briefs live in runs_exec.go next to reviewerPrompt.
func defaultRunSkills(canonical string) []string {
	switch canonical {
	case "worker":
		return []string{specBrief, standardsBrief}
	case reviewerSpecType:
		return []string{specBrief}
	case reviewerStandardsType:
		return []string{standardsBrief}
	default:
		return nil
	}
}

// readRunSkillsRaw reads the "run_skills" map from the settings file:
// role -> list of skill texts. Missing or unreadable reads empty.
func readRunSkillsRaw() map[string][]string {
	out := map[string][]string{}
	all, err := readSettingsMap()
	if err != nil {
		return out
	}
	raw, ok := all["run_skills"]
	if !ok {
		return out
	}
	// Accept both []string and []any shapes; a malformed role reads empty.
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return out
	}
	for role, rraw := range decoded {
		var list []string
		if err := json.Unmarshal(rraw, &list); err != nil {
			continue
		}
		if kept := cleanSkillList(list); len(kept) > 0 {
			out[role] = kept
		}
	}
	return out
}

// cleanSkillList drops blank entries from one role's skill list. Both the
// settings read and the settings save share it, so blank handling cannot
// drift between the two.
func cleanSkillList(list []string) []string {
	kept := make([]string, 0, len(list))
	for _, s := range list {
		if strings.TrimSpace(s) == "" {
			continue
		}
		kept = append(kept, s)
	}
	return kept
}

// validateRunSkills rejects a run_skills write that is not an object of
// string arrays: unknown roles are allowed (forward-compatible), but every
// entry must be a string. Empty strings are dropped on save, not refused,
// so a write carrying one still succeeds. Overlong entries are refused so
// one write cannot blow the first prompt past the context cap.
func validateRunSkills(skills map[string][]string) string {
	if skills == nil {
		return ""
	}
	const maxSkillBytes = 16 * 1024
	const maxSkillsPerRole = 20
	for role, list := range skills {
		if strings.TrimSpace(role) == "" {
			return "run_skills role must not be empty"
		}
		if len(list) > maxSkillsPerRole {
			return "run_skills " + role + ": at most 20 skills per role"
		}
		for _, s := range list {
			if len(s) > maxSkillBytes {
				return "run_skills " + role + ": one skill is over 16 KB"
			}
		}
	}
	return ""
}

// runSkillsFor returns the trimmed skill list for one role: the built-in
// first entries plus the configured custom skills for the role and its
// aliases. Every entry passes through trimRunSkill exactly once.
func runSkillsFor(role string) []string {
	canonical := canonicalSkillRole(role)
	var untrimmed []string
	untrimmed = append(untrimmed, defaultRunSkills(canonical)...)
	raw := readRunSkillsRaw()
	for _, alias := range skillAliases(canonical) {
		untrimmed = append(untrimmed, raw[alias]...)
	}
	out := make([]string, 0, len(untrimmed))
	for _, s := range untrimmed {
		if trimmed := trimRunSkill(s); strings.TrimSpace(trimmed) != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// trimRunSkill drops sections marked optional from one skill text, so the
// first prompt stays small. Two markers count:
//   - HTML blocks: everything between "<!-- optional:start -->" and
//     "<!-- optional:end -->" (markers included, case-insensitive).
//   - Markdown headings whose text contains "(optional)"
//     (case-insensitive): the heading plus its body up to the next heading
//     of equal or higher level (# count <= the optional heading's).
//
// Lines inside fenced code blocks (``` or ~~~) never count as headings, so a
// shell comment like `# run (optional)` in an example cannot swallow the
// rest of the skill. Unclosed markers drop to the end. Surrounding blank
// lines collapse.
func trimRunSkill(s string) string {
	s = dropOptionalHTMLBlocks(s)
	lines := strings.Split(s, "\n")
	var out []string
	skipLevel := 0 // 0 means keeping; else the # count of the optional heading
	inFence := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			if skipLevel != 0 {
				continue
			}
			out = append(out, line)
			continue
		}
		if !inFence && strings.HasPrefix(trimmed, "#") {
			level := 0
			for level < len(trimmed) && trimmed[level] == '#' {
				level++
			}
			rest := strings.TrimSpace(trimmed[level:])
			// A heading of equal or higher level ends the optional section.
			if skipLevel != 0 && level <= skipLevel {
				skipLevel = 0
			}
			if skipLevel == 0 && optionalHeadingRe.MatchString(rest) {
				skipLevel = level
				continue
			}
		}
		if skipLevel != 0 {
			continue
		}
		out = append(out, line)
	}
	// Collapse 3+ blank lines to two; trim leading/trailing blanks.
	joined := strings.Join(out, "\n")
	for strings.Contains(joined, "\n\n\n") {
		joined = strings.ReplaceAll(joined, "\n\n\n", "\n\n")
	}
	return strings.Trim(joined, "\n")
}

// dropOptionalHTMLBlocks removes <!-- optional:start --> ... <!-- optional:end -->
// blocks case-insensitively. An unclosed start drops to the end. Indices come
// from the original text, never a lowercased copy.
func dropOptionalHTMLBlocks(s string) string {
	for {
		startLoc := optionalStartRe.FindStringIndex(s)
		if startLoc == nil {
			return s
		}
		rest := s[startLoc[1]:]
		endLoc := optionalEndRe.FindStringIndex(rest)
		if endLoc == nil {
			return s[:startLoc[0]]
		}
		s = s[:startLoc[0]] + rest[endLoc[1]:]
	}
}

// readRunFile returns one prompt source file trimmed of a BOM. Missing or
// unreadable reads empty; callers still name the path.
func readRunFile(path string) string {
	if path == "" {
		return ""
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(string(raw), "\xef\xbb\xbf")
}

// agentsChainForWorker loads the root AGENTS.md chain for a Worker first
// prompt: the worktree's chain up to the filesystem root (repo root and the
// seeded worktree root included), outermost-first. Changed paths are unknown
// before the Worker edits, so the root chain is the chain.
func agentsChainForWorker(worktree string) []claudeContextFile {
	if worktree == "" {
		return nil
	}
	return loadClaudeContextFiles(worktree, "")
}

// reviewChangedFiles lists the files changed between base and tip in repo,
// as repo-relative paths. Empty (unknown) when base or tip is empty or git
// fails, so the caller falls back to the root chain.
func reviewChangedFiles(repo, base, tip string) []string {
	if repo == "" || base == "" || tip == "" || base == tip {
		return nil
	}
	out, err := runGit(repo, "diff", "--name-only", base, tip)
	if err != nil || strings.TrimSpace(out) == "" {
		return nil
	}
	var rel []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		rel = append(rel, line)
	}
	return rel
}

// agentsChainForPaths loads the AGENTS.md chain for the given repo-relative
// paths as seen from the worktree: each path's directory chain, merged and
// deduped outermost-first. Empty paths read the root chain.
func agentsChainForPaths(worktree string, relPaths []string) []claudeContextFile {
	if worktree == "" {
		return nil
	}
	if len(relPaths) == 0 {
		return agentsChainForWorker(worktree)
	}
	seen := map[string]claudeContextFile{}
	for _, rel := range relPaths {
		rel = strings.TrimSpace(rel)
		if rel == "" {
			continue
		}
		abs := filepath.Join(worktree, filepath.FromSlash(rel))
		for _, cf := range loadClaudeContextFiles(abs, "") {
			if _, dup := seen[cf.Path]; !dup {
				seen[cf.Path] = cf
			}
		}
	}
	if len(seen) == 0 {
		return agentsChainForWorker(worktree)
	}
	out := make([]claudeContextFile, 0, len(seen))
	for _, cf := range seen {
		out = append(out, cf)
	}
	// Outermost-first: shallowest path first, then lexicographic.
	sort.Slice(out, func(i, j int) bool {
		di := strings.Count(out[i].Path, string(filepath.Separator))
		dj := strings.Count(out[j].Path, string(filepath.Separator))
		if di != dj {
			return di < dj
		}
		return out[i].Path < out[j].Path
	})
	return out
}

// agentsChainForReviewer loads the chain for a Reviewer first prompt: the
// chain for the changed files between base and tip, or the root chain when
// the diff is unknown or empty.
func agentsChainForReviewer(worktree, repo, base, tip string) []claudeContextFile {
	if changed := reviewChangedFiles(repo, base, tip); len(changed) > 0 {
		return agentsChainForPaths(worktree, changed)
	}
	return agentsChainForWorker(worktree)
}

// formatRunFirstPrompt assembles one injected first prompt: the task line
// (workerTaskPrompt or the reviewer prefix), then the Ticket, the spec, the
// AGENTS.md chain and the role's skills. Empty sources still name their
// path so the agent can report the gap instead of guessing.
func formatRunFirstPrompt(base, ticketPath, ticketContent, specPath, specContent string, chain []claudeContextFile, role string, skills []string) string {
	var b strings.Builder
	b.WriteString(base)
	if ticketPath != "" || strings.TrimSpace(ticketContent) != "" {
		b.WriteString("\n\n--- Ticket " + ticketPath + " ---\n")
		if strings.TrimSpace(ticketContent) == "" {
			b.WriteString("(ticket file unreadable)\n")
		} else {
			b.WriteString(strings.TrimRight(ticketContent, "\n") + "\n")
		}
	}
	if specPath != "" || strings.TrimSpace(specContent) != "" {
		b.WriteString("\n--- Spec " + specPath + " ---\n")
		if strings.TrimSpace(specContent) == "" {
			b.WriteString("(spec file unreadable)\n")
		} else {
			b.WriteString(strings.TrimRight(specContent, "\n") + "\n")
		}
	}
	b.WriteString("\n--- AGENTS.md chain ---\n")
	if len(chain) == 0 {
		b.WriteString("(no AGENTS.md chain found)\n")
	} else {
		b.WriteString(strings.TrimRight(claudeProjectContextBlock(chain), "\n") + "\n")
	}
	b.WriteString("\n--- Skills (" + role + ") ---\n")
	if len(skills) == 0 {
		b.WriteString("(no skills)\n")
	} else {
		for i, s := range skills {
			if i > 0 {
				b.WriteString("\n")
			}
			b.WriteString("--- skill " + strconv.Itoa(i+1) + " ---\n")
			b.WriteString(strings.TrimRight(s, "\n") + "\n")
		}
	}
	return b.String()
}

// buildWorkerFirstPrompt reads the Ticket, spec, root chain and worker
// skills for a Worker start. A read failure still names the path.
func buildWorkerFirstPrompt(ticketID, specPath, ticketPath, branch string, seeded bool, worktree string) string {
	base := workerTaskPrompt(ticketID, specPath, ticketPath, branch, seeded)
	return formatRunFirstPrompt(base, ticketPath, readRunFile(ticketPath), specPath, readRunFile(specPath), agentsChainForWorker(worktree), "worker", runSkillsFor("worker"))
}

// buildReviewerFirstPrompt reads the Ticket, spec, changed-paths chain and
// reviewer skills for a Reviewer start. base is the Bases entry for the
// ticket (empty falls back to the root chain). A read failure still names
// the path.
func buildReviewerFirstPrompt(ticketID, specPath, ticketPath, reviewerType, branch, tip, worktree, repo, base string) string {
	return buildReviewerFirstPromptWithDeclined(ticketID, specPath, ticketPath, reviewerType, branch, tip, worktree, repo, base, nil)
}

// buildReviewerFirstPromptWithDeclined also carries prior Worker declines
// when a Reviewer Session is started after earlier review rounds.
func buildReviewerFirstPromptWithDeclined(ticketID, specPath, ticketPath, reviewerType, branch, tip, worktree, repo, base string, declined []declinedReviewFinding) string {
	prefix := reviewerTaskPrefix(ticketID, specPath, reviewerType, branch, tip)
	if feedback := declinedFindingsPrompt(declined); feedback != "" {
		prefix += "\n\n" + feedback
	}
	return formatRunFirstPrompt(prefix, ticketPath, readRunFile(ticketPath), specPath, readRunFile(specPath), agentsChainForReviewer(worktree, repo, base, tip), reviewerType, runSkillsFor(reviewerType))
}
