package el

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

// gitConfigTemplate is a git repo skeleton built once per test run: `git
// init -b main` plus the local identity config, with no commits. Tests copy
// it into their own temp dir instead of spawning `git init` + two `git
// config` calls each, then write their files and commit as before.
//
// The template carries commit.gpgsign=false so a developer's global signing
// setup never breaks test commits; the local config travels with the copy.
// Test seam: test-only, no production code reads it.
var (
	gitConfigTemplateOnce sync.Once
	gitConfigTemplateDir  string
	gitConfigTemplateErr  error
)

func gitConfigTemplate(t *testing.T) string {
	t.Helper()
	gitConfigTemplateOnce.Do(func() {
		dir, err := os.MkdirTemp("", "el-gitconfig-template-*")
		if err != nil {
			gitConfigTemplateErr = err
			return
		}
		run := func(args ...string) error {
			cmd := exec.Command("git", args...)
			cmd.Dir = dir
			cmd.Env = append(os.Environ(),
				"GIT_CONFIG_GLOBAL="+os.DevNull,
				"GIT_CONFIG_SYSTEM="+os.DevNull,
				"GIT_CONFIG_NOSYSTEM=1",
				"HOME="+dir,
			)
			if out, err := cmd.CombinedOutput(); err != nil {
				return &templateGitError{args: args, out: string(out), err: err}
			}
			return nil
		}
		if err := run("init", "-q", "-b", "main"); err != nil {
			gitConfigTemplateErr = err
			return
		}
		if err := run("config", "user.email", "el@test.local"); err != nil {
			gitConfigTemplateErr = err
			return
		}
		if err := run("config", "user.name", "el"); err != nil {
			gitConfigTemplateErr = err
			return
		}
		if err := run("config", "commit.gpgsign", "false"); err != nil {
			gitConfigTemplateErr = err
			return
		}
		gitConfigTemplateDir = dir
	})
	if gitConfigTemplateErr != nil {
		t.Fatalf("build git template: %v", gitConfigTemplateErr)
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	return gitConfigTemplateDir
}

type templateGitError struct {
	args []string
	out  string
	err  error
}

func (e *templateGitError) Error() string {
	return "git " + joinTemplateArgs(e.args) + ": " + e.err.Error() + "\n" + e.out
}

// templatedGitRepo copies the shared init+config skeleton into a fresh temp
// dir and returns it. The caller writes files and commits as usual; the copy
// keeps the local identity config, so no per-test `git init`/`config` spawns.
func templatedGitRepo(t *testing.T) string {
	t.Helper()
	src := gitConfigTemplate(t)
	dst := t.TempDir()
	copyGitTemplate(t, src, dst)
	return dst
}

// copyGitTemplate copies every entry of src (including .git) into dst, which
// must exist. Pure filesystem copy: no subprocess, deterministic, and safe
// for concurrent test copies because the template is read-only after build.
func copyGitTemplate(t *testing.T, src, dst string) {
	t.Helper()
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		copyTemplateEntry(t, filepath.Join(src, e.Name()), filepath.Join(dst, e.Name()))
	}
}

func copyTemplateEntry(t *testing.T, src, dst string) {
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
			copyTemplateEntry(t, filepath.Join(src, e.Name()), filepath.Join(dst, e.Name()))
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

func joinTemplateArgs(args []string) string {
	out := ""
	for i, a := range args {
		if i > 0 {
			out += " "
		}
		out += a
	}
	return out
}

// fileBaseTemplate builds (once) a repo from the config skeleton plus one
// committed file, and copies it per test. The committed tree is
// byte-identical to what the callers used to init/write/add/commit
// themselves; only the build moves once per run. Test-only.
var (
	atxtBaseTemplateDir string
	atxtBaseTemplateErr error
)

func buildFileBaseTemplate(t *testing.T, name, content string) string {
	t.Helper()
	src := gitConfigTemplate(t)
	dir, err := os.MkdirTemp("", "el-git-filebase-*")
	if err != nil {
		t.Fatal(err)
	}
	copyGitTemplate(t, src, dir)
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "."}, {"commit", "-qm", "base"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build file base %s: git %v: %v\n%s", name, args, err, out)
		}
	}
	return dir
}

// templatedAtxtRepo copies the shared a.txt-base repo (init+config with a.txt
// "a" committed on main) for the driver merge tests.
func templatedAtxtRepo(t *testing.T) string {
	t.Helper()
	if atxtBaseTemplateDir == "" && atxtBaseTemplateErr == nil {
		atxtBaseTemplateDir = buildFileBaseTemplate(t, "a.txt", "a\n")
	}
	if atxtBaseTemplateErr != nil {
		t.Fatalf("build a.txt base: %v", atxtBaseTemplateErr)
	}
	dst := t.TempDir()
	copyGitTemplate(t, atxtBaseTemplateDir, dst)
	return dst
}

// cleanupGitTemplates removes the process-shared git template dirs built
// above. TestMain calls it after m.Run so a package run leaves nothing
// behind; each removal best-efforts with owner-write first.
func cleanupGitTemplates() {
	for _, dir := range []string{gitConfigTemplateDir, atxtBaseTemplateDir} {
		if dir == "" {
			continue
		}
		chmodTree(dir)
		removeRetry(dir)
	}
}

// TestMain cleans the shared git templates after the run.
func TestMain(m *testing.M) {
	code := m.Run()
	cleanupGitTemplates()
	os.Exit(code)
}
