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
