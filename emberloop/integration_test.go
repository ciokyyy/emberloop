package el

import (
	"os"
	"path/filepath"
	"testing"
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

func TestSpecSlugFromAFileName(t *testing.T) {
	if got := specSlug("/x/Read Anywhere.md"); got != "read-anywhere" {
		t.Errorf("slug = %q", got)
	}
}

// The first launch creates the integration branch from HEAD and checks it out;
// a second launch, after work has landed on it, reuses it as it stands.
func TestEnterIntegrationBranchCreatesThenReuses(t *testing.T) {
	repo := t.TempDir()
	gitOut(t, repo, "init", "-q", "-b", "main")
	gitOut(t, repo, "config", "user.email", "el@test.local")
	gitOut(t, repo, "config", "user.name", "el")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitOut(t, repo, "add", ".")
	gitOut(t, repo, "commit", "-qm", "base")
	main := gitOut(t, repo, "rev-parse", "main")

	created, err := EnterIntegrationBranch(osExec, repo, "spec/x")
	if err != nil || !created {
		t.Fatalf("first enter: created=%v err=%v", created, err)
	}
	if on := gitOut(t, repo, "symbolic-ref", "--short", "HEAD"); on != "spec/x" {
		t.Fatalf("repo is on %q, want spec/x", on)
	}
	if got := gitOut(t, repo, "rev-parse", "spec/x"); got != main {
		t.Errorf("spec/x starts at %s, want HEAD %s", got, main)
	}

	gitOut(t, repo, "commit", "-q", "--allow-empty", "-m", "ticket merged")
	landed := gitOut(t, repo, "rev-parse", "HEAD")
	gitOut(t, repo, "switch", "-q", "main")

	created, err = EnterIntegrationBranch(osExec, repo, "spec/x")
	if err != nil || created {
		t.Fatalf("second enter: created=%v err=%v", created, err)
	}
	if got := gitOut(t, repo, "rev-parse", "HEAD"); got != landed {
		t.Errorf("resume is at %s, want the merged work %s", got, landed)
	}
}
