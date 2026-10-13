package web

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Owner harness dirs at the Daemon HTTP seam and in the seeding rules. The
// fake Backend records the env each harness start got, so these tests read
// what the Daemon handed the harness per owner.

// hostDirsForTest points the Daemon's host harness dirs at temp dirs. The
// claude host holds a login, a CLAUDE.md and projects; the pig host holds a
// login, AGENTS.md, skills and settings naming host paths. A seeded owner dir
// may link the login and the shared pig state, and nothing else from them.
func hostDirsForTest(t *testing.T, d *Daemon) {
	t.Helper()
	claudeHost := tempDir(t)
	pigHost := tempDir(t)
	writeTestFile(t, filepath.Join(claudeHost, claudeAuthFile), `{"test":true}`)
	writeTestFile(t, filepath.Join(claudeHost, "CLAUDE.md"), "host claude memory")
	writeTestFile(t, filepath.Join(claudeHost, "projects", "p", "memory", "MEMORY.md"), "host auto memory")
	writeTestFile(t, filepath.Join(pigHost, "auth.json"), `{}`)
	writeTestFile(t, filepath.Join(pigHost, "AGENTS.md"), "host pig instructions")
	writeTestFile(t, filepath.Join(pigHost, "settings.json"), `{"defaultProvider":"test","skills":["/host/skills"]}`)
	d.hostClaudeDir = claudeHost
	d.hostPigDir = pigHost
}

// writeTestFile writes one file, making its parent dirs.
func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// ownerEnvFor is the env the Daemon must hand every harness start of one
// owner, given the Daemon's state dir.
func ownerEnvFor(stateDir, owner string) []string {
	root := filepath.Join(stateDir, "enderbloom", "users", owner, "config")
	return []string{
		"CLAUDE_CONFIG_DIR=" + filepath.Join(root, "claude"),
		"PIG_CODING_AGENT_DIR=" + filepath.Join(root, "pig"),
	}
}

// startsFor lists the harness starts the fake got for one Session.
func startsFor(be *FakeBackend, id string) []handleStart { return be.Starts(id) }

func envEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestOwnerStartsGetOwnDirsAtDaemonSeam: each User's Session starts with that
// User's dirs only. A file in B's claude config dir is absent from A's.
func TestOwnerStartsGetOwnDirsAtDaemonSeam(t *testing.T) {
	t.Parallel()
	stateDir := tempDir(t)
	be := NewFakeBackend()
	d := NewDaemon(stateDir, be)
	hostDirsForTest(t, d)
	h := d.Handler()
	alice := signedIn(t, h, "alice")
	aliceID := userOf(t, h, alice).ID
	bob := signedIn(t, h, "bob")
	bobID := userOf(t, h, bob).ID
	asAlice := httptest.NewServer(withCookie(h, alice))
	defer asAlice.Close()
	asBob := httptest.NewServer(withCookie(h, bob))
	defer asBob.Close()

	aliceSess := createSession(t, asAlice.URL, "fake", "fake-model", tempDir(t))
	bobSess := createSession(t, asBob.URL, "fake", "fake-model", tempDir(t))

	aliceStarts := startsFor(be, aliceSess)
	if len(aliceStarts) != 1 || aliceStarts[0].Kind() != "create" || aliceStarts[0].Owner() != aliceID {
		t.Fatalf("alice starts = %+v, want one create owned by %s", aliceStarts, aliceID)
	}
	if want := ownerEnvFor(stateDir, aliceID); !envEqual(aliceStarts[0].Env(), want) {
		t.Fatalf("alice env = %v, want %v", aliceStarts[0].Env(), want)
	}
	bobStarts := startsFor(be, bobSess)
	if len(bobStarts) != 1 || bobStarts[0].Owner() != bobID {
		t.Fatalf("bob starts = %+v, want one create owned by %s", bobStarts, bobID)
	}
	if want := ownerEnvFor(stateDir, bobID); !envEqual(bobStarts[0].Env(), want) {
		t.Fatalf("bob env = %v, want %v", bobStarts[0].Env(), want)
	}
	for _, kv := range aliceStarts[0].Env() {
		if strings.Contains(kv, bobID) {
			t.Fatalf("alice's env reaches bob's dir: %s", kv)
		}
	}

	// A file in B's claude dir is not in A's: the dirs are separate trees.
	aliceClaude := strings.TrimPrefix(aliceStarts[0].Env()[0], "CLAUDE_CONFIG_DIR=")
	bobClaude := strings.TrimPrefix(bobStarts[0].Env()[0], "CLAUDE_CONFIG_DIR=")
	writeTestFile(t, filepath.Join(bobClaude, "CLAUDE.md"), "bob only")
	if _, err := os.Stat(filepath.Join(aliceClaude, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Fatalf("alice's claude dir sees bob's CLAUDE.md: %v", err)
	}
	if _, err := os.Stat(filepath.Join(aliceClaude, claudeAuthFile)); err != nil {
		t.Fatalf("alice's claude dir has no login link: %v", err)
	}
}

// TestResumeAndHandoffStartsCarryOwnerEnv: a resume after restart and a
// Handoff both start with the owner's dirs, not the host's.
func TestResumeAndHandoffStartsCarryOwnerEnv(t *testing.T) {
	t.Parallel()
	d, be, base, id := handoffDaemon(t)
	hostDirsForTest(t, d)
	owner := metaOf(t, base, id).Owner
	for _, p := range []string{"build the thing", "and more"} {
		seq := sendPrompt(t, base, id, p)
		waitTurnEnd(t, base, id, seq)
	}
	if code, body := compressReqIdle(t, base, id, ""); code != 202 {
		t.Fatalf("compress = %d %v", code, body)
	}
	waitIdleAfter(t, base, id, 1)
	var handoff []handleStart
	for _, s := range startsFor(be, id) {
		if s.Kind() == "handoff" {
			handoff = append(handoff, s)
		}
	}
	if len(handoff) != 1 || handoff[0].Owner() != owner || !envEqual(handoff[0].Env(), ownerEnvFor(d.stateDir, owner)) {
		t.Fatalf("handoff starts = %+v, want one owned by %s with owner env", handoff, owner)
	}

	// Restart: the Session loads as history; the next prompt resumes it.
	be2 := NewFakeBackend()
	d2 := NewDaemon(d.stateDir, be2)
	hostDirsForTest(t, d2)
	h2 := d2.Handler()
	alice := signedIn(t, h2, "admin")
	srv := httptest.NewServer(withCookie(h2, alice))
	defer srv.Close()
	sendPrompt(t, srv.URL, id, "after restart")
	resumes := startsFor(be2, id)
	if len(resumes) != 1 || resumes[0].Kind() != "resume" || resumes[0].Owner() != owner {
		t.Fatalf("resume starts = %+v, want one resume owned by %s", resumes, owner)
	}
	if !envEqual(resumes[0].Env(), ownerEnvFor(d.stateDir, owner)) {
		t.Fatalf("resume env = %v, want owner env", resumes[0].Env())
	}
}

// TestSpawnedChildStartsWithParentOwnerEnv: a child started by another User
// takes the parent's owner, so its dirs are the parent owner's.
func TestSpawnedChildStartsWithParentOwnerEnv(t *testing.T) {
	t.Parallel()
	stateDir := tempDir(t)
	be := NewFakeBackend()
	d := NewDaemon(stateDir, be)
	hostDirsForTest(t, d)
	h := d.Handler()
	alice := signedIn(t, h, "alice")
	aliceID := userOf(t, h, alice).ID
	bob := signedIn(t, h, "bob")
	asAlice := httptest.NewServer(withCookie(h, alice))
	defer asAlice.Close()
	asBob := httptest.NewServer(withCookie(h, bob))
	defer asBob.Close()

	parent := createSession(t, asAlice.URL, "fake", "fake-model", tempDir(t))
	kid := spawnOne(t, asBob.URL, parent, "bob asks")
	waitUntil(t, "child start", func() bool { return len(startsFor(be, kid)) == 1 })
	got := startsFor(be, kid)[0]
	if got.Owner() != aliceID || !envEqual(got.Env(), ownerEnvFor(stateDir, aliceID)) {
		t.Fatalf("child start = %+v, want parent owner %s with its env", got, aliceID)
	}
}

// TestOwnerDirsRefusePathLikeOwners: an owner names a directory under users/,
// so an empty or path-like owner is refused before any dir is made.
func TestOwnerDirsRefusePathLikeOwners(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	for _, owner := range []string{"", ".", "..", "a/b", `a\b`} {
		if _, err := d.ownerDirsFor(owner); err == nil {
			t.Fatalf("ownerDirsFor(%q) returned no error", owner)
		}
	}
}

// TestSeedClaudeDirLinksOnlyTheLogin: the owner's CLAUDE_CONFIG_DIR gets the
// host login as a link and no host memory, settings or projects. A second
// seed changes nothing.
func TestSeedClaudeDirLinksOnlyTheLogin(t *testing.T) {
	t.Parallel()
	host := tempDir(t)
	writeTestFile(t, filepath.Join(host, claudeAuthFile), `{"test":true}`)
	writeTestFile(t, filepath.Join(host, "CLAUDE.md"), "host claude memory")
	writeTestFile(t, filepath.Join(host, "settings.json"), "{}")
	writeTestFile(t, filepath.Join(host, "projects", "p", "memory", "MEMORY.md"), "auto memory")
	dir := filepath.Join(tempDir(t), "claude")
	for i := 0; i < 2; i++ {
		if err := seedClaudeDir(dir, host, false); err != nil {
			t.Fatal(err)
		}
	}
	target, err := os.Readlink(filepath.Join(dir, claudeAuthFile))
	if err != nil || target != filepath.Join(host, claudeAuthFile) {
		t.Fatalf("login link = %q, %v; want the host login", target, err)
	}
	for _, name := range []string{"CLAUDE.md", "settings.json", "projects"} {
		if _, err := os.Lstat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("host %s reached the owner dir: %v", name, err)
		}
	}

	// A host with no login links nothing and fails nothing.
	bare := filepath.Join(tempDir(t), "claude")
	if err := seedClaudeDir(bare, tempDir(t), false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(bare, claudeAuthFile)); !os.IsNotExist(err) {
		t.Fatalf("login linked from a host with none: %v", err)
	}
}

