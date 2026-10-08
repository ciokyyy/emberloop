package web

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ClaudeAdapterPackage pins the ACP adapter. Ticket 01 ruled out any
// fallback: this is the only way a claude Session starts.
const ClaudeAdapterPackage = "@agentclientprotocol/claude-agent-acp@0.88.0"

// ClaudeModels is the fixed alias list the spec offers for claude.
func ClaudeModels() []string { return []string{"opus", "sonnet", "haiku"} }

// claudeDisallowedTools removes every native tool the model could call, so
// only the emberloop MCP tools remain. Proven by the ticket-01 spike.
var claudeDisallowedTools = []string{
	"Task", "Agent", "Bash", "BashOutput", "KillShell", "Read", "Write", "Edit", "MultiEdit", "NotebookEdit",
	"Glob", "Grep", "WebFetch", "WebSearch", "ToolSearch", "Skill", "TodoWrite", "ExitPlanMode",
	"EnterPlanMode", "AskUserQuestion", "SlashCommand", "ListMcpResources", "ReadMcpResource",
}

// claudeAllowedTools is the MCP prefix every emberloop tool carries, the same
// policy the wick claude harness launches with. Bare entries auto-approve.
var claudeAllowedTools = []string{"mcp__emberloop", "mcp__emberloop-spawn"}

// claudeStrippingOptions builds the full _meta.claudeCode.options set. One
// builder serves session/new and session/load: the spike showed a load
// without them re-asks permission.
func claudeStrippingOptions(model string) map[string]any {
	opts := map[string]any{
		"settingSources":  []string{},
		"strictMcpConfig": true,
		// Every tool left is an emberloop MCP tool the user already chose:
		// auto-allow all of them instead of asking.
		"permissionMode":                  "bypassPermissions",
		"allowDangerouslySkipPermissions": true,
		"allowedTools":                    append([]string{}, claudeAllowedTools...),
		"disallowedTools":                 append([]string{}, claudeDisallowedTools...),
	}
	if model != "" {
		opts["model"] = model
	}
	return opts
}

// claudeAdapterEnv carries the same disabling variables the wick claude
// harness sets, so no memory files, git instructions, background tasks or
// telemetry leak into the ACP adapter's SDK.
func claudeAdapterEnv() []string {
	return []string{
		// Load every MCP tool up front: with deferral the model must first
		// call ToolSearch to see the emberloop tools.
		"ENABLE_TOOL_SEARCH=false",
		"CLAUDE_CODE_DISABLE_AUTO_MEMORY=1",
		"CLAUDE_CODE_DISABLE_CLAUDE_MDS=1",
		"CLAUDE_CODE_DISABLE_GIT_INSTRUCTIONS=1",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1",
		"CLAUDE_CODE_DISABLE_BACKGROUND_TASKS=1",
		"CLAUDE_CODE_MCP_AUTO_BACKGROUND_MS=0",
		"CLAUDE_CODE_ENABLE_AWAY_SUMMARY=0",
		"CLAUDE_CODE_NO_FLICKER=1",
		"CLAUDE_CODE_DISABLE_MOUSE=1",
		"DISABLE_TELEMETRY=1",
		"DISABLE_ERROR_REPORTING=1",
	}
}

// acpMCPServer is one ACP stdio MCP server. The command must be absolute:
// the spike showed a relative command leaves the server silently absent.
type acpMCPServer struct {
	Name    string
	Command string
	Args    []string
	Env     [][2]string
}

func (s acpMCPServer) marshal() map[string]any {
	env := []map[string]string{}
	for _, kv := range s.Env {
		env = append(env, map[string]string{"name": kv[0], "value": kv[1]})
	}
	args := s.Args
	if args == nil {
		args = []string{}
	}
	return map[string]any{"name": s.Name, "command": s.Command, "args": args, "env": env}
}

// acpCallbacks receives agent-side traffic while a prompt runs.
type acpCallbacks struct {
	// OnUpdate gets every session/update payload.
	OnUpdate func(update map[string]any)
	// OnPermission fires after a permission request was auto-approved.
	OnPermission func(toolName, optionID string)
}

// acpConn is one adapter process: exactly one Daemon Session. The stub in
// claude_test.go implements the same seam without a real agent.
type acpConn interface {
	Initialize() error
	NewSession(cwd string, mcp []acpMCPServer, meta map[string]any) (string, error)
	LoadSession(acpID, cwd string, mcp []acpMCPServer, meta map[string]any) error
	Prompt(acpID, text string, cb acpCallbacks) (string, error)
	Close() error
}

