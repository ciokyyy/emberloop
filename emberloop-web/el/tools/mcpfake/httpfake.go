// The Streamable-HTTP twin of fake.go: one httptest server answering MCP
// initialize, tools/list and tools/call over POST (plain JSON or SSE), plus
// a miniature OAuth authorization server (protected-resource metadata,
// server metadata, dynamic registration, authorize, token) so the client's
// 401 flow runs without the network. Tests drive it through HTTPFake:
// sessions, bearer tokens, revocation and hangs are all knobs, and Log
// records the MCP methods the server saw.
package mcpfake

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// HTTPConfig drives one fake HTTP MCP server.
type HTTPConfig struct {
	Tools []ToolDef `json:"tools,omitempty"`
	// Calls scripts per-tool results by original name, the way Config does.
	Calls map[string]Result `json:"calls,omitempty"`
	// HangOn never answers one method: "initialize", "list" or "call". The
	// handler blocks until the request context ends.
	HangOn string `json:"hang_on,omitempty"`
	// Bearer, when set, is the only accepted token: anything else gets 401
	// plus a WWW-Authenticate challenge naming the resource metadata.
	Bearer string `json:"bearer,omitempty"`
	// RequireIssuedToken accepts any token the fake's own token endpoint
	// issued and has not revoked, modelling a real authorization server.
	// Anything else gets the same 401 challenge as Bearer.
	RequireIssuedToken bool `json:"require_issued_token,omitempty"`
	// IssueAccess overrides the access token the token endpoint issues, so a
	// test can make the approval succeed while the MCP side still 401s.
	IssueAccess string `json:"issue_access,omitempty"`
	// HideResourceMeta answers 401 without a challenge: discovery can then
	// only succeed through the settings metadata override.
	HideResourceMeta bool `json:"hide_resource_meta,omitempty"`
	// SSE frames MCP replies as text/event-stream instead of plain JSON.
	SSE bool `json:"sse,omitempty"`
	// StrictSessions answers 404 to an unknown session id, the way a
	// restarted server drops its sessions.
	StrictSessions bool `json:"strict_sessions,omitempty"`
}

// HTTPFake is one running fake: its Server answers MCP and OAuth, and the
// knobs below steer it mid-test.
type HTTPFake struct {
	Server        *httptest.Server
	MCPURL        string
	mu            sync.Mutex
	methods       []string
	sessions      map[string]bool
	nextSession   int
	codes         map[string]httpCode
	access        map[string]bool
	refresh       map[string]bool
	nextToken     int
	registrations []map[string]any
	authorizes    []url.Values
	bearer        string
}

// httpCode is one issued authorization code with the PKCE challenge and the
// client it belongs to.
type httpCode struct {
	challenge   string
	clientID    string
	redirectURI string
}

// NewHTTPFake starts the fake and closes it when the test ends.
func NewHTTPFake(t *testing.T, cfg HTTPConfig) *HTTPFake {
	t.Helper()
	f := &HTTPFake{
		sessions: map[string]bool{},
		codes:    map[string]httpCode{},
		access:   map[string]bool{},
		refresh:  map[string]bool{},
		bearer:   cfg.Bearer,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) { f.serveMCP(w, r, cfg) })
	mux.HandleFunc("/.well-known/oauth-protected-resource", func(w http.ResponseWriter, r *http.Request) { f.serveResourceMeta(w, r) })
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) { f.serveServerMeta(w, r) })
	mux.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) { f.serveRegister(w, r) })
	mux.HandleFunc("/authorize", func(w http.ResponseWriter, r *http.Request) { f.serveAuthorize(w, r) })
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) { f.serveToken(w, r, cfg) })
	f.Server = httptest.NewServer(mux)
	f.MCPURL = f.Server.URL + "/mcp"
	t.Cleanup(f.Server.Close)
	return f
}

// Log returns the MCP method names the server saw, in order.
func (f *HTTPFake) Log() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.methods...)
}

// Registrations returns the registration bodies the server saw.
func (f *HTTPFake) Registrations() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]map[string]any(nil), f.registrations...)
}

// Authorizes returns the authorize query strings the server saw.
func (f *HTTPFake) Authorizes() []url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]url.Values(nil), f.authorizes...)
}

