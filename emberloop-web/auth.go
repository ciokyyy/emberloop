package web

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// sessionCookie names the login cookie: HttpOnly, SameSite=Lax, Path=/.
const sessionCookie = "el_session"

// machineTokenFile is the file under enderbloom/ that holds the machine token.
const machineTokenFile = "daemon.token"

// publicAPIRoutes answer without a cookie. The auth routes check the cookie
// themselves; /api/auth/me answers 401 without one. /api/auth/state reveals
// only whether any account exists and whether signup is open.
var publicAPIRoutes = map[string]bool{
	"/api/auth/signup": true,
	"/api/auth/login":  true,
	"/api/auth/logout": true,
	"/api/auth/me":     true,
	"/api/auth/state":  true,
}

// requester is who sends a request: a signed-in User, or a machine caller
// that presented the Daemon's machine token (agent callbacks, the el CLI).
type requester struct {
	user *User // nil for a machine caller
}

// requesterOf resolves the bearer token or the login cookie of r.
func (d *Daemon) requesterOf(r *http.Request) (requester, bool) {
	if tok := bearerToken(r); tok != "" && subtle.ConstantTimeCompare([]byte(tok), []byte(d.machineToken)) == 1 {
		return requester{}, true
	}
	if c, err := r.Cookie(sessionCookie); err == nil {
		if u, ok := d.users.userForToken(c.Value); ok {
			return requester{user: &u}, true
		}
	}
	return requester{}, false
}

// requireRequester answers 401 to every /api request that is not a public
// auth route and carries neither a live cookie nor the machine token. The
// path is cleaned first, so a dot segment cannot slip past the prefix check.
// The static shell passes through untouched.
func (d *Daemon) requireRequester(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := path.Clean(r.URL.Path)
		if (p != "/api" && !strings.HasPrefix(p, "/api/")) || publicAPIRoutes[p] {
			next.ServeHTTP(w, r)
			return
		}
		if _, ok := d.requesterOf(r); !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func decodeCredentials(w http.ResponseWriter, r *http.Request) (credentials, bool) {
	var c credentials
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return c, false
	}
	return c, true
}

func (d *Daemon) handleSignup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !signupEnabledSetting() {
		http.Error(w, "signup is disabled", http.StatusForbidden)
		return
	}
	c, ok := decodeCredentials(w, r)
	if !ok {
		return
	}
	u, err := d.users.signup(c.Username, c.Password)
	switch {
	case errors.Is(err, errUsernameTaken):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, errBadUsername), errors.Is(err, errBadPassword):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case err != nil:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	default:
		d.startLogin(w, r, u, http.StatusCreated)
	}
}

func (d *Daemon) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	c, ok := decodeCredentials(w, r)
	if !ok {
		return
	}
	u, err := d.users.authenticate(c.Username, c.Password)
	switch {
	case errors.Is(err, errBadCredentials):
		http.Error(w, err.Error(), http.StatusUnauthorized)
	case err != nil:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	default:
		d.startLogin(w, r, u, http.StatusOK)
	}
}

// startLogin issues a login token for u, sets the cookie and answers the User.
func (d *Daemon) startLogin(w http.ResponseWriter, r *http.Request, u User, status int) {
	raw, expires, err := d.users.issueToken(u.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	setSessionCookie(w, r, raw, expires)
	writeJSON(w, status, u)
}

func (d *Daemon) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if c, err := r.Cookie(sessionCookie); err == nil {
		if err := d.users.revoke(c.Value); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	clearSessionCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
}

func (d *Daemon) handleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	who, ok := d.requesterOf(r)
	if !ok || who.user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, who.user)
}

// handleAuthState tells the sign-in screen whether the Daemon has no account
// yet and whether signup is open. It reveals nothing else: no names, no counts.
func (d *Daemon) handleAuthState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	f, err := d.users.read()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{
		"empty":          len(f.Users) == 0,
		"signup_enabled": signupEnabledSetting(),
	})
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, raw string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    raw,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isHTTPS(r),
	})
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isHTTPS(r),
	})
}

// isHTTPS reports a TLS request, or one a reverse proxy saw as https.
func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// bearerToken returns the token of an Authorization: Bearer header, or "".
func bearerToken(r *http.Request) string {
	const scheme = "Bearer "
	h := r.Header.Get("Authorization")
	if len(h) < len(scheme) || !strings.EqualFold(h[:len(scheme)], scheme) {
		return ""
	}
	return strings.TrimSpace(h[len(scheme):])
}

// newMachineToken makes the Daemon's machine token, new on every start. It is
// written owner-only to enderbloom/daemon.token so local tools (el restart,
// el update) can read it. The token works even when the file cannot be written.
func newMachineToken(stateDir string) (string, error) {
	tok, err := randomToken()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(stateDir, "enderbloom")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return tok, err
	}
	return tok, writeFileAtomic(filepath.Join(dir, machineTokenFile), []byte(tok+"\n"), 0o600)
}
