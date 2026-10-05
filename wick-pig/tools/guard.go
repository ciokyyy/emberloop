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

// ProjectSettingsRel is the per-project settings file, under the workspace.
const ProjectSettingsRel = ".wick-pig/settings.json"

// ProjectWritePaths are the paths the orchestrator may edit and write in the
// workspace at root: the "orchestrator_paths" array of the project settings
// file. Entries are workspace-relative files or directories; an absolute entry
// or one that climbs out of the workspace is ignored, so the file can only
// ever widen access inside its own project. A missing or broken file allows
// nothing.
func ProjectWritePaths(root string) []string {
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(ProjectSettingsRel)))
	if err != nil {
		return nil
	}
	var s struct {
		Paths []string `json:"orchestrator_paths"`
	}
	if json.Unmarshal(raw, &s) != nil {
		return nil
	}
	var out []string
	for _, p := range s.Paths {
		p = filepath.Clean(strings.TrimSpace(p))
		if p == "" || p == "." || filepath.IsAbs(p) || p == ".." || strings.HasPrefix(p, ".."+string(filepath.Separator)) {
			continue
		}
		out = append(out, p)
	}
	return out
}

// Guard limits an edit or write tool to the paths allow returns for the
// workspace. The check is made on the resolved path, so a symlink or `..`
// cannot lead out of an allowed directory, and the project settings file is
// never writable: the allowlist cannot be widened by the one it limits.
func Guard(t Tool, allow func(root string) []string) Tool { return guarded{t, allow} }

type guarded struct {
	Tool
	allow func(root string) []string
}

func (g guarded) Description() string {
	return g.Tool.Description() + "\n\nIn this session only the paths listed under \"orchestrator_paths\" in " +
		ProjectSettingsRel + " may be changed; any other path is refused."
}

func (g guarded) Execute(ctx context.Context, root string, args json.RawMessage) (string, error) {
	path, err := targetPath(g.Name(), args)
	if err != nil {
		return "", err
	}
	if root == "" {
		root = "."
	}
	if err := checkAllowed(root, path, g.allow(root)); err != nil {
		return "", err
	}
	return g.Tool.Execute(ctx, root, args)
}

// targetPath is the one file a call would change.
func targetPath(tool string, args json.RawMessage) (string, error) {
	switch tool {
	case "write":
		var p struct{ Path string }
		if err := json.Unmarshal(args, &p); err != nil || strings.TrimSpace(p.Path) == "" {
			return "", errors.New("write requires a non-empty path")
		}
		return p.Path, nil
	case "edit":
		var p struct{ Input string }
		if err := json.Unmarshal(args, &p); err != nil {
			return "", fmt.Errorf("invalid edit parameters: %w", err)
		}
		h := headerRegex.FindStringSubmatch(p.Input)
		if len(h) < 3 {
			return "", errors.New("patch has no [path#TAG] header naming a file")
		}
		return h[1], nil
	}
	return "", fmt.Errorf("guard does not know the %q tool", tool)
}

func checkAllowed(root, path string, allowed []string) error {
	full, err := resolveInside(root, filepath.Clean(strings.TrimSpace(path)))
	if err != nil {
		return err
	}
	real := realPath(full)
	if realRoot := realPath(root); real == filepath.Join(realRoot, filepath.FromSlash(ProjectSettingsRel)) ||
		strings.HasPrefix(real, filepath.Join(realRoot, ".wick-pig")+string(filepath.Separator)) {
		return fmt.Errorf("%q holds the project settings, which the orchestrator cannot change", path)
	}
	for _, a := range allowed {
		ar := realPath(filepath.Join(root, a))
		if real == ar || strings.HasPrefix(real, ar+string(filepath.Separator)) {
			return nil
		}
	}
	if len(allowed) == 0 {
		return fmt.Errorf("the orchestrator may not change %q: no paths are allowed; list them under \"orchestrator_paths\" in %s",
			path, ProjectSettingsRel)
	}
	return fmt.Errorf("the orchestrator may not change %q: allowed paths are %s (from %s)",
		path, strings.Join(allowed, ", "), ProjectSettingsRel)
}

// realPath resolves symlinks in the longest existing prefix of p and appends
// the rest, so a path that does not exist yet is judged where it would land.
func realPath(p string) string {
	p = filepath.Clean(p)
	rest := ""
	for {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return filepath.Join(r, rest)
		}
		parent := filepath.Dir(p)
		if parent == p {
			return filepath.Join(p, rest)
		}
		rest = filepath.Join(filepath.Base(p), rest)
		p = parent
	}
}
