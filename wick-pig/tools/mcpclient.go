package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"sort"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// The MCP client speaks the same newline-delimited JSON-RPC the hand-written
// server in mcp/ does, with no third-party library. A Client owns one server
// child process; a Registry holds one client per configured server for the
// wick-mcp process. Nothing starts until a tool is listed or called, the
// tools/list answer is cached for the session, and Close kills every child.
const (
	mcpProtocolVersion = "2024-11-05"
	mcpMaxFrameBytes   = 8 << 20
	// DefaultMCPInitTimeout bounds starting a server and finishing its
	// initialize and tools/list handshake. It allows a cold npx download
	// while keeping a dead server from hanging the session.
	DefaultMCPInitTimeout = 30 * time.Second
	// DefaultMCPCallTimeout bounds one tool call, matching bash's default.
	DefaultMCPCallTimeout = 120 * time.Second
)

// mcpRequest is one JSON-RPC frame. A nil ID is a notification and is never
// answered.
type mcpRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      *int64 `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// mcpResponse is the matching reply. Exactly one of Result and Error is set.
type mcpResponse struct {
	ID     *int64          `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *mcpError       `json:"error,omitempty"`
}

// mcpError is a JSON-RPC error object.
type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *mcpError) Error() string { return fmt.Sprintf("code %d: %s", e.Code, e.Message) }

// mcpCallResult is a delivered reply: either the parsed frame or the reason
// the wait ended.
type mcpCallResult struct {
	resp mcpResponse
	err  error
}

// remoteTool is one entry of a tools/list answer.
type remoteTool struct {
	name        string
	description string
	schema      map[string]any
	readOnly    bool
}

// Client runs one configured MCP server as a child process and calls its
// tools. It is safe for concurrent use: calls take ids from an atomic
// counter and a reader goroutine delivers replies by id.
type Client struct {
	name string
	cfg  MCPServerSetting

	mu          sync.Mutex
	initTimeout time.Duration
	callTimeout time.Duration
	cmd         *exec.Cmd
	stdin       io.WriteCloser
	writeMu     sync.Mutex
	started     bool
	closed      bool

	nextID  int64
	pendMu  sync.Mutex
	pending map[int64]chan mcpCallResult

	listed    bool
	listCache []remoteTool
}

// newClient holds cfg without starting anything: the process spawns on the
// first list or call.
func newClient(name string, cfg MCPServerSetting, initTimeout, callTimeout time.Duration) *Client {
	return &Client{
		name:        name,
		cfg:         cloneMCPServerSetting(cfg),
		initTimeout: initTimeout,
		callTimeout: callTimeout,
		pending:     map[int64]chan mcpCallResult{},
	}
}

// setTimeouts replaces the handshake and call bounds. Set it before first
// use; tests shorten the waits this way.
func (c *Client) setTimeouts(init, call time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.initTimeout, c.callTimeout = init, call
}

// ensureStarted spawns the server and runs the initialize handshake when it
// is not running. A failure kills any half-started child and leaves the
// client unstarted, so the next use retries with a fresh process.
func (c *Client) ensureStarted(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return fmt.Errorf("mcp server %q: client is closed", c.name)
	}
	if c.started {
		return nil
	}
	if err := c.startLocked(ctx); err != nil {
		c.killLocked()
		return err
	}
	c.started = true
	return nil
}

// startLocked spawns the child, starts the reply reader, and runs initialize
// plus notifications/initialized. The caller kills the child on error.
func (c *Client) startLocked(ctx context.Context) error {
	if c.cfg.Command == "" {
		return fmt.Errorf("mcp server %q: no command configured", c.name)
	}
	cmd := exec.Command(c.cfg.Command, c.cfg.Args...)
	cmd.Env = os.Environ()
	for k, v := range c.cfg.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	// Own process group, killed as a group: a timeout must reach the
	// grandchildren too, the way bash does it.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stderr = nil
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("mcp server %q: failed to start %q: %w", c.name, c.cfg.Command, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("mcp server %q: failed to start %q: %w", c.name, c.cfg.Command, err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("mcp server %q: failed to start %q: %w", c.name, c.cfg.Command, err)
	}
	c.cmd, c.stdin = cmd, stdin
	go c.readLoop(stdout)
	initCtx, cancel := context.WithTimeout(ctx, c.initTimeout)
	defer cancel()
	var res struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := c.roundTrip(initCtx, "initialize", map[string]any{
		"protocolVersion": mcpProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "wick", "version": "1"},
	}, &res); err != nil {
		if initCtx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("mcp server %q: initialize timed out after %s", c.name, c.initTimeout)
		}
		return fmt.Errorf("mcp server %q: initialize failed: %w", c.name, err)
	}
	c.notifyLocked("notifications/initialized", nil)
	return nil
}

