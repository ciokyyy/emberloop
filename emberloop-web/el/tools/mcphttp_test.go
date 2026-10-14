// HTTP MCP transport tests: the Streamable-HTTP round trip against the
// httptest fake, mirroring the stdio client tests. No live network (loopback
// only), no real home.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"example.com/emberloop/tools/mcpfake"
)

// httpRegistry builds a registry with one HTTP server and closes it when
// the test ends, so nothing outlives the test.
func httpRegistry(t *testing.T, name string, cfg MCPServerSetting) (*Registry, *mcpHTTPClient) {
	t.Helper()
	reg := NewRegistry(map[string]MCPServerSetting{name: cfg})
	t.Cleanup(reg.Close)
	reg.mu.Lock()
	defer reg.mu.Unlock()
	h, ok := reg.httpClients[name]
	if !ok {
		t.Fatalf("server %q did not register as an HTTP client", name)
	}
	return reg, h
}

func httpToolNames(ts []Tool) []string {
	out := make([]string, 0, len(ts))
	for _, tl := range ts {
		out = append(out, tl.Name())
	}
	return out
}

func mustHTTPContain(t *testing.T, err error, wants ...string) string {
	t.Helper()
	if err == nil {
		t.Fatalf("want error containing %q, got nil", wants)
	}
	for _, want := range wants {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not contain %q", err, want)
		}
	}
	return err.Error()
}

// Listing a server's tools opens the session (initialize plus
// notifications/initialized), then returns its tools with descriptions,
// schemas and read-only marks.
func TestMCPHTTPListsToolsFromFakeServer(t *testing.T) {
	fake := mcpfake.NewHTTPFake(t, mcpfake.HTTPConfig{
		Tools: []mcpfake.ToolDef{
			{Name: "search", Description: "search the docs",
				Schema: map[string]any{"type": "object", "properties": map[string]any{
					"query": map[string]any{"type": "string"},
				}}},
			{Name: "fetch", Description: "fetch a page", ReadOnly: true},
		},
	})
	reg, _ := httpRegistry(t, "docs", MCPServerSetting{URL: fake.MCPURL})
	got, err := reg.Tools(context.Background(), "docs")
	if err != nil {
		t.Fatal(err)
	}
	if names := httpToolNames(got); len(names) != 2 || names[0] != "search" || names[1] != "fetch" {
		t.Fatalf("tools = %v, want [search fetch]", names)
	}
	for _, tl := range got {
		if tl.Name() == "search" {
			if tl.Description() != "search the docs" {
				t.Errorf("search description = %q", tl.Description())
			}
			props, _ := tl.Schema()["properties"].(map[string]any)
			if _, ok := props["query"]; !ok {
				t.Errorf("search schema lost its query property: %v", tl.Schema())
			}
			if ro, ok := tl.(ReadOnly); !ok || ro.ReadOnly() {
				t.Errorf("search must not be marked read-only")
			}
		}
		if tl.Name() == "fetch" {
			ro, ok := tl.(ReadOnly)
			if !ok || !ro.ReadOnly() {
				t.Errorf("fetch must carry the server's readOnlyHint")
			}
		}
	}
	for _, want := range []string{"initialize", "notifications/initialized", "tools/list"} {
		found := false
		for _, m := range fake.Log() {
			if m == want {
				found = true
			}
		}
		if !found {
			t.Errorf("server never saw %s: %v", want, fake.Log())
		}
	}
}

// The same round trip works when the server frames replies as SSE.
func TestMCPHTTPListsToolsOverSSE(t *testing.T) {
	fake := mcpfake.NewHTTPFake(t, mcpfake.HTTPConfig{
		Tools: []mcpfake.ToolDef{{Name: "search", Description: "search the docs"}},
		SSE:   true,
	})
	reg, _ := httpRegistry(t, "docs", MCPServerSetting{URL: fake.MCPURL})
	got, err := reg.Tools(context.Background(), "docs")
	if err != nil {
		t.Fatal(err)
	}
	if names := httpToolNames(got); len(names) != 1 || names[0] != "search" {
		t.Fatalf("tools = %v, want [search]", names)
	}
	text, err := got[0].Execute(context.Background(), t.TempDir(), []byte(`{"query":"el"}`))
	if err != nil {
		t.Fatal(err)
	}
	if text != `search({"query":"el"})` {
		t.Fatalf("call text = %q, want the echo with arguments", text)
	}
}

