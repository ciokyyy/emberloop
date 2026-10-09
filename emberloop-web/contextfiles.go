package web

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/MichaelKinsy/PiG/coding"
)

// claudeContextCap caps the injected project-context block at 64 KB.
const claudeContextCap = 64 * 1024

// claudeContextCandidates mirrors pig's per-directory candidate priority:
// the first file found wins for its directory.
var claudeContextCandidates = []string{"AGENTS.override.md", "AGENTS.md", "AGENTS.MD", "CLAUDE.md", "CLAUDE.MD"}

// claudeContextFile is one loaded project context file.
type claudeContextFile struct {
	Path    string
	Content string
}

// claudeSystemPromptFor is the Session system prompt for a WorkDir: el's
// rule set plus the WorkDir's project context as real content. Scouts get it
// too; it is content, so read-only still holds. The global file loads from
// pig's default agent dir, matching what pig Sessions get from the SDK
// loader.
func claudeSystemPromptFor(workDir string) string {
	return claudeSystemPromptWith(workDir, coding.DefaultAgentDir())
}

// claudeSystemPromptWith is claudeSystemPromptFor with an explicit global
// agent dir; tests point it at a temp dir.
func claudeSystemPromptWith(workDir, agentDir string) string {
	p := claudeSystemPrompt()
	if block := claudeProjectContextBlock(loadClaudeContextFiles(workDir, agentDir)); block != "" {
		p += "\n\n" + block
	}
	return p
}

// loadClaudeContextFiles mirrors pig's LoadProjectContextFiles: the global
// agent-dir file first, then every candidate from the WorkDir up to the
// filesystem root, outermost-first. The first candidate wins per directory,
// duplicate paths load once, unreadable files skip silently.
func loadClaudeContextFiles(cwd, agentDir string) []claudeContextFile {
	if cwd != "" {
		if abs, err := filepath.Abs(cwd); err == nil {
			cwd = abs
		}
	}
	if agentDir != "" {
		if abs, err := filepath.Abs(agentDir); err == nil {
			agentDir = abs
		}
	}
	var out []claudeContextFile
	seen := map[string]struct{}{}
	if agentDir != "" {
		if cf, ok := loadClaudeContextFileFromDir(agentDir); ok {
			seen[cf.Path] = struct{}{}
			out = append(out, cf)
		}
	}
	if cwd != "" {
		var chain []claudeContextFile
		dir := cwd
		for {
			if cf, ok := loadClaudeContextFileFromDir(dir); ok {
				if _, dup := seen[cf.Path]; !dup {
					seen[cf.Path] = struct{}{}
					chain = append([]claudeContextFile{cf}, chain...)
				}
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
		out = append(out, chain...)
	}
	return out
}

// loadClaudeContextFileFromDir returns the first candidate in a directory,
// or false when none is present and readable.
func loadClaudeContextFileFromDir(dir string) (claudeContextFile, bool) {
	for _, name := range claudeContextCandidates {
		p := filepath.Join(dir, name)
		info, err := os.Stat(p)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		return claudeContextFile{Path: p, Content: strings.TrimPrefix(string(data), "\xef\xbb\xbf")}, true
	}
	return claudeContextFile{}, false
}

// claudeProjectContextBlock formats loaded files as one delimited block:
// outermost-first, one "## <path>" heading per file. Empty input returns "".
// Over-cap output truncates with a note.
func claudeProjectContextBlock(files []claudeContextFile) string {
	if len(files) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("# Project context\n")
	for _, f := range files {
		sb.WriteString("\n## " + f.Path + "\n" + f.Content)
		if !strings.HasSuffix(f.Content, "\n") {
			sb.WriteString("\n")
		}
	}
	block := sb.String()
	if len(block) > claudeContextCap {
		block = block[:claudeContextCap] + "\n[... project context truncated at 64 KB ...]\n"
	}
	return block
}
