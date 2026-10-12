package web

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	el "example.com/emberloop"
)

// ClaudeAdapterPackage pins the ACP adapter. Ticket 01 ruled out any
// fallback: this is the only way a claude Session starts.
const ClaudeAdapterPackage = "@agentclientprotocol/claude-agent-acp@0.88.0"

// ClaudeModels is the fixed alias list the spec offers for claude.
func ClaudeModels() []string { return []string{"opus", "sonnet", "haiku"} }

// claudeDisallowedTools removes every native tool the model could call, so
// only the el MCP tools remain. Proven by the ticket-01 spike.
var claudeDisallowedTools = []string{
	"Task", "Agent", "Bash", "BashOutput", "KillShell", "Read", "Write", "Edit", "MultiEdit", "NotebookEdit",
	"Glob", "Grep", "WebFetch", "WebSearch", "ToolSearch", "Skill", "TodoWrite", "ExitPlanMode",
	"EnterPlanMode", "AskUserQuestion", "SlashCommand", "ListMcpResources", "ReadMcpResource",
	"ListAgents", "ListPeers", "SendMessage", "SendUserMessage", "Brief", "TaskStop", "TaskOutput",
	"ListMcpResourcesTool", "ReadMcpResourceTool", "ReadMcpResourceDirTool",
}

// claudeAllowedTools is the MCP prefix every el tool carries, the same
// policy the wick claude harness launches with. Bare entries auto-approve.
var claudeAllowedTools = []string{"mcp__el"}

// claudeSystemPrompt is the base Session system prompt: el's rule set.
// Project context arrives as injected content via claudeSystemPromptFor,
// never as a read-these-files instruction.
// Web children settle on their turn alone (no submit tool), so while
// el.WebSubmitEnabled is false the two shared submit-settlement lines are
// reworded for the Daemon: delivery on settle, last message as the result.
func claudeSystemPrompt() string {
	p := el.SystemPrompt()
	if !el.WebSubmitEnabled {
		p = strings.ReplaceAll(p,
			"one notice with every result arrives when the last agent of that call has submitted.",
			"one notice with every result arrives when the last agent of that call settles.")
		p = strings.ReplaceAll(p,
			" A spawned agent yields right after submit: end the turn at once, with no text.",
			" A spawned agent ends its turn when the work is done: its last message is delivered as its result.")
	}
	return p
}

// claudeStrippingOptions builds the full _meta.claudeCode.options set. One
// builder serves session/new and session/load: the spike showed a load
// without them re-asks permission.
func claudeStrippingOptions(model string) map[string]any {
	opts := map[string]any{
		"settingSources":  []string{},
		"strictMcpConfig": true,
		// Every tool left is an el MCP tool the user already chose:
		// auto-allow all of them instead of asking.
		"permissionMode":                  "bypassPermissions",
		"allowDangerouslySkipPermissions": true,
		"allowedTools":                    append([]string{}, claudeAllowedTools...),
		"disallowedTools":                 append([]string{}, claudeDisallowedTools...),
		// tools: [] drops every built-in tool the SDK ships, including ones newer
		// than the deny list (Cron*, worktree, RemoteTrigger, ReportFindings).
		"tools": []string{},
		// Nothing to list or preload: no skills, plugins or subagents.
		"skills":                 []string{},
		"plugins":                []string{},
		"agents":                 map[string]any{},
		"promptSuggestions":      false,
		"agentProgressSummaries": false,
	}
	if model != "" {
		opts["model"] = model
	}
	if st := claudeInlineSettings(); st != nil {
		opts["settings"] = st
	}
	return opts
}

// claudeInlineSettings is the inline settings object the CLI harness passes
// with --settings, which still applies under settingSources []: the caveman
// ultra reminder on every user message (UserPromptSubmit), as the harness and
// the pig extension deliver it, so the register in the system prompt is not
// outvoted by the latest message. ACP has no other hook. The hook needs the
// el binary; without it on PATH the system-prompt layer alone remains.
func claudeInlineSettings() map[string]any {
	bin, err := exec.LookPath("el")
	if err != nil {
		return nil
	}
	return map[string]any{
		"awaySummaryEnabled": false,
		"hooks": map[string]any{"UserPromptSubmit": []any{map[string]any{
			"hooks": []any{map[string]any{"type": "command", "command": bin + " voice-reminder hook"}},
		}}},
	}
}

