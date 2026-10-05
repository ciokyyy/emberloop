package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// read opens any path, so a short deny list refuses the handful of files whose
// contents must never reach a model's context or a transcript: ssh keys, the
// pig and claude harness auth and credentials files, and any .env* file. The
// list is deliberately narrow — read's point is to reach briefs, state, logs
// and transcripts, not to police the filesystem.
//
// The cleaned path and, when it exists, its symlink-resolved path are both
// checked, so a link into a secret is refused the same as the secret itself.
// Both the filesystem read and a wick:// read call this, so the deny list
// holds whichever address scheme reached the file.
func refuseCredential(target, full string) error {
	for _, path := range []string{full, resolveForDeny(full)} {
		if path == "" {
			continue
		}
		if reason := credentialReason(path); reason != "" {
			return fmt.Errorf("%s is a credential path (%s); refusing to read it", target, reason)
		}
	}
	return nil
}

// resolveForDeny follows symlinks, tolerating a missing leaf: read of a missing
// file should report the missing file, not a credential error, and a link that
// does not resolve to a credential is not one.
func resolveForDeny(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return ""
	}
	return resolved
}

// credentialReason names why a path is on the deny list, or "" when read may
// serve it.
func credentialReason(path string) string {
	clean := filepath.Clean(path)
	if home := os.Getenv("HOME"); home != "" {
		home = filepath.Clean(home)
		switch {
		case underDir(filepath.Join(home, ".ssh"), clean):
			return "it is under ~/.ssh"
		case clean == filepath.Join(home, ".pig", "agent", "auth.json"):
			return "it is pig's agent auth file"
		case clean == filepath.Join(home, ".pig", "agent", "mcp-auth.json"):
			return "it is pig's MCP auth file"
		case clean == filepath.Join(home, ".claude.json"):
			return "it is claude's config file"
		case clean == filepath.Join(home, ".claude", ".credentials.json"):
			return "it is claude's credentials file"
		}
	}
	if strings.HasPrefix(filepath.Base(clean), ".env") {
		return "it matches .env*"
	}
	return ""
}

// underDir reports whether path is dir itself or lies beneath it.
func underDir(dir, path string) bool {
	return path == dir || strings.HasPrefix(path, dir+string(filepath.Separator))
}
