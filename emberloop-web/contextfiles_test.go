package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Table test on the project-context prompt builder: root only, nested chain,
// CLAUDE.md fallback, oversize cap, and no files.
func TestClaudeSystemPromptProjectContext(t *testing.T) {
	t.Parallel()
	write := func(t *testing.T, dir, name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("root only", func(t *testing.T) {
		work := tempDir(t)
		write(t, work, "AGENTS.md", "root rules\n")
		sp := claudeSystemPromptWith(work, tempDir(t))
		if !strings.Contains(sp, "# Project context") {
			t.Fatalf("prompt lacks the project-context block")
		}
		if !strings.Contains(sp, "## "+filepath.Join(work, "AGENTS.md")) {
			t.Fatalf("prompt lacks the file heading: %q", sp)
		}
		if !strings.Contains(sp, "root rules") {
			t.Fatalf("prompt lacks the file content: %q", sp)
		}
		if strings.Contains(sp, "Before editing, read") {
			t.Fatalf("prompt tells the agent to read files itself: %q", sp)
		}
	})

	t.Run("nested outermost first", func(t *testing.T) {
		root := tempDir(t)
		sub := filepath.Join(root, "sub")
		if err := os.Mkdir(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		write(t, root, "AGENTS.md", "outer rules\n")
		write(t, sub, "AGENTS.md", "inner rules\n")
		sp := claudeSystemPromptWith(sub, tempDir(t))
		if !strings.Contains(sp, "outer rules") || !strings.Contains(sp, "inner rules") {
			t.Fatalf("prompt lacks both layers: %q", sp)
		}
		if strings.Index(sp, "outer rules") > strings.Index(sp, "inner rules") {
			t.Fatalf("outer layer must come first: %q", sp)
		}
	})

	t.Run("claude.md fallback", func(t *testing.T) {
		work := tempDir(t)
		write(t, work, "CLAUDE.md", "claude rules\n")
		sp := claudeSystemPromptWith(work, tempDir(t))
		if !strings.Contains(sp, "claude rules") {
			t.Fatalf("prompt lacks the CLAUDE.md fallback: %q", sp)
		}
		// AGENTS.md wins over CLAUDE.md in the same directory.
		write(t, work, "AGENTS.md", "agents rules\n")
		sp = claudeSystemPromptWith(work, tempDir(t))
		if !strings.Contains(sp, "agents rules") || strings.Contains(sp, "claude rules") {
			t.Fatalf("AGENTS.md must win per directory: %q", sp)
		}
	})

	t.Run("global first", func(t *testing.T) {
		global := tempDir(t)
		work := tempDir(t)
		write(t, global, "AGENTS.md", "global rules\n")
		write(t, work, "AGENTS.md", "work rules\n")
		sp := claudeSystemPromptWith(work, global)
		if !strings.Contains(sp, "global rules") || !strings.Contains(sp, "work rules") {
			t.Fatalf("prompt lacks global plus work files: %q", sp)
		}
		if strings.Index(sp, "global rules") > strings.Index(sp, "work rules") {
			t.Fatalf("global file must come first: %q", sp)
		}
	})

	t.Run("oversize cap", func(t *testing.T) {
		work := tempDir(t)
		write(t, work, "AGENTS.md", strings.Repeat("x", claudeContextCap+1024))
		base := claudeSystemPrompt()
		sp := claudeSystemPromptWith(work, tempDir(t))
		if !strings.Contains(sp, "truncated") {
			t.Fatalf("over-cap prompt lacks the truncation note")
		}
		if len(sp) > len(base)+claudeContextCap+256 {
			t.Fatalf("over-cap prompt length %d exceeds the cap", len(sp))
		}
	})

	t.Run("none", func(t *testing.T) {
		sp := claudeSystemPromptWith(tempDir(t), tempDir(t))
		if strings.Contains(sp, "# Project context") {
			t.Fatalf("prompt carries a block with no files: %q", sp)
		}
		if sp != claudeSystemPrompt() {
			t.Fatalf("prompt without files must equal the base prompt")
		}
	})
}

// The injected context reaches the ACP session params on create and resume.
func TestClaudeContextReachesSessionParams(t *testing.T) {
	t.Parallel()
	work := tempDir(t)
	marker := "session-marker-rules"
	if err := os.WriteFile(filepath.Join(work, "AGENTS.md"), []byte(marker+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stub := &stubConn{}
	be := newStubClaudeBackend(func() (acpConn, error) { return stub, nil })
	if err := be.StartSession(SessionMeta{ID: "s1", Model: "sonnet", WorkDir: work}); err != nil {
		t.Fatal(err)
	}
	sp, _ := stub.newMeta["systemPrompt"].(string)
	if !strings.Contains(sp, "# Project context") || !strings.Contains(sp, marker) {
		t.Fatalf("session/new systemPrompt lacks the injected context: %q", sp)
	}

	loadStub := &stubConn{}
	beLoad := newStubClaudeBackend(func() (acpConn, error) { return loadStub, nil })
	if err := beLoad.LoadSession("s1", "acp-session-1", "sonnet", work, "web", ownerDirs{}); err != nil {
		t.Fatal(err)
	}
	lsp, _ := loadStub.loadMeta["systemPrompt"].(string)
	if !strings.Contains(lsp, "# Project context") || !strings.Contains(lsp, marker) {
		t.Fatalf("session/load systemPrompt lacks the injected context: %q", lsp)
	}
}
