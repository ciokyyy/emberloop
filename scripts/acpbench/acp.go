// Package main is acpbench: a black-box ACP client probe/benchmark.
//
// It speaks ACP (agentclientprotocol.com, JSON-RPC over stdio,
// newline-delimited) to any agent command, mirroring the client behavior of
// the emberloop-web Daemon (see emberloop-web/claude.go acpProcessConn):
// initialize with fs read/write caps and no terminal, session/new+load with
// an MCP server list and a _meta judgment, auto-approve permission answers,
// unsupported answers to agent-initiated fs/terminal requests, idle-bound
// call timeouts, and session/cancel as a notification.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Timeouts. Control calls answer in seconds; prompts run agent turns. Each
// is overridable via ACPBENCH_<NAME> (a Go duration) for fast self-tests,
// and --init-timeout / --turn-timeout flags override the env for the two
// most common knobs.
var (
	initTimeout     = 30 * time.Second
	ctlTimeout      = 30 * time.Second
	promptTimeout   = 90 * time.Second
	cancelWait      = 10 * time.Second
	steerWait       = 15 * time.Second
	crashDetectWait = 10 * time.Second
	mcpReadyWait    = 5 * time.Second
	raceWait        = 1200 * time.Millisecond
	settleWait      = 600 * time.Millisecond
)

const (
	maxACPLine       = 64 * 1024 * 1024
	version          = "0.2.0"
	schemaVersion    = 2
	protocolVersion  = 1
	fakeLLMTokenName = "{{FAKE_LLM_URL}}"
)

// applyTimeoutEnv applies ACPBENCH_* duration overrides.
func applyTimeoutEnv() {
	for env, dst := range map[string]*time.Duration{
		"ACPBENCH_INIT_TIMEOUT":   &initTimeout,
		"ACPBENCH_CTL_TIMEOUT":    &ctlTimeout,
		"ACPBENCH_PROMPT_TIMEOUT": &promptTimeout,
		"ACPBENCH_CANCEL_WAIT":    &cancelWait,
		"ACPBENCH_STEER_WAIT":     &steerWait,
		"ACPBENCH_CRASH_WAIT":     &crashDetectWait,
		"ACPBENCH_MCP_WAIT":       &mcpReadyWait,
		"ACPBENCH_RACE_WAIT":      &raceWait,
		"ACPBENCH_SETTLE_WAIT":    &settleWait,
	} {
		if v := os.Getenv(env); v != "" {
			if d, err := time.ParseDuration(v); err == nil && d > 0 {
				*dst = d
			}
		}
	}
}

// Verdicts. Every scenario ends in exactly one.
const (
	VerdictOK              = "ok"
	VerdictTimeoutHang     = "timeout-hang"
	VerdictProtocolError   = "protocol-error"
	VerdictCrash           = "crash"
	VerdictSilentEmptyTurn = "silent-empty-turn"
	VerdictWrongResult     = "wrong-result"
	VerdictSkipped         = "skipped"
)

// Update is one decoded session/update payload.
type Update struct {
	SessionID string
	Kind      string // update type, e.g. agent_message_chunk, tool_call, tool_call_update
	Raw       map[string]any
	At        time.Time
}

// PermissionAsk is one session/request_permission round trip.
type PermissionAsk struct {
	ToolCall map[string]any
	Options  []any
}

// PermissionMode selects how the client answers permission requests.
type PermissionMode int

const (
	// PermAutoApprove mirrors the Daemon: pick the allow-once option.
	PermAutoApprove PermissionMode = iota
	// PermDeny answers every request with outcome cancelled.
	PermDeny
)

// acpConn is one spawned agent process speaking ACP over stdio.
type acpConn struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	pid   int

	mu   sync.Mutex
	wmu  sync.Mutex
	next int64
	wait map[string]chan acpResult

	onUpdate func(Update)
	permMode PermissionMode
	onPerm   func(toolName, optionID string)

	readDone chan struct{}
	closed   atomic.Bool
	closeErr atomic.Value // error
	lastRx   atomic.Int64 // unix-nano of last adapter message

	steering bool
	mcpReady chan struct{}
	mcpOnce  sync.Once

	rawInit json.RawMessage
}

type acpResult struct {
	result json.RawMessage
	err    error
}

