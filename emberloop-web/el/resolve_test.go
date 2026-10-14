package el

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example.com/emberloop-web/el/tools"
)

func TestResolveSpecAcceptsFileDirAndIssuesDir(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "dogfood9")
	if err := os.MkdirAll(filepath.Join(dir, "issues"), 0o755); err != nil {
		t.Fatal(err)
	}
	spec := filepath.Join(dir, "spec.md")
	if err := os.WriteFile(spec, []byte("# spec\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, arg := range []string{spec, dir, filepath.Join(dir, "issues")} {
		got, err := ResolveSpec(arg)
		if err != nil {
			t.Fatalf("ResolveSpec(%s): %v", arg, err)
		}
		want := SpecRun{Spec: spec, Tickets: filepath.Join(dir, "issues"), Branch: "spec/dogfood9"}
		if got != want {
			t.Errorf("ResolveSpec(%s) = %+v, want %+v", arg, got, want)
		}
	}
	if _, err := ResolveSpec(t.TempDir()); err == nil {
		t.Error("a directory with no spec.md must be refused")
	}
}

// A local spec-*.md plan resolves like any other spec file: the tickets
// live beside it and the branch takes the file's own name.
func TestResolveSpecLocalPlansFile(t *testing.T) {
	root := t.TempDir()
	plans := filepath.Join(root, "docs", "plans")
	if err := os.MkdirAll(plans, 0o755); err != nil {
		t.Fatal(err)
	}
	spec := filepath.Join(plans, "spec-05-demo.md")
	if err := os.WriteFile(spec, []byte("# Spec 05: Demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveSpec(spec)
	if err != nil {
		t.Fatalf("ResolveSpec: %v", err)
	}
	want := SpecRun{Spec: spec, Tickets: plans, Branch: "spec/spec-05-demo"}
	if got != want {
		t.Errorf("ResolveSpec = %+v, want %+v", got, want)
	}
}

func TestSpecMaxTicketsFrontmatter(t *testing.T) {
	write := func(content string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "spec-05-demo.md")
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	if got, ok := SpecMaxTickets(write("---\nmax_tickets: 2\n---\n\n# Spec 05: Demo\n")); !ok || got != 2 {
		t.Errorf("capped spec = %d, %v, want 2, true", got, ok)
	}
	if got, ok := SpecMaxTickets(write("---\nmax_agents: 1\n---\n\n# Spec 05: Demo\n")); !ok || got != 1 {
		t.Errorf("alias spec = %d, %v, want 1, true", got, ok)
	}
	if got, ok := SpecMaxTickets(write("---\nmax_tickets: 2\nmax_agents: 1\n---\n\n# Spec 05: Demo\n")); !ok || got != 2 {
		t.Errorf("both keys = %d, %v, want max_tickets 2 to win", got, ok)
	}
	if _, ok := SpecMaxTickets(write("# Spec 05: Demo\n")); ok {
		t.Error("a spec with no frontmatter must run under the global cap")
	}
	if _, ok := SpecMaxTickets(write("---\nmax_tickets: lots\n---\n\n# Spec 05: Demo\n")); ok {
		t.Error("a non-numeric max_tickets must be ignored")
	}
	if _, ok := SpecMaxTickets(write("---\nmax_tickets: 0\n---\n\n# Spec 05: Demo\n")); ok {
		t.Error("a non-positive max_tickets must be ignored")
	}
	if _, ok := SpecMaxTickets(filepath.Join(t.TempDir(), "missing.md")); ok {
		t.Error("an unreadable spec must run under the global cap")
	}
}

// The per-spec cap only lowers the global one: a spec cap above the global
// never raises it, and an empty spec leaves the global untouched.
func TestMaxTicketsForSpecLowersOnly(t *testing.T) {
	capped := filepath.Join(t.TempDir(), "spec-05-demo.md")
	if err := os.WriteFile(capped, []byte("---\nmax_tickets: 1\n---\n\n# Spec 05: Demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plain := filepath.Join(t.TempDir(), "spec-06-demo.md")
	if err := os.WriteFile(plain, []byte("# Spec 06: Demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	greedy := filepath.Join(t.TempDir(), "spec-07-demo.md")
	if err := os.WriteFile(greedy, []byte("---\nmax_tickets: 9\n---\n\n# Spec 07: Demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := MaxTicketsForSpec(capped, 5); got != 1 {
		t.Errorf("explicit global 5 with spec 1 = %d, want 1", got)
	}
	if got := MaxTicketsForSpec(plain, 5); got != 5 {
		t.Errorf("explicit global 5 without spec cap = %d, want 5", got)
	}
	if got := MaxTicketsForSpec(greedy, 5); got != 5 {
		t.Errorf("spec above the global cap = %d, want the global 5", got)
	}
	if got := MaxTicketsForSpec("", 5); got != 5 {
		t.Errorf("empty spec = %d, want the global 5", got)
	}
	if got := MaxTicketsForSpec("", 0); got != tools.DefaultMaxTickets {
		t.Errorf("empty global = %d, want the default %d", got, tools.DefaultMaxTickets)
	}
}

// Ticket scans skip spec files, so sibling specs beside the tickets never
func TestReadDriverTicketsSkipsSpecFiles(t *testing.T) {
	spec := filepath.Join("docs", "plans", "spec-05-demo.md")
	run := SpecRun{Spec: spec, Tickets: filepath.Join("docs", "plans"), Branch: "spec/spec-05-demo"}
	files := map[string]string{
		spec: "# Spec 05: Demo\n",
		filepath.Join("docs", "plans", "spec-06-other.md"): "# Spec 06: Other\n",
		filepath.Join("docs", "plans", "01-first.md"):      "# 01: First\n\n**Blocked by:** None\n",
	}
	tickets, err := readDriverTickets(run, files)
	if err != nil {
		t.Fatalf("readDriverTickets: %v", err)
	}
	if len(tickets) != 1 {
		t.Fatalf("tickets = %v, want only 01-first", tickets)
	}
	if _, ok := tickets["01-first"]; !ok {
		t.Errorf("tickets = %v, want 01-first", tickets)
	}
}

// A spec with no issues/ directory reads tickets beside the spec, where the
// directory doc lives. AGENTS.md, CLAUDE.md and README.md are never tickets:
// with real tickets they are skipped, and alone they leave "no tickets", not
// "has no ticket number".
func TestTicketScansSkipDirectoryDocs(t *testing.T) {
	dir := filepath.Join(".scratch", "x")
	spec := filepath.Join(dir, "spec.md")
	run := SpecRun{Spec: spec, Tickets: dir, Branch: "spec/x"}
	docs := map[string]string{
		spec:                            "# Spec\n",
		filepath.Join(dir, "AGENTS.md"): "# AGENTS.md\n",
		filepath.Join(dir, "CLAUDE.md"): "@AGENTS.md\n",
		filepath.Join(dir, "README.md"): "# Readme\n",
		filepath.Join(dir, "readme.md"): "# readme\n",
	}
	if _, err := readDriverTickets(run, docs); err == nil || !strings.Contains(err.Error(), "no tickets under") {
		t.Fatalf("docs only: err = %v, want no tickets under %s", err, dir)
	}
	if got := PreviewTickets(run, docs); len(got) != 0 {
		t.Fatalf("docs only: preview = %+v, want no rows", got)
	}
	files := map[string]string{filepath.Join(dir, "01-first.md"): "# 01: First\n\n**Blocked by:** None\n"}
	for k, v := range docs {
		files[k] = v
	}
	tickets, err := readDriverTickets(run, files)
	if err != nil {
		t.Fatalf("readDriverTickets: %v", err)
	}
	if len(tickets) != 1 || tickets["01-first"] == nil {
		t.Fatalf("tickets = %v, want only 01-first", tickets)
	}
	if got := PreviewTickets(run, files); len(got) != 1 || got[0].ID != "01-first" || got[0].Error != "" {
		t.Fatalf("preview = %+v, want one clean 01-first row", got)
	}
}

func TestSpecSlugFromAFileName(t *testing.T) {
	if got := specSlug("/x/Read Anywhere.md"); got != "read-anywhere" {
		t.Errorf("slug = %q", got)
	}
}