// TestSeedPigDirLinksSharedStateOnly: pig's owner dir links credentials, the
// session store and extensions. Instruction files, skills and prompts stay
// out, and settings drop the skills and prompts path lists.
func TestSeedPigDirLinksSharedStateOnly(t *testing.T) {
	t.Parallel()
	host := tempDir(t)
	writeTestFile(t, filepath.Join(host, "auth.json"), `{}`)
	writeTestFile(t, filepath.Join(host, "AGENTS.md"), "host instructions")
	writeTestFile(t, filepath.Join(host, "SYSTEM.md"), "host system prompt")
	writeTestFile(t, filepath.Join(host, "skills", "s", "SKILL.md"), "a skill")
	writeTestFile(t, filepath.Join(host, "prompts", "p.md"), "a prompt")
	writeTestFile(t, filepath.Join(host, "extensions", "ext1", "index.js"), "code")
	writeTestFile(t, filepath.Join(host, "sessions", "s1.jsonl"), "{}")
	writeTestFile(t, filepath.Join(host, "settings.json"),
		`{"defaultProvider":"test","skills":["/host/skills"],"prompts":["/host/p"],"extensions":["/host/ext"]}`)
	dir := filepath.Join(tempDir(t), "pig")
	if err := seedPigDir(dir, host); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"auth.json", "extensions", "sessions"} {
		if target, err := os.Readlink(filepath.Join(dir, name)); err != nil || target != filepath.Join(host, name) {
			t.Fatalf("%s not linked from the host: %q %v", name, target, err)
		}
	}
	for _, name := range []string{"AGENTS.md", "SYSTEM.md", "skills", "prompts"} {
		if _, err := os.Lstat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("host %s reached the owner dir: %v", name, err)
		}
	}
	raw, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got["defaultProvider"] != "test" {
		t.Fatalf("settings lost defaultProvider: %v", got)
	}
	if _, ok := got["extensions"]; !ok {
		t.Fatalf("settings lost the extensions list: %v", got)
	}
	for _, key := range []string{"skills", "prompts"} {
		if _, ok := got[key]; ok {
			t.Fatalf("settings kept %s: %v", key, got)
		}
	}
}