// claudeAdapterEnv carries the same disabling variables the wick claude
// harness sets, so no memory files, git instructions, background tasks or
// telemetry leak into the ACP adapter's SDK.
func claudeAdapterEnv() []string {
	return []string{
		// Load every MCP tool up front: with deferral the model must first
		// call ToolSearch to see the el tools.
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
		// Bundled skills, workflows, agent view, cron and checkpoints only add
		// listings and bookkeeping the stripped tool set cannot use.
		"CLAUDE_CODE_DISABLE_BUNDLED_SKILLS=1",
		"CLAUDE_CODE_DISABLE_WORKFLOWS=1",
		"CLAUDE_CODE_DISABLE_AGENT_VIEW=1",
		"CLAUDE_CODE_DISABLE_CRON=1",
		"CLAUDE_CODE_DISABLE_FILE_CHECKPOINTING=1",
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
	// mcpEnv is extra environment for the el MCP server (grok: the real
	// HOME, since the adapter itself runs under a temp one).
	mcpEnv [][2]string
	// daemonAddr is the loopback Daemon the single el MCP server calls back
	// over HTTP for the Daemon-hosted tools and sudo approvals. Empty leaves
	// Sessions with an unbound base server.
	daemonAddr string
	// daemonToken is the Daemon machine token. The el MCP server sends it as
	// EL_WEB_TOKEN so its callbacks pass the /api gate (see auth.go).
	daemonToken string
	// tokenFile names the shared claude setup-token file for login-race
	// advice text; the Daemon sets it in WithClaudeBackend. Empty keeps the
	// generic location. It is only ever a path, never the token value.
	tokenFile string
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

	// plain turns the Backend into a plain ACP client for a non-claude
	// adapter (pi-acp over pig): no claude _meta stripping set, and the
	// Session's model is chosen after session/new through
	// session/set_config_option.
	plain bool
	// newConnFor, when set, starts the adapter for one Daemon Session, so
	// the process can carry per-Session environment, role and the owner's dirs.
	newConnFor func(sessionID, role string, dirs ownerDirs) (acpConn, error)
	// metaHook, when set, builds the session/new and session/load _meta
	// for a plain adapter (grok: system prompt override, yolo mode) from
	// the Daemon Session id, model, effort, WorkDir, role and owner dirs.
	// extra carries the role's memory/DOX text (prompts.go); empty leaves
	// the prompt without it (title calls, back-compat loads).
	metaHook func(daemonID, model, effort, workDir, role string, dirs ownerDirs, extra string) map[string]any
	// promptWrap, when set, rewrites each prompt before it goes on the wire
	// (grok: the per-turn register reminder, since ACP has no hook). The
	// stored prompt Event keeps the user's own text.
	promptWrap func(text string) string
	// waitMCP makes start and load wait for the adapter's MCP-ready
	// notification before the Session counts as ready (grok connects its MCP
	// servers after session/new answers).
	waitMCP bool
	// windowFor, when set, sizes fallback usage readings for one model
	// (pig-acp: the pig catalog window; grok: its known window). Unset
	// means the claude window. A plain struct field (not a method) because
	// OnPromptImages runs on the embedded *ClaudeBackend, where an outer
	// override method would never dispatch.
	windowFor func(model string) int

	mu       sync.Mutex
	sessions map[string]*claudeState
}

// claudeWindow is the context window claude adapters report: usage_update
// carries size 1,000,000.
const claudeWindow = 1000000

// grokWindow is the context window assumed for grok models when the adapter
// sends no usage_update of its own.
const grokWindow = 256000

// UsageWindow reports the context window for model: the Backend override
// when set, else the claude window. It feeds the Daemon's usage seed.
func (b *ClaudeBackend) UsageWindow(model string) int {
	if b.windowFor != nil {
		return b.windowFor(model)
	}
	return claudeWindow
}

type claudeState struct {
	acpID string
	conn  acpConn
	ready chan struct{}
	// model sizes fallback usage readings when the adapter sends none.
	model string
	// lastUsed is the highest used count an adapter usage_update reported:
	// the fallback never regresses the meter below it.
	lastUsed int64
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
		mcpName:        "el",
		mcpCommand:     "el-mcp",
		adapterCommand: []string{"npx", "-y", ClaudeAdapterPackage},
		sessions:       map[string]*claudeState{},
	}
}

// newStubClaudeBackend wires a stub connection factory for tests.
func newStubClaudeBackend(newConn func() (acpConn, error)) *ClaudeBackend {
	return &ClaudeBackend{
		mcpName:        "el",
		mcpCommand:     "/abs/stub-mcp",
		adapterCommand: []string{"stub-adapter"},
		newConn:        newConn,
		sessions:       map[string]*claudeState{},
	}
}

// strippingMeta returns the full _meta set for this Backend with the base
// prompt (the title path, which overwrites the prompt anyway).
func (b *ClaudeBackend) strippingMeta(model string) map[string]any {
	return b.strippingMetaFor(model, "", "")
}

// strippingMetaFor is strippingMeta with the Session WorkDir's project
// context and the owner's global file (agentDir) injected into the system
// prompt. settingSources stays []: context arrives as content, never as
// project settings. Title calls use this (no role extra); agent Sessions use
// strippingMetaForExtra.
func (b *ClaudeBackend) strippingMetaFor(model, workDir, agentDir string) map[string]any {
	return b.strippingMetaForExtra(model, workDir, agentDir, "")
}

// strippingMetaForExtra is strippingMetaFor with the role's memory/DOX extra
// (prompts.go, one place per role) between the base rules and the project
// context. Empty extra keeps the title-call shape.
func (b *ClaudeBackend) strippingMetaForExtra(model, workDir, agentDir, extra string) map[string]any {
	if b.plain {
		return nil
	}
	allowed := b.allowedTools
	if allowed == nil {
		allowed = claudeAllowedTools
	}
	opts := claudeStrippingOptions(model)
	opts["allowedTools"] = append([]string{}, allowed...)
	// The whole Claude Code preset is replaced: el's own rules, the role
	// extra, plus the injected project context, as the CLI harness does with
	// --system-prompt.
	return map[string]any{
		"claudeCode":   map[string]any{"options": opts},
		"systemPrompt": claudeSystemPromptWithExtra(workDir, agentDir, extra),
	}
}

