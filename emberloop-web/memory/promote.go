// Promotion to project knowledge: a Rule becomes an edit to the nearest
// AGENTS.md (DOX) or a new note file in the Session's worktree.
//
// A promotion starts as a kind=promotion queue item carrying a
// PromotionProposal: the Rule, the Session worktree it was proposed from, the
// target (agents_md or note), the repo-relative target path and a diff preview
// for approval. Approval writes the file in the Session's worktree and leaves
// it uncommitted; the Daemon never commits and the Reviewer loop reviews the
// edit as a normal change. The Rule is then stamped promoted (promoted_at/by);
// its source observations and approver stay untouched as provenance.
package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Promotion targets: an edit to the nearest AGENTS.md, or a new note file
// under .emberloop/notes/.
const (
	PromotionAgentsMD = "agents_md"
	PromotionNote     = "note"
)

// agentsMDFileNames are the files that satisfy a DOX contract, nearest first
// in lookup order per directory. They mirror the Daemon's project-context
// loader priority.
var agentsMDFileNames = []string{"AGENTS.override.md", "AGENTS.md", "AGENTS.MD"}

// PromotionProposal is the JSON payload of a kind=promotion queue item.
type PromotionProposal struct {
	RuleID               string   `json:"rule_id"`
	RuleVersion          int      `json:"rule_version,omitempty"`
	Text                 string   `json:"text"`
	Kind                 string   `json:"kind,omitempty"`
	SubjectKey           string   `json:"subject_key,omitempty"`
	Anchors              []string `json:"anchors,omitempty"`
	ObservedCommit       string   `json:"observed_commit,omitempty"`
	Author               string   `json:"author,omitempty"`
	SessionID            string   `json:"session_id,omitempty"`
	Target               string   `json:"target,omitempty"`
	TargetPath           string   `json:"target_path,omitempty"`
	Diff                 string   `json:"diff,omitempty"`
	ProposedText         string   `json:"proposed_text,omitempty"`
	SourceObservationIDs []int64  `json:"source_observation_ids,omitempty"`
	ApprovedBy           string   `json:"approved_by,omitempty"`
}

// ValidPromotionTarget reports whether t names a promotion target.
func ValidPromotionTarget(t string) bool { return validPromotionTarget(t) }

// validPromotionTarget reports whether t names a promotion target.
func validPromotionTarget(t string) bool {
	return t == PromotionAgentsMD || t == PromotionNote
}

// EncodePromotionProposal marshals p, defaulting an empty target to agents_md.
func EncodePromotionProposal(p PromotionProposal) string {
	if !validPromotionTarget(p.Target) {
		p.Target = PromotionAgentsMD
	}
	raw, _ := json.Marshal(p)
	return string(raw)
}

// DecodePromotionProposal parses a promotion payload. Unknown JSON, an empty
// rule ID, empty text, an unknown target or an empty target path fails.
func DecodePromotionProposal(payload string) (PromotionProposal, error) {
	var p PromotionProposal
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return PromotionProposal{}, fmt.Errorf("%w: promotion payload is not JSON", ErrInvalid)
	}
	if strings.TrimSpace(p.RuleID) == "" {
		return PromotionProposal{}, fmt.Errorf("%w: promotion needs a rule id", ErrInvalid)
	}
	if strings.TrimSpace(p.Text) == "" {
		return PromotionProposal{}, fmt.Errorf("%w: promotion text empty", ErrInvalid)
	}
	if !validPromotionTarget(p.Target) {
		return PromotionProposal{}, fmt.Errorf("%w: promotion target %q", ErrInvalid, p.Target)
	}
	if strings.TrimSpace(p.TargetPath) == "" {
		return PromotionProposal{}, fmt.Errorf("%w: promotion needs a target path", ErrInvalid)
	}
	return p, nil
}

// repoTopLevel returns the git toplevel containing workDir, or the absolute
// workDir itself outside git. Empty workDir gives "".
func repoTopLevel(workDir string) string {
	workDir = strings.TrimSpace(workDir)
	if workDir == "" {
		return ""
	}
	abs, err := filepath.Abs(workDir)
	if err != nil {
		abs = workDir
	}
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = abs
	out, err := cmd.Output()
	if err != nil {
		return abs
	}
	if top := strings.TrimSpace(string(out)); top != "" {
		return top
	}
	return abs
}

