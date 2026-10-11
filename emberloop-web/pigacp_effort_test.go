package web

import (
	"strings"
	"sync"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
)

// thoughtStubConn is the stub ACP adapter with a thought_level config
// option: it records the thinking levels the Backend picks.
type thoughtStubConn struct {
	stubConn
	mu     sync.Mutex
	levels []string
	setErr error
}

func (s *thoughtStubConn) SetThoughtLevel(acpID, level string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.setErr != nil {
		return s.setErr
	}
	s.levels = append(s.levels, acpID+":"+level)
	return nil
}

func newThoughtStubBackend(stub *thoughtStubConn) *PigACPBackend {
	cb := newStubClaudeBackend(func() (acpConn, error) { return stub, nil })
	cb.plain = true
	return &PigACPBackend{ClaudeBackend: cb, sdk: NewPigBackend()}
}

// oneCatalogModel returns a fully-qualified model the catalog knows, so the
// Backend's model check passes without any provider auth.
func oneCatalogModel(t *testing.T) string {
	t.Helper()
	openrouter := ai.ListModels("openrouter")
	if len(openrouter) == 0 {
		t.Fatal("no openrouter models in catalog")
	}
	return openrouter[0].Provider + "/" + openrouter[0].ID
}

// Creating a pig ACP Session with an effort picks the adapter's thought
// level; an empty effort leaves the model default alone.
func TestPigACPEffortSetsThoughtLevel(t *testing.T) {
	model := oneCatalogModel(t)
	emit := func(kind, text, status string) {}

	stub := &thoughtStubConn{}
	be := newThoughtStubBackend(stub)
	meta := SessionMeta{ID: "pig-acp-effort", Model: model, WorkDir: tempDir(t), Effort: "high"}
	if err := be.CreateSession(meta, emit); err != nil {
		t.Fatalf("create: %v", err)
	}
	stub.mu.Lock()
	levels := append([]string(nil), stub.levels...)
	stub.mu.Unlock()
	if len(levels) != 1 || levels[0] != "acp-session-1:high" {
		t.Fatalf("thought levels = %v", levels)
	}

	stub2 := &thoughtStubConn{}
	be2 := newThoughtStubBackend(stub2)
	meta2 := SessionMeta{ID: "pig-acp-default", Model: model, WorkDir: tempDir(t)}
	if err := be2.CreateSession(meta2, emit); err != nil {
		t.Fatalf("create with default effort: %v", err)
	}
	stub2.mu.Lock()
	defer stub2.mu.Unlock()
	if len(stub2.levels) != 0 {
		t.Fatalf("default effort set thought levels = %v", stub2.levels)
	}
}

// An unknown effort is refused before the adapter starts.
func TestPigACPEffortRejected(t *testing.T) {
	stub := &thoughtStubConn{}
	be := newThoughtStubBackend(stub)
	err := be.CreateSession(SessionMeta{ID: "pig-acp-bad", Model: oneCatalogModel(t), WorkDir: tempDir(t), Effort: "ultra"}, func(kind, text, status string) {})
	if err == nil || !strings.Contains(err.Error(), "effort") {
		t.Fatalf("err = %v", err)
	}
	be.mu.Lock()
	defer be.mu.Unlock()
	if len(be.sessions) != 0 {
		t.Fatalf("refused create left %d sessions", len(be.sessions))
	}
}

// Resuming re-applies the Session's effort, and an adapter refusal fails
// the create with no Session left behind.
func TestPigACPResumeReappliesEffort(t *testing.T) {
	model := oneCatalogModel(t)
	emit := func(kind, text, status string) {}

	stub := &thoughtStubConn{}
	be := newThoughtStubBackend(stub)
	meta := SessionMeta{ID: "pig-acp-resume", Model: model, WorkDir: tempDir(t), Effort: "low", ResumeToken: "acp-session-9"}
	if err := be.ResumeSession(meta, emit); err != nil {
		t.Fatalf("resume: %v", err)
	}
	stub.mu.Lock()
	levels := append([]string(nil), stub.levels...)
	stub.mu.Unlock()
	if len(levels) != 1 || levels[0] != "acp-session-9:low" {
		t.Fatalf("thought levels = %v", levels)
	}

	stubFail := &thoughtStubConn{setErr: errNoAdapter}
	beFail := newThoughtStubBackend(stubFail)
	if err := beFail.CreateSession(SessionMeta{ID: "pig-acp-refused", Model: model, WorkDir: tempDir(t), Effort: "high"}, emit); err == nil {
		t.Fatal("refused thought level accepted")
	}
	beFail.mu.Lock()
	defer beFail.mu.Unlock()
	if len(beFail.sessions) != 0 {
		t.Fatalf("failed create left %d sessions", len(beFail.sessions))
	}
}
