package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A done submit whose branch cannot rebase onto the spec branch is refused
// with an instruction to redo the rebase in the worktree and resolve it: a
// bare "branch unchanged" read as a status, and the Worker resubmitted done
// unchanged until the same-refusal needs-you alert fired.
func TestSpecMergeConflictRefusalTellsWorkerToResolve(t *testing.T) {
	t.Parallel()
	repo, _ := initRunRepo(t, map[string]string{"01-a": "# A\n"})
	testGit(t, repo, "branch", "spec/s")
	tree := filepath.Join(repo, ".el-worktrees", "01-a")
	testGit(t, repo, "worktree", "add", "-b", "implement/01-a", tree, "spec/s")
	if err := os.WriteFile(filepath.Join(tree, "shared.txt"), []byte("worker\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	testGit(t, tree, "add", "shared.txt")
	testGit(t, tree, "commit", "-m", "worker edits shared")

	specTree := filepath.Join(tempDir(t), "spec")
	testGit(t, repo, "worktree", "add", specTree, "spec/s")
	if err := os.WriteFile(filepath.Join(specTree, "shared.txt"), []byte("spec\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	testGit(t, specTree, "add", "shared.txt")
	testGit(t, specTree, "commit", "-m", "spec edits shared")

	before := testGit(t, repo, "rev-parse", "implement/01-a")
	d := NewDaemon(tempDir(t), nil)
	_, kept, err := d.mergeTicketIntoSpec(repo, tree, "implement/01-a", "spec/s", "", "01-a", "w")
	if err != nil {
		t.Fatalf("mergeTicketIntoSpec: %v", err)
	}
	for _, want := range []string{"rebase onto spec/s conflicted in shared.txt", "`git rebase spec/s`", "`git rebase --continue`", "submit done again"} {
		if !strings.Contains(kept, want) {
			t.Fatalf("kept = %q, want %q", kept, want)
		}
	}
	if got := testGit(t, repo, "rev-parse", "implement/01-a"); got != before {
		t.Fatalf("branch moved to %q, want it unchanged at %q", got, before)
	}
}
