package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Seam: the Daemon HTTP API. Requests carry a loopback Host so the Host
// guard lets them through; the tests use the raw Handler, so the gate is live.

const (
	testUser     = "alice"
	testPassword = "correct horse battery"
)

func authReq(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, "http://127.0.0.1"+path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func authDo(t testing.TB, h http.Handler, method, path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := authReq(method, path, body)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func credsJSON(user, password string) string {
	b, _ := json.Marshal(map[string]string{"username": user, "password": password})
	return string(b)
}

func decodeUser(t testing.TB, rec *httptest.ResponseRecorder) User {
	t.Helper()
	var u User
	if err := json.NewDecoder(rec.Body).Decode(&u); err != nil {
		t.Fatalf("decode user: %v (body %q)", err, rec.Body.String())
	}
	return u
}

func sessionCookieFrom(t testing.TB, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookie && c.Value != "" {
			return c
		}
	}
	t.Fatalf("no %s cookie in response headers %v", sessionCookie, rec.Header())
	return nil
}

// signedIn signs up (or logs in) user and returns its session cookie.
func signedIn(t testing.TB, h http.Handler, user string) *http.Cookie {
	t.Helper()
	rec := authDo(t, h, http.MethodPost, "/api/auth/signup", credsJSON(user, testPassword))
	if rec.Code == http.StatusConflict {
		rec = authDo(t, h, http.MethodPost, "/api/auth/login", credsJSON(user, testPassword))
	}
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("sign in %s = %d: %s", user, rec.Code, rec.Body.String())
	}
	return sessionCookieFrom(t, rec)
}

// Slice A: the first signup is the admin, later signups are plain Users.
func TestFirstSignupIsAdminLaterSignupsAreUsers(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	h := d.Handler()

	first := authDo(t, h, http.MethodPost, "/api/auth/signup", credsJSON(testUser, testPassword))
	if first.Code != http.StatusCreated {
		t.Fatalf("first signup = %d, want 201: %s", first.Code, first.Body.String())
	}
	alice := decodeUser(t, first)
	if !alice.Admin {
		t.Fatalf("first signup admin = false, want true")
	}

	second := authDo(t, h, http.MethodPost, "/api/auth/signup", credsJSON("bob", testPassword))
	if second.Code != http.StatusCreated {
		t.Fatalf("second signup = %d, want 201: %s", second.Code, second.Body.String())
	}
	bob := decodeUser(t, second)
	if bob.Admin {
		t.Fatalf("later signup admin = true, want plain User")
	}
	if alice.ID == "" || bob.ID == "" || alice.ID == bob.ID {
		t.Fatalf("IDs alice=%q bob=%q, want set and distinct", alice.ID, bob.ID)
	}

	dup := authDo(t, h, http.MethodPost, "/api/auth/signup", credsJSON(testUser, testPassword))
	if dup.Code != http.StatusConflict {
		t.Fatalf("duplicate signup = %d, want 409", dup.Code)
	}
	bad := authDo(t, h, http.MethodPost, "/api/auth/signup", credsJSON("carol", "short"))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("short password signup = %d, want 400", bad.Code)
	}
}

// Slice A: a User keeps the same ID after the Daemon restarts.
func TestUserIDIsStableAcrossRestart(t *testing.T) {
	dir := tempDir(t)
	first := authDo(t, NewDaemon(dir, NewFakeBackend()).Handler(), http.MethodPost, "/api/auth/signup", credsJSON(testUser, testPassword))
	if first.Code != http.StatusCreated {
		t.Fatalf("signup = %d, want 201", first.Code)
	}
	id := decodeUser(t, first).ID

	restarted := NewDaemon(dir, NewFakeBackend()).Handler()
	login := authDo(t, restarted, http.MethodPost, "/api/auth/login", credsJSON(testUser, testPassword))
	if login.Code != http.StatusOK {
		t.Fatalf("login after restart = %d, want 200", login.Code)
	}
	if got := decodeUser(t, login).ID; got != id {
		t.Fatalf("ID after restart = %q, want %q", got, id)
	}
}

