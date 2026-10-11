package memory

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNearestAgentsMDPicksNearest(t *testing.T) {
	dir, _ := initGitRepo(t, map[string]string{
		"AGENTS.md":         "# root\n",
		"sub/AGENTS.md":     "# sub\n",
		"sub/nested/app.go": "package app\n",
	})
	workDir := filepath.Join(dir, "sub", "nested")
	abs, rel, err := NearestAgentsMD(workDir)
	if err != nil {
		t.Fatal(err)
	}
	if abs != filepath.Join(dir, "sub", "AGENTS.md") {
		t.Fatalf("nearest = %s, want sub/AGENTS.md", abs)
	}
	if rel != "sub/AGENTS.md" {
		t.Fatalf("rel = %q, want sub/AGENTS.md", rel)
	}
}

func TestNearestAgentsMDFallsBackToRoot(t *testing.T) {
	dir, _ := initGitRepo(t, map[string]string{
		"AGENTS.md":    "# root\n",
		"other/app.go": "package app\n",
	})
	abs, rel, err := NearestAgentsMD(filepath.Join(dir, "other"))
	if err != nil {
		t.Fatal(err)
	}
	if abs != filepath.Join(dir, "AGENTS.md") {
		t.Fatalf("nearest = %s, want root AGENTS.md", abs)
	}
	if rel != "AGENTS.md" {
		t.Fatalf("rel = %q, want AGENTS.md", rel)
	}
}

func TestNearestAgentsMDProposesNewFile(t *testing.T) {
	dir, _ := initGitRepo(t, map[string]string{"app.go": "package app\n"})
	abs, rel, err := NearestAgentsMD(dir)
	if err != nil {
		t.Fatal(err)
	}
	if abs != filepath.Join(dir, "AGENTS.md") {
		t.Fatalf("proposed = %s, want toplevel AGENTS.md", abs)
	}
	if rel != "AGENTS.md" {
		t.Fatalf("rel = %q, want AGENTS.md", rel)
	}
}

func TestNearestAgentsMDPrefersOverride(t *testing.T) {
	dir, _ := initGitRepo(t, map[string]string{
		"AGENTS.md":          "# root\n",
		"AGENTS.override.md": "# override\n",
	})
	abs, _, err := NearestAgentsMD(dir)
	if err != nil {
		t.Fatal(err)
	}
	if abs != filepath.Join(dir, "AGENTS.override.md") {
		t.Fatalf("nearest = %s, want the override file", abs)
	}
}

func TestNearestAgentsMDRejectsEmpty(t *testing.T) {
	if _, _, err := NearestAgentsMD(""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty workdir err = %v, want ErrInvalid", err)
	}
}

func TestBuildAgentsMDEditAppendsAndShowsDiff(t *testing.T) {
	existing := "# AGENTS.md — demo\n\nContract.\n"
	updated, diff := BuildAgentsMDEdit(existing, "always wrap errors", "AGENTS.md")
	if !strings.Contains(updated, "- always wrap errors\n") {
		t.Fatalf("updated = %q, want the bullet appended", updated)
	}
	if !strings.HasPrefix(updated, existing) {
		t.Fatalf("updated does not preserve the old content: %q", updated)
	}
	if !strings.Contains(diff, "a/AGENTS.md") || !strings.Contains(diff, "+- always wrap errors") {
		t.Fatalf("diff = %q, want a unified preview with the new line", diff)
	}
}

func TestBuildAgentsMDEditIsIdempotent(t *testing.T) {
	existing := "# AGENTS.md\n\n- always wrap errors\n"
	updated, diff := BuildAgentsMDEdit(existing, "always wrap errors", "AGENTS.md")
	if updated != existing {
		t.Fatalf("re-edit changed the file: %q", updated)
	}
	if diff != "" {
		t.Fatalf("re-edit diff = %q, want empty when already applied", diff)
	}
}

func TestAgentsMDDiffMissingFile(t *testing.T) {
	dir := t.TempDir()
	abs := filepath.Join(dir, "AGENTS.md")
	updated, diff, err := AgentsMDDiff(abs, "AGENTS.md", "keep it simple")
	if err != nil {
		t.Fatal(err)
	}
	if updated != "- keep it simple\n" {
		t.Fatalf("new content = %q, want just the bullet", updated)
	}
	if !strings.Contains(diff, "+- keep it simple") {
		t.Fatalf("diff = %q, want the new line", diff)
	}
}

func TestFormatNoteFileParses(t *testing.T) {
	p := PromotionProposal{
		RuleID: "abc123", Text: "use the upload handler for uploads",
		Kind: KindFact, SubjectKey: "go.uploads", Anchors: []string{"b.go", "a.go"},
		ObservedCommit: "deadbeef", Author: "u1",
	}
	content := FormatNoteFile(p)
	n, err := ParseNote("abc123.md", []byte(content))
	if err != nil {
		t.Fatalf("generated note does not parse: %v\n%s", err, content)
	}
	if n.ID != "abc123" || n.Text != p.Text || n.Author != "u1" || n.SubjectKey != "go.uploads" {
		t.Fatalf("parsed note = %+v, want the proposal fields", n)
	}
	if len(n.Anchors) != 2 || n.Anchors[0] != "a.go" || n.Anchors[1] != "b.go" {
		t.Fatalf("anchors = %v, want sorted [a.go b.go]", n.Anchors)
	}
	if n.ObservedCommit != "deadbeef" {
		t.Fatalf("observed commit = %q", n.ObservedCommit)
	}
}