// RevokeAll invalidates every issued token: the next MCP call 401s and the
// next refresh fails with invalid_grant.
func (f *HTTPFake) RevokeAll() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.access = map[string]bool{}
	f.refresh = map[string]bool{}
}

// ResetSessions forgets every session: with StrictSessions the next call
// 404s and the client must re-handshake.
func (f *HTTPFake) ResetSessions() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sessions = map[string]bool{}
}

// SetBearer changes the accepted token mid-test.
func (f *HTTPFake) SetBearer(bearer string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bearer = bearer
}

type httpRequest struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

func (f *HTTPFake) serveMCP(w http.ResponseWriter, r *http.Request, cfg HTTPConfig) {
	if r.Method == http.MethodDelete {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, _ := ioReadAll(r, 8<<20)
	var req httpRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	bearer := f.bearer
	issued := f.access[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")]
	requireIssued := cfg.RequireIssuedToken
	f.mu.Unlock()
	authed := r.Header.Get("Authorization") == "Bearer "+bearer && bearer != ""
	if requireIssued {
		authed = issued
	}
	if (bearer != "" || requireIssued) && !authed {
		challenge := ""
		if !cfg.HideResourceMeta {
			challenge = fmt.Sprintf(`Bearer resource_metadata="%s/.well-known/oauth-protected-resource"`, f.Server.URL)
		}
		if challenge != "" {
			w.Header().Set("WWW-Authenticate", challenge)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintln(w, `{"error":"unauthorized"}`)
		return
	}
	if req.ID == nil {
		// A notification: log it and answer 202, like a real server.
		f.mu.Lock()
		f.methods = append(f.methods, req.Method)
		f.mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
		return
	}
	session := r.Header.Get("Mcp-Session-Id")
	f.mu.Lock()
	_, known := f.sessions[session]
	f.mu.Unlock()
	if cfg.StrictSessions && req.Method != "initialize" && !known {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	hang := ""
	switch req.Method {
	case "initialize":
		hang = "initialize"
	case "tools/list":
		hang = "list"
	case "tools/call":
		hang = "call"
	}
	if cfg.HangOn != "" && cfg.HangOn == hang {
		<-r.Context().Done()
		return
	}
	f.mu.Lock()
	f.methods = append(f.methods, req.Method)
	if req.Method == "initialize" {
		f.nextSession++
		session = fmt.Sprintf("session-%d", f.nextSession)
		f.sessions[session] = true
	}
	f.mu.Unlock()
	if session != "" {
		w.Header().Set("Mcp-Session-Id", session)
	}
	reply := func(result any, code int, message string) {
		m := map[string]any{"jsonrpc": "2.0", "id": req.ID}
		if message != "" {
			m["error"] = map[string]any{"code": code, "message": message}
		} else {
			m["result"] = result
		}
		raw, _ := json.Marshal(m)
		if cfg.SSE {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprintf(w, "data: %s\n\n", raw)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(raw)
		w.Write([]byte("\n"))
	}
	switch req.Method {
	case "initialize":
		reply(map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "mcphttpfake", "version": "1"},
		}, 0, "")
	case "tools/list":
		listed := make([]map[string]any, 0, len(cfg.Tools))
		for _, tl := range cfg.Tools {
			schema := tl.Schema
			if schema == nil {
				schema = map[string]any{"type": "object"}
			}
			entry := map[string]any{
				"name": tl.Name, "description": tl.Description, "inputSchema": schema,
			}
			if tl.ReadOnly {
				entry["annotations"] = map[string]any{"readOnlyHint": true}
			}
			listed = append(listed, entry)
		}
		reply(map[string]any{"tools": listed}, 0, "")
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil || p.Name == "" {
			reply(nil, -32602, "a tool call must name a tool")
			return
		}
		known := false
		for _, tl := range cfg.Tools {
			if tl.Name == p.Name {
				known = true
			}
		}
		if !known {
			reply(nil, -32602, fmt.Sprintf("unknown tool %q", p.Name))
			return
		}
		if scripted, ok := cfg.Calls[p.Name]; ok {
			if scripted.Message != "" {
				code := scripted.Code
				if code == 0 {
					code = -32603
				}
				reply(nil, code, scripted.Message)
				return
			}
			reply(map[string]any{
				"content": []map[string]any{{"type": "text", "text": scripted.Text}},
				"isError": scripted.IsError,
			}, 0, "")
			return
		}
		args := string(bytes.TrimSpace(p.Arguments))
		if args == "" {
			args = "{}"
		}
		reply(map[string]any{
			"content": []map[string]any{{"type": "text", "text": p.Name + "(" + args + ")"}},
			"isError": false,
		}, 0, "")
	default:
		reply(nil, -32601, fmt.Sprintf("unknown method %q", req.Method))
	}
}

func (f *HTTPFake) serveResourceMeta(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"resource":              f.MCPURL,
		"authorization_servers": []string{f.Server.URL},
	})
}

func (f *HTTPFake) serveServerMeta(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"issuer":                 f.Server.URL,
		"authorization_endpoint": f.Server.URL + "/authorize",
		"token_endpoint":         f.Server.URL + "/token",
		"registration_endpoint":  f.Server.URL + "/register",
	})
}

