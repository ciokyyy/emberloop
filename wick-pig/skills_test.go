package wick

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeSkills answers the two commands an install runs. git init returns git;
// bunx writes the tree the real skills CLI would have written into the staging
// dir, unless fail decides otherwise.
type fakeSkills struct {
	skills map[string]string // skill name -> SKILL.md body
	lock   string
	git    Result
	args   []string
	stage  string
	fail   func() (Result, error)
}

func (f *fakeSkills) run(cmd string, args []string, opts ExecOpts) (Result, error) {
	if cmd == "git" {
		return f.git, nil
	}
	if cmd != "bunx" {
		return Result{}, nil
	}
	f.args, f.stage = args, opts.Cwd
	if f.fail != nil {
		return f.fail()
	}
	for name, body := range f.skills {
		dir := filepath.Join(opts.Cwd, ".agents", "skills", name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return Result{}, err
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
			return Result{}, err
		}
	}
	if f.lock != "" {
		if err := os.WriteFile(filepath.Join(opts.Cwd, "skills-lock.json"), []byte(f.lock), 0o644); err != nil {
			return Result{}, err
		}
	}
	return Result{}, nil
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func TestInstallSkillsFillsTheLibrary(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WICK_SKILLS_DIR", root)

	fake := &fakeSkills{
		skills: map[string]string{"tdd": "# tdd", "implement": "# implement"},
		lock:   `{"version":1}`,
	}
	if err := InstallSkills(fake.run); err != nil {
		t.Fatal(err)
	}

	if got := readFile(t, filepath.Join(root, "matt-pocock", "tdd", "SKILL.md")); got != "# tdd" {
		t.Errorf("tdd skill = %q", got)
	}
	if got := readFile(t, filepath.Join(root, "matt-pocock", "implement", "SKILL.md")); got != "# implement" {
		t.Errorf("implement skill = %q", got)
	}
	if got := readFile(t, filepath.Join(root, "skills-lock.json")); got != `{"version":1}` {
		t.Errorf("lock file kept beside the library = %q", got)
	}
	if _, err := os.Stat(fake.stage); !os.IsNotExist(err) {
		t.Errorf("the staging dir %s must be gone, stat err = %v", fake.stage, err)
	}

	got := strings.Join(fake.args, " ")
	for _, want := range []string{"skills@latest", "add mattpocock/skills", "--skill *", "--agent universal", "--copy", "-y"} {
		if !strings.Contains(got, want) {
			t.Errorf("bunx invocation missing %q: %v", want, fake.args)
		}
	}
}

func TestInstallSkillsKeepsTheOldLibraryOnFailure(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WICK_SKILLS_DIR", root)
	old := filepath.Join(root, "matt-pocock", "old")
	if err := os.MkdirAll(old, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(old, "SKILL.md"), []byte("# old"), 0o644); err != nil {
		t.Fatal(err)
	}

	fake := &fakeSkills{fail: func() (Result, error) {
		return Result{Stderr: "network down", Code: 1}, nil
	}}
	err := InstallSkills(fake.run)
	if err == nil || !strings.Contains(err.Error(), "network down") {
		t.Fatalf("a failed bunx must return its output, got %v", err)
	}
	if got := readFile(t, filepath.Join(old, "SKILL.md")); got != "# old" {
		t.Errorf("the previous library must survive a failed install, got %q", got)
	}
	if _, err := os.Stat(filepath.Join(root, "matt-pocock", "new")); !os.IsNotExist(err) {
		t.Errorf("a failed install must not add skills, stat err = %v", err)
	}
}

func TestInstallSkillsReportsMissingBunx(t *testing.T) {
	t.Setenv("WICK_SKILLS_DIR", t.TempDir())
	fake := &fakeSkills{fail: func() (Result, error) {
		return Result{}, &exec.Error{Name: "bunx", Err: exec.ErrNotFound}
	}}
	err := InstallSkills(fake.run)
	if err == nil {
		t.Fatal("a missing bunx must be reported, not silently ignored")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "install bun") {
		t.Errorf("a missing bunx must say to install bun, got %v", err)
	}
}

func TestInstallSkillsReportsFailedGitInit(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WICK_SKILLS_DIR", root)
	fake := &fakeSkills{
		skills: map[string]string{"one": "# one"},
		git:    Result{Stderr: "not a git repository", Code: 128},
	}
	err := InstallSkills(fake.run)
	if err == nil || !strings.Contains(err.Error(), "not a git repository") {
		t.Fatalf("a failed git init must return its output, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "matt-pocock")); !os.IsNotExist(err) {
		t.Errorf("a failed git init must install nothing, stat err = %v", err)
	}
}

func TestInstallSkillsReplacesTheOldLibrary(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WICK_SKILLS_DIR", root)

	first := &fakeSkills{skills: map[string]string{"one": "# one"}, lock: `{"one":1}`}
	if err := InstallSkills(first.run); err != nil {
		t.Fatal(err)
	}
	second := &fakeSkills{skills: map[string]string{"two": "# two"}}
	if err := InstallSkills(second.run); err != nil {
		t.Fatal(err)
	}

	if got := readFile(t, filepath.Join(root, "matt-pocock", "two", "SKILL.md")); got != "# two" {
		t.Errorf("the second install's skill = %q", got)
	}
	if _, err := os.Stat(filepath.Join(root, "matt-pocock", "one")); !os.IsNotExist(err) {
		t.Errorf("a replaced library must keep no leftover skills, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "skills-lock.json")); !os.IsNotExist(err) {
		t.Errorf("a replaced library must keep no stale lockfile, stat err = %v", err)
	}
}