type acpWireMsg struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  map[string]any  `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// killGroup kills the whole process group on teardown (qwen-code leaves
// orphaned grandchildren otherwise). Set from --kill-group.
var killGroup = false

// spawnACP starts argv as an ACP agent with env (full environment) in cwd
// ("" keeps the current directory).
func spawnACP(argv []string, env []string, cwd string) (*acpConn, error) {
	if len(argv) == 0 {
		return nil, fmt.Errorf("empty agent command")
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = env
	if cwd != "" {
		cmd.Dir = cwd
	}
	if killGroup {
		setProcessGroup(cmd)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	c := &acpConn{
		cmd: cmd, stdin: stdin, pid: cmd.Process.Pid,
		wait:     map[string]chan acpResult{},
		readDone: make(chan struct{}),
		mcpReady: make(chan struct{}),
	}
	go c.read(bufio.NewScanner(stdout))
	return c, nil
}

// exited reports whether the agent process is gone.
func (c *acpConn) exited() bool { return c.closed.Load() }

func (c *acpConn) read(sc *bufio.Scanner) {
	defer close(c.readDone)
	defer c.closed.Store(true)
	sc.Buffer(make([]byte, 64*1024), maxACPLine)
	for sc.Scan() {
		line := append([]byte(nil), sc.Bytes()...)
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		c.lastRx.Store(time.Now().UnixNano())
		var msg acpWireMsg
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		c.dispatch(msg)
	}
	err := fmt.Errorf("adapter closed")
	if sc.Err() != nil {
		err = fmt.Errorf("adapter closed: read agent output: %w", sc.Err())
	} else if c.cmd.ProcessState != nil {
		err = fmt.Errorf("adapter closed: %v", c.cmd.ProcessState.String())
	}
	c.closeErr.Store(err)
	c.mu.Lock()
	orphans := make([]chan acpResult, 0, len(c.wait))
	for id, ch := range c.wait {
		delete(c.wait, id)
		orphans = append(orphans, ch)
	}
	c.mu.Unlock()
	for _, ch := range orphans {
		select {
		case ch <- acpResult{err: err}:
		default:
		}
	}
}

func idKey(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	return string(raw)
}

func (c *acpConn) dispatch(msg acpWireMsg) {
	key := idKey(msg.ID)
	if key != "" && (msg.Result != nil || msg.Error != nil) {
		c.mu.Lock()
		ch := c.wait[key]
		delete(c.wait, key)
		c.mu.Unlock()
		if ch != nil {
			if msg.Error != nil {
				ch <- acpResult{err: fmt.Errorf("acp: %s", msg.Error.Message)}
			} else {
				ch <- acpResult{result: msg.Result}
			}
		}
		return
	}
	if key != "" && msg.Method == "session/request_permission" {
		c.handlePermission(msg)
		return
	}
	if msg.Method == "_x.ai/mcp_initialized" {
		c.mcpOnce.Do(func() { close(c.mcpReady) })
		return
	}
	if msg.Method == "session/update" {
		if c.onUpdate != nil && msg.Params != nil {
			u, _ := msg.Params["update"].(map[string]any)
			kind, _ := u["sessionUpdate"].(string)
			if kind == "" {
				kind, _ = u["type"].(string)
			}
			sid, _ := msg.Params["sessionId"].(string)
			if sid == "" {
				sid, _ = u["sessionId"].(string)
			}
			c.onUpdate(Update{SessionID: sid, Kind: kind, Raw: u, At: time.Now()})
		}
		return
	}
	if key != "" && msg.Method != "" {
		// Agent-initiated client requests (fs reads/writes, terminals):
		// the stripped session never needs them, so answer unsupported,
		// exactly like the Daemon.
		_ = c.writeRaw(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(msg.ID),
			"error": map[string]any{"code": -32601, "message": "unsupported"}})
	}
}

// handlePermission answers one permission request per the configured mode.
func (c *acpConn) handlePermission(msg acpWireMsg) {
	var toolName string
	if tc, ok := msg.Params["toolCall"].(map[string]any); ok {
		toolName, _ = tc["name"].(string)
		if toolName == "" {
			toolName, _ = tc["title"].(string)
		}
	}
	var options []any
	if raw, ok := msg.Params["options"]; ok {
		options, _ = raw.([]any)
	}
	if c.permMode == PermDeny {
		_ = c.writeRaw(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(msg.ID),
			"result": map[string]any{"outcome": "cancelled"}})
		if c.onPerm != nil {
			c.onPerm(toolName, "denied")
		}
		return
	}
	optionID := pickAllowOption(options)
	if optionID == "" {
		_ = c.writeRaw(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(msg.ID),
			"result": map[string]any{"outcome": "cancelled"}})
		return
	}
	_ = c.writeRaw(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(msg.ID),
		"result": map[string]any{"outcome": "selected", "optionId": optionID}})
	if c.onPerm != nil {
		c.onPerm(toolName, optionID)
	}
}

// pickAllowOption mirrors the Daemon: allow-once first, then any allow.
func pickAllowOption(options []any) string {
	var firstAllow string
	for _, o := range options {
		m, ok := o.(map[string]any)
		if !ok {
			continue
		}
		id, _ := m["optionId"].(string)
		kind, _ := m["kind"].(string)
		if id == "" {
			continue
		}
		if kind == "allow_once" || id == "allow-once" {
			return id
		}
		if firstAllow == "" && (strings.HasPrefix(kind, "allow") || strings.Contains(strings.ToLower(id+kind), "allow")) {
			firstAllow = id
		}
	}
	return firstAllow
}

func (c *acpConn) writeRaw(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	c.wmu.Lock()
	defer c.wmu.Unlock()
	_, err = c.stdin.Write(b)
	return err
}

// callTimeout bounds every ACP round trip with an idle bound: any adapter
// message restarts the wait, so a healthy long turn is never cut by wall
// clock. Only a silent adapter fails the call.
func (c *acpConn) callTimeout(method string, params any, timeout time.Duration) (json.RawMessage, error) {
	c.mu.Lock()
	c.next++
	id := c.next
	key := fmt.Sprintf("%d", id)
	ch := make(chan acpResult, 1)
	c.wait[key] = ch
	c.mu.Unlock()
	payload, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	if err != nil {
		c.mu.Lock()
		delete(c.wait, key)
		c.mu.Unlock()
		return nil, err
	}
	payload = append(payload, '\n')
	c.wmu.Lock()
	_, err = c.stdin.Write(payload)
	c.wmu.Unlock()
	if err != nil {
		c.mu.Lock()
		delete(c.wait, key)
		c.mu.Unlock()
		return nil, err
	}
	if timeout <= 0 {
		timeout = ctlTimeout
	}
	start := time.Now()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case r := <-ch:
			return r.result, r.err
		case <-timer.C:
			last := time.Unix(0, c.lastRx.Load())
			if last.Before(start) {
				last = start
			}
			if idle := time.Since(last); idle < timeout {
				timer.Reset(timeout - idle)
				continue
			}
			c.mu.Lock()
			delete(c.wait, key)
			c.mu.Unlock()
			if c.exited() {
				if v := c.closeErr.Load(); v != nil {
					if cerr, ok := v.(error); ok {
						return nil, cerr
					}
				}
				return nil, fmt.Errorf("adapter closed")
			}
			return nil, fmt.Errorf("acp %s timed out after %s", method, timeout)
		}
	}
}

// Initialize runs the ACP handshake and records steering support.
func (c *acpConn) Initialize() error {
	raw, err := c.callTimeout("initialize", map[string]any{
		"protocolVersion": protocolVersion,
		"clientCapabilities": map[string]any{
			"fs":       map[string]any{"readTextFile": true, "writeTextFile": true},
			"terminal": false,
		},
	}, initTimeout)
	if err != nil {
		return err
	}
	c.rawInit = raw
	var resp struct {
		Meta struct {
			Steering struct {
				Supported bool `json:"supported"`
			} `json:"steering"`
		} `json:"_meta"`
	}
	if json.Unmarshal(raw, &resp) == nil {
		c.mu.Lock()
		c.steering = resp.Meta.Steering.Supported
		c.mu.Unlock()
	}
	return nil
}

func (c *acpConn) steeringSupported() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.steering
}

// sessionParams builds session/new + session/load params, omitting
// mcpServers entirely when there is no stub (nil would serialize as null,
// which strict agents reject) and sending [] only when explicitly asked.
func sessionParams(cwd string, mcp []map[string]any, meta map[string]any, sessionID string) map[string]any {
	p := map[string]any{"cwd": cwd, "_meta": meta}
	if sessionID != "" {
		p["sessionId"] = sessionID
	}
	if mcp != nil {
		p["mcpServers"] = mcp
	}
	return p
}

// NewSession creates an ACP session with the stub MCP list and _meta.
// It returns the session id plus the raw result so callers can discover
// real model ids (models.availableModels, configOptions).
func (c *acpConn) NewSession(cwd string, mcp []map[string]any, meta map[string]any) (string, error) {
	sid, _, err := c.NewSessionRaw(cwd, mcp, meta)
	return sid, err
}

// NewSessionRaw is NewSession that also returns the raw result JSON.
func (c *acpConn) NewSessionRaw(cwd string, mcp []map[string]any, meta map[string]any) (string, json.RawMessage, error) {
	raw, err := c.callTimeout("session/new", sessionParams(cwd, mcp, meta, ""), ctlTimeout)
	if err != nil {
		return "", nil, err
	}
	var resp struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", raw, err
	}
	if resp.SessionID == "" {
		return "", raw, fmt.Errorf("session/new returned no sessionId")
	}
	return resp.SessionID, raw, nil
}

// LoadSession reloads a known ACP session id in a fresh process.
func (c *acpConn) LoadSession(acpID, cwd string, mcp []map[string]any, meta map[string]any) error {
	_, err := c.LoadSessionRaw(acpID, cwd, mcp, meta)
	return err
}

// LoadSessionRaw is LoadSession returning the raw result (configOptions
// discovery reads session/load responses too).
func (c *acpConn) LoadSessionRaw(acpID, cwd string, mcp []map[string]any, meta map[string]any) (json.RawMessage, error) {
	return c.callTimeout("session/load", sessionParams(cwd, mcp, meta, acpID), ctlTimeout)
}

// Authenticate sends one authenticate call after initialize.
// The ACP shape is {"methodId": id}; some agents use {"method": ...} or a
// bare id string, so callers may retry variants on protocol errors.
func (c *acpConn) Authenticate(methodID string) error {
	_, err := c.callTimeout("authenticate", map[string]any{"methodId": methodID}, ctlTimeout)
	return err
}

// isAuthRequired reports the AUTH_REQUIRED session/new failure shape:
// agents that need authenticate answer session/new with an error naming
// auth (code -32000 class or "auth" text).
func isAuthRequired(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "auth_required") || strings.Contains(s, "auth required") ||
		strings.Contains(s, "authentication") || strings.Contains(s, "unauthenticated")
}

// pickAuthMethod selects the --auth-method value: an explicit id, or 'auto'
// meaning the first advertised authMethod needing no user interaction,
// preferring env-var/api-key shapes.
func pickAuthMethod(rawInit json.RawMessage, want string) (string, bool) {
	if want != "" && want != "auto" {
		return want, true
	}
	var raw map[string]any
	if json.Unmarshal(rawInit, &raw) != nil {
		return "", false
	}
	methods, _ := raw["authMethods"].([]any)
	if len(methods) == 0 {
		return "", false
	}
	// Prefer env-var / api-key types; otherwise first non-interactive one.
	fallback := ""
	for _, m := range methods {
		mm, ok := m.(map[string]any)
		if !ok {
			continue
		}
		id, _ := mm["id"].(string)
		if id == "" {
			id, _ = mm["methodId"].(string)
		}
		if id == "" {
			continue
		}
		if fallback == "" {
			fallback = id
		}
		lower := strings.ToLower(id + " " + fmt.Sprint(mm["type"]) + " " + fmt.Sprint(mm["name"]) + " " + fmt.Sprint(mm["description"]))
		if strings.Contains(lower, "env") || strings.Contains(lower, "api-key") || strings.Contains(lower, "apikey") || strings.Contains(lower, "token") {
			return id, true
		}
	}
	// Skip obviously interactive methods (oauth/browser/device) unless alone.
	for _, m := range methods {
		mm, ok := m.(map[string]any)
		if !ok {
			continue
		}
		id, _ := mm["id"].(string)
		if id == "" {
			id, _ = mm["methodId"].(string)
		}
		lower := strings.ToLower(id)
		if strings.Contains(lower, "oauth") || strings.Contains(lower, "browser") || strings.Contains(lower, "device") || strings.Contains(lower, "interactive") {
			continue
		}
		return id, true
	}
	return fallback, fallback != ""
}

// configOption is one discovered session config option.
type configOption struct {
	ID       string
	Category string
	Values   []string
}

// modelDiscovery is what the probe learned from session/new (+load).
type modelDiscovery struct {
	Current string
	Models  []string
	Options []configOption
	RawKeys []string
}

// discoverModels extracts real model ids from a session/new or session/load
// result: models.availableModels (modelId/id/name), a top-level
// availableModels/currentModelId, and configOptions entries whose category
// is model or whose id is model/thought_level/effort (or mentions model).
func discoverModels(raw json.RawMessage) modelDiscovery {
	var d modelDiscovery
	var v map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &v) != nil {
		return d
	}
	for k := range v {
		d.RawKeys = append(d.RawKeys, k)
	}
	addModel := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		for _, e := range d.Models {
			if e == s {
				return
			}
		}
		d.Models = append(d.Models, s)
	}
	strOf := func(m map[string]any, keys ...string) string {
		for _, k := range keys {
			if s, _ := m[k].(string); s != "" {
				return s
			}
		}
		return ""
	}
	var modelsObj map[string]any
	if m, ok := v["models"].(map[string]any); ok {
		modelsObj = m
	}
	avail := []any{}
	if modelsObj != nil {
		if a, ok := modelsObj["availableModels"].([]any); ok {
			avail = a
		}
		d.Current = strOf(modelsObj, "currentModelId", "currentModel", "modelId", "model")
	}
	if a, ok := v["availableModels"].([]any); ok && len(avail) == 0 {
		avail = a
	}
	if a, ok := v["models"].([]any); ok && len(avail) == 0 {
		avail = a
	}
	if d.Current == "" {
		d.Current = strOf(v, "currentModelId", "currentModel", "modelId", "model")
	}
	for _, e := range avail {
		switch t := e.(type) {
		case string:
			addModel(t)
		case map[string]any:
			addModel(strOf(t, "modelId", "id", "name", "value"))
		}
	}
	// configOptions may live top-level or under models/session.
	var opts []any
	for _, holder := range []map[string]any{v, modelsObj} {
		if holder == nil {
			continue
		}
		for _, k := range []string{"configOptions", "config", "sessionConfigOptions"} {
			if a, ok := holder[k].([]any); ok && len(opts) == 0 {
				opts = a
			}
		}
	}
	for _, e := range opts {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		id := strOf(m, "id", "configId", "name")
		if id == "" {
			continue
		}
		cat, _ := m["category"].(string)
		var vals []string
		for _, vk := range []string{"values", "options", "allowedValues", "choices"} {
			if a, ok := m[vk].([]any); ok {
				for _, vv := range a {
					switch t := vv.(type) {
					case string:
						vals = append(vals, t)
					case map[string]any:
						if s := strOf(t, "value", "id", "name"); s != "" {
							vals = append(vals, s)
						}
					}
				}
			}
		}
		if v2, ok := m["value"].(string); ok && v2 != "" {
			vals = append(vals, v2)
		}
		d.Options = append(d.Options, configOption{ID: id, Category: cat, Values: vals})
	}
	return d
}

// modelConfigTarget picks a REAL different id: prefer a model-category
// config option (id model/thought_level/effort or category model), else a
// second availableModel. Returns configId/value for set_config_option and
// the plain model id for set_model fallback.
func modelConfigTarget(d modelDiscovery) (configID, value, modelID string) {
	target := ""
	for _, m := range d.Models {
		if m != d.Current {
			target = m
			break
		}
	}
	for _, o := range d.Options {
		lower := strings.ToLower(o.ID + " " + o.Category)
		isModel := strings.Contains(lower, "model") || o.ID == "thought_level" || o.ID == "effort" ||
			strings.Contains(lower, "thought") || strings.Contains(lower, "effort")
		if !isModel {
			continue
		}
		val := target
		for _, vv := range o.Values {
			if vv != d.Current {
				val = vv
				break
			}
		}
		if val == "" && len(o.Values) > 0 {
			val = o.Values[0]
		}
		if val == "" {
			val = target
		}
		if val != "" {
			return o.ID, val, target
		}
	}
	return "", target, target
}

// Prompt sends text blocks and returns the stopReason.
func (c *acpConn) Prompt(acpID, text string) (string, error) {
	return c.PromptBlocks(acpID, []any{map[string]any{"type": "text", "text": text}}, promptTimeout)
}

// PromptBlocks sends raw prompt blocks with a custom timeout.
func (c *acpConn) PromptBlocks(acpID string, blocks []any, timeout time.Duration) (string, error) {
	raw, err := c.callTimeout("session/prompt", map[string]any{
		"sessionId": acpID, "prompt": blocks,
	}, timeout)
	if err != nil {
		return "", err
	}
	var resp struct {
		StopReason string `json:"stopReason"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", err
	}
	return resp.StopReason, nil
}

