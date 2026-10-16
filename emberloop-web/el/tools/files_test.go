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
	t.Setenv("HERDR_TAB_ID", "")
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

// Inside herdr but with no grid installed, start refuses rather than splitting
// the caller's own pane.
func TestBackgroundRefusesWithoutAGrid(t *testing.T) {
	t.Setenv("HERDR_PANE_ID", "w1:p1")
	if _, err := run(t, backgroundTool{}, t.TempDir(), map[string]any{"action": "start", "command": "sleep 1"}); err == nil || !strings.Contains(err.Error(), "no pane grid") {
		t.Fatalf("err = %v, want the missing grid named", err)
	}
}

// skillsRootWith builds a temp skills root holding the given skills. Each skill
// is a directory under the namespace with a SKILL.md; body is the bytes of that
// file, so a test can omit frontmatter.
func skillsRootWith(t *testing.T, root, namespace string, skills map[string]string) {
	t.Helper()
	t.Setenv("EL_SKILLS_DIR", root)
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

func TestReadElIndexesSkills(t *testing.T) {
	root := t.TempDir()
	skillsRootWith(t, root, "matt-pocock", map[string]string{
		"grilling": "---\nname: grilling\ndescription: Grill a plan with relentless questions.\n---\n\nBody.\n",
		"research": "---\nname: research\ndescription: Research a topic deeply.\n---\n\nBody.\n",
	})
	out, err := run(t, readTool{}, t.TempDir(), map[string]any{"path": "el://matt-pocock"})
	if err != nil {
		t.Fatalf("index read failed: %v", err)
	}
	for _, want := range []string{
		"grilling/: Grill a plan with relentless questions.",
		"research/: Research a topic deeply.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("index missing %q, got:\n%s", want, out)
		}
	}
}

func TestReadElReadsAFile(t *testing.T) {
	root := t.TempDir()
	skillsRootWith(t, root, "matt-pocock", map[string]string{
		"grilling": "---\ndescription: Grill a plan.\n---\n\nAsk hard questions.\n",
	})
	out, err := run(t, readTool{}, t.TempDir(), map[string]any{"path": "el://matt-pocock/grilling/SKILL.md"})
	if err != nil {
		t.Fatalf("file read failed: %v", err)
	}
	if !strings.Contains(out, "Ask hard questions.") || !strings.Contains(out, "[el://matt-pocock/grilling/SKILL.md] 5 lines") {
		t.Fatalf("file read = %q", out)
	}
	// A el:// file can never be edited, so it carries no tag and no line
	// numbers: they would only cost tokens.
	if strings.Contains(out, "#") || strings.Contains(out, "→") {
		t.Errorf("a el:// file must be plain text, got:\n%s", out)
	}
}

func TestReadElUnknownNamespaceNamesWhatExists(t *testing.T) {
	root := t.TempDir()
	skillsRootWith(t, root, "matt-pocock", map[string]string{"grilling": "---\ndescription: x\n---\n"})
	_, err := run(t, readTool{}, t.TempDir(), map[string]any{"path": "el://local"})
	if err == nil || !strings.Contains(err.Error(), "local") || !strings.Contains(err.Error(), "matt-pocock") {
		t.Fatalf("unknown namespace error = %v", err)
	}
	if _, err := run(t, readTool{}, t.TempDir(), map[string]any{"path": "el://local/x"}); err == nil {
		t.Fatal("a file under an unknown namespace must be refused")
	}
}

func TestReadElRefusesEscapes(t *testing.T) {
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
		"el://matt-pocock/../../secret.txt",
		"el://matt-pocock/link.txt",
	} {
		out, err := run(t, readTool{}, t.TempDir(), map[string]any{"path": path})
		if err == nil {
			t.Errorf("read %q served %q, must refuse", path, out)
		}
	}
}

