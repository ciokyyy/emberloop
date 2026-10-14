package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func installFakeClaude(t *testing.T, script string) string {
	t.Helper()
	dir := tempDir(t)
	binary := filepath.Join(dir, "claude")
	content := "#!/bin/sh\n" + script
	if err := os.WriteFile(binary, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
	home := tempDir(t)
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return home
}

func startFakeClaudeLogin(t *testing.T, h http.Handler, admin *http.Cookie) claudeLoginView {
	t.Helper()
	rec := authDo(t, h, http.MethodPost, "/api/providers/claude/login", "", admin)
	if rec.Code != http.StatusCreated {
		t.Fatalf("start Claude login = %d: %s", rec.Code, rec.Body.String())
	}
	var view claudeLoginView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode Claude login start: %v (%s)", err, rec.Body.String())
	}
	if view.ID == "" {
		t.Fatalf("Claude login start = %+v, want session id", view)
	}
	return view
}

func readClaudeLogin(t *testing.T, h http.Handler, admin *http.Cookie, id string) claudeLoginView {
	t.Helper()
	rec := authDo(t, h, http.MethodGet, "/api/providers/claude/login/"+id, "", admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("read Claude login = %d: %s", rec.Code, rec.Body.String())
	}
	var view claudeLoginView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode Claude login status: %v (%s)", err, rec.Body.String())
	}
	return view
}

func waitClaudeLogin(t *testing.T, h http.Handler, admin *http.Cookie, id string, done func(claudeLoginView) bool) claudeLoginView {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		view := readClaudeLogin(t, h, admin, id)
		if done(view) {
			return view
		}
		time.Sleep(10 * time.Millisecond)
	}
	view := readClaudeLogin(t, h, admin, id)
	t.Fatalf("Claude login did not reach expected state; last view: %+v", view)
	return view
}

func TestClaudeLoginPTYSuccessWritesOnlyHostClaudeCredentials(t *testing.T) {
	t.Setenv("EL_PIG_AUTH", "")
	t.Setenv("TERM", "")
	home := installFakeClaude(t, `
if [ "$1" != "auth" ] || [ "$2" != "login" ]; then exit 9; fi
	if [ "${TERM-}" != "xterm-256color" ] || [ "$(stty size)" != "40 240" ]; then exit 10; fi
	printf '\033[31mContinue at https://claude.ai/oauth/authorize?code=login-test\033[0m\nPaste code: '
	IFS= read -r code
	if [ "$code" != "right-code" ]; then printf 'Invalid code\n'; exit 2; fi
	mkdir -p "$HOME/.claude"
	printf '{"claudeAiOauth":{"accessToken":"host-only-token","expiresAt":4102444800000}}' > "$HOME/.claude/.credentials.json"
	printf '\033[32mLogin successful\033[0m\n'
`)
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.hostPigDir = filepath.Join(tempDir(t), "pig")
	h := d.Handler()
	admin := signedIn(t, h, "admin")
	t.Cleanup(d.stopClaudeLogin)

	login := startFakeClaudeLogin(t, h, admin)
	view := waitClaudeLogin(t, h, admin, login.ID, func(view claudeLoginView) bool {
		return strings.Contains(view.Output, "https://claude.ai/oauth/authorize")
	})
	if !strings.Contains(view.Output, "Paste code") {
		t.Fatalf("Claude pty output = %q, want code prompt", view.Output)
	}
	if strings.Contains(view.Output, "\x1b") {
		t.Fatalf("Claude pty output retained ANSI escape sequences: %q", view.Output)
	}

	body, _ := json.Marshal(map[string]string{"code": "right-code"})
	sent := authDo(t, h, http.MethodPost, "/api/providers/claude/login/"+login.ID+"/code", string(body), admin)
	if sent.Code != http.StatusOK {
		t.Fatalf("send Claude code = %d: %s", sent.Code, sent.Body.String())
	}
	final := waitClaudeLogin(t, h, admin, login.ID, func(view claudeLoginView) bool { return view.State == "succeeded" })
	if !strings.Contains(final.Output, "Login successful") {
		t.Fatalf("successful login output = %q", final.Output)
	}

	credentialsPath := filepath.Join(home, ".claude", ".credentials.json")
	if _, err := os.Stat(credentialsPath); err != nil {
		t.Fatalf("Claude CLI did not write host credentials: %v", err)
	}
	pigAuth := filepath.Join(d.hostPigDir, "auth.json")
	if _, err := os.Stat(pigAuth); !os.IsNotExist(err) {
		t.Fatalf("Claude login wrote pig auth.json (stat error %v)", err)
	}
	providers, response := decodeProviderList(t, h, admin)
	claude := findProvider(t, providers, "claude")
	if !claude.Connected || claude.Source != "host_claude" || claude.KeySet {
		t.Fatalf("Claude provider status = %+v, want host login without key", claude)
	}
	if strings.Contains(response.Body.String(), "host-only-token") {
		t.Fatal("provider status leaked the Claude access token")
	}
}

