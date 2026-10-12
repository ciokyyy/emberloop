package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ownerDirs are one owner's harness config dirs. The Daemon derives them from
// the owner's user ID on every harness start (ownerDirsFor). A harness that
// loads user-level memory or instructions on its own reads only these dirs,
// so one User's instructions never reach another User's Session.
type ownerDirs struct {
	// claude is the claude adapter's CLAUDE_CONFIG_DIR. It holds the CLI's
	// own state; instructions and memory stay out. In fallback mode it also
	// holds the host login link; with a shared token there is no login file.
	claude string
	// claudeToken is the shared setup-token login (claudetoken.go), resolved
	// env-over-file on every start. claudeEnv injects it into the adapter's
	// environment. env() deliberately omits it: the fake Backend records
	// env() per start, and the token value never belongs in records, logs or
	// API output. Dirs itself is never persisted (SessionMeta.Dirs is json:"-").
	claudeToken string
	// pig is the owner's pig agent dir. Pig Sessions run on a per-Session
	// copy of it (pigacp.go); the SDK backend reads it directly.
	pig string
}

// env is the owner's dirs as NAME=value pairs, one per dir that is set. The
// fake Backend records it per start, so tests assert what each owner gets.
// It never carries the shared claude token (see claudeToken).
func (d ownerDirs) env() []string {
	var env []string
	if d.claude != "" {
		env = append(env, "CLAUDE_CONFIG_DIR="+d.claude)
	}
	if d.pig != "" {
		env = append(env, "PIG_CODING_AGENT_DIR="+d.pig)
	}
	return env
}

// claudeAuthFile is the login file the claude CLI reads from its config dir.
const claudeAuthFile = ".credentials.json"

// pigSharedEntries are the host pig agent dir entries an owner dir links to:
// the credential and model stores, pig's own state, extensions and tool
// caches. Every other entry, instruction files and skills included, reaches
// an owner only from that owner's own dir.
var pigSharedEntries = []string{
	"auth.json", "models.json", "models-store.json", "keybindings.json",
	"trust.json", "sessions", "extensions", "bin", "npm", "git",
}

// claudeHostDir is the config dir the claude CLI reads with no
// CLAUDE_CONFIG_DIR: the variable when set, else ~/.claude.
func claudeHostDir() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude")
}

// ownerDirsFor returns the owner's harness dirs, creating and seeding them
// under <stateDir>/enderbloom/users/<userId>/config. Every harness start calls
// it, so a host login or settings change reaches the next start. It also
// resolves the shared claude token once per start; with a token the adapter
// authenticates over the environment and per-User credential copies are
// removed instead of linked (seedClaudeDir).
func (d *Daemon) ownerDirsFor(owner string) (ownerDirs, error) {
	if owner == "" || owner == "." || owner == ".." || strings.ContainsAny(owner, `/\`) {
		return ownerDirs{}, fmt.Errorf("session owner %q is not a user ID", owner)
	}
	root := filepath.Join(d.stateDir, "enderbloom", "users", owner, "config")
	dirs := ownerDirs{claude: filepath.Join(root, "claude"), pig: filepath.Join(root, "pig")}
	dirs.claudeToken = loadClaudeOAuthToken(d.stateDir)
	d.dirsMu.Lock()
	defer d.dirsMu.Unlock()
	if err := seedClaudeDir(dirs.claude, d.hostClaudeDir, dirs.claudeToken != ""); err != nil {
		return ownerDirs{}, fmt.Errorf("claude config dir: %w", err)
	}
	if err := seedPigDir(dirs.pig, d.hostPigDir); err != nil {
		return ownerDirs{}, fmt.Errorf("pig agent dir: %w", err)
	}
	return dirs, nil
}

// seedClaudeDir makes the owner's CLAUDE_CONFIG_DIR. With a shared token
// (hasToken) it links nothing and removes a stale .credentials.json copy —
// symlink or regular file only, nothing else — so no diverging login can
// shadow the token (the CLI replaces a symlink with a private file on its
// first credential write). Without a token it links the host login in, as
// before, and repairs one divergence: an owner regular file that differs
// from the host and is older than the host is replaced with the link, so a
// stale refresh copy reconverges on the host login. The tradeoff is
// deliberate: divergent copies already race each other (single-use rotating
// refresh), and the host login is the newest deliberate `claude login`. An
// owner copy newer than the host is left alone — it may hold a fresher
// refresh the host has not seen — as are symlinks and identical copies.
// Nothing else links in: the CLI reads CLAUDE.md, projects and settings
// from this dir only. With CLAUDE_CONFIG_DIR set, the CLI keeps .claude.json
// in this dir too and creates it on first use.
func seedClaudeDir(dir, host string, hasToken bool) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	link := filepath.Join(dir, claudeAuthFile)
	if hasToken {
		if fi, err := os.Lstat(link); err == nil && !fi.IsDir() {
			_ = os.Remove(link)
		}
		return nil
	}
	if host == "" {
		return nil
	}
	src := filepath.Join(host, claudeAuthFile)
	hostBytes, err := os.ReadFile(src)
	if err != nil {
		return nil // host not logged in: the owner's CLI is not either
	}
	fi, err := os.Lstat(link)
	if err != nil {
		if err := os.Symlink(src, link); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		return nil
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return nil // the host link stands
	}
	if fi.IsDir() {
		return nil // never touch non-file entries
	}
	ownerBytes, err := os.ReadFile(link)
	if err != nil || string(ownerBytes) == string(hostBytes) {
		return nil // unreadable or identical: nothing divergent to repair
	}
	hostFi, err := os.Stat(src)
	if err != nil {
		return nil
	}
	if !fi.ModTime().Before(hostFi.ModTime()) {
		return nil // owner copy is newer: it may hold a fresher refresh
	}
	if err := os.Remove(link); err != nil {
		return err
	}
	if err := os.Symlink(src, link); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	return nil
}

// seedPigDir makes the owner's pig agent dir: the shared entries by link and
// settings.json from the host without skills and prompts. Pig loads skills and
// prompt templates from the paths settings name, and those would reach every
// owner.
func seedPigDir(dir, host string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if host == "" {
		return nil
	}
	for _, name := range pigSharedEntries {
		src := filepath.Join(host, name)
		if _, err := os.Stat(src); err != nil {
			continue // absent on the host: a later start links it
		}
		link := filepath.Join(dir, name)
		if _, err := os.Lstat(link); err == nil {
			continue
		}
		if err := os.Symlink(src, link); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
	}
	return writePigOwnerSettings(dir, filepath.Join(host, "settings.json"))
}

// writePigOwnerSettings writes <dir>/settings.json from the host settings at
// src, minus skills and prompts. A missing or unparsable host file gives an
// empty object.
func writePigOwnerSettings(dir, src string) error {
	m := map[string]any{}
	if raw, err := os.ReadFile(src); err == nil {
		var host map[string]any
		if json.Unmarshal(raw, &host) == nil {
			m = host
		}
	}
	delete(m, "skills")
	delete(m, "prompts")
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(dir, "settings.json"), append(out, '\n'), 0o600)
}