func TestReadElIndexHandlesSkillWithNoDescription(t *testing.T) {
	root := t.TempDir()
	skillsRootWith(t, root, "matt-pocock", map[string]string{
		"described": "---\ndescription: Has one.\n---\n",
		"bare":      "# Just a heading, no frontmatter.\n",
	})
	out, err := run(t, readTool{}, t.TempDir(), map[string]any{"path": "el://matt-pocock"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "described/: Has one.") {
		t.Errorf("index dropped a description, got:\n%s", out)
	}
	if !strings.Contains(out, "bare") || strings.Contains(out, "bare:") {
		t.Errorf("a skill with no description must be listed by name alone, got:\n%s", out)
	}
}

func TestReadOpensAnAbsolutePathOutsideTheWorkspace(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "brief.md")
	if err := os.WriteFile(outside, []byte("the brief\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, readTool{}, root, map[string]any{"path": outside})
	if err != nil {
		t.Fatalf("read of an absolute path outside the workspace failed: %v", err)
	}
	if !strings.Contains(out, "the brief") {
		t.Fatalf("read = %q", out)
	}
}

func TestReadRefusesCredentialPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	secret := "SUPER-SECRET-VALUE"
	paths := []string{
		filepath.Join(home, ".ssh", "id_rsa"),
		filepath.Join(home, ".pig", "agent", "auth.json"),
		filepath.Join(home, ".pig", "agent", "mcp-auth.json"),
		filepath.Join(home, ".claude.json"),
		filepath.Join(home, ".claude", ".credentials.json"),
		filepath.Join(home, "project", ".env"),
		filepath.Join(home, "project", ".env.local"),
	}
	for _, p := range paths {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(secret), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// The whole ~/.ssh directory is a credential path, not only the files in it.
	paths = append(paths, filepath.Join(home, ".ssh"))

	root := t.TempDir()
	for _, p := range paths {
		out, err := run(t, readTool{}, root, map[string]any{"path": p})
		if err == nil {
			t.Errorf("read served credential path %s: %q", p, out)
			continue
		}
		if !strings.Contains(err.Error(), "credential path") {
			t.Errorf("read %s = %q, must name it a credential path", p, err)
		}
		if strings.Contains(out, secret) {
			t.Errorf("read leaked the secret at %s", p)
		}
	}
	// A neighbour is still readable, so the deny list is narrow.
	ok := filepath.Join(home, "notes.md")
	if err := os.WriteFile(ok, []byte("plain note\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := run(t, readTool{}, root, map[string]any{"path": ok}); err != nil || !strings.Contains(out, "plain note") {
		t.Errorf("a non-credential file under HOME must be readable: %q, %v", out, err)
	}
}

func TestReadRefusesEnvFileInsideTheWorkspace(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("TOKEN=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, readTool{}, root, map[string]any{"path": ".env"}); err == nil || !strings.Contains(err.Error(), "credential path") {
		t.Fatalf("relative .env read = %v", err)
	}
}

func TestReadRefusesASymlinkIntoACredentialPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".ssh", "id_rsa"), []byte("PRIVATE KEY\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	link := filepath.Join(root, "innocent.txt")
	if err := os.Symlink(filepath.Join(home, ".ssh", "id_rsa"), link); err != nil {
		t.Fatal(err)
	}
	if out, err := run(t, readTool{}, root, map[string]any{"path": link}); err == nil {
		t.Fatalf("a symlink into ~/.ssh must be refused, served %q", out)
	}
}

func TestReadElIndexSkipsSymlinkedSkillFile(t *testing.T) {
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
	out, err := run(t, readTool{}, t.TempDir(), map[string]any{"path": "el://matt-pocock"})
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
func TestReadElRefusesSymlinkedDirectory(t *testing.T) {
	root := t.TempDir()
	skillsRootWith(t, root, "matt-pocock", map[string]string{"ok": "---\ndescription: x\n---\n"})
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("top secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "matt-pocock", "escape")); err != nil {
		t.Fatal(err)
	}
	if out, err := run(t, readTool{}, t.TempDir(), map[string]any{"path": "el://matt-pocock/escape/secret.txt"}); err == nil {
		t.Fatalf("served %q through a symlinked directory", out)
	}
}

func TestReadElIndexReadsBlockScalarDescription(t *testing.T) {
	root := t.TempDir()
	skillsRootWith(t, root, "matt-pocock", map[string]string{
		"folded": "---\nname: folded\ndescription: >\n  A long description\n  wrapped over lines.\nmetadata:\n  description: should be ignored\n---\n",
	})
	out, err := run(t, readTool{}, t.TempDir(), map[string]any{"path": "el://matt-pocock"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "folded/: A long description wrapped over lines.") {
		t.Fatalf("block scalar description not read, got:\n%s", out)
	}
	if strings.Contains(out, "should be ignored") {
		t.Fatalf("a nested description was read as the skill's:\n%s", out)
	}
}

// The credential deny list is a property of read, not of one address scheme:
// a .env sitting inside a skill namespace is refused the same as one on disk.
func TestReadElRefusesCredentialPaths(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	skillsRootWith(t, root, "matt-pocock", map[string]string{"grilling": "---\ndescription: x\n---\n"})
	env := filepath.Join(root, "matt-pocock", "grilling", ".env")
	if err := os.WriteFile(env, []byte("TOKEN=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := run(t, readTool{}, t.TempDir(), map[string]any{"path": "el://matt-pocock/grilling/.env"}); err == nil {
		t.Fatalf("a .env under a namespace must be refused, served %q", out)
	}
}

// A directory reads as a one-level listing: directories end in "/", files
// carry a size. A workspace directory and a el:// directory share it.
func TestReadListsADirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "a.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"docs", filepath.Join(root, "docs")} {
		out, err := run(t, readTool{}, root, map[string]any{"path": path})
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, want := range []string{"2 entries", "sub/", "a.md  6B"} {
			if !strings.Contains(out, want) {
				t.Errorf("read %s missing %q:\n%s", path, want, out)
			}
		}
	}
}

func TestReadPagesALargeDirectory(t *testing.T) {
	root := t.TempDir()
	for _, n := range []string{"a", "b", "c", "d"} {
		if err := os.WriteFile(filepath.Join(root, n), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out, err := run(t, readTool{}, root, map[string]any{"path": ".", "offset": 2, "limit": 2})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "\nb  ") || !strings.Contains(out, "\nc  ") || strings.Contains(out, "\na  ") || strings.Contains(out, "\nd  ") {
		t.Errorf("offset 2 limit 2 must list b and c only:\n%s", out)
	}
	if !strings.Contains(out, "4 entries") {
		t.Errorf("the header must count every entry:\n%s", out)
	}
}

// Reading a skill's directory leads to the skill: the listing ends with a hint
// naming its SKILL.md.
func TestReadElSkillDirectoryPointsAtSkillFile(t *testing.T) {
	root := t.TempDir()
	skillsRootWith(t, root, "matt-pocock", map[string]string{"grilling": "---\ndescription: x\n---\n"})
	out, err := run(t, readTool{}, t.TempDir(), map[string]any{"path": "el://matt-pocock/grilling"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "SKILL.md  ") || !strings.Contains(out, "read el://matt-pocock/grilling/SKILL.md for the skill") {
		t.Errorf("a skill directory must list and point at SKILL.md:\n%s", out)
	}
}

func TestReadExpandsAGlob(t *testing.T) {
	root := t.TempDir()
	for _, n := range []string{"a.md", "b.md", "c.txt"} {
		if err := os.WriteFile(filepath.Join(root, n), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out, err := run(t, readTool{}, root, map[string]any{"path": "*.md"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "2 matches") || !strings.Contains(out, "a.md") || !strings.Contains(out, "b.md") || strings.Contains(out, "c.txt") {
		t.Errorf("glob *.md:\n%s", out)
	}
	if strings.Contains(out, "x") && strings.Contains(out, "→") {
		t.Errorf("a glob lists paths, never contents:\n%s", out)
	}
	if _, err := run(t, readTool{}, root, map[string]any{"path": "*.go"}); err == nil || !strings.Contains(err.Error(), "no match") {
		t.Errorf("a glob with no match must say so, got %v", err)
	}
}

func TestReadElFilePagesWithoutTag(t *testing.T) {
	root := t.TempDir()
	skillsRootWith(t, root, "matt-pocock", map[string]string{"s": "one\ntwo\nthree\n"})
	out, err := run(t, readTool{}, t.TempDir(), map[string]any{"path": "el://matt-pocock/s/SKILL.md", "offset": 2, "limit": 1})
	if err != nil {
		t.Fatal(err)
	}
	if out != "[el://matt-pocock/s/SKILL.md] 3 lines\ntwo\n" {
		t.Errorf("got %q", out)
	}
}

func TestReadElPageWithoutExtension(t *testing.T) {
	root := t.TempDir()
	skillsRootWith(t, root, "docs", nil)
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "update.md"), []byte("# Update\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, readTool{}, t.TempDir(), map[string]any{"path": "el://docs/update"})
	if err != nil || !strings.Contains(out, "# Update") {
		t.Fatalf("extensionless read = %q, %v", out, err)
	}
	if _, err := run(t, readTool{}, t.TempDir(), map[string]any{"path": "el://docs/nope"}); err == nil {
		t.Fatal("a missing page must still fail")
	}
}

func TestReadElPrefersOverrideAndFallsBackToUpstreamOnlySkills(t *testing.T) {
	root := t.TempDir()
	skillsRootWith(t, root, "matt-pocock", map[string]string{
		"to-tickets":    "upstream version",
		"upstream-only": "available upstream",
	})
	skillsRootWith(t, root, "emberloop", map[string]string{
		"to-tickets": "override version",
	})

	for _, test := range []struct{ path, want string }{
		{"el://matt-pocock/to-tickets/SKILL.md", "override version"},
		{"el://matt-pocock/upstream-only/SKILL.md", "available upstream"},
	} {
		out, err := run(t, readTool{}, t.TempDir(), map[string]any{"path": test.path})
		if err != nil || !strings.Contains(out, test.want) {
			t.Errorf("read %s = %q, %v; want %q", test.path, out, err, test.want)
		}
	}
}
