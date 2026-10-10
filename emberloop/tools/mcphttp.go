// The Streamable-HTTP MCP client: the same JSON-RPC the stdio Client
// speaks, carried over POST instead of a child process's stdin. One
// mcpHTTPClient owns one configured server URL; the Registry holds one per
// server beside the stdio clients, behind the same lazy start, cached
// tools/list, and Close. Timeouts match the stdio client
// (DefaultMCPInitTimeout, DefaultMCPCallTimeout).
//
// Session ids ride the Mcp-Session-Id header: the server issues one on
// initialize, the client sends it back, and a 404 on a known session drops
// it and re-handshakes once. Responses answer as plain JSON or as
// text/event-stream SSE; both decode into the same reply. A 401 naming an
// auth challenge runs the OAuth flow from mcpoauth.go (when the entry
// configures oauth) and retries the original call exactly once.
package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// mcpSessionHeader is the Streamable-HTTP session header.
const mcpSessionHeader = "Mcp-Session-Id"

// mcpHTTPBodyLimit bounds one HTTP reply body, the way mcpMaxFrameBytes
// bounds one stdio frame.
const mcpHTTPBodyLimit = 8 << 20

// mcpHTTPClient dials one configured MCP server over Streamable HTTP. It is
// safe for concurrent use: requests are independent POSTs, and the mutex
// guards session and cache state, never a network wait.
type mcpHTTPClient struct {
	name string
	cfg  MCPServerSetting
	url  string

	mu          sync.Mutex
	initTimeout time.Duration
	callTimeout time.Duration
	http        *http.Client
	started     bool
	closed      bool
	starting    chan struct{}
	sessionID   string

	nextID int64

	listed    bool
	listCache []remoteTool

	oauth *oauthFlows
	// oauthHook replaces the browser approval (tests). Nil means the
	// interactive default.
	oauthHook func(ctx context.Context, authURL string) (string, error)
}

// newMCPHTTPClient holds cfg without dialing anything: the session opens on the
// first list or call.
func newMCPHTTPClient(name string, cfg MCPServerSetting, initTimeout, callTimeout time.Duration) *mcpHTTPClient {
	c := &mcpHTTPClient{
		name:        name,
		cfg:         cloneMCPServerSetting(cfg),
		url:         cfg.URL,
		initTimeout: initTimeout,
		callTimeout: callTimeout,
		http:        &http.Client{},
	}
	if cfg.OAuth != nil {
		c.oauth = &oauthFlows{server: name, mcpURL: cfg.URL, setting: cloneMCPOAuth(cfg.OAuth)}
	}
	return c
}

// setTimeouts replaces the handshake and call bounds. Set it before first
// use; tests shorten the waits this way.
func (c *mcpHTTPClient) setTimeouts(init, call time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.initTimeout, c.callTimeout = init, call
}

// setOAuthHook replaces the browser approval step. Tests approve through
// their fake authorization server this way; production leaves it nil.
func (c *mcpHTTPClient) setOAuthHook(hook func(ctx context.Context, authURL string) (string, error)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.oauthHook = hook
	if c.oauth != nil {
		c.oauth.getCode = hook
	}
}

// ensureStarted runs the initialize handshake when no session is open.
// Concurrent callers wait on the in-flight handshake instead of starting
// twice. A failure leaves the client unstarted, so the next use retries.
func (c *mcpHTTPClient) ensureStarted(ctx context.Context) error {
	for {
		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			return fmt.Errorf("mcp server %q: client is closed", c.name)
		}
		if c.started {
			c.mu.Unlock()
			return nil
		}
		if c.starting != nil {
			wait := c.starting
			c.mu.Unlock()
			select {
			case <-wait:
				continue
			case <-ctx.Done():
				return fmt.Errorf("mcp server %q: waiting for server startup: %w", c.name, ctx.Err())
			}
		}
		c.starting = make(chan struct{})
		c.mu.Unlock()
		err := c.handshake(ctx)
		c.mu.Lock()
		close(c.starting)
		c.starting = nil
		c.mu.Unlock()
		return err
	}
}

// handshake opens the Streamable-HTTP session: initialize, then the
// initialized notification. It holds no lock across the network, so Close
// can retire the client mid-handshake.
func (c *mcpHTTPClient) handshake(ctx context.Context) error {
	c.mu.Lock()
	initTimeout := c.initTimeout
	c.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, initTimeout)
	defer cancel()
	var res struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := c.roundTrip(ctx, "initialize", map[string]any{
		"protocolVersion": mcpProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "el", "version": "1"},
	}, &res); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("mcp server %q: initialize timed out after %s", c.name, initTimeout)
		}
		return fmt.Errorf("mcp server %q: initialize failed: %w", c.name, err)
	}
	// The initialized notification is best effort: a server that dislikes it
	// still answers tools/list, where the failure would surface anyway.
	_ = c.postNotification(ctx, "notifications/initialized", nil)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return fmt.Errorf("mcp server %q: server retired during startup", c.name)
	}
	c.started = true
	return nil
}

