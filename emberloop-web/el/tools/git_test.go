package tools

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitIsRegisteredReadOnlyWithSchema(t *testing.T) {
	var found Tool
	for _, tl := range All() {
		if tl.Name() == "git" {
			found = tl
		}
	}
	if found == nil {
		t.Fatal("git is not registered in All()")
	}
	ro, ok := found.(ReadOnly)
	if !ok || !ro.ReadOnly() {
		t.Fatal("git must be ReadOnly")
	}
	schema := found.Schema()
	props, _ := schema["properties"].(map[string]any)
	if _, ok := props["args"]; !ok {
		t.Errorf("git schema missing %q", "args")
	}
	required, _ := schema["required"].([]string)
	if len(required) != 1 || required[0] != "args" {
		t.Errorf("git schema required = %v, want [args]", required)
	}
	if !strings.Contains(found.Description(), "git") {
		t.Errorf("git description should mention git, got %q", found.Description())
	}
}

// gitRepo copies the shared two-commit template and adds the unstaged
// change, so every allowed subcommand has something to report.
func gitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := copyGitHistory(t)
	// An unstaged change for diff/status to report.
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello\nworking\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestGitAllowsEachSubcommand(t *testing.T) {
	dir := gitRepo(t)
	cases := []struct {
		name string
		args []string
		want []string
	}{
		{"diff", []string{"diff"}, []string{"working"}},
		{"log", []string{"log", "--oneline"}, []string{"second", "first"}},
		{"show", []string{"show", "--stat", "HEAD"}, []string{"second"}},
		{"blame", []string{"blame", "a.txt"}, []string{"hello"}},
		{"status", []string{"status", "--short"}, []string{"a.txt"}},
		{"ls-files", []string{"ls-files"}, []string{"a.txt"}},
		{"grep", []string{"grep", "hello"}, []string{"a.txt"}},
		{"rev-parse", []string{"rev-parse", "HEAD"}, nil},
		{"merge-base", []string{"merge-base", "HEAD", "HEAD"}, nil},
		{"cat-file", []string{"cat-file", "-p", "HEAD"}, nil},
		{"branch", []string{"branch", "--list"}, nil},
		{"tag", []string{"tag", "--list"}, []string{"v1.0"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := run(t, gitTool{}, dir, map[string]any{"args": tc.args})
			if err != nil {
				t.Fatalf("git %v: %v", tc.args, err)
			}
			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Errorf("git %v output missing %q, got:\n%s", tc.args, want, out)
				}
			}
			if strings.Contains(out, "not allowed") || strings.Contains(out, "unknown option") {
				t.Errorf("git %v looks refused, got:\n%s", tc.args, out)
			}
		})
	}
}
func TestGitRefusesOtherSubcommands(t *testing.T) {
	dir := gitRepo(t)
	for _, args := range [][]string{
		{"checkout", "main"},
		{"switch", "main"},
		{"reset", "--hard", "HEAD"},
		{"clean", "-fd"},
		{"commit", "-m", "evil"},
		{"add", "a.txt"},
		{"rm", "a.txt"},
		{"mv", "a.txt", "b.txt"},
		{"push"},
		{"pull"},
		{"fetch"},
		{"clone", "https://example.com/r.git"},
		{"remote", "-v"},
		{"config", "--list"},
		{"stash", "push"},
		{"rebase", "HEAD~1"},
		{"merge", "main"},
		{"cherry-pick", "HEAD"},
		{"revert", "HEAD"},
		{"restore", "a.txt"},
		{"update-index", "--refresh"},
		{"apply"},
		{"am"},
		{"bisect", "start"},
		{"notes", "list"},
		{"worktree", "list"},
		{"submodule", "status"},
		{"ls-remote", "origin"},
	} {
		_, err := run(t, gitTool{}, dir, map[string]any{"args": args})
		if err == nil {
			t.Errorf("git %v succeeded, want a refusal", args)
			continue
		}
		msg := err.Error()
		if !strings.Contains(msg, "not allowed") || !strings.Contains(msg, "allowed subcommands") {
			t.Errorf("git %v refusal must name the allowed list, got %q", args, msg)
		}
	}
}