// TestClaudeAdapterConnUsesOwnerConfigDir: the claude adapter's process gets
// the owner's dirs, and its env ends with the owner's CLAUDE_CONFIG_DIR.
func TestClaudeAdapterConnUsesOwnerConfigDir(t *testing.T) {
	t.Parallel()
	b := newStubClaudeBackend(nil)
	b.newConn = nil
	var got ownerDirs
	b.newConnFor = func(sessionID, role string, dirs ownerDirs) (acpConn, error) {
		got = dirs
		return nil, os.ErrClosed
	}
	want := ownerDirs{claude: "/owner/claude", pig: "/owner/pig"}
	if _, err := b.connFactory("s1", "web", want)(); err == nil {
		t.Fatal("stub adapter start returned no error")
	}
	if got != want {
		t.Fatalf("adapter started with dirs %+v, want %+v", got, want)
	}

	env := claudeEnv(want)
	if last := env[len(env)-1]; last != "CLAUDE_CONFIG_DIR=/owner/claude" {
		t.Fatalf("last adapter env entry = %q, want the owner's CLAUDE_CONFIG_DIR", last)
	}
	found := false
	for _, kv := range env {
		if kv == "CLAUDE_CODE_DISABLE_CLAUDE_MDS=1" {
			found = true
		}
	}
	if !found {
		t.Fatal("adapter env lost the CLAUDE.md disabling set")
	}
}

// Shared claude login: the Daemon resolves one setup token env-over-file and
// the adapter gets it last-wins. These tests use fake token strings only.

// withCleanTokenEnv unsets both token env vars, restoring them afterwards.
// Callers stay serial: they touch process env.
func withCleanTokenEnv(t *testing.T) {
	t.Helper()
	saved := map[string]string{}
	for _, name := range claudeOAuthTokenEnvs {
		if v, ok := os.LookupEnv(name); ok {
			saved[name] = v
		}
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, name := range claudeOAuthTokenEnvs {
			_ = os.Unsetenv(name)
		}
		for name, v := range saved {
			_ = os.Setenv(name, v)
		}
	})
}

// TestClaudeTokenEnvLastWins: with a token the adapter env ends with
// CLAUDE_CODE_OAUTH_TOKEN after the owner's CLAUDE_CONFIG_DIR, keeping the
// disabling set. EL_CLAUDE_OAUTH_TOKEN outranks CLAUDE_CODE_OAUTH_TOKEN, and
// a Daemon-resolved token outranks both.
func TestClaudeTokenEnvLastWins(t *testing.T) {
	withCleanTokenEnv(t)
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "fake-file-env-token")
	t.Setenv("EL_CLAUDE_OAUTH_TOKEN", "fake-el-token")
	env := claudeEnv(ownerDirs{claude: "/owner/claude"})
	if last := env[len(env)-1]; last != "CLAUDE_CODE_OAUTH_TOKEN=fake-el-token" {
		t.Fatalf("last adapter env entry = %q, want the EL_ token last-wins", last)
	}
	if prev := env[len(env)-2]; prev != "CLAUDE_CONFIG_DIR=/owner/claude" {
		t.Fatalf("entry before the token = %q, want the owner's CLAUDE_CONFIG_DIR", prev)
	}
	// A Daemon-resolved token (env-over-file from ownerDirsFor) wins over the
	// process env: ownerDirsFor already applied the same precedence.
	env = claudeEnv(ownerDirs{claude: "/owner/claude", claudeToken: "fake-carried-token"})
	if last := env[len(env)-1]; last != "CLAUDE_CODE_OAUTH_TOKEN=fake-carried-token" {
		t.Fatalf("last adapter env entry = %q, want the carried token last-wins", last)
	}
	// Without any token there is no token entry at all.
	withCleanTokenEnv(t)
	for _, kv := range claudeEnv(ownerDirs{claude: "/owner/claude"}) {
		if strings.HasPrefix(kv, "CLAUDE_CODE_OAUTH_TOKEN=") {
			t.Fatalf("adapter env carries a token with none configured: %q", kv)
		}
	}
}

