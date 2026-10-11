package memory

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeNoteFile writes one project note file under dir/.emberloop/notes/.
func writeNoteFile(t *testing.T, dir, name, id string, anchors []string, commit, author, subject, body string) {
	t.Helper()
	anchorList := "[]"
	if len(anchors) > 0 {
		quoted := make([]string, len(anchors))
		for i, a := range anchors {
			quoted[i] = `"` + a + `"`
		}
		anchorList = "[" + strings.Join(quoted, ", ") + "]"
	}
	content := "---\n" +
		"id: " + id + "\n" +
		"anchors: " + anchorList + "\n" +
		"observed_commit: " + commit + "\n" +
		"author: " + author + "\n" +
		"subject_key: " + subject + "\n" +
		"---\n" + body + "\n"
	writeRepoFile(t, dir, filepath.ToSlash(filepath.Join(NotesSubdir, name)), content)
}

func openNotesIndexForTest(t *testing.T, notes []Note) *NotesIndex {
	t.Helper()
	idx, err := OpenNotesIndex(notes)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = idx.Close() })
	return idx
}

// Two worktrees on different branches see different note sets. The notes are
// files, so branch-awareness falls out of the filesystem: each worktree
// loads and recalls only its own branch's notes.
func TestNotesBranchAwarenessTwoWorktrees(t *testing.T) {
	gitEnv(t)
	dir := t.TempDir()
	runGitTest(t, dir, "init", "-b", "main", ".")
	writeRepoFile(t, dir, "app.go", "package app\n")
	writeNoteFile(t, dir, "main-note.md", "main-note", nil, "", "u1", "app.main", "main branch upload handler zebra")
	runGitTest(t, dir, "add", "-A")
	runGitTest(t, dir, "commit", "-m", "main note")

	runGitTest(t, dir, "checkout", "-b", "feature")
	if err := os.Remove(filepath.Join(dir, NotesSubdir, "main-note.md")); err != nil {
		t.Fatal(err)
	}
	writeNoteFile(t, dir, "feature-note.md", "feature-note", nil, "", "u1", "app.feature", "feature branch upload handler giraffe")
	runGitTest(t, dir, "add", "-A")
	runGitTest(t, dir, "commit", "-m", "feature note")
	runGitTest(t, dir, "checkout", "main")
	// Detach so both branches can take an added worktree: git refuses a
	// second worktree for a checked-out branch.
	runGitTest(t, dir, "checkout", "--detach", "HEAD")
	wtBase := t.TempDir()
	wtMain := filepath.Join(wtBase, "wt-main")
	wtFeature := filepath.Join(wtBase, "wt-feature")
	runGitTest(t, dir, "worktree", "add", wtMain, "main")
	runGitTest(t, dir, "worktree", "add", wtFeature, "feature")

	mainNotes, mainNotices, err := LoadNotes(wtMain)
	if err != nil {
		t.Fatalf("LoadNotes(main): %v", err)
	}
	if len(mainNotices) != 0 {
		t.Fatalf("main notices = %q, want none", mainNotices)
	}
	if len(mainNotes) != 1 || mainNotes[0].ID != "main-note" {
		t.Fatalf("main notes = %+v, want [main-note]", mainNotes)
	}

	featNotes, _, err := LoadNotes(wtFeature)
	if err != nil {
		t.Fatalf("LoadNotes(feature): %v", err)
	}
	if len(featNotes) != 1 || featNotes[0].ID != "feature-note" {
		t.Fatalf("feature notes = %+v, want [feature-note]", featNotes)
	}

	ctx := context.Background()
	mainIdx := openNotesIndexForTest(t, mainNotes)
	featIdx := openNotesIndexForTest(t, featNotes)

	if got, _ := mainIdx.Search(ctx, "zebra", 10); len(got) != 1 || got[0].ID != "main-note" {
		t.Fatalf("main index zebra = %+v, want [main-note]", got)
	}
	if got, _ := mainIdx.Search(ctx, "giraffe", 10); len(got) != 0 {
		t.Fatalf("main index sees feature note: %+v", got)
	}
	if got, _ := featIdx.Search(ctx, "giraffe", 10); len(got) != 1 || got[0].ID != "feature-note" {
		t.Fatalf("feature index giraffe = %+v, want [feature-note]", got)
	}
	if got, _ := featIdx.Search(ctx, "zebra", 10); len(got) != 0 {
		t.Fatalf("feature index sees main note: %+v", got)
	}

	// Recall through verification keeps the branch split: anchorless notes
	// are clean without git.
	if got, _ := RecallNotes(ctx, wtMain, mainIdx, "zebra", 10); len(got) != 1 || got[0].Note.ID != "main-note" {
		t.Fatalf("recall main zebra = %+v, want [main-note]", got)
	}
	if got, _ := RecallNotes(ctx, wtFeature, featIdx, "zebra", 10); len(got) != 0 {
		t.Fatalf("recall feature zebra leaked main note: %+v", got)
	}
}

