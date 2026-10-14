package tools

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

// gitHistoryTemplate is a repo with two commits and the v1.0 tag, built once
// per test run. gitRepo copies it instead of spawning init/config/add/commit
// per test, then writes the one unstaged change every subcommand needs.
// Test seam: test-only, no production code reads it.
var (
	gitHistoryTemplateOnce sync.Once
	gitHistoryTemplateDir  string
	gitHistoryTemplateErr  error
)

func gitHistoryTemplate(t *testing.T) string {
	t.Helper()
	gitHistoryTemplateOnce.Do(func() {
		dir, err := os.MkdirTemp("", "el-githistory-template-*")
		if err != nil {
			gitHistoryTemplateErr = err
			return
		}
		run := func(args ...string) error {
			cmd := exec.Command("git", args...)
			cmd.Dir = dir
			cmd.Env = append(os.Environ(),
				"GIT_CONFIG_GLOBAL="+os.DevNull,
				"GIT_CONFIG_SYSTEM="+os.DevNull,
			)
			if out, err := cmd.CombinedOutput(); err != nil {
				gitHistoryTemplateErr = templateGitHistoryErrorf(args, string(out), err)
				return gitHistoryTemplateErr
			}
			return nil
		}
		if err := run("init", "-q"); err != nil {
			return
		}
		if err := run("config", "user.email", "t@t.t"); err != nil {
			return
		}
		if err := run("config", "user.name", "t"); err != nil {
			return
		}
		if err := run("config", "commit.gpgsign", "false"); err != nil {
			return
		}
		if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello\nworld\n"), 0o644); err != nil {
			gitHistoryTemplateErr = err
			return
		}
		if err := run("add", "a.txt"); err != nil {
			return
		}
		if err := run("commit", "-qm", "first"); err != nil {
			return
		}
		if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello\nchanged\n"), 0o644); err != nil {
			gitHistoryTemplateErr = err
			return
		}
		if err := run("add", "a.txt"); err != nil {
			return
		}
		if err := run("commit", "-qm", "second"); err != nil {
			return
		}
		if err := run("tag", "v1.0"); err != nil {
			return
		}
		gitHistoryTemplateDir = dir
	})
	if gitHistoryTemplateErr != nil {
		t.Fatalf("build git history template: %v", gitHistoryTemplateErr)
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	return gitHistoryTemplateDir
}

func templateGitHistoryErrorf(args []string, out string, err error) error {
	return &gitHistoryTemplateError{args: args, out: out, err: err}
}

type gitHistoryTemplateError struct {
	args []string
	out  string
	err  error
}

func (e *gitHistoryTemplateError) Error() string {
	return "git history template failed: " + e.err.Error() + "\n" + e.out
}

// copyGitHistory copies the shared two-commit template into a fresh temp dir.
func copyGitHistory(t *testing.T) string {
	t.Helper()
	src := gitHistoryTemplate(t)
	dst := t.TempDir()
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		copyGitHistoryEntry(t, filepath.Join(src, e.Name()), filepath.Join(dst, e.Name()))
	}
	return dst
}

func copyGitHistoryEntry(t *testing.T, src, dst string) {
	t.Helper()
	fi, err := os.Lstat(src)
	if err != nil {
		t.Fatal(err)
	}
	if fi.IsDir() {
		if err := os.MkdirAll(dst, fi.Mode().Perm()); err != nil {
			t.Fatal(err)
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			copyGitHistoryEntry(t, filepath.Join(src, e.Name()), filepath.Join(dst, e.Name()))
		}
		return
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(src)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, dst); err != nil {
			t.Fatal(err)
		}
		return
	}
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fi.Mode().Perm())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}
