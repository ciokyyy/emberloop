package web

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCleanTitle(t *testing.T) {
	cases := map[string]string{
		"Retry with backoff":                          "Retry with backoff",
		"Here is a title:\n\n**Add retry to client**": "Here is a title",
		"\n\n**Add retry to client**\n":               "Add retry to client",
		`"Fix flaky test."`:                           "Fix flaky test",
		"Title: Fix `daemon.go` panic":                "Fix daemon.go panic",
		"   ":                                         "",
		"":                                            "",
		"# One  two\tthree":                           "One two three",
	}
	for in, want := range cases {
		if got := cleanTitle(in); got != want {
			t.Errorf("cleanTitle(%q) = %q, want %q", in, got, want)
		}
	}
	long := cleanTitle("word " + strings.Repeat("x", 90))
	if n := len([]rune(long)); n > titleMaxRunes {
		t.Errorf("title length = %d, want at most %d", n, titleMaxRunes)
	}
}

func waitTitle(t *testing.T, base, id string, want bool) string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		title := getSession(t, base, id).Meta.Title
		if (title != "") == want || time.Now().After(deadline) {
			return title
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// stubSequence hands out one stub per adapter start, in order: the Session's
// own adapter first, then each title call's throwaway one.
func stubSequence(stubs ...*stubConn) func() (acpConn, error) {
	var mu sync.Mutex
	i := 0
	return func() (acpConn, error) {
		mu.Lock()
		defer mu.Unlock()
		s := stubs[min(i, len(stubs)-1)]
		i++
		return s, nil
	}
}

// A claude Session titles itself after its first prompt with no settings: a
// throwaway ACP session on haiku, the title system prompt, no MCP servers.
func TestClaudeSessionGetsTitle(t *testing.T) {
	t.Setenv("EL_SESSION_TITLES", "1")
	t.Setenv("EL_SETTINGS", filepath.Join(tempDir(t), "settings.json"))
	own := &stubConn{updates: []map[string]any{chunk("answer")}}
	title := &stubConn{updates: []map[string]any{chunk("**Fix flaky retry test**")}}
	be := newStubClaudeBackend(stubSequence(own, title))
	d := NewDaemon(tempDir(t), NewFakeBackend()).WithClaudeBackend(be)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	id := createSession(t, srv.URL, "claude", "sonnet", tempDir(t))
	sendPrompt(t, srv.URL, id, "the retry test is flaky")
	if got := waitTitle(t, srv.URL, id, true); got != "Fix flaky retry test" {
		t.Fatalf("title = %q", got)
	}
	title.mu.Lock()
	defer title.mu.Unlock()
	if len(title.prompts) != 1 || title.prompts[0] != "Title this chat: 'the retry test is flaky'" {
		t.Fatalf("title prompts = %q", title.prompts)
	}
	if len(title.newMCP) != 0 {
		t.Fatalf("title session has MCP servers: %+v", title.newMCP)
	}
	if title.newMeta["systemPrompt"] != titleSystemPrompt {
		t.Fatalf("systemPrompt = %v", title.newMeta["systemPrompt"])
	}
	opts := title.newMeta["claudeCode"].(map[string]any)["options"].(map[string]any)
	if opts["model"] != "haiku" {
		t.Fatalf("title model = %v, want haiku", opts["model"])
	}
	// A second prompt never titles again.
	if len(own.prompts) != 1 {
		t.Fatalf("own prompts = %q", own.prompts)
	}
}

// A failed title call leaves the label alone and the Session working.
func TestTitleFailureLeavesLabel(t *testing.T) {
	t.Setenv("EL_SESSION_TITLES", "1")
	t.Setenv("EL_SETTINGS", filepath.Join(tempDir(t), "settings.json"))
	own := &stubConn{updates: []map[string]any{chunk("answer")}}
	title := &stubConn{promptErr: errNoAdapter}
	be := newStubClaudeBackend(stubSequence(own, title))
	d := NewDaemon(tempDir(t), NewFakeBackend()).WithClaudeBackend(be)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	id := createSession(t, srv.URL, "claude", "sonnet", tempDir(t))
	sendPrompt(t, srv.URL, id, "hello")
	events := readSSE(t, srv.URL, id, 0, 5, 5*time.Second)
	if len(events) < 3 {
		t.Fatalf("turn did not finish: %+v", events)
	}
	if got := waitTitle(t, srv.URL, id, true); got != "" {
		t.Fatalf("title = %q, want none after a failed call", got)
	}
}

// Sessions on a harness without a title role stay untitled: fake has no
// default, so nothing is spent.
func TestNoTitleWithoutRole(t *testing.T) {
	t.Setenv("EL_SETTINGS", filepath.Join(tempDir(t), "settings.json"))
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	sendPrompt(t, srv.URL, id, "hello")
	readSSE(t, srv.URL, id, 0, 4, 5*time.Second)
	if got := waitTitle(t, srv.URL, id, true); got != "" {
		t.Fatalf("title = %q, want none", got)
	}
}

// A pig Session titles through a bare SDK completion once a title role names
// a model; the title persists in the Session meta.
func TestPigSessionGetsTitleFromRole(t *testing.T) {
	t.Setenv("EL_SESSION_TITLES", "1")
	settings := filepath.Join(tempDir(t), "settings.json")
	role := `{"roles":{"pig-title":{"harness":"pig","model":"` + pigFauxModel + `"}}}`
	if err := os.WriteFile(settings, []byte(role), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EL_SETTINGS", settings)
	stateDir := tempDir(t)
	pb := newPigTestBackend(t)
	d := NewDaemon(stateDir, NewFakeBackend())
	d.RegisterBackend("pig", pb)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	id := createSession(t, srv.URL, "pig", pigFauxModel, tempDir(t))
	sendPrompt(t, srv.URL, id, "What is 20+22?")
	got := waitTitle(t, srv.URL, id, true)
	if got == "" {
		t.Fatal("pig Session got no title")
	}
	raw, err := os.ReadFile(filepath.Join(stateDir, "enderbloom", "sessions", id, "meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"title":`) {
		t.Fatalf("meta.json lacks the title: %s", raw)
	}
}

// The environment and the settings key both switch titles off.
func TestTitlesEnabledSwitch(t *testing.T) {
	settings := filepath.Join(tempDir(t), "settings.json")
	t.Setenv("EL_SETTINGS", settings)
	t.Setenv("EL_SESSION_TITLES", "")
	if !titlesEnabled() {
		t.Fatal("titles should default on")
	}
	if err := os.WriteFile(settings, []byte(`{"session_titles": false}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if titlesEnabled() {
		t.Fatal("settings key false should turn titles off")
	}
	t.Setenv("EL_SESSION_TITLES", "1")
	if !titlesEnabled() {
		t.Fatal("env 1 should win over the settings key")
	}
}

// Claude naming is the switch alone (haiku); pig naming is the pig-title
// role alone; the claude switch never touches pig and vice versa.
func TestTitleTargetSplitsClaudeAndPig(t *testing.T) {
	settings := filepath.Join(tempDir(t), "settings.json")
	t.Setenv("EL_SETTINGS", settings)
	t.Setenv("EL_SESSION_TITLES", "")
	if h, m := titleTarget("claude"); h != "claude" || m != "haiku" {
		t.Fatalf("claude target = %q %q, want claude haiku", h, m)
	}
	if h, _ := titleTarget("pig"); h != "" {
		t.Fatalf("pig target = %q without a pig-title role, want none", h)
	}
	cfg := `{"session_titles": false, "roles":{"pig-title":{"harness":"pig","model":"m1"},"title":{"harness":"claude","model":"opus"}}}`
	if err := os.WriteFile(settings, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	if h, _ := titleTarget("claude"); h != "" {
		t.Fatalf("claude target = %q with the switch off, want none", h)
	}
	if h, m := titleTarget("pig"); h != "pig" || m != "m1" {
		t.Fatalf("pig target = %q %q, want pig m1", h, m)
	}
	t.Setenv("EL_SESSION_TITLES", "0")
	if h, _ := titleTarget("pig"); h != "" {
		t.Fatalf("pig target = %q with env off, want none", h)
	}
}
