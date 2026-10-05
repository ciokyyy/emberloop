package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, tool Tool, root string, args map[string]any) (string, error) {
	t.Helper()
	raw, _ := json.Marshal(args)
	return tool.Execute(context.Background(), root, raw)
}

func TestWriteCreatesDirsAndReportsTag(t *testing.T) {
	dir := t.TempDir()
	out, err := run(t, writeTool{}, dir, map[string]any{"path": "a/b/c.txt", "content": "x\ny\n"})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "a/b/c.txt")); string(got) != "x\ny\n" {
		t.Fatalf("file = %q", got)
	}
	if !strings.Contains(out, "#"+contentTag([]byte("x\ny\n"))+"]") || !strings.Contains(out, "2 lines") {
		t.Fatalf("output = %q", out)
	}
}

func TestWriteRefusesEscapes(t *testing.T) {
	dir := t.TempDir()
	for _, p := range []string{"/etc/x", "../x", "a/../../x", ""} {
		if _, err := run(t, writeTool{}, dir, map[string]any{"path": p, "content": "z"}); err == nil {
			t.Errorf("write %q succeeded", p)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "x")); err == nil {
		t.Fatal("escaped the root")
	}
}

func TestToolsRoundTripThroughJSON(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("one\ntwo\n"), 0o644)
	out, err := run(t, readTool{}, dir, map[string]any{"path": "f.txt", "limit": 1})
	if err != nil || !strings.Contains(out, "     1→one") || strings.Contains(out, "two") {
		t.Fatalf("read = %q, %v", out, err)
	}
}

func TestBackgroundRefusesWithoutHerdrAndBadActions(t *testing.T) {
	t.Setenv("HERDR_PANE_ID", "")
	if _, err := run(t, backgroundTool{}, t.TempDir(), map[string]any{"action": "start", "command": "sleep 1"}); err == nil || !strings.Contains(err.Error(), "not inside herdr") {
		t.Fatalf("err = %v", err)
	}
	if _, err := run(t, backgroundTool{}, t.TempDir(), map[string]any{"action": "dance"}); err == nil {
		t.Fatal("unknown action accepted")
	}
	if _, err := run(t, backgroundTool{}, t.TempDir(), map[string]any{"action": "stop"}); err == nil {
		t.Fatal("stop without a pane accepted")
	}
}

