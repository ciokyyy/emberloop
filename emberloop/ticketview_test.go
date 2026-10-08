package emberloop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The preview seam is PreviewTickets over a ticket file map: the same files
// the driver runs against. Every test here asserts on the returned rows,
// never on the parser directly.
func previewSpec(t *testing.T) SpecRun {
	t.Helper()
	dir := t.TempDir()
	spec := filepath.Join(dir, "spec.md")
	if err := os.WriteFile(spec, []byte("# Demo spec\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tickets := filepath.Join(dir, "issues")
	if err := os.MkdirAll(tickets, 0o755); err != nil {
		t.Fatal(err)
	}
	return SpecRun{Spec: spec, Tickets: tickets}
}

func previewFiles(spec SpecRun, files map[string]string) map[string]string {
	out := make(map[string]string, len(files))
	for name, content := range files {
		out[filepath.Join(spec.Tickets, name)] = content
	}
	return out
}

func previewByID(out []TicketPreview) map[string]TicketPreview {
	byID := make(map[string]TicketPreview, len(out))
	for _, p := range out {
		byID[p.ID] = p
	}
	return byID
}

func TestSpecTitleReadsFirstHeading(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "spec.md")
	if err := os.WriteFile(spec, []byte("# Demo spec\n\n## Later\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := SpecTitle(spec); got != "Demo spec" {
		t.Fatalf("SpecTitle = %q, want %q", got, "Demo spec")
	}
}

func TestPreviewTicketsBlockersAndProgress(t *testing.T) {
	spec := previewSpec(t)
	byID := previewByID(PreviewTickets(spec, previewFiles(spec, map[string]string{
		"01-first.md":  "# 01: First\n\n**Blocked by:** None\n\n- [x] done box\n- [ ] todo box\n",
		"02-second.md": "# 02: Second\n\n**Blocked by:** 01\n\n- [ ] a box\n",
	})))
	first := byID["01-first"]
	if first.Num != 1 || first.Title != "First" {
		t.Fatalf("first = %+v, want 1/First", first)
	}
	if len(first.Blockers) != 0 || first.Done != 1 || first.Total != 2 {
		t.Fatalf("first = %+v, want no blockers and 1/2 boxes", first)
	}
	second := byID["02-second"]
	if len(second.Blockers) != 1 || second.Blockers[0] != "01-first" {
		t.Fatalf("second blockers = %q, want [01-first]", second.Blockers)
	}
	if second.Error != "" {
		t.Fatalf("second error = %q, want none", second.Error)
	}
}

func TestPreviewTicketsBadBlockedByIsolatesRow(t *testing.T) {
	spec := previewSpec(t)
	rows := PreviewTickets(spec, previewFiles(spec, map[string]string{
		"01-first.md": "# 01: First\n\n**Blocked by:** None\n\n- [ ] a box\n",
		"02-bad.md":   "# 02: Bad\n\n**Blocked by** none\n\n- [ ] a box\n",
	}))
	byID := previewByID(rows)
	if byID["01-first"].Error != "" {
		t.Fatalf("first error = %q, want none", byID["01-first"].Error)
	}
	bad := byID["02-bad"]
	if bad.Error == "" || !strings.Contains(bad.Error, "Blocked by") {
		t.Fatalf("bad error = %q, want a Blocked by error", bad.Error)
	}
}

func TestPreviewTicketsUnknownAmbiguousAndNumberless(t *testing.T) {
	spec := previewSpec(t)
	rows := PreviewTickets(spec, previewFiles(spec, map[string]string{
		"01-a.md":   "# 01: A\n\n**Blocked by:** None\n",
		"01-b.md":   "# 01: B\n\n**Blocked by:** None\n",
		"02-amb.md": "# 02: Amb\n\n**Blocked by:** 01\n",
		"03-unk.md": "# 03: Unk\n\n**Blocked by:** 01-nope\n",
		"notes.md":  "# Notes\n\n**Blocked by:** None\n",
	}))
	byID := previewByID(rows)
	if err := byID["02-amb"].Error; !strings.Contains(err, "ambiguous") {
		t.Fatalf("ambiguous error = %q, want ambiguous", err)
	}
	if err := byID["03-unk"].Error; !strings.Contains(err, "unknown") {
		t.Fatalf("unknown error = %q, want unknown", err)
	}
	for _, id := range []string{"02-amb", "03-unk", "notes"} {
		if len(byID[id].Blockers) != 0 {
			t.Fatalf("%s blockers = %q, want none on an error row", id, byID[id].Blockers)
		}
	}
	if err := byID["notes"].Error; !strings.Contains(err, "no ticket number") {
		t.Fatalf("notes error = %q, want no ticket number", err)
	}
	if len(rows) != 5 {
		t.Fatalf("rows = %d, want every ticket listed", len(rows))
	}
	want := []string{"notes", "01-a", "01-b", "02-amb", "03-unk"}
	for i, id := range want {
		if rows[i].ID != id {
			t.Fatalf("order = %v, want %v", rows, want)
		}
	}
}