func (f *HTTPFake) serveRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, _ := ioReadAll(r, 1<<20)
	var reg map[string]any
	_ = json.Unmarshal(body, &reg)
	f.mu.Lock()
	f.nextToken++
	id := fmt.Sprintf("fake-client-%d", f.nextToken)
	f.registrations = append(f.registrations, reg)
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"client_id": id})
}

func (f *HTTPFake) serveAuthorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f.mu.Lock()
	f.authorizes = append(f.authorizes, q)
	f.mu.Unlock()
	if q.Get("client_id") == "" || q.Get("code_challenge") == "" || q.Get("redirect_uri") == "" {
		http.Error(w, "missing authorize parameters", http.StatusBadRequest)
		return
	}
	if q.Get("code_challenge_method") != "" && q.Get("code_challenge_method") != "S256" {
		http.Error(w, "only S256 is supported", http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	f.nextToken++
	code := fmt.Sprintf("code-%d", f.nextToken)
	f.codes[code] = httpCode{challenge: q.Get("code_challenge"), clientID: q.Get("client_id"), redirectURI: q.Get("redirect_uri")}
	f.mu.Unlock()
	target := q.Get("redirect_uri") + "?code=" + url.QueryEscape(code) + "&state=" + url.QueryEscape(q.Get("state"))
	http.Redirect(w, r, target, http.StatusFound)
}

func (f *HTTPFake) serveToken(w http.ResponseWriter, r *http.Request, cfg HTTPConfig) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, _ := ioReadAll(r, 1<<20)
	v, err := url.ParseQuery(string(body))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	grantErr := func() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintln(w, `{"error":"invalid_grant"}`)
	}
	issue := func() (access, refresh string) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.nextToken++
		refresh = fmt.Sprintf("rt-%d", f.nextToken)
		if cfg.IssueAccess != "" {
			access = cfg.IssueAccess
		} else {
			access = fmt.Sprintf("at-%d", f.nextToken)
			f.access[access] = true
		}
		f.refresh[refresh] = true
		return access, refresh
	}
	switch v.Get("grant_type") {
	case "authorization_code":
		code := v.Get("code")
		f.mu.Lock()
		info, ok := f.codes[code]
		f.mu.Unlock()
		if !ok || info.clientID != v.Get("client_id") || info.redirectURI != v.Get("redirect_uri") {
			grantErr()
			return
		}
		sum := sha256.Sum256([]byte(v.Get("code_verifier")))
		if base64.RawURLEncoding.EncodeToString(sum[:]) != info.challenge {
			grantErr()
			return
		}
		f.mu.Lock()
		delete(f.codes, code)
		f.mu.Unlock()
		access, refresh := issue()
		writeToken(w, access, refresh, 3600)
	case "refresh_token":
		f.mu.Lock()
		valid := f.refresh[v.Get("refresh_token")]
		f.mu.Unlock()
		if !valid {
			grantErr()
			return
		}
		access, refresh := issue()
		writeToken(w, access, refresh, 3600)
	default:
		http.Error(w, "unsupported grant", http.StatusBadRequest)
	}
}

func writeToken(w http.ResponseWriter, access, refresh string, expires int) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"access_token":  access,
		"token_type":    "Bearer",
		"refresh_token": refresh,
		"expires_in":    expires,
	})
}

// ioReadAll reads to a limit.
func ioReadAll(r *http.Request, limit int64) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r.Body, limit))
}