// skillsRootWith builds a temp skills root holding the given skills. Each skill
// is a directory under the namespace with a SKILL.md; body is the bytes of that
// file, so a test can omit frontmatter.
func skillsRootWith(t *testing.T, root, namespace string, skills map[string]string) {
	t.Helper()
	t.Setenv("WICK_SKILLS_DIR", root)
	for name, body := range skills {
		dir := filepath.Join(root, namespace, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReadWickIndexesSkills(t *testing.T) {
	root := t.TempDir()
	skillsRootWith(t, root, "matt-pocock", map[string]string{
		"grilling": "---\nname: grilling\ndescription: Grill a plan with relentless questions.\n---\n\nBody.\n",
		"research": "---\nname: research\ndescription: Research a topic deeply.\n---\n\nBody.\n",
	})
	out, err := run(t, readTool{}, t.TempDir(), map[string]any{"path": "wick://matt-pocock"})
	if err != nil {
		t.Fatalf("index read failed: %v", err)
	}
	for _, want := range []string{
		"grilling: Grill a plan with relentless questions.",
		"research: Research a topic deeply.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("index missing %q, got:\n%s", want, out)
		}
	}
}

func TestReadWickReadsAFile(t *testing.T) {
	root := t.TempDir()
	skillsRootWith(t, root, "matt-pocock", map[string]string{
		"grilling": "---\ndescription: Grill a plan.\n---\n\nAsk hard questions.\n",
	})
	out, err := run(t, readTool{}, t.TempDir(), map[string]any{"path": "wick://matt-pocock/grilling/SKILL.md"})
	if err != nil {
		t.Fatalf("file read failed: %v", err)
	}
	if !strings.Contains(out, "Ask hard questions.") || !strings.Contains(out, "wick://matt-pocock/grilling/SKILL.md#") {
		t.Fatalf("file read = %q", out)
	}
}

func TestReadWickUnknownNamespaceNamesWhatExists(t *testing.T) {
	root := t.TempDir()
	skillsRootWith(t, root, "matt-pocock", map[string]string{"grilling": "---\ndescription: x\n---\n"})
	_, err := run(t, readTool{}, t.TempDir(), map[string]any{"path": "wick://local"})
	if err == nil || !strings.Contains(err.Error(), "local") || !strings.Contains(err.Error(), "matt-pocock") {
		t.Fatalf("unknown namespace error = %v", err)
	}
	if _, err := run(t, readTool{}, t.TempDir(), map[string]any{"path": "wick://local/x"}); err == nil {
		t.Fatal("a file under an unknown namespace must be refused")
	}
}

func TestReadWickRefusesEscapes(t *testing.T) {
	root := t.TempDir()
	skillsRootWith(t, root, "matt-pocock", map[string]string{"grilling": "---\ndescription: x\n---\n"})
	secret := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(secret, []byte("top secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(root, "matt-pocock", "link.txt")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"wick://matt-pocock/../../secret.txt",
		"wick://matt-pocock/link.txt",
	} {
		out, err := run(t, readTool{}, t.TempDir(), map[string]any{"path": path})
		if err == nil {
			t.Errorf("read %q served %q, must refuse", path, out)
		}
	}
}

func TestReadWickIndexHandlesSkillWithNoDescription(t *testing.T) {
	root := t.TempDir()
	skillsRootWith(t, root, "matt-pocock", map[string]string{
		"described": "---\ndescription: Has one.\n---\n",
		"bare":      "# Just a heading, no frontmatter.\n",
	})
	out, err := run(t, readTool{}, t.TempDir(), map[string]any{"path": "wick://matt-pocock"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "described: Has one.") {
		t.Errorf("index dropped a description, got:\n%s", out)
	}
	if !strings.Contains(out, "bare") || strings.Contains(out, "bare:") {
		t.Errorf("a skill with no description must be listed by name alone, got:\n%s", out)
	}
}

func TestReadAllowStillServesAbsolutePaths(t *testing.T) {
	root := t.TempDir()
	allowed := t.TempDir()
	path := filepath.Join(allowed, "diff.patch")
	if err := os.WriteFile(path, []byte("--- a\n+++ b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, readTool{}, root, map[string]any{"path": path}); err == nil {
		t.Fatal("an absolute path outside the workspace must be refused without WICK_READ_ALLOW")
	}
	t.Setenv("WICK_READ_ALLOW", allowed)
	out, err := run(t, readTool{}, root, map[string]any{"path": path})
	if err != nil || !strings.Contains(out, "+++ b") {
		t.Fatalf("WICK_READ_ALLOW read = %q, %v", out, err)
	}
}

func TestReadWickIndexSkipsSymlinkedSkillFile(t *testing.T) {
	root := t.TempDir()
	skillsRootWith(t, root, "matt-pocock", map[string]string{
		"ok": "---\ndescription: Fine.\n---\n",
	})
	secret := filepath.Join(t.TempDir(), "SECRET.md")
	if err := os.WriteFile(secret, []byte("---\ndescription: leaked secret.\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	evil := filepath.Join(root, "matt-pocock", "evil")
	if err := os.MkdirAll(evil, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(evil, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, readTool{}, t.TempDir(), map[string]any{"path": "wick://matt-pocock"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "leaked secret") {
		t.Fatalf("index read a SKILL.md through a symlink out of the namespace:\n%s", out)
	}
	if !strings.Contains(out, "evil") {
		t.Fatalf("the skill must still be listed by name, got:\n%s", out)
	}
}

// A symlinked intermediate directory is the other half of the escape: the leaf
// is an ordinary file but the path to it leaves the namespace.
func TestReadWickRefusesSymlinkedDirectory(t *testing.T) {
	root := t.TempDir()
	skillsRootWith(t, root, "matt-pocock", map[string]string{"ok": "---\ndescription: x\n---\n"})
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("top secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "matt-pocock", "escape")); err != nil {
		t.Fatal(err)
	}
	if out, err := run(t, readTool{}, t.TempDir(), map[string]any{"path": "wick://matt-pocock/escape/secret.txt"}); err == nil {
		t.Fatalf("served %q through a symlinked directory", out)
	}
}

func TestReadWickIndexReadsBlockScalarDescription(t *testing.T) {
	root := t.TempDir()
	skillsRootWith(t, root, "matt-pocock", map[string]string{
		"folded": "---\nname: folded\ndescription: >\n  A long description\n  wrapped over lines.\nmetadata:\n  description: should be ignored\n---\n",
	})
	out, err := run(t, readTool{}, t.TempDir(), map[string]any{"path": "wick://matt-pocock"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "folded: A long description wrapped over lines.") {
		t.Fatalf("block scalar description not read, got:\n%s", out)
	}
	if strings.Contains(out, "should be ignored") {
		t.Fatalf("a nested description was read as the skill's:\n%s", out)
	}
}
