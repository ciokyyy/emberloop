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
// el-mcp process. Nothing starts until a tool is listed or called, the
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
	// mcpStderrTailBytes bounds the server stderr kept for error messages.
	mcpStderrTailBytes = 4096
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
// A frame carrying a Method is a server-initiated request, never a reply.
type mcpResponse struct {
	ID     *int64          `json:"id"`
	Method string          `json:"method,omitempty"`
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
//
// The mutex guards process state, never a network wait: the handshake runs
// without it, so a dying server fails the handshake through failAll instead
// of deadlocking against it. Every spawn tags a generation, so a stale
// reader or a late failure from a retired server can never touch its
// replacement.
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
	starting    chan struct{}
	generation  int64

	nextID  int64
	pendMu  sync.Mutex
	pending map[int64]chan mcpCallResult

	listed    bool
	listCache []remoteTool

	tailMu sync.Mutex
	tail   []byte
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
// is not running. Concurrent callers wait on the in-flight spawn instead of
// starting twice. A failure kills any half-started child and leaves the
// client unstarted, so the next use retries with a fresh process.
func (c *Client) ensureStarted(ctx context.Context) error {
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
		err := c.spawn(ctx)
		c.mu.Lock()
		close(c.starting)
		c.starting = nil
		c.mu.Unlock()
		return err
	}
}

// spawn starts the child, launches its readers, and runs initialize plus
// notifications/initialized. It holds no lock across the handshake, so
// failAll can retire a dying server while the handshake waits.
func (c *Client) spawn(ctx context.Context) error {
	if c.cfg.Command == "" {
		return fmt.Errorf("mcp server %q: no command configured", c.name)
	}
	c.mu.Lock()
	initTimeout := c.initTimeout
	c.mu.Unlock()
	cmd := exec.Command(c.cfg.Command, c.cfg.Args...)
	cmd.Env = os.Environ()
	for k, v := range c.cfg.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	// Own process group, killed as a group: a timeout must reach the
	// grandchildren too, the way bash does it.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("mcp server %q: failed to start %q: %w", c.name, c.cfg.Command, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("mcp server %q: failed to start %q: %w", c.name, c.cfg.Command, err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("mcp server %q: failed to start %q: %w", c.name, c.cfg.Command, err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("mcp server %q: failed to start %q: %w", c.name, c.cfg.Command, err)
	}
	c.mu.Lock()
	c.generation++
	gen := c.generation
	c.cmd, c.stdin = cmd, stdin
	c.mu.Unlock()
	go c.readLoop(stdout, gen)
	go c.drainStderr(stderr)
	initCtx, cancel := context.WithTimeout(ctx, initTimeout)
	defer cancel()
	var res struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := c.roundTrip(initCtx, "initialize", map[string]any{
		"protocolVersion": mcpProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "el", "version": "1"},
	}, &res); err != nil {
		c.mu.Lock()
		c.killLocked()
		c.generation++
		c.mu.Unlock()
		if initCtx.Err() == context.DeadlineExceeded && ctx.Err() == nil {
			return fmt.Errorf("mcp server %q: initialize timed out after %s%s", c.name, initTimeout, c.tailSuffix())
		}
		return fmt.Errorf("mcp server %q: initialize failed: %w%s", c.name, err, c.tailSuffix())
	}
	c.notify("notifications/initialized", nil)
	c.mu.Lock()
	defer c.mu.Unlock()
	// A Close or failAll may have retired this spawn mid-handshake: never
	// mark a retired server started, or use would outlive Close.
	if c.generation != gen || c.closed || c.cmd != cmd {
		return fmt.Errorf("mcp server %q: server retired during startup", c.name)
	}
	c.started = true
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
	c.mu.Lock()
	stdin := c.stdin
	c.mu.Unlock()
	if stdin == nil {
		c.dropPending(id)
		return fmt.Errorf("mcp server %q: connection closed", c.name)
	}
	c.writeMu.Lock()
	_, werr := stdin.Write(append(frame, '\n'))
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

