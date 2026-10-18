package el

// DOX index test (ticket 16): the AGENTS.md tree cannot rot silently.
// Every AGENTS.md must be listed in its parent doc's Child DOX Index, and
// every child a parent lists must exist. Index paths are repo-relative and
// backtick-quoted, e.g. `emberloop-web/el/tools/AGENTS.md`.

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// doxIndexEntries returns the repo-relative paths listed in the doc's
// Child DOX Index section: backtick-quoted entries ending in AGENTS.md.
func doxIndexEntries(content string) []string {
	lines := strings.Split(content, "\n")
	inIndex := false
	var out []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") && strings.Contains(trimmed, "Child DOX Index") {
			inIndex = true
			continue
		}
		if inIndex && strings.HasPrefix(trimmed, "#") {
			break
		}
		if !inIndex {
			continue
		}
		for _, field := range strings.Split(line, "`") {
			if strings.HasSuffix(field, "AGENTS.md") && !strings.Contains(field, " ") && !strings.Contains(field, "\n") {
				out = append(out, strings.TrimPrefix(field, "./"))
			}
		}
	}
	return out
}

// doxTreeProblems scans repository-owned AGENTS.md files and reports index
// gaps: a doc no parent lists, or a listed child with no file. Git-ignored
// local docs are not part of a checkout's tree. Empty means clean.
func doxTreeProblems(root string) []string {
	docs, err := doxAgentDocs(root)
	if err != nil {
		return []string{"scan " + root + ": " + err.Error()}
	}
	sort.Strings(docs)

	entries := map[string][]string{} // doc -> repo-relative docs its index lists
	var problems []string
	for _, doc := range docs {
		content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(doc)))
		if err != nil {
			problems = append(problems, doc+": unreadable: "+err.Error())
			continue
		}
		docDir := filepath.ToSlash(filepath.Dir(doc))
		for _, entry := range doxIndexEntries(string(content)) {
			target := entry
			// Only an explicit relative entry is resolved against the
			// listing doc's directory; anything else (including
			// dot-directories like `.scratch/...`) is repo-relative.
			if strings.HasPrefix(target, "./") || strings.HasPrefix(target, "../") {
				target = filepath.ToSlash(filepath.Join(docDir, target))
			}
			target = filepath.Clean(target)
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(target))); err != nil {
				problems = append(problems, doc+" lists "+entry+" which does not exist")
				continue
			}
			entries[doc] = append(entries[doc], target)
		}
	}
	have := map[string]bool{}
	for _, doc := range docs {
		have[doc] = true
	}
	for _, doc := range docs {
		if doc == "AGENTS.md" {
			continue
		}
		// The parent is the nearest ancestor directory with its own
		// AGENTS.md; a listing from any other doc does not count.
		parent := ""
		dir := filepath.ToSlash(filepath.Dir(filepath.Dir(doc)))
		for {
			candidate := "AGENTS.md"
			if dir != "." {
				candidate = dir + "/AGENTS.md"
			}
			if have[candidate] {
				parent = candidate
				break
			}
			if dir == "." {
				break
			}
			dir = filepath.ToSlash(filepath.Dir(dir))
		}
		found := false
		for _, target := range entries[parent] {
			if target == doc {
				found = true
				break
			}
		}
		if !found {
			if parent == "" {
				problems = append(problems, doc+" has no parent AGENTS.md above it")
			} else {
				problems = append(problems, doc+" is not listed in its parent "+parent+"'s Child DOX Index")
			}
		}
	}
	sort.Strings(problems)
	return problems
}

// doxAgentDocs uses Git to omit ignored worktree-local files. Standalone
// fixture trees in unit tests have no Git metadata, so they are walked.
func doxAgentDocs(root string) ([]string, error) {
	cmd := exec.Command("git", "-C", root, "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	if output, err := cmd.Output(); err == nil {
		var docs []string
		for _, path := range strings.Split(string(output), "\x00") {
			if path != "" && filepath.Base(filepath.FromSlash(path)) == "AGENTS.md" {
				docs = append(docs, filepath.ToSlash(path))
			}
		}
		return docs, nil
	}
	var docs []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() && (info.Name() == ".git" || info.Name() == ".emberloop-worktrees" || info.Name() == ".el-worktrees" || info.Name() == "node_modules") {
			return filepath.SkipDir
		}
		if !info.IsDir() && info.Name() == "AGENTS.md" {
			rel, err := filepath.Rel(root, path)
			if err == nil {
				docs = append(docs, filepath.ToSlash(rel))
			}
		}
		return nil
	})
	return docs, err
}

