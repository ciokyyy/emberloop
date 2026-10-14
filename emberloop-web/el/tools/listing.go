package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// listDir renders a directory as one entry per line, one level deep:
// directories end in "/", files carry their size, symlinks end in "@". offset
// and limit page through the entries the way they page through a file's
// lines; the header always counts every entry. describe, when set, may add a
// short description after a directory's name (the el:// skill index uses
// it). A directory holding a SKILL.md ends with a hint naming it, so reading
// a skill's directory leads straight to the skill.
func listDir(display, full string, offset, limit int, describe func(name string) string) (string, error) {
	entries, err := os.ReadDir(full)
	if err != nil {
		return "", fmt.Errorf("%s: %w", display, err)
	}
	lines := make([]string, 0, len(entries))
	hasSkill := false
	for _, entry := range entries {
		name := entry.Name()
		info, err := entry.Info()
		switch {
		case err != nil:
			lines = append(lines, name)
		case info.Mode()&os.ModeSymlink != 0:
			lines = append(lines, name+"@")
		case entry.IsDir():
			line := name + "/"
			if describe != nil {
				if desc := describe(name); desc != "" {
					line += ": " + desc
				}
			}
			lines = append(lines, line)
		default:
			if name == "SKILL.md" {
				hasSkill = true
			}
			lines = append(lines, name+"  "+humanSize(info.Size()))
		}
	}

	start, end := window(len(lines), offset, limit)
	var b strings.Builder
	fmt.Fprintf(&b, "[%s] %d entries\n", strings.TrimSuffix(display, "/")+"/", len(lines))
	for _, line := range lines[start:end] {
		b.WriteString(line + "\n")
	}
	if hasSkill {
		fmt.Fprintf(&b, "\nread %s/SKILL.md for the skill\n", strings.TrimSuffix(display, "/"))
	}
	return b.String(), nil
}

// isGlob reports whether a read target is a pattern rather than a path.
func isGlob(target string) bool { return strings.ContainsAny(target, "*?[") }

// readGlob lists the paths a pattern matches, never their contents. A relative
// pattern matches under baseDir and is shown relative to it; an absolute one
// matches anywhere. Credential paths are left out of the list.
func readGlob(baseDir, pattern string, offset, limit int) (string, error) {
	abs := pattern
	if !filepath.IsAbs(pattern) {
		clean := filepath.Clean(pattern)
		if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("%q is outside the workspace; give an absolute pattern instead", pattern)
		}
		abs = filepath.Join(baseDir, clean)
	}
	matches, err := filepath.Glob(abs)
	if err != nil {
		return "", fmt.Errorf("%s: %w", pattern, err)
	}
	var shown []string
	for _, m := range matches {
		if refuseCredential(m, m) != nil {
			continue
		}
		line := m
		if !filepath.IsAbs(pattern) {
			if rel, err := filepath.Rel(baseDir, m); err == nil {
				line = rel
			}
		}
		if isDir(m) {
			line += "/"
		}
		shown = append(shown, line)
	}
	if len(shown) == 0 {
		return "", fmt.Errorf("%s: no match", pattern)
	}
	start, end := window(len(shown), offset, limit)
	var b strings.Builder
	fmt.Fprintf(&b, "[%s] %d matches\n", pattern, len(shown))
	for _, m := range shown[start:end] {
		b.WriteString(m + "\n")
	}
	return b.String(), nil
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// window turns a 1-based offset and a limit into slice bounds over n items.
func window(n, offset, limit int) (start, end int) {
	if offset < 1 {
		offset = 1
	}
	start = offset - 1
	if start > n {
		start = n
	}
	end = n
	if limit > 0 && start+limit < n {
		end = start + limit
	}
	return start, end
}

func humanSize(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%dB", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1fK", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1fM", float64(n)/(1024*1024))
	}
}