func TestGitRefusesDangerousFlags(t *testing.T) {
	dir := gitRepo(t)
	for _, args := range [][]string{
		{"-c", "user.name=evil", "log", "--oneline"},
		{"branch", "-c", "old", "new"},
		{"log", "--output=/tmp/out"},
		{"log", "--output", "/tmp/out"},
		{"diff", "--ext-diff"},
		{"diff", "--textconv"},
		{"log", "--textconv"},
		{"show", "--ext-diff"},
		{"grep", "--exec", "echo"},
		{"grep", "hello", "--upload-pack=evil"},
		{"log", "--upload-pack", "evil"},
		{"status", "--output=file"},
		// Unambiguous abbreviations parse as the full flag, so they
		// are refused too.
		{"log", "--out=/tmp/out"},
		{"log", "--ou", "--oneline"},
		{"diff", "--ext-d"},
		{"grep", "--textc", "hello"},
		{"log", "--upl=x"},
		{"log", "--upload"},
		// grep's pager runs an agent-chosen command, in any bundle.
		{"grep", "-O", "less", "hello"},
		{"grep", "-Oless", "hello"},
		{"grep", "-iOless", "hello"},
		{"grep", "--open-files-in-pager=less", "hello"},
		{"grep", "--open=less", "hello"},
		// cat-file's filters run the repo's drivers.
		{"cat-file", "--filters", "HEAD"},
		{"cat-file", "--filt", "HEAD"},
		// --show-signature runs the repo's gpg.program, and so does
		// formatting a signature out of --format/--pretty.
		{"log", "--show-signature"},
		{"show", "--show-s", "HEAD"},
		{"log", "--format=%G"},
		{"log", "--pretty=format:%G %s"},
		{"show", "--form=%s %G", "HEAD"},
		// --text is exact only on diff, log, show and grep: on blame
		// and cat-file it abbreviates --textconv (verified: `cat-file
		// --text` parses as `--textconv`), and the tool's own
		// --no-textconv comes first, so it would be overridden.
		{"blame", "--text", "a.txt"},
		{"cat-file", "--text", "HEAD"},
		{"status", "--text"},
	} {
		_, err := run(t, gitTool{}, dir, map[string]any{"args": args})
		if err == nil {
			t.Errorf("git %v succeeded, want a flag refusal", args)
			continue
		}
		if !strings.Contains(err.Error(), "not allowed") {
			t.Errorf("git %v refusal must say not allowed, got %q", args, err)
		}
	}
}

// Common read-only flags must keep working: the abbreviation matching
// above must not swallow legitimate options.
func TestGitKeepsLegitimateFlags(t *testing.T) {
	dir := gitRepo(t)
	for _, args := range [][]string{
		{"log", "--oneline", "--graph", "--decorate"},
		{"log", "--stat", "HEAD"},
		// --text is an exact option (binary as text) on diff, log,
		// show and grep only.
		{"log", "--text", "--oneline"},
		{"diff", "--text"},
		{"grep", "--text", "hello"},
		{"show", "--text", "HEAD"},
		// -O is an orderfile on diff, log and show.
		{"diff", "-O" + os.DevNull},
		{"log", "-O" + os.DevNull, "--oneline"},
		// Formats without %G never touch gpg.
		{"log", "--format=%h %s"},
		{"log", "--pretty=format:%h"},
		// %G as a search pattern is not a format.
		{"grep", "%G"},
		{"diff", "--cached", "--stat", "--name-only"},
		{"ls-files", "--exclude-standard", "--stage"},
		{"grep", "-i", "-n", "hello"},
		{"grep", "--no-textconv", "hello"},
		{"grep", "--", "--output"},
		{"show", "--name-only", "HEAD"},
		{"status", "--porcelain"},
		{"rev-parse", "--abbrev-ref", "HEAD"},
		{"branch", "--list", "--sort=-creatordate"},
		{"tag", "-l", "v*"},
		{"blame", "-L", "1,1", "a.txt"},
	} {
		if _, err := run(t, gitTool{}, dir, map[string]any{"args": args}); err != nil {
			t.Errorf("git %v refused, want it allowed: %v", args, err)
		}
	}
}

func TestGitBranchAndTagNeedAListFlag(t *testing.T) {
	dir := gitRepo(t)
	for _, args := range [][]string{
		{"branch", "--list"},
		{"branch", "-a"},
		{"branch", "--all"},
		{"branch", "-r"},
		{"branch"},
		{"tag", "--list"},
		{"tag", "-l"},
		{"tag"},
	} {
		if _, err := run(t, gitTool{}, dir, map[string]any{"args": args}); err != nil {
			t.Errorf("git %v refused, want list mode: %v", args, err)
		}
	}
	for _, args := range [][]string{
		{"branch", "-D", "main"},
		{"branch", "-d", "main"},
		{"branch", "-m", "old", "new"},
		{"branch", "-M", "old", "new"},
		{"branch", "-C", "old", "new"},
		{"branch", "newbranch"},
		{"branch", "--delete", "main"},
		{"branch", "-f", "main", "HEAD"},
		// Bundles decompose (`-ad` is `-a -d`) and long
		// abbreviations parse as the full flag (`--del` deleted a
		// branch when probed against real git).
		{"branch", "-ad"},
		{"branch", "-aD", "main"},
		{"branch", "-dr"},
		// Copy bundles the same way (`-ac` is `-a -c`).
		{"branch", "-ac", "old", "new"},
		{"tag", "-d", "v1.0"},
		{"tag", "-a", "v2.0", "-m", "msg"},
		{"tag", "-s", "v2.0", "-m", "msg"},
		{"tag", "v2.0"},
		{"tag", "--delete", "v1.0"},
		{"tag", "--del", "v1.0"},
		{"tag", "--d", "v1.0"},
		{"tag", "-dv", "v1.0"},
	} {
		_, err := run(t, gitTool{}, dir, map[string]any{"args": args})
		if err == nil {
			t.Errorf("git %v succeeded, want a list-only refusal", args)
			continue
		}
		if !strings.Contains(err.Error(), "listing only") {
			t.Errorf("git %v refusal must say listing only, got %q", args, err)
		}
	}
}
func TestGitRefusesEmptyArgs(t *testing.T) {
	dir := gitRepo(t)
	if _, err := run(t, gitTool{}, dir, map[string]any{"args": []string{}}); err == nil {
		t.Fatal("empty args succeeded")
	}
}