// NearestAgentsMD finds the nearest DOX contract for workDir: the first
// agentsMDFileNames hit walking up from the absolute workDir to the repo
// toplevel (or filesystem root outside git). It returns the absolute path and
// the repo-relative path for display. When no contract exists it proposes a
// new AGENTS.md at the toplevel (inside git) or at the workDir itself.
func NearestAgentsMD(workDir string) (absPath, relPath string, err error) {
	workDir = strings.TrimSpace(workDir)
	if workDir == "" {
		return "", "", fmt.Errorf("%w: work dir empty", ErrInvalid)
	}
	abs, err := filepath.Abs(workDir)
	if err != nil {
		return "", "", fmt.Errorf("%w: work dir: %v", ErrInvalid, err)
	}
	top := repoTopLevel(abs)
	for dir := abs; ; dir = filepath.Dir(dir) {
		for _, name := range agentsMDFileNames {
			candidate := filepath.Join(dir, name)
			if st, statErr := os.Stat(candidate); statErr == nil && !st.IsDir() {
				rel, _ := filepath.Rel(top, candidate)
				if rel == "" || strings.HasPrefix(rel, "..") {
					rel = filepath.Base(candidate)
				}
				return candidate, filepath.ToSlash(rel), nil
			}
		}
		if dir == top || dir == filepath.Dir(dir) {
			break
		}
		// Outside git the toplevel is the workDir itself, so one probe
		// suffices; inside git keep climbing to the toplevel.
		if top == abs {
			break
		}
	}
	fallback := filepath.Join(top, "AGENTS.md")
	rel, _ := filepath.Rel(top, fallback)
	if rel == "" || strings.HasPrefix(rel, "..") {
		rel = "AGENTS.md"
	}
	return fallback, filepath.ToSlash(rel), nil
}

// NotesTargetPath resolves the note file for ruleID in the worktree's
// .emberloop/notes/ directory. The file name is the sanitized rule ID: hex
// IDs pass through, anything else becomes alnum-dash form with a .md suffix.
func NotesTargetPath(workDir, ruleID string) (absPath, relPath string) {
	top := repoTopLevel(workDir)
	safe := sanitizeNoteID(ruleID)
	relPath = filepath.ToSlash(filepath.Join(filepath.FromSlash(NotesSubdir), safe+".md"))
	return filepath.Join(top, filepath.FromSlash(relPath)), relPath
}

// sanitizeNoteID maps a rule ID to a safe file stem. Hex and alnum-dash IDs
// pass through lowercased; anything else is filtered to [a-z0-9-] with a short
// fallback so every rule maps to exactly one file name.
func sanitizeNoteID(ruleID string) string {
	id := strings.ToLower(strings.TrimSpace(ruleID))
	var b strings.Builder
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		}
	}
	if out := strings.Trim(b.String(), "-_."); out != "" {
		if len(out) > 64 {
			out = out[:64]
		}
		return out
	}
	return "note"
}

// BuildAgentsMDEdit proposes the new AGENTS.md content for ruleText: the old
// content with "- <text>" appended as one bullet. It also returns a unified
// diff preview naming targetRel. When the file already holds the text the
// content is unchanged and the diff is empty, so approval retries never
// duplicate the line.
func BuildAgentsMDEdit(existing, ruleText, targetRel string) (newContent, diff string) {
	ruleText = strings.TrimSpace(ruleText)
	if ruleText == "" {
		return existing, ""
	}
	line := "- " + ruleText
	if strings.Contains(existing, ruleText) {
		return existing, ""
	}
	var b strings.Builder
	b.WriteString(existing)
	if existing != "" && !strings.HasSuffix(existing, "\n") {
		b.WriteString("\n")
	}
	b.WriteString(line + "\n")
	newContent = b.String()
	rel := strings.TrimSpace(targetRel)
	if rel == "" {
		rel = "AGENTS.md"
	}
	diff = fmt.Sprintf("--- a/%s\n+++ b/%s\n@@\n+%s\n", rel, rel, line)
	return newContent, diff
}