// ClaudeBackend drives claude Sessions through the ACP adapter. It satisfies
// the Daemon's Backend seam (Models, CreateSession, OnPrompt, CloseSession).
type ClaudeBackend struct {
	mcpName    string
	mcpCommand string
	mcpArgs    []string
	// spawnCommand starts the Daemon-hosted spawn MCP server (the Daemon
	// binary in spawn-server mode); spawnDaemonAddr is the Daemon it calls
	// back over loopback HTTP. Unset keeps Sessions on the base server only.
	spawnCommand    []string
	spawnDaemonAddr string
	// allowedTools overrides the default MCP allow-list; the live probe
	// points it at the echo server's tool.
	allowedTools []string
	// adapterCommand starts the pinned adapter; [npx -y pkg] by default.
	adapterCommand []string

	// ctlTimeout and promptTimeout override the wire defaults; tests use
	// short values, product code leaves them zero.
	ctlTimeout    time.Duration
	promptTimeout time.Duration

	newConn func() (acpConn, error)

	mu       sync.Mutex
	sessions map[string]*claudeState
}

type claudeState struct {
	acpID string
	conn  acpConn
	ready chan struct{}
	// readyOnce makes the MCP-ready signal exactly-once: success,
	// failure-drop and concurrent DropSession all signal it, whichever
	// runs first, so a waiting prompt always wakes.
	readyOnce sync.Once
}

// signalReady closes ready exactly once.
func (s *claudeState) signalReady() { s.readyOnce.Do(func() { close(s.ready) }) }

// applyTimeouts carries test timeouts onto a real wire connection.
func (b *ClaudeBackend) applyTimeouts(conn acpConn) {
	pc, ok := conn.(*acpProcessConn)
	if !ok {
		return
	}
	if b.ctlTimeout > 0 {
		pc.ctlTimeout = b.ctlTimeout
	}
	if b.promptTimeout > 0 {
		pc.promptTimeout = b.promptTimeout
	}
}

func NewClaudeBackend() *ClaudeBackend {
	return &ClaudeBackend{
		mcpName:        "emberloop",
		mcpCommand:     "emberloop-mcp",
		adapterCommand: []string{"npx", "-y", ClaudeAdapterPackage},
		sessions:       map[string]*claudeState{},
	}
}

// newStubClaudeBackend wires a stub connection factory for tests.
func newStubClaudeBackend(newConn func() (acpConn, error)) *ClaudeBackend {
	return &ClaudeBackend{
		mcpName:        "emberloop",
		mcpCommand:     "/abs/stub-mcp",
		adapterCommand: []string{"stub-adapter"},
		newConn:        newConn,
		sessions:       map[string]*claudeState{},
	}
}

// strippingMeta returns the full _meta set for this Backend: the shared
// builder with this Backend's allow-list.
func (b *ClaudeBackend) strippingMeta(model string) map[string]any {
	allowed := b.allowedTools
	if allowed == nil {
		allowed = claudeAllowedTools
	}
	opts := claudeStrippingOptions(model)
	opts["allowedTools"] = append([]string{}, allowed...)
	return map[string]any{"claudeCode": map[string]any{"options": opts}}
}

// StartSession spawns the adapter and creates the stripped ACP session. It
// returns only after session/new answers, so the MCP tools are connected
// before the first prompt can go out.
func (b *ClaudeBackend) StartSession(meta SessionMeta) error {
	mcp, err := b.mcpServersFor(meta.ID, meta.Scout)
	if err != nil {
		return err
	}
	newConn := b.newConn
	if newConn == nil {
		cfg := append([]string{}, b.adapterCommand...)
		newConn = func() (acpConn, error) { return spawnACPConn(cfg) }
	}
	conn, err := newConn()
	if err != nil {
		return fmt.Errorf("start claude adapter: %w", err)
	}
	b.applyTimeouts(conn)
	st := &claudeState{conn: conn, ready: make(chan struct{})}
	b.mu.Lock()
	b.sessions[meta.ID] = st
	b.mu.Unlock()
	if err := conn.Initialize(); err != nil {
		err = fmt.Errorf("acp initialize: %w", err)
		b.drop(meta.ID, conn)
		return err
	}
	acpID, err := conn.NewSession(meta.WorkDir, mcp, b.strippingMeta(meta.Model))
	if err != nil {
		err = fmt.Errorf("acp session/new: %w", err)
		b.drop(meta.ID, conn)
		return err
	}
	b.mu.Lock()
	st.acpID = acpID
	st.signalReady()
	b.mu.Unlock()
	return nil
}