func TestNotesTargetPathStaysInside(t *testing.T) {
	dir, _ := initGitRepo(t, map[string]string{"AGENTS.md": "# root\n"})
	abs, rel := NotesTargetPath(dir, "ABC123")
	if rel != ".emberloop/notes/abc123.md" {
		t.Fatalf("rel = %q, want the sanitized note path", rel)
	}
	if abs != filepath.Join(dir, filepath.FromSlash(rel)) {
		t.Fatalf("abs = %s, want under the toplevel", abs)
	}
	if err := VerifyPromotionTarget(dir, abs); err != nil {
		t.Fatalf("inside target rejected: %v", err)
	}
}

func TestVerifyPromotionTargetRefusesEscape(t *testing.T) {
	dir, _ := initGitRepo(t, map[string]string{"AGENTS.md": "# root\n"})
	if err := VerifyPromotionTarget(dir, "/etc/passwd"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("escape err = %v, want ErrInvalid", err)
	}
	if err := VerifyPromotionTarget("", filepath.Join(dir, "AGENTS.md")); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty workdir err = %v, want ErrInvalid", err)
	}
}

func TestPromotionPayloadRoundTrip(t *testing.T) {
	p := PromotionProposal{
		RuleID: "r1", Text: "keep it simple", Target: PromotionNote,
		TargetPath: ".emberloop/notes/r1.md", SessionID: "s1",
	}
	raw := EncodePromotionProposal(p)
	got, err := DecodePromotionProposal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.RuleID != "r1" || got.Target != PromotionNote || got.TargetPath != ".emberloop/notes/r1.md" {
		t.Fatalf("round trip = %+v", got)
	}
}

func TestPromotionPayloadRejectsBad(t *testing.T) {
	for _, payload := range []string{
		`not json`,
		`{"text":"x","target":"agents_md","target_path":"AGENTS.md"}`,
		`{"rule_id":"r1","target":"agents_md","target_path":"AGENTS.md"}`,
		`{"rule_id":"r1","text":"x","target":"bogus","target_path":"AGENTS.md"}`,
		`{"rule_id":"r1","text":"x","target":"agents_md","target_path":""}`,
	} {
		if _, err := DecodePromotionProposal(payload); !errors.Is(err, ErrInvalid) {
			t.Errorf("payload %q err = %v, want ErrInvalid", payload, err)
		}
	}
}

func TestEncodePromotionDefaultsTarget(t *testing.T) {
	raw := EncodePromotionProposal(PromotionProposal{RuleID: "r1", Text: "x", TargetPath: "AGENTS.md"})
	got, err := DecodePromotionProposal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.Target != PromotionAgentsMD {
		t.Fatalf("target = %q, want agents_md default", got.Target)
	}
}