// doxRepoRoot finds the checkout root from this test's source location, not
// the test process's working directory, which may vary with the caller.
func doxRepoRoot(t *testing.T) string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate dox_test.go")
	}
	file, err := filepath.Abs(source)
	if err != nil {
		t.Fatal(err)
	}
	for dir := filepath.Dir(file); ; {
		if _, err := os.Stat(filepath.Join(dir, "emberloop-web")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no emberloop-web directory above " + file)
		}
		dir = parent
	}
}

func TestDoxTreeIndexIsComplete(t *testing.T) {
	if problems := doxTreeProblems(doxRepoRoot(t)); len(problems) > 0 {
		t.Errorf("DOX tree index is incomplete:\n- %s", strings.Join(problems, "\n- "))
	}
}

func TestDoxRepoRootIgnoresWorkingDirectory(t *testing.T) {
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(original); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	}()

	root := doxRepoRoot(t)
	if _, err := os.Stat(filepath.Join(root, "emberloop-web", "el", "dox_test.go")); err != nil {
		t.Fatalf("resolved root %q does not contain this test: %v", root, err)
	}
}

func writeDoxDoc(t *testing.T, root, rel, index string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "# Doc\n\n## Child DOX Index\n\n" + index + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDoxTreeProblemsFailsWhenAnEntryIsRemoved(t *testing.T) {
	root := t.TempDir()
	writeDoxDoc(t, root, "AGENTS.md", "- `a/AGENTS.md` — child a\n")
	writeDoxDoc(t, root, "a/AGENTS.md", "No child docs.\n")

	if problems := doxTreeProblems(root); len(problems) != 0 {
		t.Fatalf("complete mini tree should be clean, got %v", problems)
	}

	writeDoxDoc(t, root, "AGENTS.md", "No child docs.\n")
	problems := doxTreeProblems(root)
	if len(problems) != 1 || !strings.Contains(problems[0], "a/AGENTS.md") {
		t.Fatalf("removing the index entry must flag the orphaned doc, got %v", problems)
	}
}

func TestDoxTreeProblemsFailsWhenAChildIsRemoved(t *testing.T) {
	root := t.TempDir()
	writeDoxDoc(t, root, "AGENTS.md", "- `a/AGENTS.md` — child a\n")
	writeDoxDoc(t, root, "a/AGENTS.md", "No child docs.\n")

	if err := os.Remove(filepath.Join(root, "a", "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	problems := doxTreeProblems(root)
	if len(problems) != 1 || !strings.Contains(problems[0], "does not exist") {
		t.Fatalf("removing the child file must flag the dangling entry, got %v", problems)
	}
}
func TestDoxTreeProblemsSkipsLiveWorktrees(t *testing.T) {
	root := t.TempDir()
	writeDoxDoc(t, root, "AGENTS.md", "- `a/AGENTS.md` — child a\n")
	writeDoxDoc(t, root, "a/AGENTS.md", "No child docs.\n")
	// A sibling worktree carries its own copy of the tree; it is not part
	// of this checkout's index and must not be flagged.
	writeDoxDoc(t, root, ".emberloop-worktrees/other/AGENTS.md", "No child docs.\n")
	writeDoxDoc(t, root, ".emberloop-worktrees/other/emberloop/AGENTS.md", "No child docs.\n")

	if problems := doxTreeProblems(root); len(problems) != 0 {
		t.Fatalf("docs under .emberloop-worktrees must be ignored, got %v", problems)
	}
}

func TestDoxTreeProblemsRequiresTheParentIndex(t *testing.T) {
	root := t.TempDir()
	writeDoxDoc(t, root, "AGENTS.md", "- `a/AGENTS.md` — child a\n")
	writeDoxDoc(t, root, "a/AGENTS.md", "- `b/AGENTS.md` — not its child\n")
	writeDoxDoc(t, root, "b/AGENTS.md", "No child docs.\n")

	problems := doxTreeProblems(root)
	if len(problems) != 1 || !strings.Contains(problems[0], "b/AGENTS.md") {
		t.Fatalf("a doc listed only by a non-parent must be flagged, got %v", problems)
	}
}

func TestDoxTreeProblemsSkipsGitIgnoredDocs(t *testing.T) {
	root := t.TempDir()
	if output, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	writeDoxDoc(t, root, "AGENTS.md", "No child docs.")
	writeDoxDoc(t, root, "local/AGENTS.md", "Ignored local instructions.")
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("local/AGENTS.md\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if problems := doxTreeProblems(root); len(problems) != 0 {
		t.Fatalf("ignored local docs should not affect the repository index, got %v", problems)
	}
}
