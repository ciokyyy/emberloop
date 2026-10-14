package el

import (
	"os"
	"path/filepath"
	"testing"
)

// The sync mirrors docs/agent into the docs namespace and drops pages the
// checkout no longer has.
func TestSyncAgentDocsMirrorsSource(t *testing.T) {
	src, skills := t.TempDir(), t.TempDir()
	t.Setenv("EL_DOCS_DIR", src)
	t.Setenv("EL_SKILLS_DIR", skills)
	if err := os.WriteFile(filepath.Join(src, "update.md"), []byte("# Update"), 0o644); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(skills, docsNamespace, "gone.md")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	n, err := SyncAgentDocs()
	if err != nil || n != 1 {
		t.Fatalf("SyncAgentDocs = %d, %v; want 1 page", n, err)
	}
	got, err := os.ReadFile(filepath.Join(skills, docsNamespace, "update.md"))
	if err != nil || string(got) != "# Update" {
		t.Fatalf("update.md = %q, %v", got, err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale page kept: %v", err)
	}
}

// No docs directory is no pages and no error.
func TestSyncAgentDocsMissingSource(t *testing.T) {
	t.Setenv("EL_DOCS_DIR", filepath.Join(t.TempDir(), "nope"))
	t.Setenv("EL_SKILLS_DIR", t.TempDir())
	if n, err := SyncAgentDocs(); n != 0 || err != nil {
		t.Fatalf("SyncAgentDocs = %d, %v; want 0, nil", n, err)
	}
}
