package web

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

var grokFixtureTools = []grokMCPTool{
	{
		Name:        "read",
		Description: "Read a file. Returns numbered lines.",
		InputSchema: map[string]any{
			"properties": map[string]any{
				"path":   map[string]any{"type": "string"},
				"offset": map[string]any{"type": "integer"},
				"limit":  map[string]any{"type": "integer"},
			},
			"required": []any{"path"},
		},
	},
	{
		Name:        "ctx_search",
		Description: "Search stored output",
		InputSchema: map[string]any{
			"properties": map[string]any{
				"queries": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"source":  map[string]any{"type": []any{"string", "null"}},
			},
			"required": []any{"queries"},
		},
	},
}

func TestRenderGrokCatalogListsTypedArguments(t *testing.T) {
	got := renderGrokCatalog("el", grokFixtureTools)
	want := "- el__read(path:str, limit?:int, offset?:int): Read a file.\n" +
		"- el__ctx_search(queries:str[], source?:str): Search stored output"
	if got != want {
		t.Fatalf("catalog:\n%s\nwant:\n%s", got, want)
	}
}

func TestFirstSentenceCutsLongDescriptions(t *testing.T) {
	if got := firstSentence("One. Two."); got != "One." {
		t.Fatalf("got %q", got)
	}
	long := strings.Repeat("x", 300)
	if got := firstSentence(long); len([]rune(got)) != 110 {
		t.Fatalf("len %d", len([]rune(got)))
	}
}

func TestGrokToolBlockFallsBackToSearchWithoutCatalog(t *testing.T) {
	if b := grokToolBlock(""); !strings.Contains(b, "search_tool") {
		t.Fatalf("no-catalog block should name search_tool: %q", b)
	}
	b := grokToolBlock("- el__read(path:str)")
	if !strings.Contains(b, "Never call search_tool") || !strings.Contains(b, "- el__read(path:str)") {
		t.Fatalf("catalog block: %q", b)
	}
}

func TestParseGrokModels(t *testing.T) {
	out := "You are logged in with grok.com.\n\nDefault model: grok-4.7\n\nAvailable models:\n  * grok-4.7 (default)\n  grok-5\n"
	got := parseGrokModels(out)
	if len(got) != 2 || got[0] != "grok-4.7" || got[1] != "grok-5" {
		t.Fatalf("models %v", got)
	}
	if len(parseGrokModels("garbage")) != 0 {
		t.Fatal("no model section should parse empty")
	}
}

func TestGrokProfileAllowlist(t *testing.T) {
	p := grokProfile(false)
	if !strings.Contains(p, "agents_md: false") || !strings.Contains(p, "  - use_tool\n") || strings.Contains(p, "search_tool") {
		t.Fatalf("profile without search:\n%s", p)
	}
	if !strings.Contains(grokProfile(true), "  - search_tool\n") {
		t.Fatal("fallback profile should keep search_tool")
	}
}

func newTestGrokBackend(t *testing.T) (*GrokACPBackend, string) {
	t.Helper()
	src := tempDir(t)
	cb := &ClaudeBackend{plain: true, waitMCP: true, mcpName: "el", sessions: map[string]*claudeState{}}
	b := &GrokACPBackend{
		ClaudeBackend: cb, stateDir: tempDir(t), grokBin: "/bin/true", srcHome: src,
		catalogs: map[string]string{"web": "- el__read(path:str)", "scout": "- el__read(path:str)"},
	}
	cb.metaHook = b.meta
	cb.promptWrap = grokPromptWrap
	return b, src
}