// LoadSession restarts the adapter around a known ACP session id. It repeats
// the full stripping set: without it the loaded session re-asks permission.
func (b *ClaudeBackend) LoadSession(daemonID, acpID, model, cwd string, scout bool) error {
	mcp, err := b.mcpServersFor(daemonID, scout)
	if err != nil {
		return err
	}
	newConn := b.newConn
	if newConn == nil {
		cfg := append([]string{}, b.adapterCommand...)
		newConn = func() (acpConn, error) { return spawnACPConn(cfg) }
	}
	conn, err := newConn()
	if err != nil {
		return fmt.Errorf("restart claude adapter: %w", err)
	}
	b.applyTimeouts(conn)
	st := &claudeState{acpID: acpID, conn: conn, ready: make(chan struct{})}
	b.mu.Lock()
	b.sessions[daemonID] = st
	b.mu.Unlock()
	if err := conn.Initialize(); err != nil {
		err = fmt.Errorf("acp initialize: %w", err)
		b.drop(daemonID, conn)
		return err
	}
	if err := conn.LoadSession(acpID, cwd, mcp, b.strippingMeta(model)); err != nil {
		err = fmt.Errorf("acp session/load: %w", err)
		b.drop(daemonID, conn)
		return err
	}
	b.mu.Lock()
	st.signalReady()
	b.mu.Unlock()
	return nil
}

// drop releases a Session that never started: the map entry goes away and
// ready signals under the lock, so a prompt waiting on MCP-ready wakes and
// reports failure; the process close runs outside the lock so a hung
// adapter never blocks other Sessions. A concurrent DropSession may have
// removed the entry first; the Once keeps the signal exactly-once either way.
func (b *ClaudeBackend) drop(id string, conn acpConn) {
	b.mu.Lock()
	if st, ok := b.sessions[id]; ok {
		delete(b.sessions, id)
		st.signalReady()
	}
	b.mu.Unlock()
	_ = conn.Close()
}

// Models lists the fixed claude aliases for the Daemon's Backend seam.
func (b *ClaudeBackend) Models() []string { return ClaudeModels() }

// CreateSession validates the model then starts the stripped ACP session.
// The Daemon maps validation failures to 400 and start failures to 500.
func (b *ClaudeBackend) CreateSession(meta SessionMeta, emit func(kind, text, status string)) error {
	if !validClaudeModel(meta.Model) {
		return fmt.Errorf("unknown claude model (want opus, sonnet or haiku)")
	}
	return b.StartSession(meta)
}

// CloseSession releases the adapter process without deleting history.
func (b *ClaudeBackend) CloseSession(sessionID string) { b.DropSession(sessionID) }

// DropSession releases a Session id without reporting: creation-failure
// cleanup and DELETE teardown call it when the Daemon already removed the
// Session. It signals ready so a prompt blocked on a still-starting
// Session wakes instead of hanging.
func (b *ClaudeBackend) DropSession(id string) {
	b.mu.Lock()
	st, ok := b.sessions[id]
	if ok {
		delete(b.sessions, id)
		st.signalReady()
	}
	b.mu.Unlock()
	if ok {
		_ = st.conn.Close()
	}
}

// dropConn removes the entry only when it still holds this connection, so
// a release racing a resume never drops the resumed handle.
func (b *ClaudeBackend) dropConn(id string, conn acpConn) {
	b.mu.Lock()
	if st, ok := b.sessions[id]; ok && st.conn == conn {
		delete(b.sessions, id)
		st.signalReady()
	}
	b.mu.Unlock()
	_ = conn.Close()
}

// ReleaseSession captures the ACP session id, then drops the adapter
// process so an idle Session holds no process. The id reloads through
// session/load.
func (b *ClaudeBackend) ReleaseSession(sessionID string) string {
	b.mu.Lock()
	st, ok := b.sessions[sessionID]
	var token string
	if ok {
		token = st.acpID
	}
	b.mu.Unlock()
	if !ok {
		return ""
	}
	b.dropConn(sessionID, st.conn)
	return token
}