// AgentsMDDiff reads the file at absPath (missing means empty) and returns the
// proposed content and diff preview for ruleText.
func AgentsMDDiff(absPath, relPath, ruleText string) (newContent, diff string, err error) {
	raw, readErr := os.ReadFile(absPath)
	if readErr != nil && !os.IsNotExist(readErr) {
		return "", "", fmt.Errorf("memory: read %s: %w", relPath, readErr)
	}
	newContent, diff = BuildAgentsMDEdit(string(raw), ruleText, relPath)
	return newContent, diff, nil
}

// FormatNoteFile renders a promotion as a project note file: frontmatter with
// id, anchors, observed_commit, author and subject_key, then the rule text as
// the body. The output parses with ParseNote.
func FormatNoteFile(p PromotionProposal) string {
	anchors := append([]string(nil), p.Anchors...)
	sort.Strings(anchors)
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "id: %s\n", quoteFrontmatter(p.RuleID))
	if len(anchors) == 0 {
		b.WriteString("anchors: []\n")
	} else {
		b.WriteString("anchors: [")
		for i, a := range anchors {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%q", a)
		}
		b.WriteString("]\n")
	}
	fmt.Fprintf(&b, "observed_commit: %s\n", quoteFrontmatter(p.ObservedCommit))
	fmt.Fprintf(&b, "author: %s\n", quoteFrontmatter(p.Author))
	fmt.Fprintf(&b, "subject_key: %s\n", quoteFrontmatter(p.SubjectKey))
	b.WriteString("---\n")
	b.WriteString(strings.TrimSpace(p.Text) + "\n")
	return b.String()
}

// quoteFrontmatter quotes a frontmatter scalar when it needs it; plain
// scalars stay bare so files stay readable.
func quoteFrontmatter(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return `""`
	}
	if strings.ContainsAny(s, ":#\"'\n\r\t[]{}") || strings.HasPrefix(s, "-") {
		return fmt.Sprintf("%q", s)
	}
	return s
}

// IsPathInside reports whether child resolves inside parent. Both are cleaned
// and made absolute; a child equal to the parent counts as inside.
func IsPathInside(child, parent string) bool {
	childAbs, err := filepath.Abs(child)
	if err != nil {
		return false
	}
	parentAbs, err := filepath.Abs(parent)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(parentAbs, childAbs)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// VerifyPromotionTarget checks that absTarget lies in the Session worktree:
// inside the git toplevel when workDir is in git, else inside workDir itself.
// It refuses empty inputs and escapes, so approval can never write outside
// the worktree it was proposed from.
func VerifyPromotionTarget(workDir, absTarget string) error {
	if strings.TrimSpace(workDir) == "" || strings.TrimSpace(absTarget) == "" {
		return fmt.Errorf("%w: promotion target outside the worktree", ErrInvalid)
	}
	abs, err := filepath.Abs(workDir)
	if err != nil {
		return fmt.Errorf("%w: promotion target outside the worktree", ErrInvalid)
	}
	top := repoTopLevel(abs)
	if !IsPathInside(absTarget, top) {
		return fmt.Errorf("%w: promotion target outside the worktree", ErrInvalid)
	}
	return nil
}

// PromotionNotePreview renders the diff preview for a note target: a new-file
// unified diff with the frontmatter, so approval shows the same shape as the
// AGENTS.md path.
func PromotionNotePreview(relPath, content string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "--- /dev/null\n+++ b/%s\n", strings.TrimSpace(relPath))
	for _, line := range strings.Split(content, "\n") {
		b.WriteString("+" + line + "\n")
	}
	return b.String()
}

// MarkRulePromotedCommand validates the promoted-by identity for the marking
// step shared by the bank and the fake.
func MarkRulePromotedCommand(ruleID, promotedBy string) error {
	if strings.TrimSpace(ruleID) == "" {
		return fmt.Errorf("%w: rule id empty", ErrInvalid)
	}
	if strings.TrimSpace(promotedBy) == "" {
		return fmt.Errorf("%w: promotion needs who promoted it", ErrInvalid)
	}
	return nil
}