func TestClaudeLoginPTYBadCode(t *testing.T) {
	t.Setenv("EL_PIG_AUTH", "")
	installFakeClaude(t, `
printf 'Visit https://claude.ai/oauth/authorize\nPaste code: '
IFS= read -r code
printf 'Invalid code\n'
exit 2
`)
	d := NewDaemon(tempDir(t), NewFakeBackend())
	h := d.Handler()
	admin := signedIn(t, h, "admin")
	t.Cleanup(d.stopClaudeLogin)

	login := startFakeClaudeLogin(t, h, admin)
	waitClaudeLogin(t, h, admin, login.ID, func(view claudeLoginView) bool {
		return strings.Contains(view.Output, "Paste code")
	})
	body, _ := json.Marshal(map[string]string{"code": "wrong-code"})
	sent := authDo(t, h, http.MethodPost, "/api/providers/claude/login/"+login.ID+"/code", string(body), admin)
	if sent.Code != http.StatusOK {
		t.Fatalf("send wrong Claude code = %d: %s", sent.Code, sent.Body.String())
	}
	failed := waitClaudeLogin(t, h, admin, login.ID, func(view claudeLoginView) bool { return view.State == "failed" })
	if !strings.Contains(failed.Output, "Invalid code") || failed.Error == "" {
		t.Fatalf("bad-code result = %+v, want CLI error output and exit state", failed)
	}
}

func TestClaudeLoginPTYExitIsReported(t *testing.T) {
	t.Setenv("EL_PIG_AUTH", "")
	installFakeClaude(t, `printf 'Visit https://claude.ai/oauth/authorize\n'
exit 7
`)
	d := NewDaemon(tempDir(t), NewFakeBackend())
	h := d.Handler()
	admin := signedIn(t, h, "admin")
	t.Cleanup(d.stopClaudeLogin)

	login := startFakeClaudeLogin(t, h, admin)
	failed := waitClaudeLogin(t, h, admin, login.ID, func(view claudeLoginView) bool { return view.State == "failed" })
	if failed.Error == "" || !strings.Contains(failed.Output, "https://claude.ai/oauth/authorize") {
		t.Fatalf("pty exit result = %+v, want output and exit error", failed)
	}
}

func TestClaudeLoginPTYIsAdminOnly(t *testing.T) {
	t.Setenv("EL_PIG_AUTH", "")
	installFakeClaude(t, `exit 0`)
	d := NewDaemon(tempDir(t), NewFakeBackend())
	h := d.Handler()
	signedIn(t, h, "admin")
	user := signedIn(t, h, "mira")

	response := authDo(t, h, http.MethodPost, "/api/providers/claude/login", "", user)
	if response.Code != http.StatusForbidden {
		t.Fatalf("non-admin Claude login start = %d: %s, want 403", response.Code, response.Body.String())
	}
	if d.claudeLogin != nil {
		t.Fatal("non-admin request started a Claude login process")
	}
}