// Building the registry dials nothing: the session opens on first use.
func TestMCPHTTPStartsServerLazily(t *testing.T) {
	fake := mcpfake.NewHTTPFake(t, mcpfake.HTTPConfig{
		Tools: []mcpfake.ToolDef{{Name: "search"}},
	})
	reg, _ := httpRegistry(t, "docs", MCPServerSetting{URL: fake.MCPURL})
	if len(fake.Log()) != 0 {
		t.Fatalf("registry construction hit the server: %v", fake.Log())
	}
	if _, err := reg.Tools(context.Background(), "docs"); err != nil {
		t.Fatal(err)
	}
	if len(fake.Log()) == 0 {
		t.Fatal("listing did not open the session")
	}
}

// The tools/list answer is cached: two listings cost one round trip.
func TestMCPHTTPCachesTheToolList(t *testing.T) {
	fake := mcpfake.NewHTTPFake(t, mcpfake.HTTPConfig{
		Tools: []mcpfake.ToolDef{{Name: "search"}},
	})
	reg, _ := httpRegistry(t, "docs", MCPServerSetting{URL: fake.MCPURL})
	ctx := context.Background()
	if _, err := reg.Tools(ctx, "docs"); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Tools(ctx, "docs"); err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, m := range fake.Log() {
		if m == "tools/list" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("tools/list reached the server %d times, want once: %v", n, fake.Log())
	}
}

// Configured static headers ride every request.
func TestMCPHTTPSendsConfiguredHeaders(t *testing.T) {
	var sawHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawHeader = r.Header.Get("X-Custom-Auth")
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "initialize":
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":"2024-11-05"}}`, req.ID)
		case "tools/list":
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":{"tools":[]}}`, req.ID)
		default:
			w.WriteHeader(http.StatusAccepted)
		}
	}))
	t.Cleanup(srv.Close)
	reg, _ := httpRegistry(t, "docs", MCPServerSetting{
		URL:     srv.URL,
		Headers: map[string]string{"X-Custom-Auth": "secret-yes"},
	})
	if _, err := reg.Tools(context.Background(), "docs"); err != nil {
		t.Fatal(err)
	}
	if sawHeader != "secret-yes" {
		t.Fatalf("server saw X-Custom-Auth %q, want the configured header", sawHeader)
	}
}

// A server that never answers initialize fails bounded with an error naming
// the server.
func TestMCPHTTPInitTimeoutNamesTheServer(t *testing.T) {
	fake := mcpfake.NewHTTPFake(t, mcpfake.HTTPConfig{
		Tools: []mcpfake.ToolDef{{Name: "search"}}, HangOn: "initialize",
	})
	reg, _ := httpRegistry(t, "docs", MCPServerSetting{URL: fake.MCPURL})
	reg.SetTimeouts(300*time.Millisecond, 300*time.Millisecond)
	_, err := reg.Tools(context.Background(), "docs")
	mustHTTPContain(t, err, `mcp server "docs"`, "initialize")
}

// A call that never answers fails bounded with an error naming the server
// and the tool.
func TestMCPHTTPCallTimeoutNamesTheServer(t *testing.T) {
	fake := mcpfake.NewHTTPFake(t, mcpfake.HTTPConfig{
		Tools: []mcpfake.ToolDef{{Name: "search"}}, HangOn: "call",
	})
	reg, _ := httpRegistry(t, "docs", MCPServerSetting{URL: fake.MCPURL})
	reg.SetTimeouts(2*time.Second, 300*time.Millisecond)
	got, err := reg.Tools(context.Background(), "docs")
	if err != nil {
		t.Fatal(err)
	}
	_, err = got[0].Execute(context.Background(), t.TempDir(), []byte(`{}`))
	mustHTTPContain(t, err, `mcp server "docs"`, "search", "timed out")
}

// A JSON-RPC error reply is a tool error naming the server and the message;
// a scripted isError answer carries its text.
func TestMCPHTTPProtocolErrorNamesTheServer(t *testing.T) {
	fake := mcpfake.NewHTTPFake(t, mcpfake.HTTPConfig{
		Tools: []mcpfake.ToolDef{{Name: "search"}, {Name: "quota"}},
		Calls: map[string]mcpfake.Result{
			"search": {Message: "boom"},
			"quota":  {Text: "quota spent", IsError: true},
		},
	})
	reg, _ := httpRegistry(t, "docs", MCPServerSetting{URL: fake.MCPURL})
	got, err := reg.Tools(context.Background(), "docs")
	if err != nil {
		t.Fatal(err)
	}
	_, err = got[0].Execute(context.Background(), t.TempDir(), []byte(`{}`))
	mustHTTPContain(t, err, `mcp server "docs"`, "search", "boom")
	_, err = got[1].Execute(context.Background(), t.TempDir(), []byte(`{}`))
	mustHTTPContain(t, err, `mcp server "docs"`, "quota spent")
}