// TestMarkRulePromoted runs the marking contract against the bank and the
// fake: the Rule keeps its text, status and provenance and gains
// promoted_by/at. The stamp never changes status: an active Rule stays
// active and searchable.
func TestMarkRulePromoted(t *testing.T) {
	for _, fac := range factories() {
		t.Run(fac.name, func(t *testing.T) {
			clock := newTestClock()
			m := fac.open(t, clock)
			ctx := context.Background()
			obs, err := m.AddObservation(ctx, Observation{
				SessionID: "s1", EventID: "e1", Author: "u1",
				Quote: "always wrap errors", Kind: KindPreference, SubjectKey: "wrap.errors",
			})
			if err != nil {
				t.Fatal(err)
			}
			rule, err := m.AddRule(ctx, Rule{
				Text: "always wrap errors in this repo", Kind: KindPreference,
				SubjectKey: "wrap.errors", ApprovedBy: "u1",
				SourceObservationIDs: []int64{obs},
			})
			if err != nil {
				t.Fatal(err)
			}
			if !rule.PromotedAt.IsZero() || rule.PromotedBy != "" {
				t.Fatalf("new rule promoted = %+v, want unmarked", rule)
			}
			clock.Advance(0)
			if err := m.MarkRulePromoted(ctx, rule.RuleID, "u1"); err != nil {
				t.Fatal(err)
			}
			versions, err := m.RuleVersions(ctx, rule.RuleID)
			if err != nil || len(versions) != 1 {
				t.Fatalf("versions = %+v %v, want the one version", versions, err)
			}
			got := versions[0]
			if got.PromotedBy != "u1" {
				t.Fatalf("promoted_by = %q, want u1", got.PromotedBy)
			}
			if got.PromotedAt.IsZero() {
				t.Fatalf("promoted_at is zero, want the marking time")
			}
			if got.Status != StatusActive {
				t.Fatalf("status = %q, want active left untouched", got.Status)
			}
			if got.Text != rule.Text || got.ApprovedBy != "u1" || len(got.SourceObservationIDs) != 1 || got.SourceObservationIDs[0] != obs {
				t.Fatalf("provenance changed: %+v from %+v", got, rule)
			}
			// The stamped Rule stays searchable through manual recall.
			hits, err := m.Recall(ctx, "wrap errors", 10)
			if err != nil || len(hits) != 1 || hits[0].RuleID != rule.RuleID {
				t.Fatalf("recall = %+v %v, want the stamped promoted Rule", hits, err)
			}
			if err := m.MarkRulePromoted(ctx, "missing", "u1"); !errors.Is(err, ErrNotFound) {
				t.Fatalf("missing err = %v, want ErrNotFound", err)
			}
			if err := m.MarkRulePromoted(ctx, rule.RuleID, ""); !errors.Is(err, ErrInvalid) {
				t.Fatalf("empty promoter err = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestMarkRulePromotedKeepsDormant(t *testing.T) {
	for _, fac := range factories() {
		t.Run(fac.name, func(t *testing.T) {
			clock := newTestClock()
			m := fac.open(t, clock)
			ctx := context.Background()
			rule, err := m.AddRule(ctx, Rule{Text: "stale note", Kind: KindFact, ApprovedBy: "u1"})
			if err != nil {
				t.Fatal(err)
			}
			if err := m.SetRuleStatus(ctx, rule.RuleID, StatusDormant); err != nil {
				t.Fatal(err)
			}
			if err := m.MarkRulePromoted(ctx, rule.RuleID, "u1"); err != nil {
				t.Fatal(err)
			}
			got, err := m.RuleVersions(ctx, rule.RuleID)
			if err != nil || len(got) != 1 {
				t.Fatal(err)
			}
			if got[0].Status != StatusDormant {
				t.Fatalf("status = %q, want dormant left alone", got[0].Status)
			}
			if got[0].PromotedBy != "u1" || got[0].PromotedAt.IsZero() {
				t.Fatalf("dormant rule not marked: %+v", got[0])
			}
		})
	}
}

func TestPromotionGuardStillRefusesTextEdits(t *testing.T) {
	b, _, _ := openTestBank(t)
	ctx := context.Background()
	rule, err := b.AddRule(ctx, Rule{Text: "keep it", Kind: KindFact, ApprovedBy: "u1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.MarkRulePromoted(ctx, rule.RuleID, "u1"); err != nil {
		t.Fatal(err)
	}
	_, err = b.wdb.Exec(`UPDATE rules SET text = 'changed' WHERE rule_id = ?`, rule.RuleID)
	if err == nil || !strings.Contains(err.Error(), "only status or promotion") {
		t.Fatalf("text edit err = %v, want the guard refusal", err)
	}
	if err := b.SetRuleStatus(ctx, rule.RuleID, StatusActive); err != nil {
		t.Fatalf("status change after promotion = %v, want it allowed", err)
	}
}

func TestPromotionMigrationVersion(t *testing.T) {
	b, _, _ := openTestBank(t)
	var version int
	if err := b.wdb.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != len(migrations) || version < 2 {
		t.Fatalf("user_version = %d, want %d with the promotion migration", version, len(migrations))
	}
	var sql string
	if err := b.wdb.QueryRow(`SELECT sql FROM sqlite_master WHERE name = 'rules'`).Scan(&sql); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "promoted_at") || !strings.Contains(sql, "promoted_by") {
		t.Fatalf("rules schema = %q, want the promoted columns", sql)
	}
}

func TestIsPathInside(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub", "file.md")
	if !IsPathInside(sub, dir) {
		t.Fatalf("%s should be inside %s", sub, dir)
	}
	if IsPathInside(filepath.Join(dir, "..", "other"), dir) {
		t.Fatalf("sibling should not be inside")
	}
	if !IsPathInside(dir, dir) {
		t.Fatalf("dir should be inside itself")
	}
}

func TestPromotionNotePreviewShape(t *testing.T) {
	preview := PromotionNotePreview(".emberloop/notes/r1.md", "---\nid: r1\n---\nbody\n")
	if !strings.HasPrefix(preview, "--- /dev/null") || !strings.Contains(preview, "+++ b/.emberloop/notes/r1.md") {
		t.Fatalf("preview = %q, want a new-file diff", preview)
	}
	if !strings.Contains(preview, "+body") {
		t.Fatalf("preview = %q, want the body as added lines", preview)
	}
}

func TestSanitizeNoteID(t *testing.T) {
	if got := sanitizeNoteID("ABC123"); got != "abc123" {
		t.Fatalf("sanitize = %q", got)
	}
	if got := sanitizeNoteID("../../etc"); got == "" || strings.Contains(got, "/") {
		t.Fatalf("sanitize traversal = %q, want a safe stem", got)
	}
	abs, rel := NotesTargetPath(t.TempDir(), "../../etc")
	if strings.Contains(rel, "..") || strings.Contains(abs, "..") {
		// The rel must be slash-clean; abs resolution keeps it inside.
		t.Fatalf("note path escapes: %q %q", abs, rel)
	}
	_ = os.Getenv
}
