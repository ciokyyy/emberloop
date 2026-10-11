// Profile lines with stable IDs, versioned in profile_versions. Every
// version is kept; a retired line is hidden, not deleted.
package memory

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// MaxProfileBytes caps the rendered Profile, counted in bytes. An over-cap
// render is refused with a condense message; the caller trims lines and tries
// again.
const MaxProfileBytes = 2048

// Profile tags. Workflow and style lines go to Reviewers; other lines stay
// with the owner's Sessions only.
const (
	ProfileWorkflow = "workflow"
	ProfileStyle    = "style"
	ProfileOther    = "other"
)

// Profile patch operations.
const (
	ProfileAdd    = "add"
	ProfileEdit   = "edit"
	ProfileRetire = "retire"
)

// ProfileLine is one standing instruction. ID is stable across versions and
// never reused; Retired hides the line without deleting it.
type ProfileLine struct {
	ID      string `json:"id"`
	Text    string `json:"text"`
	Tag     string `json:"tag"`
	Retired bool   `json:"retired,omitempty"`
}

// ProfileVersion is one approved Profile. Version 0 with no lines means the
// User never saved a Profile.
type ProfileVersion struct {
	Version    int           `json:"version"`
	Lines      []ProfileLine `json:"lines"`
	Patch      string        `json:"patch,omitempty"`
	ApprovedBy string        `json:"approvedBy"`
	CreatedAt  time.Time     `json:"createdAt"`
}

// ProfileOp is one line-ID'd change: add a line, edit one's text or tag, or
// retire it. Add carries no ID; edit and retire name one.
type ProfileOp struct {
	Op   string `json:"op"`
	ID   string `json:"id,omitempty"`
	Text string `json:"text,omitempty"`
	Tag  string `json:"tag,omitempty"`
}

// ProfilePatch is the JSON a proposer sends: {"ops":[...]}. The nightly job
// sends {"ops":[]} as none when no stable signal exists.
type ProfilePatch struct {
	Ops []ProfileOp `json:"ops"`
}

// RenderProfile joins the live (non-retired) lines with newlines. An over-cap
// result is refused with a condense message.
func RenderProfile(v ProfileVersion) (string, error) {
	var live []string
	for _, l := range v.Lines {
		if !l.Retired {
			live = append(live, l.Text)
		}
	}
	out := strings.Join(live, "\n")
	if err := checkProfileBytes(len(out)); err != nil {
		return "", err
	}
	return out, nil
}

// checkProfileBytes refuses an over-cap render with the condense message.
// Bank.AppendProfileVersion, Fake.AppendProfileVersion and RenderProfile all
// use it so the cap cannot drift apart.
func checkProfileBytes(n int) error {
	if n > MaxProfileBytes {
		return fmt.Errorf("%w: profile is %d bytes, max %d: condense it to fewer, shorter lines",
			ErrInvalid, n, MaxProfileBytes)
	}
	return nil
}

// checkProfileCap renders lines for the cap check without building a version.
func checkProfileCap(lines []ProfileLine) error {
	return checkProfileBytes(profileLinesBytes(lines))
}

// ReviewerLines returns the lines a Reviewer may see: live lines tagged
// workflow or style, excluding lines that assert expertise or lower the
// review bar even when so tagged.
func ReviewerLines(lines []ProfileLine) []ProfileLine {
	var out []ProfileLine
	for _, l := range lines {
		if l.Retired {
			continue
		}
		if l.Tag != ProfileWorkflow && l.Tag != ProfileStyle {
			continue
		}
		if profileLineBarred(l.Text) {
			continue
		}
		out = append(out, l)
	}
	return out
}

// RenderReviewerProfile renders the Reviewer-visible subset. The same cap and
// condense message apply.
func RenderReviewerProfile(v ProfileVersion) (string, error) {
	return RenderProfile(ProfileVersion{Version: v.Version, Lines: ReviewerLines(v.Lines)})
}

// IsReviewerTag reports whether a tag travels to Reviewers.
func IsReviewerTag(tag string) bool {
	return tag == ProfileWorkflow || tag == ProfileStyle
}

func validProfileTag(tag string) bool {
	switch tag {
	case ProfileWorkflow, ProfileStyle, ProfileOther:
		return true
	}
	return false
}

// barredPhrases are substrings of lines that assert expertise or lower the
// review bar. Such lines must stay tagged other so Reviewers never see them.
// The list is a heuristic, not a semantic judge: reviewers still read the
// approved tag, and borderline wording stays visible until a human retags it.
var barredPhrases = []string{
	"you are an expert",
	"i am an expert",
	"as an expert",
	"expert in",
	"senior engineer",
	"10x engineer",
	"you are senior",
	"you are the best",
	"approve anyway",
	"just approve",
	"skip review",
	"no need to review",
	"don't review",
	"do not review",
	"don't check",
	"do not check",
	"lower the bar",
	"don't be strict",
	"do not be strict",
	"rubber stamp",
	"lgtm without",
	"approve without",
	"always approve",
	"trust me",
	"passes review",
	"no need to check",
}