// ResumeToken reports the live ACP session id for persistence.
func (b *ClaudeBackend) ResumeToken(sessionID string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if st, ok := b.sessions[sessionID]; ok {
		return st.acpID
	}
	return ""
}

// ResumeOK holds when a saved ACP session id exists to load.
func (b *ClaudeBackend) ResumeOK(meta SessionMeta) (bool, string) {
	if meta.ResumeToken == "" {
		return false, "claude Session has no saved agent to load (it started before resume support); history is read-only"
	}
	return true, ""
}

// ResumeSession restarts the adapter around the saved ACP session id.
func (b *ClaudeBackend) ResumeSession(meta SessionMeta, emit func(kind, text, status string)) error {
	_ = emit
	if meta.ResumeToken == "" {
		return fmt.Errorf("claude: no saved session to resume")
	}
	return b.LoadSession(meta.ID, meta.ResumeToken, meta.Model, meta.WorkDir, meta.Scout)
}

// OnPrompt runs after the prompt Event is stored. It waits for MCP-ready,
// sends the prompt, and streams normalized Events through appendEvent.
func (b *ClaudeBackend) OnPrompt(sessionID, text string, appendEvent func(kind, text, status string)) {
	b.mu.Lock()
	st, ok := b.sessions[sessionID]
	b.mu.Unlock()
	if !ok {
		appendEvent("error", "claude session not started", "")
		appendEvent("status", "", "failed")
		return
	}
	ready := st.ready
	<-ready
	// Recheck: start may have failed after the first lookup, in which
	// case drop deleted the entry and closed ready.
	b.mu.Lock()
	st, ok = b.sessions[sessionID]
	b.mu.Unlock()
	if !ok {
		appendEvent("error", "claude session not started", "")
		appendEvent("status", "", "failed")
		return
	}
	acpID, conn := st.acpID, st.conn
	// Mid-turn prompts are assumed to queue inside the adapter until the
	// turn ends (unverified in repo: no spike run log records MIDTURN-RESP),
	// so no Daemon guard serializes them.
	appendEvent("status", "", "working")
	tracker := newToolTracker()
	if _, perr := conn.Prompt(acpID, text, acpCallbacks{
		OnUpdate: func(update map[string]any) {
			mapACPUpdate(update, appendEvent, tracker)
		},
		OnPermission: func(toolName, optionID string) {
			appendEvent("permission", toolName+" auto-approved as "+optionID, "auto-approved")
		},
	}); perr != nil {
		appendEvent("error", perr.Error(), "")
		appendEvent("status", "", "failed")
		return
	}
	appendEvent("status", "", "idle")
}

// SetSpawnServer serves the spawn tool to claude Sessions through a
// Daemon-hosted MCP server: command starts it (the Daemon binary in
// spawn-server mode) and daemonAddr is the loopback Daemon it calls back.
func (b *ClaudeBackend) SetSpawnServer(command []string, daemonAddr string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.spawnCommand = append([]string(nil), command...)
	b.spawnDaemonAddr = daemonAddr
}

func (b *ClaudeBackend) mcpServers() ([]acpMCPServer, error) {
	return b.mcpServersFor("", false)
}

// mcpServersFor lists the base emberloop MCP server plus, when configured,
// the Daemon-hosted spawn server bound to one Session: its parent env names
// the spawning Session, so the Daemon always knows the parent. Scout
// Sessions get neither spawn nor write tools: the base server runs under
// the read-only scout role and the spawn server is skipped.
func (b *ClaudeBackend) mcpServersFor(sessionID string, scout bool) ([]acpMCPServer, error) {
	cmd := b.mcpCommand
	if !filepath.IsAbs(cmd) {
		abs, err := exec.LookPath(cmd)
		if err != nil {
			return nil, fmt.Errorf("mcp command %q not on PATH", cmd)
		}
		cmd = abs
	}
	name := b.mcpName
	if name == "" {
		name = "emberloop"
	}
	args := b.mcpArgs
	if scout {
		args = []string{"--role", "scout"}
	}
	out := []acpMCPServer{{Name: name, Command: cmd, Args: args}}
	if len(b.spawnCommand) > 0 && sessionID != "" && !scout {
		out = append(out, acpMCPServer{
			Name:    "emberloop-spawn",
			Command: b.spawnCommand[0],
			Args:    append([]string(nil), b.spawnCommand[1:]...),
			Env: [][2]string{
				{"EMBERLOOP_WEB_PARENT", sessionID},
				{"EMBERLOOP_WEB_DAEMON_URL", b.spawnDaemonAddr},
			},
		})
	}
	return out, nil
}