// roundTrip sends one request and decodes its reply into out. out may be nil
// to skip the body. A reply carrying a JSON-RPC error is an error.
func (c *Client) roundTrip(ctx context.Context, method string, params any, out any) error {
	id := atomic.AddInt64(&c.nextID, 1)
	frame, err := json.Marshal(mcpRequest{JSONRPC: "2.0", ID: &id, Method: method, Params: params})
	if err != nil {
		return err
	}
	ch := make(chan mcpCallResult, 1)
	c.pendMu.Lock()
	c.pending[id] = ch
	c.pendMu.Unlock()
	c.writeMu.Lock()
	_, werr := c.stdin.Write(append(frame, '\n'))
	c.writeMu.Unlock()
	if werr != nil {
		c.dropPending(id)
		return werr
	}
	select {
	case r := <-ch:
		if r.err != nil {
			return r.err
		}
		if r.resp.Error != nil {
			return r.resp.Error
		}
		if out != nil && len(r.resp.Result) > 0 {
			if err := json.Unmarshal(r.resp.Result, out); err != nil {
				return fmt.Errorf("bad %s reply: %w", method, err)
			}
		}
		return nil
	case <-ctx.Done():
		c.dropPending(id)
		return ctx.Err()
	}
}

// notifyLocked sends a notification: no id, no reply.
func (c *Client) notifyLocked(method string, params any) {
	frame, err := json.Marshal(mcpRequest{JSONRPC: "2.0", Method: method, Params: params})
	if err != nil {
		return
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_, _ = c.stdin.Write(append(frame, '\n'))
}

// dropPending removes a waiter whose caller went away; a late reply is
// dropped by the reader when it finds no waiter.
func (c *Client) dropPending(id int64) {
	c.pendMu.Lock()
	defer c.pendMu.Unlock()
	delete(c.pending, id)
}

// readLoop delivers replies to their waiters until the server's stdout ends,
// then fails every waiter with an error naming the server. It also marks the
// client unstarted, so the next use spawns a fresh server.
func (c *Client) readLoop(stdout io.Reader) {
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), mcpMaxFrameBytes)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var resp mcpResponse
		if err := json.Unmarshal(line, &resp); err != nil || resp.ID == nil {
			continue
		}
		c.pendMu.Lock()
		ch := c.pending[*resp.ID]
		delete(c.pending, *resp.ID)
		c.pendMu.Unlock()
		if ch != nil {
			ch <- mcpCallResult{resp: resp}
		}
	}
	c.failAll(fmt.Errorf("mcp server %q: server exited", c.name))
}

// failAll ends every waiter with err and marks the client unstarted.
func (c *Client) failAll(err error) {
	c.pendMu.Lock()
	waiters := c.pending
	c.pending = map[int64]chan mcpCallResult{}
	c.pendMu.Unlock()
	for _, ch := range waiters {
		ch <- mcpCallResult{err: err}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cmd != nil {
		// The child is gone but not reaped: Wait for it so no zombie
		// outlives the session.
		_ = c.cmd.Wait()
		c.cmd, c.stdin = nil, nil
	}
	c.started, c.listed = false, false
	c.listCache = nil
}

// ListTools returns the server's tools, starting it on first use. The answer
// is cached for the session.
func (c *Client) ListTools(ctx context.Context) ([]remoteTool, error) {
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
		if listCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("mcp server %q: tools/list timed out after %s", c.name, timeout)
		}
		// The server is gone mid-list: unstart so the next use respawns.
		c.failAll(fmt.Errorf("mcp server %q: tools/list failed: %w", c.name, err))
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
	c.listCache = append([]remoteTool(nil), out...)
	c.listed = true
	c.mu.Unlock()
	return out, nil
}

// CallTool calls one tool by its original server-side name. A tool that
// answers isError, a JSON-RPC error, or nothing within the call timeout is a
// Go error naming the server and the tool, never a hang.
func (c *Client) CallTool(ctx context.Context, tool string, args json.RawMessage) (string, error) {
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
		if callCtx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("mcp server %q: tool %q timed out after %s", c.name, tool, timeout)
		}
		if re, ok := err.(*mcpError); ok {
			return "", fmt.Errorf("mcp server %q: tool %q failed: %s", c.name, tool, re)
		}
		// The server is gone mid-call: unstart so the next use respawns.
		c.failAll(fmt.Errorf("mcp server %q: tool %q failed: %w", c.name, tool, err))
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

// killLocked kills the child's process group and reaps it. A nil command is
// nothing to kill.
func (c *Client) killLocked() {
	if c.cmd == nil || c.cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-c.cmd.Process.Pid, syscall.SIGKILL)
	_ = c.cmd.Wait()
	c.cmd, c.stdin = nil, nil
}

// Close kills the server's process group and drops the session. Calls after
// Close fail instead of respawning.
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	c.failAllLocked(fmt.Errorf("mcp server %q: client is closed", c.name))
	c.killLocked()
	c.started, c.listed = false, false
	c.listCache = nil
}