// A note with deleted anchors is skipped: verification reports dormant and
// recall drops it, like a Rule that would go dormant.
func TestNotesDeletedAnchorsSkipped(t *testing.T) {
	dir, base := initGitRepo(t, map[string]string{"app.go": "package app\n"})
	writeNoteFile(t, dir, "keep.md", "keep-app", []string{"app.go"}, base, "u1", "app.keep", "keep app simple with upload handler")
	runGitTest(t, dir, "add", "-A")
	runGitTest(t, dir, "commit", "-m", "add note")

	ctx := context.Background()
	notes, notices, err := LoadNotes(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(notices) != 0 {
		t.Fatalf("notices = %q, want none", notices)
	}
	if len(notes) != 1 {
		t.Fatalf("notes = %+v, want 1", notes)
	}
	idx := openNotesIndexForTest(t, notes)

	if res := VerifyNote(ctx, dir, notes[0]); res.Status != VerifyClean {
		t.Fatalf("before delete status = %q (%s), want clean", res.Status, res.Reason)
	}
	if got, err := RecallNotes(ctx, dir, idx, "keep app simple", 10); err != nil || len(got) != 1 {
		t.Fatalf("recall before delete = %d, %v; want 1", len(got), err)
	}

	runGitTest(t, dir, "rm", "app.go")
	runGitTest(t, dir, "commit", "-m", "drop app")

	if res := VerifyNote(ctx, dir, notes[0]); res.Status != VerifyDormant {
		t.Fatalf("after delete status = %q (%s), want dormant", res.Status, res.Reason)
	}
	if got, err := RecallNotes(ctx, dir, idx, "keep app simple", 10); err != nil || len(got) != 0 {
		t.Fatalf("recall after delete = %d, %v; want 0 (skipped)", len(got), err)
	}
}

// A malformed frontmatter file is skipped with a notice, not an error. The
// good note still loads and recalls.
func TestNotesMalformedSkippedWithNotice(t *testing.T) {
	dir := t.TempDir()
	notesDir := filepath.Join(dir, filepath.FromSlash(NotesSubdir))
	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeNoteFile(t, dir, "good.md", "good-note", nil, "", "u1", "app.good", "good note about upload handler")
	for name, content := range map[string]string{
		"no-frontmatter.md": "just body, no delimiters\n",
		"missing-id.md":     "---\nauthor: u1\n---\nbody without id\n",
		"empty-body.md":     "---\nid: empty-note\nauthor: u1\n---\n",
		"unclosed.md":       "---\nid: unclosed\nbody without close\n",
	} {
		if err := os.WriteFile(filepath.Join(notesDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	notes, notices, err := LoadNotes(dir)
	if err != nil {
		t.Fatalf("LoadNotes with malformed files err = %v, want nil", err)
	}
	if len(notes) != 1 || notes[0].ID != "good-note" {
		t.Fatalf("notes = %+v, want [good-note]", notes)
	}
	if len(notices) != 4 {
		t.Fatalf("notices = %q, want 4 skips", notices)
	}
	for _, n := range notices {
		if !strings.Contains(n, "skipped") {
			t.Errorf("notice %q does not say skipped", n)
		}
	}

	ctx := context.Background()
	idx := openNotesIndexForTest(t, notes)
	if got, err := RecallNotes(ctx, dir, idx, "upload handler", 10); err != nil || len(got) != 1 {
		t.Fatalf("recall good note = %d, %v; want 1", len(got), err)
	}

	// Duplicate ids keep the first file and notice the rest.
	writeNoteFile(t, dir, "aaa-dup.md", "dup", nil, "", "u1", "s", "first duplicate note")
	writeNoteFile(t, dir, "zzz-dup.md", "dup", nil, "", "u1", "s", "second duplicate note")
	got, dupNotices, err := LoadNotes(dir)
	if err != nil {
		t.Fatal(err)
	}
	dups := 0
	for _, n := range got {
		if n.ID == "dup" {
			dups++
		}
	}
	if dups != 1 {
		t.Fatalf("duplicate id loaded %d times, want 1", dups)
	}
	found := false
	for _, n := range dupNotices {
		if strings.Contains(n, "duplicate id") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no duplicate notice in %q", dupNotices)
	}
}

// Recall merges notes and Rules by rank fusion, never by comparing BM25
// scores across tables. FuseRecall takes only ordered slices, so a score
// cannot leak across; the scores are 1/(k+rank) from each side's own rank.
func TestNotesRecallMergesWithoutBM25(t *testing.T) {
	mkRule := func(id, text string) VerifiedRule {
		return VerifiedRule{
			Rule:   Rule{RuleID: id, Version: 1, Text: text, Kind: KindFact, Status: StatusActive, ApprovedBy: "u1"},
			Result: VerifyResult{Status: VerifyClean},
		}
	}
	mkNote := func(id, text string) VerifiedNote {
		return VerifiedNote{
			Note:   Note{ID: id, Text: text},
			Result: VerifyResult{Status: VerifyClean},
		}
	}
	rules := []VerifiedRule{mkRule("r1", "alpha"), mkRule("r2", "beta")}
	notes := []VerifiedNote{mkNote("n1", "gamma"), mkNote("n2", "delta")}

	fused := FuseRecall(rules, notes)
	if len(fused) != 4 {
		t.Fatalf("fused = %d hits, want 4", len(fused))
	}
	// Rank 1 on each side scores 1/61; the tie breaks Rules first, then by
	// id, so the merge interleaves instead of grouping one table first.
	wantOrder := []string{"r1", "n1", "r2", "n2"}
	for i, want := range wantOrder {
		var got string
		if fused[i].Source == MergeRule {
			got = fused[i].Rule.RuleID
		} else {
			got = fused[i].Note.ID
		}
		if got != want {
			t.Fatalf("fused[%d] = %s (%s), want %s: %+v", i, got, fused[i].Source, want, fused)
		}
	}
	wantScore := 1.0 / float64(RRFK+1)
	if fused[0].Score != wantScore || fused[1].Score != wantScore {
		t.Fatalf("rank-1 scores = %v, %v; want both %v (no BM25)", fused[0].Score, fused[1].Score, wantScore)
	}
	if fused[2].Score != 1.0/float64(RRFK+2) {
		t.Fatalf("rank-2 score = %v, want 1/(k+2)", fused[2].Score)
	}
	if got := FuseRecall(nil, nil); len(got) != 0 {
		t.Fatalf("empty fuse = %d, want 0", len(got))
	}

	// End to end: a bank Rule and a project note about the same query fuse
	// into one recall order carrying both sources.
	b, _, _ := openTestBank(t)
	ctx := context.Background()
	rule, err := b.AddRule(ctx, Rule{Text: "upload handler must close the body", Kind: KindFact, ApprovedBy: "u1"})
	if err != nil {
		t.Fatal(err)
	}
	bankHits, err := b.Recall(ctx, "upload handler", MaxRecall)
	if err != nil {
		t.Fatal(err)
	}
	noteIdx := openNotesIndexForTest(t, []Note{{ID: "note-1", Text: "upload handler retries on timeout"}})
	noteHits, err := RecallNotes(ctx, "", noteIdx, "upload handler", MaxRecall)
	if err != nil {
		t.Fatal(err)
	}
	var verified []VerifiedRule
	for _, r := range bankHits {
		verified = append(verified, VerifiedRule{Rule: r, Result: VerifyResult{Status: VerifyClean}})
	}
	merged := FuseRecall(verified, noteHits)
	if len(merged) != 2 {
		t.Fatalf("merged = %d hits, want 1 rule + 1 note", len(merged))
	}
	seen := map[string]bool{}
	for _, h := range merged {
		seen[h.Source] = true
	}
	if !seen[MergeRule] || !seen[MergeNote] {
		t.Fatalf("merged sources = %+v, want rule and note", merged)
	}
	found := false
	for _, h := range merged {
		if h.Source == MergeRule && h.Rule.RuleID == rule.RuleID {
			found = true
		}
	}
	if !found {
		t.Fatalf("bank rule %s missing from merge: %+v", rule.RuleID, merged)
	}
}

// Notes recall uses the same identifier-split matching as Rules: a camelCase
// note matches split words and vice versa. Hostile input never errors.
func TestNotesIdentifierSplitAndHostileInput(t *testing.T) {
	idx := openNotesIndexForTest(t, []Note{
		{ID: "ident", Text: "TestUploadHandler owns retries"},
		{ID: "plain", Text: "upload handler must close the body"},
	})
	ctx := context.Background()
	if got, err := idx.Search(ctx, "TestUploadHandler", 10); err != nil || len(got) != 2 {
		t.Fatalf("ident search = %d, %v; want 2", len(got), err)
	}
	if got, err := idx.Search(ctx, "upload handler", 10); err != nil || len(got) != 2 {
		t.Fatalf("words search = %d, %v; want 2", len(got), err)
	}
	if got, err := idx.Search(ctx, "", 10); err != nil || len(got) != 0 {
		t.Fatalf("empty search = %d, %v; want nil", len(got), err)
	}
	for _, q := range hostileQueries {
		if _, err := idx.Search(ctx, q, 10); err != nil {
			t.Fatalf("Search(%q): %v", q, err)
		}
		if _, err := RecallNotes(ctx, "", idx, q, 10); err != nil {
			t.Fatalf("RecallNotes(%q): %v", q, err)
		}
	}
	if got, err := idx.Search(ctx, `upload handler "AND" OR (`, 10); err != nil || len(got) != 2 {
		t.Fatalf("hostile matching search = %d, %v; want 2", len(got), err)
	}
}

func TestNotesIndexLenCloseAndNil(t *testing.T) {
	ctx := context.Background()
	idx, err := OpenNotesIndex(nil)
	if err != nil {
		t.Fatal(err)
	}
	if idx.Len() != 0 {
		t.Fatalf("empty index len = %d, want 0", idx.Len())
	}
	if err := idx.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	var nilIdx *NotesIndex
	if nilIdx.Len() != 0 {
		t.Fatal("nil index len != 0")
	}
	if err := nilIdx.Close(); err != nil {
		t.Fatalf("nil Close: %v", err)
	}
	if got, err := nilIdx.Search(ctx, "x", 5); err != nil || len(got) != 0 {
		t.Fatalf("nil search = %d, %v; want nil", len(got), err)
	}
	if got, err := RecallNotes(ctx, "", nil, "x", 5); err != nil || len(got) != 0 {
		t.Fatalf("nil recall = %d, %v; want nil", len(got), err)
	}
	if _, _, err := LoadNotes(""); err != nil {
		t.Fatalf("LoadNotes(\"\") err = %v, want nil", err)
	}
}

func TestParseNoteFrontmatterForms(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
		id      string
		anchors []string
	}{
		{"bracket list", "---\nid: a\nanchors: [\"x.go\", 'y.go']\n---\nbody\n", "a", []string{"x.go", "y.go"}},
		{"csv scalar", "---\nid: b\nanchors: x.go, y.go\n---\nbody\n", "b", []string{"x.go", "y.go"}},
		{"multiline list", "---\nid: c\nanchors:\n  - y.go\n  - x.go\n---\nbody\n", "c", []string{"x.go", "y.go"}},
		{"empty anchors", "---\nid: d\n---\nbody\n", "d", nil},
	} {
		n, err := ParseNote(tc.name, []byte(tc.content))
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if n.ID != tc.id {
			t.Fatalf("%s id = %q, want %q", tc.name, n.ID, tc.id)
		}
		if strings.Join(n.Anchors, ",") != strings.Join(tc.anchors, ",") {
			t.Fatalf("%s anchors = %q, want %q", tc.name, n.Anchors, tc.anchors)
		}
	}
	if _, err := ParseNote("quoted", []byte("---\nid: \"q\"\n---\nbody\n")); err != nil {
		t.Fatalf("quoted id: %v", err)
	} else {
		n, _ := ParseNote("quoted", []byte("---\nid: \"q\"\n---\nbody\n"))
		if n.ID != "q" {
			t.Fatalf("quoted id = %q, want q", n.ID)
		}
	}
}