// mapACPUpdate normalizes one session/update payload. Plans and slash
// commands are out of scope (the UI shows none for either harness), and
// subagent updates never occur with Task disallowed, so all three are
// ignored; everything else the ticket names becomes an Event.
func mapACPUpdate(update map[string]any, emit func(kind, text, status string), tr *toolTracker) {
	kind, _ := update["sessionUpdate"].(string)
	switch kind {
	case "agent_message_chunk":
		if t := acpText(update["content"]); t != "" {
			emit("assistant_chunk", t, "")
		}
	case "agent_thought_chunk":
		if t := acpText(update["content"]); t != "" {
			emit("thinking_chunk", t, "")
		}
	case "tool_call":
		id, _ := update["toolCallId"].(string)
		name, _ := update["name"].(string)
		title, _ := update["title"].(string)
		label := title
		if label == "" {
			label = name
		}
		if label == "" {
			label = id
		}
		t := tr.get(id)
		t.Name = label
		if raw, ok := update["rawInput"]; ok && raw != nil {
			t.Input = prettyJSON(raw)
		}
		emit("tool_call_started", t.json(id, "running"), "")
	case "tool_call_update":
		id, _ := update["toolCallId"].(string)
		status, _ := update["status"].(string)
		t := tr.get(id)
		if title, _ := update["title"].(string); title != "" {
			t.Name = title
		}
		if raw, ok := update["rawInput"]; ok && raw != nil {
			if in := prettyJSON(raw); in != "" && in != "{}" {
				t.Input = in
			}
		}
		var parts []string
		if c, ok := update["content"]; ok {
			parts = append(parts, acpToolTexts(c)...)
		}
		if out, ok := update["rawOutput"]; ok && out != nil && len(parts) == 0 {
			if s := acpText(out); s != "" {
				parts = append(parts, s)
			}
		}
		if len(parts) > 0 {
			t.Output = strings.Join(parts, "\n")
		}
		emit("tool_call_updated", t.json(id, status), status)
		for _, d := range acpDiffs(update["content"]) {
			emit("diff", d, "")
		}
	case "usage_update":
		used, _ := update["used"].(float64)
		size, _ := update["size"].(float64)
		if size > 0 {
			emit("usage", compactJSON(map[string]any{"used": int64(used), "size": int64(size)}), "")
		}
	default:
		// plan, plan_update, available_commands_update, usage_update and
		// friends carry nothing the transcript shows.
	}
}

// acpText pulls display text from an ACP content block or value.
func acpText(v any) string {
	switch c := v.(type) {
	case string:
		return c
	case map[string]any:
		if c["type"] == "text" {
			if t, _ := c["text"].(string); t != "" {
				return t
			}
		}
		// Tool result content nests one level: {content: {text}}.
		if inner, ok := c["content"]; ok {
			if t := acpText(inner); t != "" {
				return t
			}
		}
		if t, _ := c["text"].(string); t != "" {
			return t
		}
		return ""
	case []any:
		var parts []string
		for _, e := range c {
			if t := acpText(e); t != "" {
				parts = append(parts, t)
			}
		}
		return strings.Join(parts, "")
	default:
		return ""
	}
}