// failAllLocked ends every waiter with err. The caller holds c.mu.
func (c *Client) failAllLocked(err error) {
	c.pendMu.Lock()
	waiters := c.pending
	c.pending = map[int64]chan mcpCallResult{}
	c.pendMu.Unlock()
	for _, ch := range waiters {
		ch <- mcpCallResult{err: err}
	}
}

// mcpTool serves one remote tool as a tools.Tool under its configured name.
// OriginalName is the server-side name the call goes out with, so the
// resolution can match include and exclude globs over the original even after
// a rename.
type mcpTool struct {
	client      *Client
	server      string
	original    string
	name        string
	description string
	schema      map[string]any
	readOnly    bool
}

func (t *mcpTool) Name() string { return t.name }

// OriginalName is the tool's name on its server, before the rename.
func (t *mcpTool) OriginalName() string { return t.original }

func (t *mcpTool) Description() string {
	if t.description == "" {
		return fmt.Sprintf("MCP tool %q from server %q.", t.original, t.server)
	}
	return t.description
}

func (t *mcpTool) Schema() map[string]any {
	if t.schema == nil {
		return map[string]any{"type": "object"}
	}
	return t.schema
}

func (t *mcpTool) ReadOnly() bool { return t.readOnly }

func (t *mcpTool) Execute(ctx context.Context, _ string, args json.RawMessage) (string, error) {
	return t.client.CallTool(ctx, t.original, args)
}

// Registry holds one client per configured server for the wick-mcp process.
// Servers are never shared across sessions: build one registry per process
// and Close it when the process ends.
type Registry struct {
	mu          sync.Mutex
	clients     map[string]*Client
	initTimeout time.Duration
	callTimeout time.Duration
}

// NewRegistry holds the configured servers without starting any of them. A
// nil map is an empty registry.
func NewRegistry(cfgs map[string]MCPServerSetting) *Registry {
	r := &Registry{
		clients:     map[string]*Client{},
		initTimeout: DefaultMCPInitTimeout,
		callTimeout: DefaultMCPCallTimeout,
	}
	for name, cfg := range cfgs {
		r.clients[name] = newClient(name, cfg, r.initTimeout, r.callTimeout)
	}
	return r
}

// SetTimeouts replaces the initialize and call bounds on the registry and
// every client it holds. Call it before first use.
func (r *Registry) SetTimeouts(init, call time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.initTimeout, r.callTimeout = init, call
	for _, c := range r.clients {
		c.setTimeouts(init, call)
	}
}

// Servers names every configured server, sorted.
func (r *Registry) Servers() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.clients))
	for name := range r.clients {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Tools lists one server's tools as tools.Tool values, starting the server
// on first use. The server's include, exclude and rename shape the set over
// the tools' original names; every error names the server.
func (r *Registry) Tools(ctx context.Context, server string) ([]Tool, error) {
	r.mu.Lock()
	c, ok := r.clients[server]
	r.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("unknown mcp server %q", server)
	}
	remote, err := c.ListTools(ctx)
	if err != nil {
		return nil, err
	}
	return shapeTools(c.cfg, c, remote), nil
}

// Snapshot lists every configured server, in sorted order. A server that
// fails contributes an error naming it and no tools; the rest still serve.
func (r *Registry) Snapshot(ctx context.Context) (map[string][]Tool, []error) {
	listed := map[string][]Tool{}
	var errs []error
	for _, server := range r.Servers() {
		ts, err := r.Tools(ctx, server)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		listed[server] = ts
	}
	return listed, errs
}

// Close kills every server child. A registry is done after Close.
func (r *Registry) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.clients {
		c.Close()
	}
}

// shapeTools filters a listing by include and exclude globs over each tool's
// original name and serves survivors under their renamed names.
func shapeTools(cfg MCPServerSetting, c *Client, remote []remoteTool) []Tool {
	var out []Tool
	for _, rt := range remote {
		if len(cfg.Include) > 0 && !matchMCPGlobs(cfg.Include, rt.name) {
			continue
		}
		if matchMCPGlobs(cfg.Exclude, rt.name) {
			continue
		}
		name := rt.name
		if renamed, ok := cfg.Rename[rt.name]; ok && renamed != "" {
			name = renamed
		}
		out = append(out, &mcpTool{
			client: c, server: c.name, original: rt.name, name: name,
			description: rt.description, schema: rt.schema, readOnly: rt.readOnly,
		})
	}
	return out
}

// matchMCPGlobs reports whether any glob matches name. An invalid glob never
// matches: it is a settings typo, not a pattern.
func matchMCPGlobs(patterns []string, name string) bool {
	for _, p := range patterns {
		if ok, err := path.Match(p, name); err == nil && ok {
			return true
		}
	}
	return false
}