// Slice B: login sets an HttpOnly SameSite=Lax cookie; logout clears it and
// the server stops accepting the old cookie.
func TestLoginSetsCookieAndLogoutClearsIt(t *testing.T) {
	h := NewDaemon(tempDir(t), NewFakeBackend()).Handler()
	if rec := authDo(t, h, http.MethodPost, "/api/auth/signup", credsJSON(testUser, testPassword)); rec.Code != http.StatusCreated {
		t.Fatalf("signup = %d, want 201", rec.Code)
	}

	login := authDo(t, h, http.MethodPost, "/api/auth/login", credsJSON(testUser, testPassword))
	if login.Code != http.StatusOK {
		t.Fatalf("login = %d, want 200", login.Code)
	}
	setCookie := login.Header().Get("Set-Cookie")
	for _, want := range []string{"HttpOnly", "SameSite=Lax", "Path=/"} {
		if !strings.Contains(setCookie, want) {
			t.Fatalf("Set-Cookie %q lacks %s", setCookie, want)
		}
	}
	cookie := sessionCookieFrom(t, login)

	me := authDo(t, h, http.MethodGet, "/api/auth/me", "", cookie)
	if me.Code != http.StatusOK || decodeUser(t, me).Username != testUser {
		t.Fatalf("me with cookie = %d %s, want 200 for %s", me.Code, me.Body.String(), testUser)
	}

	out := authDo(t, h, http.MethodPost, "/api/auth/logout", "", cookie)
	if out.Code != http.StatusNoContent {
		t.Fatalf("logout = %d, want 204", out.Code)
	}
	if !strings.Contains(out.Header().Get("Set-Cookie"), "Max-Age=0") {
		t.Fatalf("logout Set-Cookie %q does not clear the cookie", out.Header().Get("Set-Cookie"))
	}
	after := authDo(t, h, http.MethodGet, "/api/auth/me", "", cookie)
	if after.Code != http.StatusUnauthorized {
		t.Fatalf("me with revoked cookie = %d, want 401", after.Code)
	}
}

// Slice B: a wrong password and an unknown name get the same answer.
func TestWrongPasswordAndUnknownUserAnswerAlike(t *testing.T) {
	h := NewDaemon(tempDir(t), NewFakeBackend()).Handler()
	authDo(t, h, http.MethodPost, "/api/auth/signup", credsJSON(testUser, testPassword))

	wrong := authDo(t, h, http.MethodPost, "/api/auth/login", credsJSON(testUser, "not the password"))
	unknown := authDo(t, h, http.MethodPost, "/api/auth/login", credsJSON("nobody", testPassword))
	if wrong.Code != http.StatusUnauthorized || unknown.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password %d, unknown user %d, want both 401", wrong.Code, unknown.Code)
	}
	if wrong.Body.String() != unknown.Body.String() {
		t.Fatalf("answers differ: %q vs %q", wrong.Body.String(), unknown.Body.String())
	}
}

// Slice C: every /api route and SSE stream answers 401 without a cookie; the
// static shell stays public.
func TestAPIRoutesNeedCookieStaticShellIsPublic(t *testing.T) {
	h := NewDaemon(tempDir(t), NewFakeBackend()).Handler()
	gets := []string{
		"/api/live", "/api/push/key", "/api/models?harness=claude", "/api/settings",
		"/api/stt", "/api/sessions", "/api/sessions/s1", "/api/sessions/s1/events",
		"/api/specs", "/api/specs/preview", "/api/runs", "/api/runs/r1", "/api/runs/r1/events",
		"/api/projects", "/api/projects/p1", "/api/quota", "/api/version", "/api/health",
		"/api/auth/unknown", "/x/../api/sessions",
	}
	for _, p := range gets {
		if rec := authDo(t, h, http.MethodGet, p, ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("GET %s without cookie = %d, want 401", p, rec.Code)
		}
	}
	posts := []string{"/api/sessions", "/api/restart", "/api/quota/refresh", "/api/stt", "/api/settings"}
	for _, p := range posts {
		if rec := authDo(t, h, http.MethodPost, p, "{}"); rec.Code != http.StatusUnauthorized {
			t.Errorf("POST %s without cookie = %d, want 401", p, rec.Code)
		}
	}
	if rec := authDo(t, h, http.MethodGet, "/", ""); rec.Code != http.StatusOK {
		t.Fatalf("GET / without cookie = %d, want 200 (static shell)", rec.Code)
	}

	cookie := signedIn(t, h, testUser)
	for _, p := range []string{"/api/sessions", "/api/version", "/api/settings"} {
		if rec := authDo(t, h, http.MethodGet, p, "", cookie); rec.Code != http.StatusOK {
			t.Errorf("GET %s with cookie = %d, want 200", p, rec.Code)
		}
	}
}

