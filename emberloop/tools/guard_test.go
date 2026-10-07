package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func guardWorkspace(t *testing.T, paths string) string {
	t.Helper()
	root := t.TempDir()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(root, ".emberloop"), 0o755))
	must(os.MkdirAll(filepath.Join(root, ".scratch", "issues"), 0o755))
	must(os.MkdirAll(filepath.Join(root, "src"), 0o755))
	must(os.WriteFile(filepath.Join(root, ".emberloop", "settings.json"), []byte(`{"orchestrator_paths":`+paths+`}`), 0o644))
	must(os.WriteFile(filepath.Join(root, ".scratch", "issues", "01.md"), []byte("Status: ready\n"), 0o644))
	must(os.WriteFile(filepath.Join(root, "src", "a.go"), []byte("package a\n"), 0o644))
	return root
}

func call(t *testing.T, tool Tool, root string, args any) error {
	t.Helper()
	b, _ := json.Marshal(args)
	_, err := tool.Execute(context.Background(), root, b)
	return err
}

func guardedWrite() Tool { return Guard(writeTool{}, ProjectWritePaths) }
func guardedEdit() Tool  { return Guard(editTool{}, ProjectWritePaths) }

func TestGuardWritesInsideAllowedDirAndFile(t *testing.T) {
	root := guardWorkspace(t, `[".scratch/", "NOTES.md"]`)
	for _, p := range []string{".scratch/issues/02.md", ".scratch/new/dir/x.md", "NOTES.md"} {
		if err := call(t, guardedWrite(), root, map[string]string{"path": p, "content": "x\n"}); err != nil {
			t.Errorf("write %s: %v", p, err)
		}
	}
}

func TestGuardRefusesOutsideTheAllowlist(t *testing.T) {
	root := guardWorkspace(t, `[".scratch/"]`)
	cases := []string{"src/a.go", "src/new.go", ".scratch/../src/a.go", "../escape.md", "/etc/passwd", ".scratchy/x.md"}
	for _, p := range cases {
		err := call(t, guardedWrite(), root, map[string]string{"path": p, "content": "x"})
		if err == nil {
			t.Errorf("write %s must be refused", p)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "src", "new.go")); err == nil {
		t.Errorf("a refused write must create nothing")
	}
}

func TestGuardEditFollowsTheSamePolicy(t *testing.T) {
	root := guardWorkspace(t, `[".scratch/"]`)
	file := filepath.Join(root, ".scratch", "issues", "01.md")
	data, _ := os.ReadFile(file)
	patch := "[.scratch/issues/01.md#" + contentTag(data) + "]\nPUT 1.=1:\n+Status: done"
	if err := call(t, guardedEdit(), root, map[string]string{"input": patch}); err != nil {
		t.Fatalf("edit inside allowlist: %v", err)
	}
	if got, _ := os.ReadFile(file); !strings.Contains(string(got), "Status: done") {
		t.Errorf("edit not applied: %q", got)
	}
	src, _ := os.ReadFile(filepath.Join(root, "src", "a.go"))
	out := "[src/a.go#" + contentTag(src) + "]\nPUT 1.=1:\n+package b"
	if err := call(t, guardedEdit(), root, map[string]string{"input": out}); err == nil {
		t.Errorf("edit outside the allowlist must be refused")
	}
	if err := call(t, guardedEdit(), root, map[string]string{"input": "no header"}); err == nil {
		t.Errorf("a patch without a header must be refused")
	}
}

func TestGuardSymlinkCannotLeaveAnAllowedDir(t *testing.T) {
	root := guardWorkspace(t, `[".scratch/"]`)
	if err := os.Symlink(filepath.Join(root, "src"), filepath.Join(root, ".scratch", "link")); err != nil {
		t.Skip("symlinks unavailable")
	}
	if err := call(t, guardedWrite(), root, map[string]string{"path": ".scratch/link/a.go", "content": "x"}); err == nil {
		t.Errorf("a symlink out of an allowed directory must not be followed")
	}
}

func TestGuardNeverLetsTheAllowlistBeWidened(t *testing.T) {
	root := guardWorkspace(t, `[".", ".emberloop/", ".scratch/"]`)
	if err := call(t, guardedWrite(), root, map[string]string{"path": ".emberloop/settings.json", "content": `{"orchestrator_paths":["."]}`}); err == nil {
		t.Errorf("the project settings must not be writable")
	}
	if err := call(t, guardedWrite(), root, map[string]string{"path": "src/a.go", "content": "x"}); err == nil {
		t.Errorf(`"." must not allow the whole workspace`)
	}
}

func TestGuardWithNoSettingsAllowsNothing(t *testing.T) {
	root := t.TempDir()
	err := call(t, guardedWrite(), root, map[string]string{"path": "a.md", "content": "x"})
	if err == nil || !strings.Contains(err.Error(), "orchestrator_paths") {
		t.Errorf("want a refusal that names orchestrator_paths, got %v", err)
	}
}