// TestLoadClaudeOAuthTokenFileModes: the token file loads trimmed, env beats
// the file, a missing file loads empty, and a group/world-readable file is
// refused. A Set write lands owner-only.
func TestLoadClaudeOAuthTokenFileModes(t *testing.T) {
	withCleanTokenEnv(t)
	stateDir := tempDir(t)
	if got := loadClaudeOAuthToken(stateDir); got != "" {
		t.Fatalf("token with no env or file = %q, want empty", got)
	}
	if err := SetClaudeOAuthToken(stateDir, "  fake-setup-token \n"); err != nil {
		t.Fatal(err)
	}
	path := ClaudeOAuthTokenPath(stateDir)
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("token file mode = %v, %v; want 0600", fi.Mode(), err)
	}
	if got := loadClaudeOAuthToken(stateDir); got != "fake-setup-token" {
		t.Fatalf("loaded token = %q, want trimmed", got)
	}
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "fake-env-token")
	if got := loadClaudeOAuthToken(stateDir); got != "fake-env-token" {
		t.Fatalf("loaded token = %q, want env over file", got)
	}
	withCleanTokenEnv(t)
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := loadClaudeOAuthToken(stateDir); got != "" {
		t.Fatalf("loaded token from a 0644 file = %q, want refusal", got)
	}
	if err := SetClaudeOAuthToken(stateDir, "   "); err == nil {
		t.Fatal("empty token stored without an error")
	}
}

// TestOwnerDirsEnvNeverCarriesToken: the recorded per-start env holds dirs
// only, so the token value never reaches records, logs or API output.
func TestOwnerDirsEnvNeverCarriesToken(t *testing.T) {
	t.Parallel()
	env := ownerDirs{claude: "/owner/claude", pig: "/owner/pig", claudeToken: "fake-token-value"}.env()
	if len(env) != 2 {
		t.Fatalf("owner env = %v, want exactly the two dir entries", env)
	}
	for _, kv := range env {
		if strings.Contains(kv, "fake-token-value") {
			t.Fatalf("owner env leaks the token: %q", kv)
		}
	}
}

// TestSeedClaudeDirWithTokenRemovesStaleCopies: in token mode nothing links
// and a stale symlink or regular-file credential copy goes away, while every
// other entry in the User dir is untouched.
func TestSeedClaudeDirWithTokenRemovesStaleCopies(t *testing.T) {
	t.Parallel()
	host := tempDir(t)
	writeTestFile(t, filepath.Join(host, claudeAuthFile), `{"host":true}`)
	dir := filepath.Join(tempDir(t), "claude")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(dir, claudeAuthFile), `{"stale":true}`)
	writeTestFile(t, filepath.Join(dir, "settings.json"), "{}")
	writeTestFile(t, filepath.Join(dir, "CLAUDE.md"), "owner notes")
	if err := seedClaudeDir(dir, host, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(dir, claudeAuthFile)); !os.IsNotExist(err) {
		t.Fatalf("stale regular copy survives token mode: %v", err)
	}
	for _, name := range []string{"settings.json", "CLAUDE.md"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("owner %s lost in token mode: %v", name, err)
		}
	}
	// A stale symlink goes the same way, and reseeding stays empty.
	if err := os.Symlink(filepath.Join(host, claudeAuthFile), filepath.Join(dir, claudeAuthFile)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := seedClaudeDir(dir, host, true); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Lstat(filepath.Join(dir, claudeAuthFile)); !os.IsNotExist(err) {
		t.Fatalf("stale link survives token mode: %v", err)
	}
}