// Slice D: the machine token passes the gate as a machine caller, never as a
// User; a wrong bearer does not pass.
func TestMachineTokenPassesGateButIsNoUser(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	h := d.Handler()
	tok := d.MachineToken()
	if tok == "" {
		t.Fatal("machine token is empty")
	}
	bearer := func(method, path, token string) *httptest.ResponseRecorder {
		req := authReq(method, path, "")
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	if rec := bearer(http.MethodGet, "/api/sessions", tok); rec.Code != http.StatusOK {
		t.Fatalf("GET /api/sessions with machine token = %d, want 200", rec.Code)
	}
	if rec := bearer(http.MethodGet, "/api/auth/me", tok); rec.Code != http.StatusUnauthorized {
		t.Fatalf("me with machine token = %d, want 401 (no User)", rec.Code)
	}
	if rec := bearer(http.MethodGet, "/api/sessions", "wrong"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/sessions with wrong bearer = %d, want 401", rec.Code)
	}
}

// Slice D: local tools read the machine token from enderbloom/daemon.token,
// readable by the owner only.
func TestMachineTokenFileIsOwnerOnly(t *testing.T) {
	dir := tempDir(t)
	d := NewDaemon(dir, NewFakeBackend())
	path := filepath.Join(dir, "enderbloom", "daemon.token")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read token file: %v", err)
	}
	if got := strings.TrimSpace(string(raw)); got != d.MachineToken() {
		t.Fatalf("token file holds %q, want the Daemon's machine token", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("token file mode = %o, want 600", perm)
	}
}

// Slice E: only the admin writes signup_enabled; off refuses signup; the
// default is on.
func TestSignupEnabledIsAdminOnlyAndOffRefusesSignup(t *testing.T) {
	t.Setenv("EL_SETTINGS", filepath.Join(tempDir(t), "settings.json"))
	d := NewDaemon(tempDir(t), NewFakeBackend())
	h := d.Handler()
	admin := signedIn(t, h, testUser)
	plain := signedIn(t, h, "bob")
	toggle := func(cookies ...*http.Cookie) *httptest.ResponseRecorder {
		return authDo(t, h, http.MethodPut, "/api/settings", `{"signup_enabled":false}`, cookies...)
	}

	view := authDo(t, h, http.MethodGet, "/api/settings", "", admin)
	var got map[string]any
	if err := json.NewDecoder(view.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if on, ok := got["signup_enabled"].(bool); !ok || !on {
		t.Fatalf("default signup_enabled = %v, want true", got["signup_enabled"])
	}

	if rec := toggle(plain); rec.Code != http.StatusForbidden {
		t.Fatalf("plain User writes signup_enabled = %d, want 403", rec.Code)
	}
	if rec := toggle(); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous writes signup_enabled = %d, want 401", rec.Code)
	}
	if rec := toggle(admin); rec.Code != http.StatusOK {
		t.Fatalf("admin writes signup_enabled = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	if rec := authDo(t, h, http.MethodPost, "/api/auth/signup", credsJSON("carol", testPassword)); rec.Code != http.StatusForbidden {
		t.Fatalf("signup while off = %d, want 403", rec.Code)
	}

	on := authDo(t, h, http.MethodPut, "/api/settings", `{"signup_enabled":true}`, admin)
	if on.Code != http.StatusOK {
		t.Fatalf("admin turns signup on = %d, want 200", on.Code)
	}
	if rec := authDo(t, h, http.MethodPost, "/api/auth/signup", credsJSON("carol", testPassword)); rec.Code != http.StatusCreated {
		t.Fatalf("signup after turning on = %d, want 201", rec.Code)
	}
}

// Slice F: SetUserPassword resets the password, signs the User out and the
// running Daemon sees the change without a restart.
func TestSetUserPasswordResetsLoginAndSignsOut(t *testing.T) {
	dir := tempDir(t)
	h := NewDaemon(dir, NewFakeBackend()).Handler()
	old := signedIn(t, h, testUser)

	if err := SetUserPassword(dir, testUser, "new password 4321"); err != nil {
		t.Fatalf("SetUserPassword: %v", err)
	}
	if rec := authDo(t, h, http.MethodPost, "/api/auth/login", credsJSON(testUser, testPassword)); rec.Code != http.StatusUnauthorized {
		t.Fatalf("old password after reset = %d, want 401", rec.Code)
	}
	if rec := authDo(t, h, http.MethodPost, "/api/auth/login", credsJSON(testUser, "new password 4321")); rec.Code != http.StatusOK {
		t.Fatalf("new password after reset = %d, want 200", rec.Code)
	}
	if rec := authDo(t, h, http.MethodGet, "/api/auth/me", "", old); rec.Code != http.StatusUnauthorized {
		t.Fatalf("old cookie after reset = %d, want 401", rec.Code)
	}
}

// Slice F: an unknown name and a short password are refused.
func TestSetUserPasswordRefusesUnknownUserAndShortPassword(t *testing.T) {
	dir := tempDir(t)
	h := NewDaemon(dir, NewFakeBackend()).Handler()
	signedIn(t, h, testUser)

	if err := SetUserPassword(dir, "nobody", "long enough pw"); err == nil {
		t.Fatal("unknown user: no error")
	}
	if err := SetUserPassword(dir, testUser, "short"); err == nil {
		t.Fatal("short password: no error")
	}
}

// authedHandler is d.Handler() with a signed-in admin cookie added to every
// request. Tests of routes and behavior use it; the first call per Daemon
// creates the admin, later calls log in as the same account.
func authedHandler(t testing.TB, d *Daemon) http.Handler {
	t.Helper()
	cookie := signedIn(t, d.Handler(), "admin")
	h := d.Handler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.AddCookie(cookie)
		h.ServeHTTP(w, r)
	})
}