// acpToolTexts renders tool_call content blocks, skipping diffs (they get
// their own diff Event).
func acpToolTexts(content any) []string {
	items, ok := content.([]any)
	if !ok {
		if t := acpText(content); t != "" {
			return []string{t}
		}
		return nil
	}
	var out []string
	for _, e := range items {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		if m["type"] == "diff" {
			continue
		}
		if inner, ok := m["content"]; ok {
			if t := acpText(inner); t != "" {
				out = append(out, t)
				continue
			}
		}
		if t := acpText(m); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// acpDiffs pulls file diffs from tool_call content blocks.
func acpDiffs(content any) []string {
	items, ok := content.([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, e := range items {
		m, ok := e.(map[string]any)
		if !ok || m["type"] != "diff" {
			continue
		}
		path, _ := m["path"].(string)
		newText, _ := m["newText"].(string)
		if newText == "" {
			newText = compactJSON(m)
		}
		if path != "" {
			out = append(out, path+"\n"+newText)
		} else {
			out = append(out, newText)
		}
	}
	return out
}

func compactJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

// pickAllowOption chooses the auto-approve answer: the allow-once option
// first, then any allow option, never a rejection. Empty means nothing was
// allowable.
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

type acpWireMsg struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  map[string]any  `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type acpResult struct {
	result json.RawMessage
	err    error
}

// acpProcessConn spawns the pinned adapter and talks ACP over stdio.
type acpProcessConn struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	// mu guards next, wait and cb. wmu serializes stdin writes so a
	// full pipe never stalls a reader that needs mu.
	mu   sync.Mutex
	wmu  sync.Mutex
	next int64
	wait map[int64]chan acpResult
	cb   acpCallbacks
	// readDone closes when the stdout reader exits; Close waits for it
	// before cmd.Wait, as os/exec requires.
	readDone      chan struct{}
	ctlTimeout    time.Duration
	promptTimeout time.Duration
	// closeTimeout bounds the wait for a deaf adapter before Kill.
	closeTimeout time.Duration
}

func spawnACPConn(command []string) (acpConn, error) {
	if len(command) == 0 {
		return nil, fmt.Errorf("empty adapter command")
	}
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Env = append(os.Environ(), claudeAdapterEnv()...)
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
	c := &acpProcessConn{
		cmd: cmd, stdin: stdin, wait: map[int64]chan acpResult{},
		readDone: make(chan struct{}),
		// Control calls answer in seconds; a hung adapter must not hang
		// the Daemon. Prompts run agent turns, so they get minutes.
		// Close kills a deaf adapter after seconds.
		ctlTimeout: 60 * time.Second, promptTimeout: 15 * time.Minute,
		closeTimeout: 5 * time.Second,
	}
	go c.read(bufio.NewScanner(stdout))
	return c, nil
}

func (c *acpProcessConn) read(sc *bufio.Scanner) {
	defer close(c.readDone)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := append([]byte(nil), sc.Bytes()...)
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		var msg acpWireMsg
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		c.dispatch(msg)
	}
	c.mu.Lock()
	orphans := make([]chan acpResult, 0, len(c.wait))
	for id, ch := range c.wait {
		delete(c.wait, id)
		orphans = append(orphans, ch)
	}
	c.mu.Unlock()
	for _, ch := range orphans {
		select {
		case ch <- acpResult{err: fmt.Errorf("adapter closed")}:
		default:
		}
	}
}

func (c *acpProcessConn) dispatch(msg acpWireMsg) {
	if msg.ID != nil && (msg.Result != nil || msg.Error != nil) {
		c.mu.Lock()
		ch := c.wait[*msg.ID]
		delete(c.wait, *msg.ID)
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
	if msg.ID != nil && msg.Method == "session/request_permission" {
		c.handlePermission(msg)
		return
	}
	if msg.Method == "session/update" {
		c.mu.Lock()
		cb := c.cb
		c.mu.Unlock()
		if cb.OnUpdate != nil && msg.Params != nil {
			if u, ok := msg.Params["update"].(map[string]any); ok {
				cb.OnUpdate(u)
			}
		}
		return
	}
	if msg.ID != nil && msg.Method != "" {
		// Client requests the probe answered as unsupported (fs reads and
		// writes, terminals): the stripped session never needs them.
		_ = c.writeRaw(map[string]any{"jsonrpc": "2.0", "id": *msg.ID,
			"error": map[string]any{"code": -32601, "message": "unsupported"}})
	}
}

func (c *acpProcessConn) handlePermission(msg acpWireMsg) {
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
	optionID := pickAllowOption(options)
	c.mu.Lock()
	cb := c.cb
	c.mu.Unlock()
	if optionID == "" {
		_ = c.writeRaw(map[string]any{"jsonrpc": "2.0", "id": *msg.ID,
			"result": map[string]any{"outcome": "cancelled"}})
		return
	}
	_ = c.writeRaw(map[string]any{"jsonrpc": "2.0", "id": *msg.ID,
		"result": map[string]any{"outcome": "selected", "optionId": optionID}})
	if cb.OnPermission != nil {
		cb.OnPermission(toolName, optionID)
	}
}

func (c *acpProcessConn) writeRaw(v any) error {
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

func (c *acpProcessConn) call(method string, params any) (json.RawMessage, error) {
	return c.callTimeout(method, params, c.ctlTimeout)
}

// callTimeout bounds every ACP round trip: a hung adapter fails the call
// instead of hanging the Daemon. A late answer lands in a dropped entry.
func (c *acpProcessConn) callTimeout(method string, params any, timeout time.Duration) (json.RawMessage, error) {
	c.mu.Lock()
	c.next++
	id := c.next
	ch := make(chan acpResult, 1)
	c.wait[id] = ch
	c.mu.Unlock()
	payload, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	if err != nil {
		c.mu.Lock()
		delete(c.wait, id)
		c.mu.Unlock()
		return nil, err
	}
	payload = append(payload, '\n')
	c.wmu.Lock()
	_, err = c.stdin.Write(payload)
	c.wmu.Unlock()
	if err != nil {
		c.mu.Lock()
		delete(c.wait, id)
		c.mu.Unlock()
		return nil, err
	}
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case r := <-ch:
		return r.result, r.err
	case <-timer.C:
		c.mu.Lock()
		delete(c.wait, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("acp %s timed out after %s", method, timeout)
	}
}

func (c *acpProcessConn) setCallbacks(cb acpCallbacks) {
	c.mu.Lock()
	c.cb = cb
	c.mu.Unlock()
}

func (c *acpProcessConn) Initialize() error {
	_, err := c.call("initialize", map[string]any{
		"protocolVersion": 1,
		"clientCapabilities": map[string]any{
			"fs":       map[string]any{"readTextFile": true, "writeTextFile": true},
			"terminal": false,
		},
	})
	return err
}

func mcpParams(mcp []acpMCPServer) []any {
	out := []any{}
	for _, s := range mcp {
		out = append(out, s.marshal())
	}
	return out
}

func (c *acpProcessConn) NewSession(cwd string, mcp []acpMCPServer, meta map[string]any) (string, error) {
	raw, err := c.call("session/new", map[string]any{
		"cwd": cwd, "mcpServers": mcpParams(mcp), "_meta": meta,
	})
	if err != nil {
		return "", err
	}
	var resp struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", err
	}
	if resp.SessionID == "" {
		return "", fmt.Errorf("session/new returned no sessionId")
	}
	return resp.SessionID, nil
}

func (c *acpProcessConn) LoadSession(acpID, cwd string, mcp []acpMCPServer, meta map[string]any) error {
	_, err := c.call("session/load", map[string]any{
		"sessionId": acpID, "cwd": cwd, "mcpServers": mcpParams(mcp), "_meta": meta,
	})
	return err
}
func (c *acpProcessConn) Prompt(acpID, text string, cb acpCallbacks) (string, error) {
	c.setCallbacks(cb)
	timeout := c.promptTimeout
	if timeout <= 0 {
		timeout = 15 * time.Minute
	}
	raw, err := c.callTimeout("session/prompt", map[string]any{
		"sessionId": acpID,
		"prompt":    []any{map[string]any{"type": "text", "text": text}},
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

func (c *acpProcessConn) Close() error {
	_ = c.stdin.Close()
	timeout := c.closeTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	if !waitClosed(c.readDone, timeout) {
		// The adapter ignores stdin EOF and holds stdout open: kill it
		// so DELETE and failed-start cleanup never wedge on Close.
		_ = c.cmd.Process.Kill()
		waitClosed(c.readDone, timeout)
	}
	return c.cmd.Wait()
}

// waitClosed reports whether ch closes within timeout.
func waitClosed(ch <-chan struct{}, timeout time.Duration) bool {
	select {
	case <-ch:
		return true
	case <-time.After(timeout):
		return false
	}
}

// toolTracker remembers each tool call across its started and update frames:
// updates often omit the name and input, so the tracker keeps the latest
// known value and every Event carries the whole call as JSON.
type toolTracker struct{ calls map[string]*trackedTool }

type trackedTool struct{ Name, Input, Output string }

func newToolTracker() *toolTracker { return &toolTracker{calls: map[string]*trackedTool{}} }

func (tr *toolTracker) get(id string) *trackedTool {
	if tr == nil {
		return &trackedTool{}
	}
	t, ok := tr.calls[id]
	if !ok {
		t = &trackedTool{}
		tr.calls[id] = t
	}
	return t
}

func (t *trackedTool) json(id, status string) string {
	return compactJSON(map[string]any{"id": id, "name": t.Name, "input": t.Input, "output": t.Output, "status": status})
}

func prettyJSON(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return ""
	}
	return string(b)
}