// TestSeedClaudeDirFallbackRepairsStaleCopy: without a token an owner regular
// file that differs from the host and is older than the host re-links, while
// a newer or identical copy is left alone.
func TestSeedClaudeDirFallbackRepairsStaleCopy(t *testing.T) {
	t.Parallel()
	host := tempDir(t)
	writeTestFile(t, filepath.Join(host, claudeAuthFile), `{"v":2}`)
	old := time.Now().Add(-time.Hour)
	fresh := time.Now()

	stale := filepath.Join(tempDir(t), "claude")
	if err := os.MkdirAll(stale, 0o700); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(stale, claudeAuthFile), `{"v":1}`)
	if err := os.Chtimes(filepath.Join(host, claudeAuthFile), fresh, fresh); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(stale, claudeAuthFile), old, old); err != nil {
		t.Fatal(err)
	}
	if err := seedClaudeDir(stale, host, false); err != nil {
		t.Fatal(err)
	}
	if target, err := os.Readlink(filepath.Join(stale, claudeAuthFile)); err != nil || target != filepath.Join(host, claudeAuthFile) {
		t.Fatalf("stale copy = %q, %v; want a re-link to the host login", target, err)
	}

	newer := filepath.Join(tempDir(t), "claude")
	if err := os.MkdirAll(newer, 0o700); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(newer, claudeAuthFile), `{"v":3}`)
	if err := os.Chtimes(filepath.Join(newer, claudeAuthFile), fresh.Add(time.Hour), fresh.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := seedClaudeDir(newer, host, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Readlink(filepath.Join(newer, claudeAuthFile)); err == nil {
		t.Fatal("newer owner copy was re-linked; it may hold a fresher refresh")
	}
	raw, err := os.ReadFile(filepath.Join(newer, claudeAuthFile))
	if err != nil || string(raw) != `{"v":3}` {
		t.Fatalf("newer owner copy changed to %q, %v", raw, err)
	}

	same := filepath.Join(tempDir(t), "claude")
	if err := os.MkdirAll(same, 0o700); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(same, claudeAuthFile), `{"v":2}`)
	if err := seedClaudeDir(same, host, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Readlink(filepath.Join(same, claudeAuthFile)); err == nil {
		t.Fatal("identical owner copy was re-linked for no divergence")
	}
}

// TestOwnerDirsTokenModeEndToEnd: with a token file the Daemon resolves the
// token per start, links no credential, and clears a copy the CLI wrote —
// while the adapter env carries the token last and the recorded env does not.
func TestOwnerDirsTokenModeEndToEnd(t *testing.T) {
	withCleanTokenEnv(t)
	stateDir := tempDir(t)
	if err := SetClaudeOAuthToken(stateDir, "fake-e2e-token"); err != nil {
		t.Fatal(err)
	}
	d := NewDaemon(stateDir, NewFakeBackend())
	hostDirsForTest(t, d)
	dirs, err := d.ownerDirsFor("alice")
	if err != nil {
		t.Fatal(err)
	}
	if dirs.claudeToken != "fake-e2e-token" {
		t.Fatal("owner dirs did not resolve the token file")
	}
	if _, err := os.Lstat(filepath.Join(dirs.claude, claudeAuthFile)); !os.IsNotExist(err) {
		t.Fatalf("credential linked in token mode: %v", err)
	}
	// The CLI replaces links with private files on write: the next start
	// clears the copy so it cannot shadow the token.
	writeTestFile(t, filepath.Join(dirs.claude, claudeAuthFile), `{"cli":true}`)
	if _, err := d.ownerDirsFor("alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(dirs.claude, claudeAuthFile)); !os.IsNotExist(err) {
		t.Fatalf("CLI-written copy survives token mode: %v", err)
	}
	env := claudeEnv(dirs)
	if last := env[len(env)-1]; last != "CLAUDE_CODE_OAUTH_TOKEN=fake-e2e-token" {
		t.Fatalf("last adapter env entry = %q, want the shared token", last)
	}
	for _, kv := range dirs.env() {
		if strings.Contains(kv, "fake-e2e-token") {
			t.Fatalf("recorded owner env leaks the token: %q", kv)
		}
	}
}