// validateClaudeSession refuses an unknown model or effort before the adapter
// starts. Create and Handoff swap share it.
func validateClaudeSession(model, effort string) error {
	if !validClaudeModel(model) {
		return fmt.Errorf("unknown claude model (want opus, sonnet or haiku)")
	}
	if !validClaudeEffort(effort) {
		return fmt.Errorf("unknown claude effort (want low, medium, high, xhigh or max)")
	}
	return nil
}

// claudeEfforts are the reasoning effort levels a claude Session accepts.
var claudeEfforts = []string{"low", "medium", "high", "xhigh", "max"}

func validClaudeEffort(e string) bool {
	if e == "" {
		return true
	}
	for _, v := range claudeEfforts {
		if v == e {
			return true
		}
	}
	return false
}

// strippingMetaEffort is strippingMetaFor with the Session's reasoning
// effort set in the SDK options; an empty effort keeps the model default.
// Title calls use this (no role extra); agent Sessions use
// strippingMetaEffortExtra.
func (b *ClaudeBackend) strippingMetaEffort(model, effort, workDir, agentDir string) map[string]any {
	return b.strippingMetaEffortExtra(model, effort, workDir, agentDir, "")
}

// strippingMetaEffortExtra is strippingMetaEffort with the role's memory/DOX
// extra. Empty extra keeps the title-call shape.
func (b *ClaudeBackend) strippingMetaEffortExtra(model, effort, workDir, agentDir, extra string) map[string]any {
	m := b.strippingMetaForExtra(model, workDir, agentDir, extra)
	if m == nil || effort == "" {
		return m
	}
	m["claudeCode"].(map[string]any)["options"].(map[string]any)["effort"] = effort
	return m
}