func TestClaudeLoginPTYAllowsCodeRetryAfterInvalidInput(t *testing.T) {
	t.Setenv("EL_PIG_AUTH", "")
	installFakeClaude(t, `printf 'Paste code: '
IFS= read -r first
printf 'Invalid code\nPaste code again: '
IFS= read -r second
printf '%s\n%s' "$first" "$second" > "$HOME/code"
`)
	d := NewDaemon(tempDir(t), NewFakeBackend())
	h := d.Handler()
	admin := signedIn(t, h, "admin")
	t.Cleanup(d.stopClaudeLogin)
	login := startFakeClaudeLogin(t, h, admin)
	waitClaudeLogin(t, h, admin, login.ID, func(view claudeLoginView) bool { return strings.Contains(view.Output, "Paste code") })

	for _, code := range []string{"first\nsecond", ""} {
		body, _ := json.Marshal(map[string]string{"code": code})
		response := authDo(t, h, http.MethodPost, "/api/providers/claude/login/"+login.ID+"/code", string(body), admin)
		if response.Code != http.StatusBadRequest {
			t.Errorf("send invalid code %q = %d: %s, want 400", code, response.Code, response.Body.String())
		}
	}
	send := func(code string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{"code": code})
		return authDo(t, h, http.MethodPost, "/api/providers/claude/login/"+login.ID+"/code", string(body), admin)
	}
	if first := send("wrong-code"); first.Code != http.StatusOK {
		t.Fatalf("send first code = %d: %s", first.Code, first.Body.String())
	}
	waitClaudeLogin(t, h, admin, login.ID, func(view claudeLoginView) bool { return strings.Contains(view.Output, "Paste code again") })
	if second := send("right-code"); second.Code != http.StatusOK {
		t.Fatalf("send retry code = %d: %s", second.Code, second.Body.String())
	}
	waitClaudeLogin(t, h, admin, login.ID, func(view claudeLoginView) bool { return view.State == "succeeded" })
	codeOutput, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), "code"))
	if err != nil {
		t.Fatalf("fake CLI did not receive retry codes: %v", err)
	}
	if string(codeOutput) != "wrong-code\nright-code" {
		t.Fatalf("fake CLI received codes %q, want both attempts", codeOutput)
	}
}

func TestClaudeLoginPTYDuplicateStartReturnsConflict(t *testing.T) {
	t.Setenv("EL_PIG_AUTH", "")
	installFakeClaude(t, `printf 'waiting for code'
IFS= read -r code
`)
	d := NewDaemon(tempDir(t), NewFakeBackend())
	h := d.Handler()
	admin := signedIn(t, h, "admin")
	t.Cleanup(d.stopClaudeLogin)
	startFakeClaudeLogin(t, h, admin)
	second := authDo(t, h, http.MethodPost, "/api/providers/claude/login", "", admin)
	if second.Code != http.StatusConflict {
		t.Fatalf("second start = %d: %s, want 409", second.Code, second.Body.String())
	}
}

func TestClaudeProviderViewShowsExpiredLoginWithoutCredentials(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.hostClaudeDir = tempDir(t)
	credentials := `{"claudeAiOauth":{"accessToken":"expired-test-token","expiresAt":1}}`
	if err := os.WriteFile(filepath.Join(d.hostClaudeDir, claudeAuthFile), []byte(credentials), 0o600); err != nil {
		t.Fatal(err)
	}
	view := d.claudeHostProviderView()
	if view.Connected || view.Source != "expired" || view.SourceLabel != "Claude Code login expired" {
		t.Fatalf("expired Claude provider status = %+v", view)
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "expired-test-token") {
		t.Fatalf("Claude provider view exposed credential: %s", encoded)
	}
	if view.ID != "claude" || view.Group != providerSubscriptionLogins {
		t.Fatalf("Claude provider identity = %+v", view)
	}
}
