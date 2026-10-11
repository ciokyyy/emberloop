package web

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// grokEffortStub is the stub ACP adapter with a reasoning-effort config
// option. It records the efforts the Backend picks, and its Close writes a
// file, standing in for an agent flushing its login on exit.
type grokEffortStub struct {
	stubConn
	mu        sync.Mutex
	efforts   []string
	setErr    error
	closeFile string
	closeData string
}

func (s *grokEffortStub) SetConfigOption(acpID, configID, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.setErr != nil {
		return s.setErr
	}
	s.efforts = append(s.efforts, acpID+":"+configID+"="+value)
	return nil
}

func (s *grokEffortStub) Close() error {
	if s.closeFile != "" {
		_ = os.WriteFile(s.closeFile, []byte(s.closeData), 0o600)
	}
	return nil
}

func newGrokHandoffBackend(stub acpConn, stateDir, srcHome string) *GrokACPBackend {
	cb := newStubClaudeBackend(func() (acpConn, error) { return stub, nil })
	cb.plain = true
	return &GrokACPBackend{ClaudeBackend: cb, stateDir: stateDir, srcHome: srcHome, catalogs: map[string]string{}, models: []string{"grok-test"}}
}

func writeAuth(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

// A grok Handoff swap re-applies the reasoning effort on the new agent and
// keeps a rotated login: the old agent's refresh syncs before the new start
// copies auth.json, and again when the old handle releases.
func TestGrokStartHandleSwapsEffortAndLogin(t *testing.T) {
	emit := func(kind, text, status string) {}
	stateDir := tempDir(t)
	srcHome := tempDir(t)
	writeAuth(t, filepath.Join(srcHome, "auth.json"), `{"refresh":"v1"}`)

	stub := &grokEffortStub{}
	be := newGrokHandoffBackend(stub, stateDir, srcHome)
	meta := SessionMeta{ID: "grok-h1", Model: "grok-test", WorkDir: tempDir(t), Effort: "max"}
	if err := be.CreateSession(meta, emit); err != nil {
		t.Fatalf("create: %v", err)
	}

	// The old agent refreshes its login mid-run.
	tmpAuth := filepath.Join(stateDir, "enderbloom", "grok-acp", meta.ID, "grok", "auth.json")
	writeAuth(t, tmpAuth, `{"refresh":"v2"}`)
	// Its exit flushes once more.
	stub.closeFile, stub.closeData = tmpAuth, `{"refresh":"v3"}`

	tok, release, err := be.StartHandle(meta, emit)
	if err != nil {
		t.Fatalf("StartHandle: %v", err)
	}
	if tok == "" || tok == "acp-session-1" {
		t.Fatalf("token = %q, want a rotated handle", tok)
	}
	// The pre-start sync already saved the mid-run refresh.
	if raw, err := os.ReadFile(filepath.Join(srcHome, "auth.json")); err != nil || string(raw) != `{"refresh":"v2"}` {
		t.Fatalf("login after swap = %q, %v, want the v2 refresh", raw, err)
	}

	release()
	if raw, err := os.ReadFile(filepath.Join(srcHome, "auth.json")); err != nil || string(raw) != `{"refresh":"v3"}` {
		t.Fatalf("login after release = %q, %v, want the v3 refresh", raw, err)
	}

	stub.mu.Lock()
	efforts := append([]string(nil), stub.efforts...)
	stub.mu.Unlock()
	if len(efforts) != 2 || efforts[0] != "acp-session-1:reasoning_effort=xhigh" || efforts[1] != tok+":reasoning_effort=xhigh" {
		t.Fatalf("efforts = %v, want create plus Handoff with max mapped to xhigh", efforts)
	}
}

// A failed grok swap keeps the old handle, and an unknown model or effort
// fails before anything starts.
func TestGrokStartHandleFailureKeepsOldHandle(t *testing.T) {
	emit := func(kind, text, status string) {}
	stub := &grokEffortStub{}
	be := newGrokHandoffBackend(stub, tempDir(t), tempDir(t))
	meta := SessionMeta{ID: "grok-h2", Model: "grok-test", WorkDir: tempDir(t)}
	if err := be.CreateSession(meta, emit); err != nil {
		t.Fatalf("create: %v", err)
	}

	stub.newErr = errNoAdapter
	if _, _, err := be.StartHandle(meta, emit); err == nil {
		t.Fatal("StartHandle succeeded, want the adapter error")
	}
	stub.newErr = nil
	be.mu.Lock()
	st, ok := be.sessions[meta.ID]
	be.mu.Unlock()
	if !ok || st.acpID != "acp-session-1" {
		t.Fatalf("backend handle = %+v, want the old acp-session-1", st)
	}

	for _, bad := range []SessionMeta{
		{ID: "grok-bad-model", Model: "nope", WorkDir: tempDir(t)},
		{ID: "grok-bad-effort", Model: "grok-test", Effort: "ultra", WorkDir: tempDir(t)},
	} {
		if _, _, err := be.StartHandle(bad, emit); err == nil {
			t.Fatalf("StartHandle(%+v) succeeded, want a validation error", bad)
		}
	}
}