// PromptImages sends text plus one ACP image block per attachment.
func (c *acpConn) PromptImages(acpID, text string, images []testImage) (string, error) {
	blocks := []any{}
	if text != "" {
		blocks = append(blocks, map[string]any{"type": "text", "text": text})
	}
	for _, img := range images {
		blocks = append(blocks, map[string]any{"type": "image", "mimeType": strings.ToLower(img.Mime), "data": img.Data})
	}
	return c.PromptBlocks(acpID, blocks, promptTimeout)
}

// Steer injects text into the running turn.
func (c *acpConn) Steer(acpID, text string) (string, error) {
	raw, err := c.callTimeout("_session/steering", map[string]any{
		"sessionId": acpID,
		"prompt":    []any{map[string]any{"type": "text", "text": text}},
		"_meta":     map[string]any{"steering": map[string]any{"idleBehavior": "promptRequired"}},
	}, steerWait)
	if err != nil {
		return "", err
	}
	var resp struct {
		Outcome string `json:"outcome"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", err
	}
	return resp.Outcome, nil
}

// Cancel sends the session/cancel notification (no id, no reply).
func (c *acpConn) Cancel(acpID string) error {
	return c.writeRaw(map[string]any{"jsonrpc": "2.0", "method": "session/cancel",
		"params": map[string]any{"sessionId": acpID}})
}

// SetModel tries session/set_model with the spec key modelId.
// Callers treat method-not-found as "unsupported".
func (c *acpConn) SetModel(acpID, modelID string) error {
	_, err := c.callTimeout("session/set_model", map[string]any{
		"sessionId": acpID, "modelId": modelID,
	}, ctlTimeout)
	return err
}

// SetConfigOption sets one session config option by id.
func (c *acpConn) SetConfigOption(acpID, configID, value string) error {
	_, err := c.callTimeout("session/set_config_option", map[string]any{
		"sessionId": acpID, "configId": configID, "value": value,
	}, ctlTimeout)
	return err
}

// WaitMCPReady waits for the adapter's MCP-initialized notification.
func (c *acpConn) WaitMCPReady(timeout time.Duration) error {
	select {
	case <-c.mcpReady:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("adapter MCP servers not ready after %s", timeout)
	}
}

// MCPReadySeen reports whether the initialized notification arrived.
func (c *acpConn) MCPReadySeen() bool {
	select {
	case <-c.mcpReady:
		return true
	default:
		return false
	}
}

// Kill ends the agent process immediately (whole group with --kill-group).
func (c *acpConn) Kill() {
	killProcess(c.cmd)
}

// Close ends the agent: stdin EOF first, SIGKILL after a grace period.
func (c *acpConn) Close() {
	_ = c.stdin.Close()
	select {
	case <-c.readDone:
	case <-time.After(5 * time.Second):
		c.Kill()
		<-c.readDone
	}
	_ = c.cmd.Wait()
}

// isMethodNotFound reports the JSON-RPC method-not-found shape.
func isMethodNotFound(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "Method not found") || strings.Contains(s, "method not found") || strings.Contains(s, "-32601")
}

// isTimeout reports a client-side timeout or an adapter-closed-while-waiting.
func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "timed out after")
}