// profileLineBarred reports whether text asserts expertise or lowers the
// review bar, case-insensitively.
func profileLineBarred(text string) bool {
	lower := strings.ToLower(text)
	for _, p := range barredPhrases {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// ValidateProfilePatch checks ops against the current lines without applying
// them. Ops apply in order, so a retire then an edit of the same line fails.
func ValidateProfilePatch(patch ProfilePatch, current []ProfileLine) error {
	if len(patch.Ops) == 0 {
		return fmt.Errorf("%w: profile patch has no ops", ErrInvalid)
	}
	byID := make(map[string]ProfileLine, len(current))
	for _, l := range current {
		byID[l.ID] = l
	}
	retiredInPatch := make(map[string]bool)
	for i, op := range patch.Ops {
		switch op.Op {
		case ProfileAdd:
			if op.ID != "" {
				return fmt.Errorf("%w: op %d: add carries no id", ErrInvalid, i)
			}
			if strings.TrimSpace(op.Text) == "" {
				return fmt.Errorf("%w: op %d: add needs text", ErrInvalid, i)
			}
			if !validProfileTag(op.Tag) {
				return fmt.Errorf("%w: op %d: tag %q", ErrInvalid, i, op.Tag)
			}
			if IsReviewerTag(op.Tag) && profileLineBarred(op.Text) {
				return fmt.Errorf("%w: op %d: lines that assert expertise or lower the review bar cannot carry tag %q",
					ErrInvalid, i, op.Tag)
			}
		case ProfileEdit:
			if op.ID == "" {
				return fmt.Errorf("%w: op %d: edit needs a line id", ErrInvalid, i)
			}
			cur, ok := byID[op.ID]
			if !ok {
				return fmt.Errorf("%w: op %d: no line %q", ErrInvalid, i, op.ID)
			}
			if cur.Retired || retiredInPatch[op.ID] {
				return fmt.Errorf("%w: op %d: line %q is retired", ErrInvalid, i, op.ID)
			}
			if strings.TrimSpace(op.Text) == "" {
				return fmt.Errorf("%w: op %d: edit needs text", ErrInvalid, i)
			}
			if !validProfileTag(op.Tag) {
				return fmt.Errorf("%w: op %d: tag %q", ErrInvalid, i, op.Tag)
			}
			if IsReviewerTag(op.Tag) && profileLineBarred(op.Text) {
				return fmt.Errorf("%w: op %d: lines that assert expertise or lower the review bar cannot carry tag %q",
					ErrInvalid, i, op.Tag)
			}
		case ProfileRetire:
			if op.ID == "" {
				return fmt.Errorf("%w: op %d: retire needs a line id", ErrInvalid, i)
			}
			cur, ok := byID[op.ID]
			if !ok {
				return fmt.Errorf("%w: op %d: no line %q", ErrInvalid, i, op.ID)
			}
			if cur.Retired || retiredInPatch[op.ID] {
				return fmt.Errorf("%w: op %d: line %q is already retired", ErrInvalid, i, op.ID)
			}
			retiredInPatch[op.ID] = true
		default:
			return fmt.Errorf("%w: op %d: op %q (want add, edit or retire)", ErrInvalid, i, op.Op)
		}
	}
	return nil
}

// ApplyProfilePatch validates patch against current and returns the new line
// set. Adds get the next stable ID (l1, l2, ...); edits replace text and tag;
// retires hide the line. The input is not mutated.
func ApplyProfilePatch(current []ProfileLine, patch ProfilePatch) ([]ProfileLine, error) {
	if err := ValidateProfilePatch(patch, current); err != nil {
		return nil, err
	}
	out := append([]ProfileLine(nil), current...)
	byIdx := make(map[string]int, len(out))
	for i, l := range out {
		byIdx[l.ID] = i
	}
	next := nextProfileLineNum(out)
	for _, op := range patch.Ops {
		switch op.Op {
		case ProfileAdd:
			id := fmt.Sprintf("l%d", next)
			next++
			out = append(out, ProfileLine{ID: id, Text: op.Text, Tag: op.Tag})
			byIdx[id] = len(out) - 1
		case ProfileEdit:
			i := byIdx[op.ID]
			out[i].Text = op.Text
			out[i].Tag = op.Tag
		case ProfileRetire:
			i := byIdx[op.ID]
			out[i].Retired = true
		}
	}
	return out, nil
}

// nextProfileLineNum returns one past the highest lN ID in use, so IDs stay
// stable and never repeat even after a retire.
func nextProfileLineNum(lines []ProfileLine) int {
	max := 0
	for _, l := range lines {
		var n int
		if _, err := fmt.Sscanf(l.ID, "l%d", &n); err == nil && n > max {
			max = n
		}
	}
	return max + 1
}

// profileLinesBytes renders lines for the cap check without building a
// version.
func profileLinesBytes(lines []ProfileLine) int {
	var live []string
	for _, l := range lines {
		if !l.Retired {
			live = append(live, l.Text)
		}
	}
	return len(strings.Join(live, "\n"))
}

// profilePatchJSON marshals ops for profile_versions.patch_json.
func profilePatchJSON(patch ProfilePatch) string {
	raw, err := json.Marshal(patch)
	if err != nil {
		return ""
	}
	return string(raw)
}

// scanProfileVersion maps one profile_versions row.
func scanProfileVersion(s rowScanner) (ProfileVersion, error) {
	var v ProfileVersion
	var linesDoc, patchDoc, approved, created string
	var version int
	if err := s.Scan(&version, &linesDoc, &patchDoc, &approved, &created); err != nil {
		return ProfileVersion{}, err
	}
	var lines []ProfileLine
	if err := json.Unmarshal([]byte(linesDoc), &lines); err != nil {
		return ProfileVersion{}, fmt.Errorf("memory: profile lines of v%d: %w", version, err)
	}
	for i, l := range lines {
		if l.ID == "" || l.Text == "" || !validProfileTag(l.Tag) {
			return ProfileVersion{}, fmt.Errorf("memory: profile v%d line %d is invalid", version, i)
		}
	}
	v = ProfileVersion{
		Version:    version,
		Lines:      lines,
		Patch:      patchDoc,
		ApprovedBy: approved,
		CreatedAt:  parseTimeText(created),
	}
	return v, nil
}