// Calling an unknown tool is the server's -32602, naming the server.
func TestMCPHTTPUnknownToolNamesTheServer(t *testing.T) {
	fake := mcpfake.NewHTTPFake(t, mcpfake.HTTPConfig{
		Tools: []mcpfake.ToolDef{{Name: "search"}},
	})
	reg, _ := httpRegistry(t, "docs", MCPServerSetting{URL: fake.MCPURL})
	_, err := reg.Call(context.Background(), "docs", "ghost", json.RawMessage(`{}`))
	mustHTTPContain(t, err, `mcp server "docs"`, "ghost")
}

// A server that forgot the session (it restarted) costs one re-handshake,
// not the call: the next use re-initializes and answers.
func TestMCPHTTPRehandshakesAfterSessionGone(t *testing.T) {
	fake := mcpfake.NewHTTPFake(t, mcpfake.HTTPConfig{
		Tools:          []mcpfake.ToolDef{{Name: "search"}},
		StrictSessions: true,
	})
	reg, _ := httpRegistry(t, "docs", MCPServerSetting{URL: fake.MCPURL})
	got, err := reg.Tools(context.Background(), "docs")
	if err != nil {
		t.Fatal(err)
	}
	fake.ResetSessions()
	text, err := got[0].Execute(context.Background(), t.TempDir(), []byte(`{}`))
	if err != nil {
		t.Fatalf("call after session loss: %v", err)
	}
	if text != "search({})" {
		t.Fatalf("call after session loss = %q", text)
	}
	n := 0
	for _, m := range fake.Log() {
		if m == "initialize" {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("initialize reached the server %d times, want twice (handshake plus re-handshake): %v", n, fake.Log())
	}
}

// Calls after Close fail instead of redialing: the registry stays dead.
func TestMCPHTTPUseAfterCloseFails(t *testing.T) {
	fake := mcpfake.NewHTTPFake(t, mcpfake.HTTPConfig{
		Tools: []mcpfake.ToolDef{{Name: "search"}},
	})
	reg, h := httpRegistry(t, "docs", MCPServerSetting{URL: fake.MCPURL})
	got, err := reg.Tools(context.Background(), "docs")
	if err != nil {
		t.Fatal(err)
	}
	reg.Close()
	if _, err := reg.Tools(context.Background(), "docs"); err == nil {
		t.Fatal("listing after Close must fail")
	} else {
		mustHTTPContain(t, err, `mcp server "docs"`, "closed")
	}
	if _, err := got[0].Execute(context.Background(), t.TempDir(), []byte(`{}`)); err == nil {
		t.Fatal("call after Close must fail")
	}
	_ = h
}

// A snapshot lists every server and reports the broken ones: one bad server
// costs its own error, not the good server's tools.
func TestMCPHTTPSnapshotSkipsFailedServers(t *testing.T) {
	fake := mcpfake.NewHTTPFake(t, mcpfake.HTTPConfig{
		Tools: []mcpfake.ToolDef{{Name: "search"}},
	})
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := dead.URL
	dead.Close()
	reg := NewRegistry(map[string]MCPServerSetting{
		"good": {URL: fake.MCPURL},
		"bad":  {URL: deadURL},
	})
	t.Cleanup(reg.Close)
	reg.SetTimeouts(2*time.Second, 2*time.Second)
	listed, errs := reg.Snapshot(context.Background())
	if names := httpToolNames(listed["good"]); len(names) != 1 || names[0] != "search" {
		t.Fatalf("good server tools = %v", listed["good"])
	}
	if _, ok := listed["bad"]; ok {
		t.Fatalf("bad server listed tools: %v", listed["bad"])
	}
	if len(errs) != 1 {
		t.Fatalf("errs = %v, want one naming the bad server", errs)
	}
	if !strings.Contains(errs[0].Error(), `mcp server "bad"`) {
		t.Fatalf("errs = %v, want the bad server named", errs)
	}
}

// An entry with both command and url, or with neither, reports a settings
// error naming the server instead of dialing anything.
func TestMCPHTTPRegistryRejectsBadEntries(t *testing.T) {
	reg := NewRegistry(map[string]MCPServerSetting{
		"both":    {Command: "npx", URL: "http://127.0.0.1:9/mcp"},
		"neither": {},
	})
	t.Cleanup(reg.Close)
	_, err := reg.Tools(context.Background(), "both")
	mustHTTPContain(t, err, `mcp server "both"`, "either command or url")
	_, err = reg.Tools(context.Background(), "neither")
	mustHTTPContain(t, err, `mcp server "neither"`, "no command or url")
	if got := reg.Servers(); len(got) != 2 || got[0] != "both" || got[1] != "neither" {
		t.Fatalf("Servers = %v, want both entries listed so snapshots report them", got)
	}
	_, errs := reg.Snapshot(context.Background())
	if len(errs) != 2 {
		t.Fatalf("snapshot errs = %v, want both entries reported", errs)
	}
}

// Listing an unconfigured server is an error, not a hang.
func TestMCPHTTPRegistryRejectsUnknownServers(t *testing.T) {
	fake := mcpfake.NewHTTPFake(t, mcpfake.HTTPConfig{Tools: []mcpfake.ToolDef{{Name: "search"}}})
	reg, _ := httpRegistry(t, "docs", MCPServerSetting{URL: fake.MCPURL})
	_, err := reg.Tools(context.Background(), "ghost")
	mustHTTPContain(t, err, `unknown mcp server "ghost"`)
}

// The server's include, exclude and rename shape the HTTP set too: globs
// filter over the original names and a renamed tool still calls the server
// under its original name.
func TestMCPHTTPShapesToolsWithIncludeExcludeAndRename(t *testing.T) {
	fake := mcpfake.NewHTTPFake(t, mcpfake.HTTPConfig{
		Tools: []mcpfake.ToolDef{{Name: "search_docs"}, {Name: "delete_doc"}, {Name: "get_doc"}},
	})
	reg, _ := httpRegistry(t, "docs", MCPServerSetting{
		URL:     fake.MCPURL,
		Include: []string{"search_*", "get_*"},
		Exclude: []string{"delete_*", "get_doc"},
		Rename:  map[string]string{"search_docs": "docs_search"},
	})
	got, err := reg.Tools(context.Background(), "docs")
	if err != nil {
		t.Fatal(err)
	}
	if names := httpToolNames(got); len(names) != 1 || names[0] != "docs_search" {
		t.Fatalf("tools = %v, want [docs_search]", names)
	}
	if orig, ok := got[0].(interface{ OriginalName() string }); !ok || orig.OriginalName() != "search_docs" {
		t.Fatalf("renamed tool lost its original name: %#v", got[0])
	}
	text, err := got[0].Execute(context.Background(), t.TempDir(), []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if text != "search_docs({})" {
		t.Fatalf("renamed tool called as %q, want the original name", text)
	}
}

// HTTP tools serve through execute under their configured names and as
// tools.<server>.<name>: the stub carries both.
func TestMCPHTTPToolsServeThroughExecuteNamespace(t *testing.T) {
	fake := mcpfake.NewHTTPFake(t, mcpfake.HTTPConfig{
		Tools: []mcpfake.ToolDef{{Name: "search", Description: "search the docs"}},
	})
	reg, _ := httpRegistry(t, "docs", MCPServerSetting{URL: fake.MCPURL})
	got, err := reg.Tools(context.Background(), "docs")
	if err != nil {
		t.Fatal(err)
	}
	stub, err := buildToolsModule("/tmp/sock", got)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stub, "def search(") {
		t.Fatalf("the stub must define the configured name:\n%s", stub)
	}
	if !strings.Contains(stub, "docs.search") || !strings.Contains(stub, "_mcp_docs") {
		t.Fatalf("the stub must carry the server namespace alias:\n%s", stub)
	}
	for _, tl := range got {
		if s, ok := tl.(interface{ MCPServer() string }); !ok || s.MCPServer() != "docs" {
			t.Fatalf("HTTP tool lost its server marker: %#v", tl)
		}
	}
}

// sseResult pulls the matching reply out of an event stream and ignores
// anything else.
func TestSSEResultPicksTheMatchingReply(t *testing.T) {
	raw, err := sseResult([]byte("data: {\"jsonrpc\":\"2.0\",\"id\":7,\"result\":{\"ok\":true}}\n\n"), 7)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"ok":true`) {
		t.Fatalf("sse reply = %s, want the result", raw)
	}
	if _, err := sseResult([]byte("data: {\"jsonrpc\":\"2.0\",\"id\":8,\"result\":{}}\n\n"), 7); err == nil {
		t.Fatal("an event for another id must not answer")
	}
	if _, err := sseResult([]byte(": keepalive\n\ndata: not json\n\n"), 7); err == nil {
		t.Fatal("a stream with no matching reply must fail")
	}
}

// parseResourceMetadata pulls the metadata URL out of a 401 challenge.
func TestParseResourceMetadata(t *testing.T) {
	got := parseResourceMetadata(`Bearer resource_metadata="https://example.com/.well-known/oauth-protected-resource", error="invalid_token"`)
	if got != "https://example.com/.well-known/oauth-protected-resource" {
		t.Fatalf("challenge parsed as %q", got)
	}
	if got := parseResourceMetadata(`Bearer error="invalid_token"`); got != "" {
		t.Fatalf("a challenge without metadata must parse empty, got %q", got)
	}
}