// gitHostileRepo wires a hostile repo-local config: an external diff
// driver, a textconv driver, a pager and an fsmonitor hook, each pointing
// at a script that marks the filesystem and prints a canary. Every allowed
// command must run without the marker appearing or the canary in its
// output.
func gitHostileRepo(t *testing.T) (dir, marker string) {
	t.Helper()
	dir = gitRepo(t)
	stash := t.TempDir()
	marker = stash + "/evil-ran"
	driver := stash + "/evil-driver.sh"
	script := "#!/bin/sh\ntouch " + marker + "\necho EVIL-DRIVER\n"
	if err := os.WriteFile(driver, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	cfg("config", "diff.external", driver)
	cfg("config", "diff.evil.textconv", driver)
	cfg("config", "core.pager", driver)
	// An fsmonitor hook runs on status and other index reads; the tool
	// forces it off through the environment, so it must stay quiet too.
	// It is wired last: the setup's own add/commit above would run it.
	monitor := stash + "/evil-fsmonitor.sh"
	if err := os.WriteFile(monitor, []byte("#!/bin/sh\ntouch "+marker+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitattributes"), []byte("*.bin diff=evil\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "evil.bin"), []byte{0x1, 0x2, 0x3, 0x4}, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg("add", ".gitattributes", "evil.bin")
	cfg("commit", "-qm", "attributes")
	if err := os.WriteFile(filepath.Join(dir, "evil.bin"), []byte{0x5, 0x6, 0x7, 0x8}, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg("config", "core.fsmonitor", monitor)
	return dir, marker
}

func TestGitHostileConfigNeverExecutes(t *testing.T) {
	dir, marker := gitHostileRepo(t)
	calls := [][]string{
		{"diff"}, {"diff", "HEAD"}, {"diff", "--cached"},
		{"log", "--oneline"}, {"log", "-p"},
		{"show", "--stat", "HEAD"}, {"show", "HEAD"},
		{"blame", "a.txt"},
		{"status", "--short"},
		{"ls-files"},
		{"grep", "hello"},
		{"rev-parse", "HEAD"},
		{"merge-base", "HEAD", "HEAD"},
		{"cat-file", "-p", "HEAD"},
		{"branch", "--list"},
		{"tag", "--list"},
	}
	for _, args := range calls {
		out, err := run(t, gitTool{}, dir, map[string]any{"args": args})
		if err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
		if strings.Contains(out, "EVIL-DRIVER") {
			t.Errorf("git %v ran the hostile driver, got:\n%s", args, out)
		}
		if _, err := os.Stat(marker); err == nil {
			t.Fatalf("git %v executed the hostile driver (marker exists)", args)
		}
	}
}

// gitSnapshot records the logical repo state: worktree bytes plus the
// refs, index and tree as git reports them.
func gitSnapshot(t *testing.T, dir string) string {
	t.Helper()
	var sb strings.Builder
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		if rel == ".git" || strings.HasPrefix(rel, ".git"+string(filepath.Separator)) {
			return nil
		}
		if !info.IsDir() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			sb.WriteString(rel + "\x00" + string(data) + "\x00")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"status", "--porcelain"},
		{"rev-parse", "HEAD"},
		{"for-each-ref"},
		{"diff", "HEAD"},
		{"diff", "--cached"},
		{"ls-files", "--stage"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_SYSTEM="+os.DevNull)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("snapshot git %v: %v\n%s", args, err, out)
		}
		sb.WriteString("\x00git " + strings.Join(args, " ") + "\n" + string(out))
	}
	return sb.String()
}

func TestGitLeavesRepoUntouched(t *testing.T) {
	dir, _ := gitHostileRepo(t)
	before := gitSnapshot(t, dir)
	calls := [][]string{
		{"diff"}, {"diff", "HEAD"},
		{"log", "--oneline"}, {"log", "-p"},
		{"show", "--stat", "HEAD"}, {"show", "HEAD"},
		{"blame", "a.txt"},
		{"status", "--short"}, {"status"},
		{"ls-files"}, {"ls-files", "--stage"},
		{"grep", "hello"}, {"grep", "--cached", "hello"},
		{"rev-parse", "HEAD"}, {"rev-parse", "--abbrev-ref", "HEAD"},
		{"merge-base", "HEAD", "HEAD"},
		{"cat-file", "-p", "HEAD"}, {"cat-file", "-t", "HEAD"},
		{"branch", "--list"}, {"branch", "-a"},
		{"tag", "--list"}, {"tag", "-l"},
	}
	for _, args := range calls {
		if _, err := run(t, gitTool{}, dir, map[string]any{"args": args}); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	if after := gitSnapshot(t, dir); after != before {
		t.Error("repository changed after a run of every allowed command")
	}
}