// notify sends a notification: no id, no reply.
func (c *Client) notify(method string, params any) {
	frame, err := json.Marshal(mcpRequest{JSONRPC: "2.0", Method: method, Params: params})
	if err != nil {
		return
	}
	c.mu.Lock()
	stdin := c.stdin
	c.mu.Unlock()
	if stdin == nil {
		return
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_, _ = stdin.Write(append(frame, '\n'))
}

// dropPending removes a waiter whose caller went away; a late reply is
// dropped by the reader when it finds no waiter.
func (c *Client) dropPending(id int64) {
	c.pendMu.Lock()
	defer c.pendMu.Unlock()
	delete(c.pending, id)
}

// readLoop delivers replies to their waiters until the server's stdout ends,
// then retires the server. gen is the spawn this reader belongs to: when a
// replacement already runs, the cleanup is a no-op.
func (c *Client) readLoop(stdout io.Reader, gen int64) {
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
		if resp.Method != "" {
			// A server-initiated request is never a reply to one of ours,
			// and its id may collide with a pending call: it must not
			// reach a waiter. This client asks nothing of the server, so
			// there is nothing to answer.
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
	c.failAll(gen, fmt.Errorf("mcp server %q: server exited%s", c.name, c.tailSuffix()))
}

// drainStderr keeps the tail of the server's stderr for error messages. A
// server that crashes on startup explains itself there.
func (c *Client) drainStderr(stderr io.Reader) {
	buf := make([]byte, 512)
	for {
		n, err := stderr.Read(buf)
		if n > 0 {
			c.tailMu.Lock()
			c.tail = append(c.tail, buf[:n]...)
			if len(c.tail) > mcpStderrTailBytes {
				c.tail = append([]byte(nil), c.tail[len(c.tail)-mcpStderrTailBytes:]...)
			}
			c.tailMu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

// tailSuffix is the kept stderr, colon-prefixed for appending to an error, or
// empty when the server said nothing.
func (c *Client) tailSuffix() string {
	c.tailMu.Lock()
	defer c.tailMu.Unlock()
	if t := bytes.TrimSpace(c.tail); len(t) > 0 {
		return ": " + string(t)
	}
	return ""
}

// failAll retires one spawn: it kills the child, ends every waiter with err,
// and unstarts the client so the next use respawns. A generation from a
// retired spawn is a no-op, so a stale reader can never touch its
// replacement. The kill comes before the reap: the child may still be alive
// (a wedged server past its timeout), and waiting on a live child with an
// open pipe reader would block.
func (c *Client) failAll(gen int64, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failAllLocked(gen, err)
}

// failAllLocked is failAll with the mutex held.
func (c *Client) failAllLocked(gen int64, err error) {
	if gen != c.generation {
		return
	}
	c.killLocked()
	c.pendMu.Lock()
	waiters := c.pending
	c.pending = map[int64]chan mcpCallResult{}
	c.pendMu.Unlock()
	for _, ch := range waiters {
		ch <- mcpCallResult{err: err}
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
	gen, timeout := c.generation, c.callTimeout
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
			e := fmt.Errorf("mcp server %q: tools/list timed out after %s", c.name, timeout)
			c.failAll(gen, e)
			return nil, e
		}
		if ctx.Err() != nil {
			return nil, fmt.Errorf("mcp server %q: tools/list cancelled: %w", c.name, ctx.Err())
		}
		e := fmt.Errorf("mcp server %q: tools/list failed: %v", c.name, err)
		c.failAll(gen, fmt.Errorf("mcp server %q: connection lost: %v", c.name, err))
		return nil, e
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
	// A retire during the list (Close, or a failed concurrent call) drops
	// the answer instead of caching it onto a dead client.
	if c.generation != gen || c.closed || !c.started {
		c.mu.Unlock()
		return nil, fmt.Errorf("mcp server %q: server retired during tools/list", c.name)
	}
	c.listCache = append([]remoteTool(nil), out...)
	c.listed = true
	c.mu.Unlock()
	return out, nil
}

// CallTool calls one tool by its original server-side name. A tool that
// answers isError, a JSON-RPC error, or nothing within the call timeout is a
// Go error naming the server and the tool, never a hang. A call the caller
// abandons leaves the server running for the next use.
func (c *Client) CallTool(ctx context.Context, tool string, args json.RawMessage) (string, error) {
	if err := c.ensureStarted(ctx); err != nil {
		return "", err
	}
	c.mu.Lock()
	gen, timeout := c.generation, c.callTimeout
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
			e := fmt.Errorf("mcp server %q: tool %q timed out after %s", c.name, tool, timeout)
			c.failAll(gen, e)
			return "", e
		}
		if ctx.Err() != nil {
			// The caller went away; the server is untouched and stays
			// running. In particular nothing here waits on the child.
			return "", fmt.Errorf("mcp server %q: tool %q cancelled: %w", c.name, tool, ctx.Err())
		}
		if re, ok := err.(*mcpError); ok {
			// The server answered: it is alive, only the tool failed.
			return "", fmt.Errorf("mcp server %q: tool %q failed: %s", c.name, tool, re)
		}
		e := fmt.Errorf("mcp server %q: tool %q failed: %v", c.name, tool, err)
		c.failAll(gen, fmt.Errorf("mcp server %q: connection lost: %v", c.name, err))
		return "", e
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
// nothing to kill. The kill comes first: grandchildren may hold the stdio
// pipes, and reaping before they die would block.
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
	c.generation++
	c.failAllLocked(c.generation, fmt.Errorf("mcp server %q: client is closed", c.name))
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

// MCPServer names the configured server the tool was listed from. The
// execute stub reads it to serve the tool as tools.<server>.<name> beside
// its configured name; renames forward it through the embedded Tool.
func (t *mcpTool) MCPServer() string { return t.server }

func (t *mcpTool) Execute(ctx context.Context, _ string, args json.RawMessage) (string, error) {
	return t.client.CallTool(ctx, t.original, args)
}

// Registry holds one client per configured server for the el-mcp process.
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
