package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	web "example.com/emberloop-web"
)

// Seam: `emberloop-web user set-password` against the Daemon HTTP API. The
// Daemon signs in and out through the same state dir the command writes.

const (
	cliUser        = "alice"
	cliOldPassword = "old password 1234"
)

// stdinWith gives runUserCommand a pipe that holds text, like `printf text |`.
func stdinWith(t *testing.T, text string) *os.File {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	if _, err := w.WriteString(text); err != nil {
		t.Fatal(err)
	}
	w.Close()
	return r
}

// loginStatus posts a login to the Daemon at base and returns the status.
func loginStatus(t *testing.T, base, name, password string) int {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": name, "password": password})
	resp, err := http.Post(base+"/api/auth/login", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestUserSetPasswordFromPipeResetsLogin(t *testing.T) {
	dir := tempDir(t)
	srv := httptest.NewServer(web.NewDaemon(dir, web.NewFakeBackend()).Handler())
	defer srv.Close()
	body, _ := json.Marshal(map[string]string{"username": cliUser, "password": cliOldPassword})
	resp, err := http.Post(srv.URL+"/api/auth/signup", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("signup = %d, want 201", resp.StatusCode)
	}

	var out, errOut bytes.Buffer
	code := runUserCommand([]string{"set-password", "-state-dir", dir, cliUser}, stdinWith(t, "new password 4321\n"), &out, &errOut)
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr %q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "password reset for alice") {
		t.Fatalf("stdout = %q, want the reset notice", out.String())
	}
	if got := loginStatus(t, srv.URL, cliUser, "new password 4321"); got != http.StatusOK {
		t.Fatalf("login with new password = %d, want 200", got)
	}
	if got := loginStatus(t, srv.URL, cliUser, cliOldPassword); got != http.StatusUnauthorized {
		t.Fatalf("login with old password = %d, want 401", got)
	}
}

func TestUserSetPasswordRefusesUnknownUserAndShortPassword(t *testing.T) {
	dir := tempDir(t)
	srv := httptest.NewServer(web.NewDaemon(dir, web.NewFakeBackend()).Handler())
	defer srv.Close()
	body, _ := json.Marshal(map[string]string{"username": cliUser, "password": cliOldPassword})
	if resp, err := http.Post(srv.URL+"/api/auth/signup", "application/json", strings.NewReader(string(body))); err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("signup failed: %v", err)
	}

	var out, errOut bytes.Buffer
	if code := runUserCommand([]string{"set-password", "-state-dir", dir, "nobody"}, stdinWith(t, "new password 4321\n"), &out, &errOut); code != 1 {
		t.Fatalf("unknown user exit = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), `no user named "nobody"`) {
		t.Fatalf("stderr = %q, want the unknown-user error", errOut.String())
	}

	errOut.Reset()
	if code := runUserCommand([]string{"set-password", "-state-dir", dir, cliUser}, stdinWith(t, "short\n"), &out, &errOut); code != 1 {
		t.Fatalf("short password exit = %d, want 1", code)
	}
	if got := loginStatus(t, srv.URL, cliUser, cliOldPassword); got != http.StatusOK {
		t.Fatalf("old password after refused reset = %d, want 200", got)
	}
}

func TestUserCommandUsage(t *testing.T) {
	for _, args := range [][]string{nil, {"passwd", "alice"}, {"set-password"}, {"set-password", "a", "b"}} {
		var out, errOut bytes.Buffer
		if code := runUserCommand(args, stdinWith(t, ""), &out, &errOut); code != 2 {
			t.Errorf("args %q exit = %d, want 2", args, code)
		}
		if !strings.Contains(errOut.String(), "usage: emberloop-web user set-password") {
			t.Errorf("args %q stderr = %q, want usage", args, errOut.String())
		}
	}
}

// tempDir replaces t.TempDir for tests that start Daemons: their writes can
// land after the test returns, so cleanup retries removal for a few seconds
// instead of failing the test (see emberloop-web/tempdir_test.go).
func tempDir(t testing.TB) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "el-web-user-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		deadline := time.Now().Add(5 * time.Second)
		for {
			if os.RemoveAll(dir) == nil || time.Now().After(deadline) {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	})
	return dir
}