// StartSession spawns the adapter and creates the stripped ACP session. It
// returns only after session/new answers, so the MCP tools are connected
// before the first prompt can go out.
func (b *ClaudeBackend) StartSession(meta SessionMeta) error {
	role := elRoleFor(meta)
	mcp, err := b.mcpServersFor(meta.ID, role)
	if err != nil {
		return err
	}
	newConn := b.connFactory(meta.ID, role, meta.Dirs)
	conn, err := newConn()
	if err != nil {
		return fmt.Errorf("start claude adapter: %w", err)
	}
	b.applyTimeouts(conn)
	st := &claudeState{conn: conn, ready: make(chan struct{}), model: meta.Model}
	b.mu.Lock()
	b.sessions[meta.ID] = st
	b.mu.Unlock()
	if err := conn.Initialize(); err != nil {
		err = fmt.Errorf("acp initialize: %w", err)
		b.drop(meta.ID, conn)
		return err
	}
	acpID, err := conn.NewSession(meta.WorkDir, mcp, b.sessionMetaFor(meta, meta.Dirs))
	if err != nil {
		err = fmt.Errorf("acp session/new: %w", err)
		b.drop(meta.ID, conn)
		return err
	}
	if err := b.afterStart(conn, acpID, meta.Model); err != nil {
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
func (b *ClaudeBackend) LoadSession(daemonID, acpID, model, cwd, role string, dirs ownerDirs) error {
	return b.loadSessionEffort(daemonID, acpID, model, "", cwd, role, dirs)
}

// loadSessionEffort is LoadSession carrying the Session's reasoning effort.
func (b *ClaudeBackend) loadSessionEffort(daemonID, acpID, model, effort, cwd, role string, dirs ownerDirs) error {
	mcp, err := b.mcpServersFor(daemonID, role)
	if err != nil {
		return err
	}
	newConn := b.connFactory(daemonID, role, dirs)
	conn, err := newConn()
	if err != nil {
		return fmt.Errorf("restart claude adapter: %w", err)
	}
	b.applyTimeouts(conn)
	st := &claudeState{acpID: acpID, conn: conn, ready: make(chan struct{}), model: model}
	b.mu.Lock()
	b.sessions[daemonID] = st
	b.mu.Unlock()
	if err := conn.Initialize(); err != nil {
		err = fmt.Errorf("acp initialize: %w", err)
		b.drop(daemonID, conn)
		return err
	}
	if err := conn.LoadSession(acpID, cwd, mcp, b.sessionMeta(daemonID, model, effort, cwd, role, dirs)); err != nil {
		err = fmt.Errorf("acp session/load: %w", err)
		b.drop(daemonID, conn)
		return err
	}
	if err := b.afterStart(conn, acpID, model); err != nil {
		b.drop(daemonID, conn)
		return err
	}
	b.mu.Lock()
	st.signalReady()
	b.mu.Unlock()
	return nil
}

// connFactory picks how the adapter process starts for one Daemon Session.
// The adapter runs under the owner's dirs (claudeEnv).
func (b *ClaudeBackend) connFactory(sessionID, role string, dirs ownerDirs) func() (acpConn, error) {
	if b.newConnFor != nil {
		return func() (acpConn, error) { return b.newConnFor(sessionID, role, dirs) }
	}
	if b.newConn != nil {
		return b.newConn
	}
	cfg := append([]string{}, b.adapterCommand...)
	return func() (acpConn, error) { return spawnACPConnEnv(cfg, claudeEnv(dirs)) }
}

// acpModelSetter is the optional seam to pick the Session's model after
// session/new, for adapters that take it as a config option.
type acpModelSetter interface {
	SetModel(acpID, model string) error
}

// applyModel selects the Session's model on a plain adapter.
func (b *ClaudeBackend) applyModel(conn acpConn, acpID, model string) error {
	if !b.plain || model == "" {
		return nil
	}
	ms, ok := conn.(acpModelSetter)
	if !ok {
		return nil
	}
	if err := ms.SetModel(acpID, model); err != nil {
		return fmt.Errorf("acp set model %q: %w", model, err)
	}
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
	if err := validateClaudeSession(meta.Model, meta.Effort); err != nil {
		return err
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

// ResumeSession restarts the adapter around the saved ACP session id. The
// system prompt is rebuilt from the record, so the resumed handle keeps the
// role's memory/DOX extra.
func (b *ClaudeBackend) ResumeSession(meta SessionMeta, emit func(kind, text, status string)) error {
	_ = emit
	if meta.ResumeToken == "" {
		return fmt.Errorf("claude: no saved session to resume")
	}
	return b.loadSessionWithMeta(meta, meta.ResumeToken)
}

// loadSessionWithMeta is loadSessionEffort for one agent record: the role
// comes from the record and the system prompt carries its memory/DOX extra.
func (b *ClaudeBackend) loadSessionWithMeta(meta SessionMeta, acpID string) error {
	role := elRoleFor(meta)
	mcp, err := b.mcpServersFor(meta.ID, role)
	if err != nil {
		return err
	}
	newConn := b.connFactory(meta.ID, role, meta.Dirs)
	conn, err := newConn()
	if err != nil {
		return fmt.Errorf("restart claude adapter: %w", err)
	}
	b.applyTimeouts(conn)
	st := &claudeState{acpID: acpID, conn: conn, ready: make(chan struct{}), model: meta.Model}
	b.mu.Lock()
	b.sessions[meta.ID] = st
	b.mu.Unlock()
	if err := conn.Initialize(); err != nil {
		err = fmt.Errorf("acp initialize: %w", err)
		b.drop(meta.ID, conn)
		return err
	}
	if err := conn.LoadSession(acpID, meta.WorkDir, mcp, b.sessionMetaFor(meta, meta.Dirs)); err != nil {
		err = fmt.Errorf("acp session/load: %w", err)
		b.drop(meta.ID, conn)
		return err
	}
	if err := b.afterStart(conn, acpID, meta.Model); err != nil {
		b.drop(meta.ID, conn)
		return err
	}
	b.mu.Lock()
	st.signalReady()
	b.mu.Unlock()
	return nil
}

// newHandoffConn starts a fresh adapter and ACP session beside the live one
// for a Handoff swap. The role comes from startHandle, so the new session
// gets the same tool set the record derives; the model, effort and
// voice-reminder hook ride the same session meta a create sends. It returns
// only after session/new answers, so the MCP tools are connected before the
// Handoff prompt can go out. Nothing in the backend map moves: the caller
// swaps it in after any post-start step, so a failure leaves the old handle
// untouched.
func (b *ClaudeBackend) newHandoffConn(meta SessionMeta) (acpConn, string, error) {
	role := startHandle(meta, handleCompress).role
	mcp, err := b.mcpServersFor(meta.ID, role)
	if err != nil {
		return nil, "", err
	}
	newConn := b.connFactory(meta.ID, role, meta.Dirs)
	conn, err := newConn()
	if err != nil {
		return nil, "", fmt.Errorf("start claude adapter: %w", err)
	}
	b.applyTimeouts(conn)
	if err := conn.Initialize(); err != nil {
		_ = conn.Close()
		return nil, "", fmt.Errorf("acp initialize: %w", err)
	}
	acpID, err := conn.NewSession(meta.WorkDir, mcp, b.sessionMetaFor(meta, meta.Dirs))
	if err != nil {
		_ = conn.Close()
		return nil, "", fmt.Errorf("acp session/new: %w", err)
	}
	if err := b.afterStart(conn, acpID, meta.Model); err != nil {
		_ = conn.Close()
		return nil, "", err
	}
	return conn, acpID, nil
}

// swapHandoffHandle publishes a Handoff session built by newHandoffConn: the
// map entry moves to the new handle and signals ready, so a prompt waiting on
// it wakes. It returns the new resume token with a release that drops the old
// handle; the Daemon calls release once the first turn on the new handle
// starts cleanly.
func (b *ClaudeBackend) swapHandoffHandle(meta SessionMeta, conn acpConn, acpID string) (string, func()) {
	b.mu.Lock()
	old, ok := b.sessions[meta.ID]
	st := &claudeState{conn: conn, acpID: acpID, ready: make(chan struct{}), model: meta.Model}
	if ok && old.lastUsed > 0 {
		st.lastUsed = old.lastUsed
	}
	b.sessions[meta.ID] = st
	st.signalReady()
	b.mu.Unlock()
	if !ok {
		return acpID, func() {}
	}
	oldConn := old.conn
	return acpID, func() { _ = oldConn.Close() }
}

// StartHandle swaps in a fresh ACP session for a Handoff compress, so the
// Session reports compressible and compresses by Handoff restart. Model,
// effort and the voice-reminder hook survive through the shared session meta;
// the resume token is the new ACP session id. A failure keeps the old handle.
func (b *ClaudeBackend) StartHandle(meta SessionMeta, emit func(kind, text, status string)) (string, func(), error) {
	_ = emit
	if err := validateClaudeSession(meta.Model, meta.Effort); err != nil {
		return "", nil, err
	}
	conn, acpID, err := b.newHandoffConn(meta)
	if err != nil {
		return "", nil, err
	}
	tok, release := b.swapHandoffHandle(meta, conn, acpID)
	return tok, release, nil
}

// acpSteerer is the optional steering seam of an acpConn: the adapter
// advertised _session/steering and Steer injects into the running turn.
type acpSteerer interface {
	SteeringSupported() bool
	Steer(acpID, text string) (bool, error)
}

// SteerLive injects text into the Session's running turn. It reports false
// when the adapter cannot steer, no turn runs, or the call failed; the caller
// then falls back to a Follow-up.
func (b *ClaudeBackend) SteerLive(sessionID, text string, images []Image) bool {
	b.mu.Lock()
	st, ok := b.sessions[sessionID]
	b.mu.Unlock()
	if !ok || st.acpID == "" {
		return false
	}
	sc, ok := st.conn.(acpSteerer)
	if !ok || !sc.SteeringSupported() {
		return false
	}
	if len(images) > 0 {
		is, ok := st.conn.(acpImageSteerer)
		if !ok {
			return false
		}
		injected, err := is.SteerImages(st.acpID, text, images)
		return err == nil && injected
	}
	injected, err := sc.Steer(st.acpID, text)
	return err == nil && injected
}

// acpCanceller is the optional conn seam that stops the running turn.
type acpCanceller interface {
	Cancel(acpID string) error
}

// Interrupt asks the adapter to cancel the Session's running turn. The
// pending prompt call then returns with stopReason cancelled and the turn
// ends idle through the normal path.
func (b *ClaudeBackend) Interrupt(sessionID string) bool {
	b.mu.Lock()
	st, ok := b.sessions[sessionID]
	b.mu.Unlock()
	if !ok || st.acpID == "" {
		return false
	}
	c, ok := st.conn.(acpCanceller)
	return ok && c.Cancel(st.acpID) == nil
}

// acpImageSteerer is the optional seam for a Steer that carries images.
type acpImageSteerer interface {
	SteerImages(acpID, text string, images []Image) (bool, error)
}

// OnPrompt runs after the prompt Event is stored. It waits for MCP-ready,
// sends the prompt, and streams normalized Events through appendEvent.
func (b *ClaudeBackend) OnPrompt(sessionID, text string, appendEvent func(kind, text, status string)) {
	b.OnPromptImages(sessionID, text, nil, appendEvent)
}

// OnPromptImages is OnPrompt with image attachments sent as ACP image blocks.
func (b *ClaudeBackend) OnPromptImages(sessionID, text string, images []Image, appendEvent func(kind, text, status string)) {
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
	text = b.wrapPrompt(text)
	// Mid-turn prompts are assumed to queue inside the adapter until the
	// turn ends (unverified in repo: no spike run log records MIDTURN-RESP),
	// so no Daemon guard serializes them.
	appendEvent("status", "", "working")
	// usageTap watches the turn for an adapter usage_update: some adapters
	// (grok always, older claude builds at times) send none, which would
	// leave the meter and the auto-compress gates blind. The tap records a
	// sighting, banks the highest used count (so the fallback never
	// regresses the meter), and sizes model text for the estimate.
	uw := &usageWatcher{chars: len(text)}
	b.mu.Lock()
	uw.model, uw.floor = st.model, st.lastUsed
	b.mu.Unlock()
	tap := func(kind, text, status string) {
		uw.note(kind, text)
		if kind == "usage" {
			if used, _, ok := parseUsageEvent(text); ok && used > 0 {
				b.mu.Lock()
				if cur, ok := b.sessions[sessionID]; ok && used > cur.lastUsed {
					cur.lastUsed = used
				}
				b.mu.Unlock()
			}
		}
		appendEvent(kind, text, status)
	}
	tracker := newToolTracker()
	cbs := acpCallbacks{
		OnUpdate: func(update map[string]any) {
			mapACPUpdate(update, tap, tracker)
		},
		OnPermission: func(toolName, optionID string) {
			tap("permission", toolName+" auto-approved as "+optionID, "auto-approved")
		},
	}
	promptOnce := func() error {
		var perr error
		if len(images) == 0 {
			_, perr = conn.Prompt(acpID, text, cbs)
		} else if ip, ok := conn.(acpImagePrompter); ok {
			_, perr = ip.PromptImages(acpID, text, images, cbs)
		} else {
			perr = errors.New("claude adapter cannot take images")
		}
		return perr
	}
	perr := promptOnce()
	if isClaudeLoginRaceErr(perr) {
		// The refresh lock is documented as transient: one retry after a
		// pause. With a shared token this error should not occur at all;
		// the retry is harmless there and the advice still applies.
		time.Sleep(claudeLoginRetry)
		perr = promptOnce()
	}
	tracker.closeOpen(tap)
	// Without an adapter reading, estimate one from the known window so the
	// meter moves on every ACP Backend (claude, pig-acp, grok) even mid
	// first turn. The estimate never regresses the banked high.
	if fb, ok := uw.fallback(b.UsageWindow(uw.model)); ok {
		tap("usage", fb, "")
	}
	if perr != nil {
		msg := perr.Error()
		if isClaudeLoginRaceErr(perr) {
			msg += "\n" + claudeLoginRaceAdvice(b.tokenFile)
		}
		appendEvent("error", msg, "")
		appendEvent("status", "", "failed")
		return
	}
	appendEvent("status", "", "idle")
}

// usageWatcher notes one ACP turn's usage sightings and model text, then
// derives the fallback reading when the adapter sent none.
type usageWatcher struct {
	model string
	floor int64
	chars int
	saw   bool
}

// note records one tapped Event: a usage sighting, or model text that sizes
// the fallback estimate.
func (w *usageWatcher) note(kind, text string) {
	switch kind {
	case "usage":
		w.saw = true
	case "assistant_chunk", "thinking_chunk":
		w.chars += len(text)
	}
}

// fallback renders the estimated usage reading: used is the banked high or
// the turn's characters at four per token, whichever is higher, against the
// known window. False when the adapter sent a reading or no window is known.
func (w *usageWatcher) fallback(window int) (string, bool) {
	if w.saw || window <= 0 {
		return "", false
	}
	used := w.floor
	if est := int64(w.chars / 4); est > used {
		used = est
	}
	return usageText(int(used), window), true
}

// parseUsageEvent reads a usage Event's used and size counts.
func parseUsageEvent(text string) (int64, int64, bool) {
	var u struct {
		Used int64 `json:"used"`
		Size int64 `json:"size"`
	}
	if json.Unmarshal([]byte(text), &u) != nil || u.Size <= 0 {
		return 0, 0, false
	}
	return u.Used, u.Size, true
}

// SetDaemonAddr binds the single el MCP server to the loopback Daemon it
// calls back for the Daemon-hosted tools and sudo approvals.
func (b *ClaudeBackend) SetDaemonAddr(daemonAddr string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.daemonAddr = daemonAddr
}

// SetDaemonToken sets the machine token the el MCP server presents on its
// Daemon callbacks.
func (b *ClaudeBackend) SetDaemonToken(token string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.daemonToken = token
}

func (b *ClaudeBackend) mcpServers() ([]acpMCPServer, error) {
	return b.mcpServersFor("", "web")
}

// elRoleFor is the el MCP role one Session's server runs under: spawned group
// members run as workers (base tools plus, nesting kept, spawn), scouts stay
// read-only, everything else runs as a top-level parent (base tools plus
// spawn, steer, check and cancel). Run workers and Run reviewers run
// run-worker, which always serves submit. Submit stays registered for
// web-worker and scout but is unoffered while el.WebSubmitEnabled is false.
// Reviewers run as run-worker for the submit tool only; the reviewer system
// prompt (rolePromptFor) still carries the judge-only extra, and the spawn
// route refuses reviewer spawns, so the extra spawn tool stays inert.
func elRoleFor(meta SessionMeta) string {
	if meta.Scout {
		return "scout"
	}
	if meta.RunWorker {
		return "run-worker"
	}
	if isReviewerSession(meta) {
		return "run-worker"
	}
	if meta.GroupID != "" {
		return "web-worker"
	}
	return "web"
}

// mcpServersFor lists the single el MCP server for one Session: the base
// tools plus, when the role allows, the Daemon-hosted tools, all bound to
// the Session through the parent and Daemon URL in its env. Scout Sessions
// run the read-only role with no Daemon management tools.
func (b *ClaudeBackend) mcpServersFor(sessionID, role string) ([]acpMCPServer, error) {
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
		name = "el"
	}
	args := b.mcpArgs
	if len(args) == 0 {
		args = []string{"--role", role}
	}
	base := acpMCPServer{Name: name, Command: cmd, Args: args}
	// The server gates sudo commands through the Daemon and serves the
	// Daemon-hosted tools of its role, both keyed on this binding.
	if b.daemonAddr != "" && sessionID != "" {
		base.Env = [][2]string{
			{"EL_WEB_PARENT", sessionID},
			{"EL_WEB_DAEMON_URL", b.daemonAddr},
		}
		if b.daemonToken != "" {
			base.Env = append(base.Env, [2]string{"EL_WEB_TOKEN", b.daemonToken})
		}
	}
	base.Env = append(base.Env, b.mcpEnv...)
	return []acpMCPServer{base}, nil
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
		if status == "completed" || status == "failed" {
			t.Done = true
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
	// lastRx is the unix-nano time of the last message from the adapter.
	lastRx atomic.Int64
	// closeTimeout bounds the wait for a deaf adapter before Kill.
	closeTimeout time.Duration
	// steering is set by Initialize from _meta.steering.supported.
	steering bool
	// mcpReady closes once on the adapter's _x.ai/mcp_initialized
	// notification (grok); WaitMCPReady reads it.
	mcpReady chan struct{}
	mcpOnce  sync.Once
}

func spawnACPConn(command []string) (acpConn, error) {
	return spawnACPConnEnv(command, claudeEnv(ownerDirs{}))
}

// claudeEnv is the claude adapter's environment for one owner: the host env,
// the disabling set, the owner's CLAUDE_CONFIG_DIR and, when configured, the
// shared setup token last-wins. The token comes from the owner's dirs (the
// Daemon resolves it env-over-file on every start) or, for paths that never
// went through ownerDirsFor, from the Daemon process env. The last entry
// wins, so both override a host CLAUDE_CONFIG_DIR or token. An empty owner
// dir keeps the host's. The grok and pig-acp paths need none of this: they
// start non-claude adapters (grok agent, pi-acp) that never read
// CLAUDE_CODE_OAUTH_TOKEN. Title calls use this too, through ownerDirsFor.
func claudeEnv(dirs ownerDirs) []string {
	env := append(os.Environ(), claudeAdapterEnv()...)
	if dirs.claude != "" {
		env = append(env, "CLAUDE_CONFIG_DIR="+dirs.claude)
	}
	if tok := claudeTokenForEnv(dirs.claudeToken); tok != "" {
		env = append(env, "CLAUDE_CODE_OAUTH_TOKEN="+tok)
	}
	return env
}

// spawnACPConnEnv starts one adapter process with exactly this environment.
func spawnACPConnEnv(command, env []string) (acpConn, error) {
	if len(command) == 0 {
		return nil, fmt.Errorf("empty adapter command")
	}
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Env = env
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
		closeTimeout: 5 * time.Second, mcpReady: make(chan struct{}),
	}
	go c.read(bufio.NewScanner(stdout))
	return c, nil
}

// maxACPLine bounds one adapter output line.
const maxACPLine = 256 * 1024 * 1024

func (c *acpProcessConn) read(sc *bufio.Scanner) {
	defer close(c.readDone)
	// Resume replays carry whole tool-result images as base64, so one
	// line can pass 1 MiB; a hard cap would read as "adapter closed".
	sc.Buffer(make([]byte, 1024*1024), maxACPLine)
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
	closeErr := fmt.Errorf("adapter closed")
	if err := sc.Err(); err != nil {
		closeErr = fmt.Errorf("adapter closed: read adapter output: %w", err)
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
		case ch <- acpResult{err: closeErr}:
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
	if msg.ID == nil && msg.Method == "_x.ai/mcp_initialized" {
		c.mcpOnce.Do(func() { close(c.mcpReady) })
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
	// The timeout is an idle bound: any adapter message (stream chunk, tool
	// update, permission ask) restarts the wait, so a healthy long turn is
	// never cut by wall clock. Only a silent adapter fails the call.
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
			delete(c.wait, id)
			c.mu.Unlock()
			return nil, fmt.Errorf("acp %s timed out after %s", method, timeout)
		}
	}
}

func (c *acpProcessConn) setCallbacks(cb acpCallbacks) {
	c.mu.Lock()
	c.cb = cb
	c.mu.Unlock()
}

func (c *acpProcessConn) Initialize() error {
	raw, err := c.call("initialize", map[string]any{
		"protocolVersion": 1,
		"clientCapabilities": map[string]any{
			"fs":       map[string]any{"readTextFile": true, "writeTextFile": true},
			"terminal": false,
		},
	})
	if err != nil {
		return err
	}
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

// SetModel picks the model through the adapter's model config option.
func (c *acpProcessConn) SetModel(acpID, model string) error {
	_, err := c.call("session/set_config_option", map[string]any{
		"sessionId": acpID, "configId": "model", "value": model,
	})
	return err
}

// acpThoughtLevelSetter is the optional seam to pick the Session's thinking
// level after session/new, for adapters that take it as a config option
// (pi-acp: thought_level).
type acpThoughtLevelSetter interface {
	SetThoughtLevel(acpID, level string) error
}

// SetThoughtLevel picks the thinking level through the adapter's thought
// level config option.
func (c *acpProcessConn) SetThoughtLevel(acpID, level string) error {
	_, err := c.call("session/set_config_option", map[string]any{
		"sessionId": acpID, "configId": "thought_level", "value": level,
	})
	return err
}

func (c *acpProcessConn) LoadSession(acpID, cwd string, mcp []acpMCPServer, meta map[string]any) error {
	_, err := c.call("session/load", map[string]any{
		"sessionId": acpID, "cwd": cwd, "mcpServers": mcpParams(mcp), "_meta": meta,
	})
	return err
}
func (c *acpProcessConn) Prompt(acpID, text string, cb acpCallbacks) (string, error) {
	return c.promptBlocks(acpID, []any{map[string]any{"type": "text", "text": text}}, cb)
}

// acpImagePrompter is the optional conn seam for prompts with attachments.
type acpImagePrompter interface {
	PromptImages(acpID, text string, images []Image, cb acpCallbacks) (string, error)
}

// PromptImages sends the text, then one ACP image block per attachment.
func (c *acpProcessConn) PromptImages(acpID, text string, images []Image, cb acpCallbacks) (string, error) {
	blocks := []any{}
	if text != "" {
		blocks = append(blocks, map[string]any{"type": "text", "text": text})
	}
	for _, img := range images {
		blocks = append(blocks, map[string]any{"type": "image", "mimeType": strings.ToLower(img.Mime), "data": img.Data})
	}
	return c.promptBlocks(acpID, blocks, cb)
}

func (c *acpProcessConn) promptBlocks(acpID string, blocks []any, cb acpCallbacks) (string, error) {
	c.setCallbacks(cb)
	timeout := c.promptTimeout
	if timeout <= 0 {
		timeout = 15 * time.Minute
	}
	raw, err := c.callTimeout("session/prompt", map[string]any{
		"sessionId": acpID,
		"prompt":    blocks,
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

// SteeringSupported reports whether the adapter advertised _session/steering
// in its initialize reply.
func (c *acpProcessConn) SteeringSupported() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.steering
}

// Steer injects text into the turn that is running. The adapter answers
// injected, or promptRequired when no turn runs; promptRequired is opted into
// so the adapter never starts a detached turn the Daemon cannot track.
func (c *acpProcessConn) Steer(acpID, text string) (bool, error) {
	return c.SteerImages(acpID, text, nil)
}

// SteerImages is Steer with one ACP image block per attachment.
func (c *acpProcessConn) SteerImages(acpID, text string, images []Image) (bool, error) {
	blocks := []any{map[string]any{"type": "text", "text": text}}
	for _, img := range images {
		blocks = append(blocks, map[string]any{"type": "image", "mimeType": strings.ToLower(img.Mime), "data": img.Data})
	}
	raw, err := c.call("_session/steering", map[string]any{
		"sessionId": acpID,
		"prompt":    blocks,
		"_meta":     map[string]any{"steering": map[string]any{"idleBehavior": "promptRequired"}},
	})
	if err != nil {
		return false, err
	}
	var resp struct {
		Outcome string `json:"outcome"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return false, err
	}
	return resp.Outcome == "injected", nil
}

// Cancel sends the session/cancel notification (no id, no reply).
func (c *acpProcessConn) Cancel(acpID string) error {
	return c.writeRaw(map[string]any{"jsonrpc": "2.0", "method": "session/cancel",
		"params": map[string]any{"sessionId": acpID}})
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

type trackedTool struct {
	Name, Input, Output string
	Done                bool
}

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

// closeOpen ends every tool call that never reported a terminal status, so
// a cancelled or failed turn leaves no spinner running in the transcript.
func (tr *toolTracker) closeOpen(emit func(kind, text, status string)) {
	if tr == nil {
		return
	}
	for id, t := range tr.calls {
		if t.Done {
			continue
		}
		t.Done = true
		emit("tool_call_updated", t.json(id, "failed"), "failed")
	}
}

func (t *trackedTool) json(id, status string) string {
	return ToolCall{ID: id, Name: t.Name, Input: t.Input, Output: t.Output, Status: status}.Text()
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

// sessionMeta is the _meta for session/new and session/load: the plain-adapter
// hook when set, the claude stripping set otherwise. The owner's global file
// comes from dirs.pig. Back-compat loads use this (no role extra); agent
// Sessions use sessionMetaFor.
func (b *ClaudeBackend) sessionMeta(daemonID, model, effort, workDir, role string, dirs ownerDirs) map[string]any {
	if b.metaHook != nil {
		return b.metaHook(daemonID, model, effort, workDir, role, dirs, "")
	}
	return b.strippingMetaEffort(model, effort, workDir, dirs.pig)
}

// sessionMetaFor is sessionMeta for one agent record: the role's memory/DOX
// extra (prompts.go) rides the system prompt. Reviewers get the reviewer
// extra (judge-only plus the submit tool) while their el role is run-worker
// for the submit tool.
func (b *ClaudeBackend) sessionMetaFor(meta SessionMeta, dirs ownerDirs) map[string]any {
	extra := rolePromptFor(meta)
	role := elRoleFor(meta)
	if b.metaHook != nil {
		return b.metaHook(meta.ID, meta.Model, meta.Effort, meta.WorkDir, role, dirs, extra)
	}
	return b.strippingMetaEffortExtra(meta.Model, meta.Effort, meta.WorkDir, dirs.pig, extra)
}

// wrapPrompt applies the per-turn prompt rewrite, if any.
func (b *ClaudeBackend) wrapPrompt(text string) string {
	if b.promptWrap == nil {
		return text
	}
	return b.promptWrap(text)
}

// acpMCPWaiter is the optional seam of an acpConn that reports when the
// adapter has connected its MCP servers.
type acpMCPWaiter interface {
	WaitMCPReady(timeout time.Duration) error
}

// mcpReadyTimeout bounds the wait for the adapter's MCP servers.
const mcpReadyTimeout = 45 * time.Second

// WaitMCPReady blocks until the adapter's MCP-initialized notification.
func (c *acpProcessConn) WaitMCPReady(timeout time.Duration) error {
	select {
	case <-c.mcpReady:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("adapter MCP servers not ready after %s", timeout)
	}
}

// SetConfigOption sets one ACP session config option by id; the value is a
// plain string.
func (c *acpProcessConn) SetConfigOption(acpID, configID, value string) error {
	_, err := c.call("session/set_config_option", map[string]any{
		"sessionId": acpID, "configId": configID, "value": value,
	})
	return err
}

// afterStart finishes a start or load: it picks the model, then waits for
// the adapter's MCP servers when the Backend needs that.
func (b *ClaudeBackend) afterStart(conn acpConn, acpID, model string) error {
	if err := b.applyModel(conn, acpID, model); err != nil {
		return err
	}
	if b.waitMCP {
		if w, ok := conn.(acpMCPWaiter); ok {
			if err := w.WaitMCPReady(mcpReadyTimeout); err != nil {
				return err
			}
		}
	}
	return nil
}