// httpResult is one POST's outcome.
type httpResult struct {
	status    int
	challenge string
	session   string
	body      []byte
	isSSE     bool
}

// post sends one JSON-RPC frame and returns the raw outcome. A nil id is a
// notification: the reply is read and dropped. The bearer, when set, rides
// the Authorization header beside the configured static headers.
func (c *mcpHTTPClient) post(ctx context.Context, id *int64, method string, params any, bearer string) (*httpResult, error) {
	frame, err := json.Marshal(mcpRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(frame))
	if err != nil {
		return nil, fmt.Errorf("mcp server %q: bad server url %q: %w", c.name, c.url, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	c.mu.Lock()
	for k, v := range c.cfg.Headers {
		req.Header.Set(k, v)
	}
	if session := c.sessionID; session != "" {
		req.Header.Set(mcpSessionHeader, session)
	}
	c.mu.Unlock()
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mcp server %q: %s failed: %w", c.name, method, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, mcpHTTPBodyLimit+1))
	if err != nil {
		return nil, fmt.Errorf("mcp server %q: reading %s reply: %w", c.name, method, err)
	}
	if int64(len(body)) > mcpHTTPBodyLimit {
		return nil, fmt.Errorf("mcp server %q: %s reply exceeds %d bytes", c.name, method, mcpHTTPBodyLimit)
	}
	out := &httpResult{
		status:    resp.StatusCode,
		challenge: resp.Header.Get("WWW-Authenticate"),
		session:   resp.Header.Get(mcpSessionHeader),
		body:      body,
		isSSE:     strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream"),
	}
	if out.session != "" {
		c.mu.Lock()
		c.sessionID = out.session
		c.mu.Unlock()
	}
	return out, nil
}

// decodeResult decodes one outcome's body into out. A reply carrying a
// JSON-RPC error is an *mcpError, the way the stdio client answers one.
func (c *mcpHTTPClient) decodeResult(res *httpResult, id int64, method string, out any) error {
	if res.status == http.StatusUnauthorized {
		return &httpAuthError{challenge: res.challenge}
	}
	if res.status == http.StatusNotFound || res.status == http.StatusBadRequest {
		return &httpSessionError{status: res.status}
	}
	if res.status == http.StatusAccepted {
		// A notification's 202 carries no body.
		return nil
	}
	if res.status < 200 || res.status >= 300 {
		return fmt.Errorf("mcp server %q: %s failed: status %s: %s", c.name, method, http.StatusText(res.status), truncateHTTPBody(res.body))
	}
	raw := res.body
	if res.isSSE {
		var err error
		raw, err = sseResult(res.body, id)
		if err != nil {
			return fmt.Errorf("mcp server %q: bad %s reply: %w", c.name, method, err)
		}
	}
	var resp mcpResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return fmt.Errorf("mcp server %q: bad %s reply: %w", c.name, method, err)
	}
	if resp.Error != nil {
		return resp.Error
	}
	if out != nil && len(resp.Result) > 0 {
		if err := json.Unmarshal(resp.Result, out); err != nil {
			return fmt.Errorf("mcp server %q: bad %s reply: %w", c.name, method, err)
		}
	}
	return nil
}

// truncateHTTPBody keeps an error's body tail short: a status already says
// most of it.
func truncateHTTPBody(body []byte) string {
	body = bytes.TrimSpace(body)
	if len(body) > 200 {
		body = body[:200]
	}
	if len(body) == 0 {
		return "(empty body)"
	}
	return string(body)
}

// httpAuthError is a 401 carrying the server's auth challenge.
type httpAuthError struct {
	challenge string
}

func (e *httpAuthError) Error() string { return "unauthorized" }

// httpSessionError is a 404 or 400 on a request carrying a session id: the
// server dropped the session.
type httpSessionError struct {
	status int
}

func (e *httpSessionError) Error() string { return fmt.Sprintf("session gone (status %d)", e.status) }

// sseResult pulls the JSON-RPC reply for id out of a text/event-stream
// body: events split on blank lines, payloads ride data: lines, and the
// event whose payload answers id wins.
func sseResult(body []byte, id int64) (json.RawMessage, error) {
	text := strings.ReplaceAll(string(body), "\r\n", "\n")
	for _, event := range strings.Split(text, "\n\n") {
		var data strings.Builder
		for _, line := range strings.Split(event, "\n") {
			line = strings.TrimRight(line, "\r")
			if rest, ok := strings.CutPrefix(line, "data:"); ok {
				rest = strings.TrimPrefix(rest, " ")
				data.WriteString(rest)
				data.WriteString("\n")
			}
		}
		raw := bytes.TrimSpace([]byte(data.String()))
		if len(raw) == 0 {
			continue
		}
		var resp mcpResponse
		if err := json.Unmarshal(raw, &resp); err != nil || resp.ID == nil || *resp.ID != id {
			continue
		}
		return raw, nil
	}
	return nil, fmt.Errorf("no reply for id %d in SSE stream", id)
}

// roundTrip sends one request and decodes its reply into out, handling one
// dropped session (re-handshake and retry) and one auth challenge (OAuth
// flow and retry) per call. Anything further is an error, never a loop.
func (c *mcpHTTPClient) roundTrip(ctx context.Context, method string, params any, out any) error {
	id := atomic.AddInt64(&c.nextID, 1)
	bearer := c.bearerForRequest(ctx)
	res, err := c.post(ctx, &id, method, params, bearer)
	if err != nil {
		return err
	}
	if err := c.decodeResult(res, id, method, out); err == nil {
		return nil
	} else if sess, ok := err.(*httpSessionError); ok && sess != nil && c.hadSession() {
		// The server forgot the session (it restarted): open a fresh one
		// and send the call again, once.
		c.mu.Lock()
		c.sessionID, c.started, c.listed, c.listCache = "", false, false, nil
		c.mu.Unlock()
		if herr := c.handshake(ctx); herr != nil {
			return herr
		}
		id = atomic.AddInt64(&c.nextID, 1)
		res, err = c.post(ctx, &id, method, params, c.bearerForRequest(ctx))
		if err != nil {
			return err
		}
		if derr := c.decodeResult(res, id, method, out); derr != nil {
			return c.maybeAuthRetry(ctx, derr, res, method, params, out)
		}
		return nil
	} else {
		return c.maybeAuthRetry(ctx, err, res, method, params, out)
	}
}

// maybeAuthRetry runs the OAuth flow on a 401 and retries the call exactly
// once with the fresh token. Without oauth configured the 401 stands as a
// plain error; a retry that still 401s is reauth-needed.
func (c *mcpHTTPClient) maybeAuthRetry(ctx context.Context, err error, res *httpResult, method string, params any, out any) error {
	auth, ok := err.(*httpAuthError)
	if !ok {
		return err
	}
	if c.oauth == nil {
		return fmt.Errorf("mcp server %q: %s failed: unauthorized (status 401); configure headers or oauth for this server", c.name, method)
	}
	c.mu.Lock()
	hook := c.oauthHook
	c.mu.Unlock()
	flows := &oauthFlows{server: c.oauth.server, mcpURL: c.oauth.mcpURL, setting: c.oauth.setting, getCode: hook}
	bearer, ferr := flows.bearerFor401(ctx, auth.challenge)
	if ferr != nil {
		return fmt.Errorf("mcp server %q: %s failed: %w", c.name, method, ferr)
	}
	id := atomic.AddInt64(&c.nextID, 1)
	res, err = c.post(ctx, &id, method, params, bearer)
	if err != nil {
		return err
	}
	if err := c.decodeResult(res, id, method, out); err != nil {
		if _, ok := err.(*httpAuthError); ok {
			return &ReauthNeededError{Server: c.name, TokenFile: mcpTokenPath(c.name)}
		}
		return err
	}
	return nil
}

// bearerForRequest returns the stored bearer for a first attempt: the
// unexpired stored token, if any, discovered without approving. Empty means
// go without one; a 401 then runs the flow.
func (c *mcpHTTPClient) bearerForRequest(ctx context.Context) string {
	if c.oauth == nil {
		return ""
	}
	t, err := loadOAuthToken(c.name)
	if err != nil || t == nil || t.AccessToken == "" || t.expired() {
		return ""
	}
	return t.AccessToken
}

// hadSession reports whether a session id was in force: only then is a
// 404 a dropped session rather than a wrong path.
func (c *mcpHTTPClient) hadSession() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessionID != ""
}

