package web

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/flock"
	"golang.org/x/crypto/argon2"
)

// User is one account on the Daemon. ID is random and never changes.
type User struct {
	ID       string    `json:"id"`
	Username string    `json:"username"`
	Admin    bool      `json:"admin"`
	Created  time.Time `json:"created"`
}

// sessionLifetime is how long a login cookie stays valid.
const sessionLifetime = 30 * 24 * time.Hour

const (
	minPasswordLen = 8
	maxPasswordLen = 1024
)

// Argon2id parameters: the OWASP minimum (19 MiB, two passes, one lane).
const (
	argonTime    = 2
	argonMemory  = 19 * 1024 // KiB
	argonThreads = 1
	argonKeyLen  = 32
	argonSaltLen = 16
)

var (
	errUsernameTaken  = errors.New("username taken")
	errBadUsername    = errors.New("username must be 1 to 32 characters: a-z, 0-9, dot, dash or underscore, starting with a letter or digit")
	errBadPassword    = fmt.Errorf("password must be %d to %d characters", minPasswordLen, maxPasswordLen)
	errBadCredentials = errors.New("invalid username or password")
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,31}$`)

// userFile is users.json: accounts with argon2id hashes, and login tokens
// kept as SHA-256 hashes with an expiry.
type userFile struct {
	Users  []storedUser  `json:"users"`
	Tokens []storedToken `json:"tokens"`
}

type storedUser struct {
	User
	Hash string `json:"hash"`
}

type storedToken struct {
	Hash    string    `json:"hash"`
	UserID  string    `json:"user_id"`
	Expires time.Time `json:"expires"`
}

// userStore keeps accounts under stateDir/enderbloom/users. Reads go to disk
// every time, so the Daemon and `emberloop-web user` see each other's writes.
type userStore struct {
	dir  string
	path string
	// mu serializes writers inside this process; the file lock covers other processes.
	mu sync.Mutex
}

func newUserStore(stateDir string) *userStore {
	dir := filepath.Join(stateDir, "enderbloom", "users")
	return &userStore{dir: dir, path: filepath.Join(dir, "users.json")}
}

func (s *userStore) read() (userFile, error) {
	var f userFile
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return f, err
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		return f, fmt.Errorf("read %s: %w", s.path, err)
	}
	return f, nil
}

// update applies change to the stored file under the writer lock and saves
// the result. Expired tokens are dropped on every save.
func (s *userStore) update(change func(*userFile) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	lock := flock.New(filepath.Join(s.dir, "users.lock"))
	if err := lock.Lock(); err != nil {
		return err
	}
	defer lock.Unlock()
	f, err := s.read()
	if err != nil {
		return err
	}
	if err := change(&f); err != nil {
		return err
	}
	now := time.Now()
	live := f.Tokens[:0]
	for _, t := range f.Tokens {
		if t.Expires.After(now) {
			live = append(live, t)
		}
	}
	f.Tokens = live
	raw, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(s.path, raw, 0o600)
}

// signup creates a User. The first User is the admin. The check and the write
// run under the writer lock, so two first signups cannot both be admin.
func (s *userStore) signup(username, password string) (User, error) {
	name, err := normalizeUsername(username)
	if err != nil {
		return User{}, err
	}
	if err := checkPassword(password); err != nil {
		return User{}, err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return User{}, err
	}
	var made User
	err = s.update(func(f *userFile) error {
		for _, u := range f.Users {
			if u.Username == name {
				return errUsernameTaken
			}
		}
		made = User{ID: newID(), Username: name, Admin: len(f.Users) == 0, Created: time.Now().UTC().Truncate(time.Second)}
		f.Users = append(f.Users, storedUser{User: made, Hash: hash})
		return nil
	})
	if err != nil {
		return User{}, err
	}
	return made, nil
}

// authenticate checks a password. An unknown name still runs one hash, so the
// answer takes the same time either way.
func (s *userStore) authenticate(username, password string) (User, error) {
	name := strings.ToLower(strings.TrimSpace(username))
	f, err := s.read()
	if err != nil {
		return User{}, err
	}
	for _, u := range f.Users {
		if u.Username == name {
			if verifyPassword(password, u.Hash) {
				return u.User, nil
			}
			return User{}, errBadCredentials
		}
	}
	verifyPassword(password, decoyHash())
	return User{}, errBadCredentials
}

// issueToken stores a login token for userID and returns its cookie value.
func (s *userStore) issueToken(userID string) (string, time.Time, error) {
	raw, err := randomToken()
	if err != nil {
		return "", time.Time{}, err
	}
	expires := time.Now().Add(sessionLifetime)
	err = s.update(func(f *userFile) error {
		f.Tokens = append(f.Tokens, storedToken{Hash: tokenHash(raw), UserID: userID, Expires: expires})
		return nil
	})
	return raw, expires, err
}

// userForToken resolves a cookie value to its live User.
func (s *userStore) userForToken(raw string) (User, bool) {
	if raw == "" {
		return User{}, false
	}
	f, err := s.read()
	if err != nil {
		return User{}, false
	}
	want := tokenHash(raw)
	now := time.Now()
	for _, t := range f.Tokens {
		if subtle.ConstantTimeCompare([]byte(t.Hash), []byte(want)) != 1 || !t.Expires.After(now) {
			continue
		}
		for _, u := range f.Users {
			if u.ID == t.UserID {
				return u.User, true
			}
		}
		return User{}, false
	}
	return User{}, false
}

// revoke drops one login token.
func (s *userStore) revoke(raw string) error {
	want := tokenHash(raw)
	return s.update(func(f *userFile) error {
		kept := f.Tokens[:0]
		for _, t := range f.Tokens {
			if t.Hash != want {
				kept = append(kept, t)
			}
		}
		f.Tokens = kept
		return nil
	})
}

// setPassword replaces the password of one User and drops every token that
// User holds, so a reset signs that User out everywhere.
func (s *userStore) setPassword(username, password string) error {
	name, err := normalizeUsername(username)
	if err != nil {
		return err
	}
	if err := checkPassword(password); err != nil {
		return err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	return s.update(func(f *userFile) error {
		for i := range f.Users {
			if f.Users[i].Username != name {
				continue
			}
			f.Users[i].Hash = hash
			id := f.Users[i].ID
			kept := f.Tokens[:0]
			for _, t := range f.Tokens {
				if t.UserID != id {
					kept = append(kept, t)
				}
			}
			f.Tokens = kept
			return nil
		}
		return fmt.Errorf("no user named %q", name)
	})
}

// SetUserPassword resets the password of one User under stateDir and signs
// that User out everywhere. A running Daemon sees the change at once; the
// file lock serializes the two writers.
func SetUserPassword(stateDir, username, password string) error {
	return newUserStore(stateDir).setPassword(username, password)
}

// writeFileAtomic writes data to a temp file beside path, then renames it over path.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func normalizeUsername(name string) (string, error) {
	n := strings.ToLower(strings.TrimSpace(name))
	if !usernamePattern.MatchString(n) {
		return "", errBadUsername
	}
	return n, nil
}

func checkPassword(password string) error {
	if len(password) < minPasswordLen || len(password) > maxPasswordLen {
		return errBadPassword
	}
	return nil
}

// randomToken returns 32 random bytes, base64url encoded.
func randomToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

func tokenHash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// hashPassword encodes an argon2id hash as a PHC string:
// $argon2id$v=19$m=...,t=...,p=...$salt$key.
func hashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

// verifyPassword checks password against a PHC string from hashPassword.
func verifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false
	}
	var memory, passes uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &passes, &threads); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) == 0 {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, passes, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

var decoy struct {
	once sync.Once
	hash string
}

// decoyHash is a real hash of a throwaway password, verified against unknown names.
func decoyHash() string {
	decoy.once.Do(func() {
		decoy.hash, _ = hashPassword("decoy password for unknown names")
	})
	return decoy.hash
}

// homeDir is the working directory of a User's Sessions that have no Project:
// <dir>/<name>/home. The caller creates it on first use.
func (s *userStore) homeDir(name string) string {
	return filepath.Join(s.dir, name, "home")
}
