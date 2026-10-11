package memory

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
)

// Engine owns the banks under one root directory. Each User's bank is
// root/users/<userID>.db; its snapshots are in root/snapshots/<userID>/.
type Engine struct {
	root   string
	opts   Options
	mu     sync.Mutex
	banks  map[string]*Bank
	closed bool
}

// Open prepares an engine at root. Banks open on first use in Bank.
func Open(root string, opts Options) (*Engine, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(abs, "users"), 0o755); err != nil {
		return nil, fmt.Errorf("memory: open root: %w", err)
	}
	return &Engine{root: abs, opts: opts, banks: make(map[string]*Bank)}, nil
}

// Bank returns the bank of userID. The first call creates, migrates and starts
// the bank's writer. userID must match the bank-name pattern, so it can name
// a file safely.
func (e *Engine) Bank(userID string) (*Bank, error) {
	if !validUserID(userID) {
		return nil, fmt.Errorf("%w: user id %q", ErrInvalid, userID)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil, ErrClosed
	}
	if b, ok := e.banks[userID]; ok {
		return b, nil
	}
	b, err := openBank(
		filepath.Join(e.root, "users", userID+".db"),
		filepath.Join(e.root, "snapshots", userID),
		e.opts,
	)
	if err != nil {
		return nil, err
	}
	e.banks[userID] = b
	return b, nil
}

// Close stops every bank's writer and closes its files. It is safe to call twice.
func (e *Engine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil
	}
	e.closed = true
	var errs []error
	for _, b := range e.banks {
		errs = append(errs, b.Close())
	}
	e.banks = nil
	return errors.Join(errs...)
}

// dsn builds a file: URI for path with the given query parameters.
func dsn(path string, params url.Values) string {
	u := url.URL{Scheme: "file", Path: path, RawQuery: params.Encode()}
	return u.String()
}