// postNotification sends a notification and drops the reply.
func (c *mcpHTTPClient) postNotification(ctx context.Context, method string, params any) error {
	res, err := c.post(ctx, nil, method, params, c.bearerForRequest(ctx))
	if err != nil {
		return err
	}
	if res.status == http.StatusUnauthorized || res.status == http.StatusAccepted {
		return nil
	}
	if res.status < 200 || res.status >= 300 {
		return fmt.Errorf("mcp server %q: %s failed: status %s", c.name, method, http.StatusText(res.status))
	}
	return nil
}

// ListTools returns the server's tools, opening the session on first use.
// The answer is cached for the session, the way the stdio client caches.
func (c *mcpHTTPClient) ListTools(ctx context.Context) ([]remoteTool, error) {
	if err := c.ensureStarted(ctx); err != nil {
		return nil, err
	}
	c.mu.Lock()
	if c.listed {
		cached := append([]remoteTool(nil), c.listCache...)
		c.mu.Unlock()
		return cached, nil
	}
	timeout := c.callTimeout
	c.mu.Unlock()
	listCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var res struct {
		Tools []struct {
			Name        string         `json:"name"`
			Description string         `json:"description"`
			InputSchema map[string]any `json:"inputSchema"`
			Annotations *struct {
				ReadOnlyHint bool `json:"readOnlyHint"`
			} `json:"annotations"`
		} `json:"tools"`
	}
	if err := c.roundTrip(listCtx, "tools/list", map[string]any{}, &res); err != nil {
		if listCtx.Err() == context.DeadlineExceeded && ctx.Err() == nil {
			return nil, fmt.Errorf("mcp server %q: tools/list timed out after %s", c.name, timeout)
		}
		if ctx.Err() != nil {
			return nil, fmt.Errorf("mcp server %q: tools/list cancelled: %w", c.name, ctx.Err())
		}
		return nil, fmt.Errorf("mcp server %q: tools/list failed: %w", c.name, err)
	}
	out := make([]remoteTool, 0, len(res.Tools))
	for _, tl := range res.Tools {
		if tl.Name == "" {
			continue
		}
		rt := remoteTool{name: tl.Name, description: tl.Description, schema: tl.InputSchema}
		if tl.Annotations != nil {
			rt.readOnly = tl.Annotations.ReadOnlyHint
		}
		if rt.schema == nil {
			rt.schema = map[string]any{"type": "object"}
		}
		out = append(out, rt)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || !c.started {
		return nil, fmt.Errorf("mcp server %q: server retired during tools/list", c.name)
	}
	c.listCache = append([]remoteTool(nil), out...)
	c.listed = true
	return out, nil
}

// CallTool calls one tool by its original server-side name. A tool that
// answers isError, a JSON-RPC error, or nothing within the call timeout is a
// Go error naming the server and the tool, never a hang. A call the caller
// abandons leaves the session running for the next use.
func (c *mcpHTTPClient) CallTool(ctx context.Context, tool string, args json.RawMessage) (string, error) {
	if err := c.ensureStarted(ctx); err != nil {
		return "", err
	}
	c.mu.Lock()
	timeout := c.callTimeout
	c.mu.Unlock()
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	arguments := map[string]any{}
	if len(bytes.TrimSpace(args)) > 0 {
		var decoded map[string]any
		if err := json.Unmarshal(args, &decoded); err != nil {
			return "", fmt.Errorf("mcp server %q: tool %q: invalid arguments: %w", c.name, tool, err)
		}
		arguments = decoded
	}
	var res struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := c.roundTrip(callCtx, "tools/call", map[string]any{
		"name": tool, "arguments": arguments,
	}, &res); err != nil {
		if callCtx.Err() == context.DeadlineExceeded && ctx.Err() == nil {
			return "", fmt.Errorf("mcp server %q: tool %q timed out after %s", c.name, tool, timeout)
		}
		if ctx.Err() != nil {
			return "", fmt.Errorf("mcp server %q: tool %q cancelled: %w", c.name, tool, ctx.Err())
		}
		if re, ok := err.(*mcpError); ok {
			// The server answered: it is alive, only the tool failed.
			return "", fmt.Errorf("mcp server %q: tool %q failed: %s", c.name, tool, re)
		}
		return "", fmt.Errorf("mcp server %q: tool %q failed: %w", c.name, tool, err)
	}
	var text string
	for _, part := range res.Content {
		if part.Type == "" || part.Type == "text" {
			text += part.Text
		}
	}
	if res.IsError {
		return "", fmt.Errorf("mcp server %q: tool %q failed: %s", c.name, tool, text)
	}
	return text, nil
}

// Close drops the session: a best-effort DELETE ends the server-side
// session, then calls fail instead of redialing. A registry is done after
// Close.
func (c *mcpHTTPClient) Close() {
	c.mu.Lock()
	session, url := c.sessionID, c.url
	c.closed = true
	c.started, c.listed = false, false
	c.listCache = nil
	c.sessionID = ""
	c.mu.Unlock()
	if session == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return
	}
	req.Header.Set(mcpSessionHeader, session)
	if tok := c.bearerForRequest(ctx); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	if resp, err := c.http.Do(req); err == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
	}
}
