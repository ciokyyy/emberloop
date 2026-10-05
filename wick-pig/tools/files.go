package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func obj(props map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

// ---------------------------------------------------------------------------
// read
// ---------------------------------------------------------------------------

type readTool struct{}

func (readTool) Name() string { return "read" }
func (readTool) Description() string {
	return "Read a file, with line numbers and a 4-hex snapshot tag to anchor edits. The tag goes stale if the file changes; re-read rather than editing against it."
}
func (readTool) Schema() map[string]any {
	return obj(map[string]any{
		"path":   map[string]any{"type": "string", "description": "File to read."},
		"offset": map[string]any{"type": "integer", "description": "First line, 1-based."},
		"limit":  map[string]any{"type": "integer", "description": "Maximum lines."},
	}, "path")
}
func (readTool) Execute(_ context.Context, root string, args json.RawMessage) (string, error) {
	var p struct {
		Path          string `json:"path"`
		Offset, Limit int
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid read parameters: %w", err)
	}
	if p.Path == "" {
		return "", errors.New("path is required")
	}
	// WICK_READ_ALLOW lists extra directories read may open by absolute path,
	// so a reviewer can read the diff and prompt a launcher wrote for it.
	if filepath.IsAbs(p.Path) {
		for _, dir := range filepath.SplitList(os.Getenv("WICK_READ_ALLOW")) {
			if rel, err := filepath.Rel(dir, p.Path); err == nil && !strings.HasPrefix(rel, "..") {
				return readPlain(dir, p.Path, p.Offset, p.Limit)
			}
		}
	}
	return readPlain(root, p.Path, p.Offset, p.Limit)
}

// ---------------------------------------------------------------------------
// edit
// ---------------------------------------------------------------------------

type editTool struct{}

func (editTool) Name() string        { return "edit" }
func (editTool) Description() string { return editDescription }
func (editTool) Schema() map[string]any {
	return obj(map[string]any{
		"input": map[string]any{
			"type":        "string",
			"description": `Hashline patch: a [path#TAG] header then line-anchored operations (PUT N.=M:, PUT <N:, PUT >N:, PUT >$:, CUT N.=M). One call may carry many operations; they apply against the file as you last read it.`,
		},
	}, "input")
}
func (editTool) Execute(_ context.Context, root string, args json.RawMessage) (string, error) {
	var p struct{ Input string }
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid edit parameters: %w", err)
	}
	if p.Input == "" {
		return "", errors.New("input is required")
	}
	return applyHashline(root, p.Input)
}

// ---------------------------------------------------------------------------
// write
// ---------------------------------------------------------------------------

// writeTool is ported from wick's apps/backend/tools/write.go: the file, whole,
// in one call, for a session with nothing to anchor an edit to.
type writeTool struct{}

func (writeTool) Name() string { return "write" }
func (writeTool) Description() string {
	return `Write a whole file, creating it and any directory it needs.

Use this to create a file; use edit to change lines inside one that already
exists. Paths are relative to the workspace root.`
}
func (writeTool) Schema() map[string]any {
	return obj(map[string]any{
		"path":    map[string]any{"type": "string", "description": "Workspace-relative path of the file to write."},
		"content": map[string]any{"type": "string", "description": "The complete content the file should hold."},
	}, "path", "content")
}
func (writeTool) Execute(ctx context.Context, root string, args json.RawMessage) (string, error) {
	// A cancelled turn writes nothing rather than finishing a write it started.
	if err := ctx.Err(); err != nil {
		return "", err
	}
	var p struct{ Path, Content string }
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid write parameters: %w", err)
	}
	rel := filepath.Clean(strings.TrimSpace(p.Path))
	if strings.TrimSpace(p.Path) == "" {
		return "", errors.New("write requires a non-empty path")
	}
	// Judged before anything is created: refusing after mkdir would leave the
	// directories an out-of-bounds path asked for behind.
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("write path %q must be relative to the workspace", rel)
	}
	if root == "" {
		root = "."
	}
	r, err := os.OpenRoot(root)
	if err != nil {
		return "", fmt.Errorf("open write root: %w", err)
	}
	defer r.Close()

	// Directories are made through the root, which is what refuses `..`, so a
	// file the session may not write cannot get a directory made for it.
	if err := makeDirs(r, filepath.ToSlash(rel)); err != nil {
		return "", err
	}
	data := []byte(p.Content)
	if err := r.WriteFile(filepath.ToSlash(rel), data, 0o644); err != nil {
		return "", fmt.Errorf("write %q: %w", rel, err)
	}
	return fmt.Sprintf("[%s#%s] written (%d lines)", rel, contentTag(data), lineCount(data)), nil
}

func makeDirs(r *os.Root, path string) error {
	dir := filepath.ToSlash(filepath.Dir(path))
	if dir == "." || dir == "/" {
		return nil
	}
	built := ""
	for _, part := range strings.Split(dir, "/") {
		if part == "" || part == "." {
			continue
		}
		built = strings.TrimPrefix(built+"/"+part, "/")
		if err := r.Mkdir(built, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("create directory %q: %w", built, err)
		}
	}
	return nil
}

func lineCount(data []byte) int {
	n := strings.Count(string(data), "\n")
	if len(data) > 0 && data[len(data)-1] != '\n' {
		n++
	}
	return n
}
