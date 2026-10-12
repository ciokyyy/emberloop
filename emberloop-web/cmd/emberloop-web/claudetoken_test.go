package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	web "example.com/emberloop-web"
	"example.com/emberloop-web/testsupport"
)

// Seam: `emberloop-web claude-token set` writes the 0600 shared-token file
// the Daemon loads, without ever printing the token.

func TestClaudeTokenSetFromPipeWritesOwnerOnlyFile(t *testing.T) {
	dir := testsupport.TempDir(t)
	var out, errOut bytes.Buffer
	if code := runClaudeTokenCommand([]string{"set", "-state-dir", dir}, stdinWith(t, "fake-setup-token\n"), &out, &errOut); code != 0 {
		t.Fatalf("exit = %d, want 0; stderr %q", code, errOut.String())
	}
	path := web.ClaudeOAuthTokenPath(dir)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(raw)) != "fake-setup-token" {
		t.Fatalf("stored token = %q, want the piped value", raw)
	}
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("token file mode = %v, %v; want 0600", fi.Mode(), err)
	}
	if strings.Contains(out.String(), "fake-setup-token") || strings.Contains(errOut.String(), "fake-setup-token") {
		t.Fatalf("output leaks the token: %q %q", out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), filepath.Join(dir, "enderbloom", "claude-oauth-token")) {
		t.Fatalf("stdout = %q, want the token path", out.String())
	}
}

func TestClaudeTokenSetRefusesEmpty(t *testing.T) {
	dir := testsupport.TempDir(t)
	var out, errOut bytes.Buffer
	if code := runClaudeTokenCommand([]string{"set", "-state-dir", dir}, stdinWith(t, "\n"), &out, &errOut); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if _, err := os.Stat(web.ClaudeOAuthTokenPath(dir)); !os.IsNotExist(err) {
		t.Fatalf("empty token wrote a file: %v", err)
	}
}

func TestClaudeTokenCommandUsage(t *testing.T) {
	for _, args := range [][]string{nil, {"store"}, {"set", "extra"}} {
		var out, errOut bytes.Buffer
		if code := runClaudeTokenCommand(args, stdinWith(t, ""), &out, &errOut); code != 2 {
			t.Errorf("args %q exit = %d, want 2", args, code)
		}
		if !strings.Contains(errOut.String(), "usage: emberloop-web claude-token set") {
			t.Errorf("args %q stderr = %q, want usage", args, errOut.String())
		}
	}
}
