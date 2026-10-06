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

func (readTool) Name() string   { return "read" }
func (readTool) ReadOnly() bool { return true }
func (readTool) Description() string {
	return "Read a file, with line numbers and a 4-hex snapshot tag to anchor edits. The tag goes stale if the file changes; re-read rather than editing against it. " +
		"Any absolute path works, except credential files. A directory returns a listing, a glob (*.md) the matching paths. wick:// files are plain text: they cannot be edited. " +
		"Also reads http(s):// and www. links through a tiered reader (native fetch, then Jina, then an uninstalled browser): HTML becomes Markdown, JSON is pretty-printed, " +
		"text and Markdown pass through; append :N, :N-M, :N+M, comma lists or :raw for line selectors or the unshaped body."
}
func (readTool) Schema() map[string]any {
	return obj(map[string]any{
		"path":   map[string]any{"type": "string", "description": "File, directory, glob, wick:// address, or http(s):// / www. URL to read."},
		"offset": map[string]any{"type": "integer", "description": "First line (or entry), 1-based."},
		"limit":  map[string]any{"type": "integer", "description": "Maximum lines (or entries)."},
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
	// ctx: handles page stored tool output (the overflow store): ctx:<id>
	// reads from the top, ctx:<id>:<range> reads that line range.
	if _, _, ok := ParseCtxHandle(p.Path); ok {
		return readCtxHandle(p.Path, p.Offset, p.Limit)
	}
	// wick:// addresses reach a namespace under the skills root: the library a
	// session loads a skill from. It is served here rather than in the wick
	// package because the tools package serves both hosts and cannot import it.
	if strings.HasPrefix(p.Path, wickScheme) {
		return readWick(p.Path, p.Offset, p.Limit)
	}
	// URLs read through the tiered reader (native, then Jina, then the
	// uninstalled browser tier). Selectors (:N, :raw) trail the URL.
	if isURLTarget(p.Path) {
		return readURL(root, p.Path, p.Offset, p.Limit)
	}
	// read opens any absolute path, so a session can reach its brief, state,
	// logs and transcripts outside the workspace. A relative path stays under
	// root; readPlain refuses the credential deny list in either case.
	return readPlain(root, p.Path, p.Offset, p.Limit)
}

// wickScheme is the virtual address read serves the skills library under.
const wickScheme = "wick://"

// wickSkillsRoot is where the namespaces live. It mirrors wick.skillsRoot, read
// from the environment rather than imported, so the tools package stays free of
// the wick package.
func wickSkillsRoot() string {
	if dir := os.Getenv("WICK_SKILLS_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(os.Getenv("HOME"), ".wick-pig", "skills")
}

// readWick serves wick://<namespace>/<path>. The namespace names a directory
// under the skills root; a path that resolves outside that directory, by ".."
// or by symlink, is refused. A directory returns an index of what it holds;
// a file reads as usual.
func readWick(addr string, offset, limit int) (string, error) {
	namespace, rest := splitWick(addr)
	root := wickSkillsRoot()
	namespaces := wickNamespaces(root)
	nsDir := ""
	for _, name := range namespaces {
		if name == namespace {
			nsDir = filepath.Join(root, name)
			break
		}
	}
	if nsDir == "" {
		if len(namespaces) == 0 {
			return "", fmt.Errorf("unknown wick namespace %q: no namespaces are installed under %s", namespace, root)
		}
		return "", fmt.Errorf("unknown wick namespace %q; have %s", namespace, strings.Join(namespaces, ", "))
	}
	full, err := wickInside(nsDir, rest)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(full)
	if err != nil {
		return "", fmt.Errorf("%s: %w", addr, err)
	}
	if info.IsDir() {
		return wickIndex(addr, full, offset, limit)
	}
	if err := refuseCredential(addr, full); err != nil {
		return "", err
	}
	return renderFile(addr, full, offset, limit, true)
}

// splitWick breaks the address after the scheme into namespace and the rest of
// the path. The namespace is the first segment; the rest may be empty.
func splitWick(addr string) (namespace, rest string) {
	body := strings.TrimPrefix(addr, wickScheme)
	namespace, rest, _ = strings.Cut(body, "/")
	return namespace, rest
}

// wickNamespaces lists the directories directly under the skills root. A
// symlink to a directory is not one: only a real directory is a namespace.
func wickNamespaces(root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	return names
}

// wickInside resolves rel under dir, following symlinks, and refuses a result
// that leaves dir. A missing leaf is returned as given — the caller reports it
// as a missing file, and there is nothing outside dir to read.
func wickInside(dir, rel string) (string, error) {
	rel = filepath.Clean(strings.TrimSpace(rel))
	if rel == "." {
		rel = ""
	}
	if filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%q is outside the wick namespace", rel)
	}
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	full := filepath.Join(realDir, rel)
	resolved, err := filepath.EvalSymlinks(full)
	if err != nil {
		return full, nil
	}
	inside, err := filepath.Rel(realDir, resolved)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%q resolves outside the wick namespace", rel)
	}
	return full, nil
}

// wickIndex lists a wick:// directory in the shared listing format. A
// subdirectory that holds a SKILL.md gets the description from its
// frontmatter after its name, so the namespace reads as an index of skills.
func wickIndex(addr, dir string, offset, limit int) (string, error) {
	return listDir(addr, dir, offset, limit, func(name string) string {
		// The SKILL.md is resolved the same way a read of it would be, so a
		// symlink pointing out of the namespace is skipped rather than read
		// for its description.
		skill, err := wickInside(dir, filepath.Join(name, "SKILL.md"))
		if err != nil {
			return ""
		}
		return skillDescription(skill)
	})
}

// skillDescription reads the description field of a SKILL.md. It is empty when
// the file or the field is missing.
func skillDescription(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return frontmatterDescription(string(data))
}

// frontmatterDescription pulls description from a leading --- block. It handles
// a plain value, one wrapped in quotes, and the folded/literal block scalars
// the skills CLI writes.
func frontmatterDescription(markdown string) string {
	markdown = strings.ReplaceAll(markdown, "\r\n", "\n")
	block, ok := strings.CutPrefix(markdown, "---\n")
	if !ok {
		return ""
	}
	end := strings.Index(block, "\n---")
	if end < 0 {
		return ""
	}
	lines := strings.Split(block[:end], "\n")
	for i, line := range lines {
		// Only a top-level key counts: an indented description belongs to some
		// nested block, not to the skill.
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		key, value, found := strings.Cut(line, ":")
		if !found || strings.TrimSpace(key) != "description" {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "|" || value == ">" || value == "|-" || value == ">-" {
			var parts []string
			for _, cont := range lines[i+1:] {
				if strings.TrimSpace(cont) == "" {
					continue
				}
				if !strings.HasPrefix(cont, " ") && !strings.HasPrefix(cont, "\t") {
					break
				}
				parts = append(parts, strings.TrimSpace(cont))
			}
			return strings.Join(parts, " ")
		}
		return strings.Trim(value, `"'`)
	}
	return ""
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