func TestPrepareDirStripsAndIsolates(t *testing.T) {
	b, src := newTestGrokBackend(t)
	if _, err := b.prepareDir("s1", "web"); err == nil || !strings.Contains(err.Error(), "grok login") {
		t.Fatalf("missing login should name grok login, got %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "auth.json"), []byte(`{"k":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	profile, err := b.prepareDir("s1", "web")
	if err != nil {
		t.Fatal(err)
	}
	gh := filepath.Join(b.sessionDir("s1"), "grok")
	if profile != filepath.Join(gh, "profile.md") {
		t.Fatalf("profile path %q", profile)
	}
	auth, _ := os.ReadFile(filepath.Join(gh, "auth.json"))
	if string(auth) != `{"k":1}` {
		t.Fatalf("auth copy %q", auth)
	}
	cfg, _ := os.ReadFile(filepath.Join(gh, "config.toml"))
	for _, want := range []string{"[subagents]\nenabled = false", "[compat.claude]", "mcps = false", "skills = false", "backend_tools = false"} {
		if !strings.Contains(string(cfg), want) {
			t.Fatalf("config.toml lacks %q:\n%s", want, cfg)
		}
	}
	prof, _ := os.ReadFile(profile)
	if strings.Contains(string(prof), "search_tool") {
		t.Fatalf("catalog present: profile must not allow search_tool:\n%s", prof)
	}
	if fi, err := os.Stat(filepath.Join(b.sessionDir("s1"), "home")); err != nil || !fi.IsDir() {
		t.Fatal("temp HOME missing")
	}
}

func TestSyncAuthWritesRefreshedTokenBack(t *testing.T) {
	b, src := newTestGrokBackend(t)
	srcAuth := filepath.Join(src, "auth.json")
	if err := os.WriteFile(srcAuth, []byte(`{"t":"old"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := b.prepareDir("s1", "web"); err != nil {
		t.Fatal(err)
	}
	tmp := filepath.Join(b.sessionDir("s1"), "grok", "auth.json")
	// The agent refreshes its copy.
	if err := os.WriteFile(tmp, []byte(`{"t":"new"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(tmp, later, later); err != nil {
		t.Fatal(err)
	}
	b.syncAuth("s1")
	if got, _ := os.ReadFile(srcAuth); string(got) != `{"t":"new"}` {
		t.Fatalf("refresh not written back: %q", got)
	}
	// A newer user file wins over a stale temp copy.
	if err := os.WriteFile(srcAuth, []byte(`{"t":"user"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	evenLater := later.Add(time.Minute)
	if err := os.Chtimes(srcAuth, evenLater, evenLater); err != nil {
		t.Fatal(err)
	}
	b.syncAuth("s1")
	if got, _ := os.ReadFile(srcAuth); string(got) != `{"t":"user"}` {
		t.Fatalf("newer user auth overwritten: %q", got)
	}
	// Garbage never replaces a good login.
	if err := os.WriteFile(tmp, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(tmp, evenLater.Add(time.Hour), evenLater.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	b.syncAuth("s1")
	if got, _ := os.ReadFile(srcAuth); string(got) != `{"t":"user"}` {
		t.Fatalf("invalid json replaced auth: %q", got)
	}
}

func TestGrokMetaCarriesOverrideCatalogAndYolo(t *testing.T) {
	b, _ := newTestGrokBackend(t)
	meta := b.meta("s1", "grok-4.7", "", tempDir(t), "web", ownerDirs{})
	if meta["yoloMode"] != true {
		t.Fatalf("yoloMode %v", meta["yoloMode"])
	}
	sys, _ := meta["systemPromptOverride"].(string)
	if !strings.Contains(sys, "- el__read(path:str)") || !strings.Contains(sys, "use_tool") {
		t.Fatalf("override lacks catalog or use_tool: %q", sys)
	}
	if _, ok := meta["claudeCode"]; ok {
		t.Fatal("grok meta must not carry the claude stripping set")
	}
}

func TestGrokPromptWrapAppendsReminderOnly(t *testing.T) {
	got := grokPromptWrap("do it")
	if !strings.HasPrefix(got, "do it\n\n<reminder>") || !strings.Contains(got, "CAVEMAN") {
		t.Fatalf("wrapped prompt %q", got)
	}
}

// grokStubConn adds the Grok-only seams to the shared stub.
type grokStubConn struct {
	*stubConn
	mu      sync.Mutex
	waited  int
	waitErr error
	options [][2]string
	optErr  error
}

func (g *grokStubConn) WaitMCPReady(time.Duration) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.waited++
	return g.waitErr
}

func (g *grokStubConn) SetConfigOption(acpID, configID, value string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.options = append(g.options, [2]string{configID, value})
	return g.optErr
}

func newStubGrok(t *testing.T, conn *grokStubConn) *GrokACPBackend {
	t.Helper()
	b, _ := newTestGrokBackend(t)
	b.ClaudeBackend.newConn = func() (acpConn, error) { return conn, nil }
	b.ClaudeBackend.mcpCommand = "/abs/stub-mcp"
	b.models = []string{"grok-4.7"}
	return b
}

func TestGrokStartWaitsForMCPAndSendsOverrideMeta(t *testing.T) {
	conn := &grokStubConn{stubConn: &stubConn{}}
	b := newStubGrok(t, conn)
	meta := SessionMeta{ID: "s1", Harness: "grok", Model: "grok-4.7", Effort: "max", WorkDir: tempDir(t)}
	if err := b.CreateSession(meta, func(string, string, string) {}); err != nil {
		t.Fatal(err)
	}
	if conn.waited != 1 {
		t.Fatalf("MCP-ready waited %d times, want 1", conn.waited)
	}
	if conn.newMeta["yoloMode"] != true {
		t.Fatalf("new meta %v", conn.newMeta)
	}
	if len(conn.options) != 1 || conn.options[0] != [2]string{"reasoning_effort", "xhigh"} {
		t.Fatalf("effort options %v (max must map to xhigh)", conn.options)
	}
}

func TestGrokStartFailsWhenMCPNeverReady(t *testing.T) {
	conn := &grokStubConn{stubConn: &stubConn{}, waitErr: errors.New("adapter MCP servers not ready after 45s")}
	b := newStubGrok(t, conn)
	err := b.CreateSession(SessionMeta{ID: "s1", Harness: "grok", Model: "grok-4.7", WorkDir: tempDir(t)}, func(string, string, string) {})
	if err == nil || !strings.Contains(err.Error(), "not ready") {
		t.Fatalf("want MCP not ready error, got %v", err)
	}
	if b.ResumeToken("s1") != "" {
		t.Fatal("failed start must leave no Session")
	}
}

func TestGrokCreateRejectsUnknownModelAndEffort(t *testing.T) {
	b := newStubGrok(t, &grokStubConn{stubConn: &stubConn{}})
	if err := b.CreateSession(SessionMeta{ID: "a", Model: "nope", WorkDir: tempDir(t)}, nil); err == nil || !strings.Contains(err.Error(), "unknown model") {
		t.Fatalf("unknown model: %v", err)
	}
	if err := b.CreateSession(SessionMeta{ID: "b", Model: "grok-4.7", Effort: "turbo", WorkDir: tempDir(t)}, nil); err == nil || !strings.Contains(err.Error(), "unknown effort") {
		t.Fatalf("unknown effort: %v", err)
	}
}

func TestGrokPromptReachesAgentWithReminder(t *testing.T) {
	conn := &grokStubConn{stubConn: &stubConn{}}
	b := newStubGrok(t, conn)
	if err := b.CreateSession(SessionMeta{ID: "s1", Harness: "grok", Model: "grok-4.7", WorkDir: tempDir(t)}, func(string, string, string) {}); err != nil {
		t.Fatal(err)
	}
	b.OnPrompt("s1", "hello", func(string, string, string) {})
	if len(conn.prompts) != 1 || !strings.HasPrefix(conn.prompts[0], "hello\n\n<reminder>") {
		t.Fatalf("prompts %q", conn.prompts)
	}
}

func TestUnregisteredGrokHarnessAnswers503(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/models?harness=grok")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("models status %d, want 503", resp.StatusCode)
	}
	resp, err = http.Post(srv.URL+"/api/sessions", "application/json",
		strings.NewReader(`{"harness":"grok","model":"grok-4.7","workDir":"`+tempDir(t)+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("create status %d, want 503 (never the fake backend)", resp.StatusCode)
	}
}
