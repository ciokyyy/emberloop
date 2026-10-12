package web

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/MichaelKinsy/PiG/ai"
)

// PigACPBackend runs pig Sessions through pi-acp, the ACP adapter for pi,
// with pig standing in for pi. It is the ClaudeBackend wire in plain mode:
// the same ACP process, resume and event mapping, none of the claude
// stripping. Each Session gets its own adapter process and its own pig agent
// directory, so the single el MCP server reaches pig bound to that Session's
// id and role.
//
// pi-acp ignores the mcpServers of session/new, so the el server goes
// through pig's own MCP config (mcp.json in the per-Session agent dir).
type PigACPBackend struct {
	*ClaudeBackend
	stateDir string
	// runner is how the adapter starts: bunx by default, npx without bun.
	runner []string
	pigBin string
	// shimBin is the Daemon binary, run in pig-shim mode as the pi pi-acp starts.
	shimBin string

	// sdk serves the stateless one-off completions (session titles), which
	// need no agent process.
	sdk *PigBackend
	// sockMu guards the per-handle steer sockets: each live handle gets a
	// unique path so a Handoff swap never shares one. Two handles sharing a
	// path collide: the new shim steals the Listen and the old shim's exit
	// removes the new shim's live socket, after which Steer dials a missing
	// path and drops messages.
	sockMu       sync.Mutex
	socks        map[string]string
	pendingSocks map[string]string
	sockGen      int
}

// Title names a Session through the pig SDK, as the SDK Backend does.
func (b *PigACPBackend) Title(dirs ownerDirs, model, prompt string) (string, error) {
	return b.sdk.Title(dirs, model, prompt)
}

// PigACPAdapterPackage is the pinned pi-acp release.
const PigACPAdapterPackage = "pi-acp@0.0.34"

// NewPigACPBackend builds the backend. stateDir holds the per-Session agent
// dirs; daemonBin is the Daemon binary (run in pig-shim mode as the pi pi-acp
// starts) and daemonAddr is the loopback Daemon the el server calls back
// (empty leaves the server unbound, with no Daemon tools).
func NewPigACPBackend(stateDir, daemonBin, daemonAddr string) (*PigACPBackend, error) {
	pigBin, err := exec.LookPath("pig")
	if err != nil {
		return nil, fmt.Errorf("pig not on PATH: %w", err)
	}
	runner := []string{"bunx", PigACPAdapterPackage}
	if _, err := exec.LookPath("bunx"); err != nil {
		runner = []string{"npx", "-y", PigACPAdapterPackage}
	}
	if _, err := exec.LookPath("el-mcp"); err != nil {
		return nil, fmt.Errorf("el-mcp not on PATH: %w", err)
	}
	cb := &ClaudeBackend{plain: true, mcpName: "el", mcpCommand: "el-mcp", adapterCommand: runner, sessions: map[string]*claudeState{}}
	cb.daemonAddr = daemonAddr
	// Fallback usage sizes from the pig catalog window: pig windows are far
	// smaller than claude's, so the claude default would under-read the meter.
	cb.windowFor = func(model string) int {
		if w := catalogWindow(model); w > 0 {
			return w
		}
		return defaultSummaryWindow
	}
	b := &PigACPBackend{ClaudeBackend: cb, stateDir: stateDir, runner: runner, pigBin: pigBin, shimBin: daemonBin, sdk: NewPigBackend()}
	cb.newConnFor = b.connFor
	return b, nil
}

// Models lists the pig SDK catalog filtered to providers with usable auth,
// the same list the pig CLI offers to a logged-in user.
func (b *PigACPBackend) Models() []string { return PigAvailableModels("") }

// CreateSession validates the model and effort against the catalog, then
// starts the adapter and the ACP session. Unknown models and efforts are a
// 400 like the SDK path. A thinking level the adapter refuses fails the
// start with no Session left behind.
func (b *PigACPBackend) CreateSession(meta SessionMeta, emit func(kind, text, status string)) error {
	if _, ok := ai.LookupModel(meta.Model); !ok {
		return fmt.Errorf("pig: unknown model %q", meta.Model)
	}
	if !validPigEffort(meta.Effort) {
		return fmt.Errorf("pig: unknown effort %q (want off, minimal, low, medium, high, xhigh or max)", meta.Effort)
	}
	sock := b.allocSock(meta.ID)
	b.setPendingSock(meta.ID, sock)
	if err := b.StartSession(meta); err != nil {
		return err
	}
	b.claimSock(meta.ID, sock)
	if err := b.applyPigEffort(meta.ID, meta.Effort); err != nil {
		b.DropSession(meta.ID)
		return err
	}
	return nil
}

// ResumeSession reloads the ACP session and re-applies its thinking level.
// The role comes from the record, so the new adapter keeps the same
// read-only tool allowlist. The resumed handle gets a fresh steer socket:
// the released handle's shim is gone.
func (b *PigACPBackend) ResumeSession(meta SessionMeta, emit func(kind, text, status string)) error {
	sock := b.allocSock(meta.ID)
	b.setPendingSock(meta.ID, sock)
	if err := b.ClaudeBackend.ResumeSession(meta, emit); err != nil {
		return err
	}
	b.claimSock(meta.ID, sock)
	if err := b.applyPigEffort(meta.ID, meta.Effort); err != nil {
		b.DropSession(meta.ID)
		return err
	}
	return nil
}

// StartHandle starts a fresh ACP handle for the Session and returns its
// resume token. It implements handoffBackend: Handoff compress swaps to the
// new handle and releases the old one once the Handoff turn starts cleanly.
// The new ACP session is ready, with the MCP tools, before the Handoff prompt
// is sent: StartHandle returns only after session/new answers. The model and
// thinking level come from the Session record, so they survive the swap. The
// old handle stays alive until the returned release runs, so a failed swap
// leaves it untouched. Each handle gets its own steer socket (see socks), so
// Steer never crosses handles; queued native messages move to the new handle
// as follow-ups, degrading held steers per the swap contract.
func (b *PigACPBackend) StartHandle(meta SessionMeta, emit func(kind, text, status string)) (string, func(), error) {
	_ = emit
	if _, ok := ai.LookupModel(meta.Model); !ok {
		return "", nil, fmt.Errorf("pig: unknown model %q", meta.Model)
	}
	if !validPigEffort(meta.Effort) {
		return "", nil, fmt.Errorf("pig: unknown effort %q (want off, minimal, low, medium, high, xhigh or max)", meta.Effort)
	}
	oldSock := b.steerSock(meta.ID)
	oldQueue := readSockQueue(oldSock)
	newSock := b.allocSock(meta.ID)
	b.setPendingSock(meta.ID, newSock)
	role := elRoleFor(meta)
	mcp, err := b.mcpServersFor(meta.ID, role)
	if err != nil {
		return "", nil, err
	}
	newConn := b.connFactory(meta.ID, role, meta.Dirs)
	conn, err := newConn()
	if err != nil {
		return "", nil, fmt.Errorf("start pig adapter: %w", err)
	}
	b.applyTimeouts(conn)
	if err := conn.Initialize(); err != nil {
		_ = conn.Close()
		return "", nil, fmt.Errorf("acp initialize: %w", err)
	}
	acpID, err := conn.NewSession(meta.WorkDir, mcp, b.sessionMetaFor(meta, meta.Dirs))
	if err != nil {
		_ = conn.Close()
		return "", nil, fmt.Errorf("acp session/new: %w", err)
	}
	if err := b.afterStart(conn, acpID, meta.Model); err != nil {
		_ = conn.Close()
		return "", nil, err
	}
	fresh := &claudeState{conn: conn, acpID: acpID, ready: make(chan struct{})}
	fresh.signalReady()
	if ts, ok := conn.(acpThoughtLevelSetter); ok && meta.Effort != "" {
		if err := ts.SetThoughtLevel(acpID, meta.Effort); err != nil {
			_ = conn.Close()
			return "", nil, fmt.Errorf("pig: set thinking level %q: %w", meta.Effort, err)
		}
	}
	b.claimSock(meta.ID, newSock)
	b.mu.Lock()
	old, hadOld := b.sessions[meta.ID]
	b.sessions[meta.ID] = fresh
	b.mu.Unlock()
	carrySockQueue(newSock, oldQueue)
	var release func()
	if hadOld && old != nil {
		oldConn := old.conn
		release = func() { _ = oldConn.Close(); old.signalReady() }
	} else {
		release = func() {}
	}
	return acpID, release, nil
}

// readSockQueue reads the native pig queue behind one steer socket. A missing
// or unreachable shim reports empty, so a Handoff with no queued messages
// carries nothing.
func readSockQueue(sock string) pigQueue {
	var resp struct {
		Data pigQueue `json:"data"`
	}
	if !shimCall(sock, map[string]any{"op": "queue"}, &resp) {
		return pigQueue{}
	}
	return resp.Data
}

// carrySockQueue moves queued native messages to the new handle's shim as
// follow-ups. Held steers degrade per the swap contract: the Handoff leads,
// so nothing steers the fresh turn.
func carrySockQueue(newSock string, q pigQueue) {
	for _, text := range q.Steering {
		shimQueueCmd(newSock, "follow_up", text, nil)
	}
	for _, text := range q.FollowUp {
		shimQueueCmd(newSock, "follow_up", text, nil)
	}
}

// CloseSession releases the adapter process and forgets the Session's steer
// socket mapping. The shim removes its own socket file on exit.
func (b *PigACPBackend) CloseSession(sessionID string) {
	b.dropSock(sessionID)
	b.ClaudeBackend.CloseSession(sessionID)
}

// DropSession releases a Session id without reporting. See CloseSession for
// the socket cleanup.
func (b *PigACPBackend) DropSession(id string) {
	b.dropSock(id)
	b.ClaudeBackend.DropSession(id)
}

// applyPigEffort picks the Session's thinking level through the adapter's
// thought_level config option. An empty effort keeps the model default;
// adapters without the option keep it too.
func (b *PigACPBackend) applyPigEffort(sessionID, effort string) error {
	if effort == "" {
		return nil
	}
	b.mu.Lock()
	st, ok := b.sessions[sessionID]
	b.mu.Unlock()
	if !ok {
		return fmt.Errorf("pig: session %s not started", sessionID)
	}
	ts, ok := st.conn.(acpThoughtLevelSetter)
	if !ok {
		return nil
	}
	if err := ts.SetThoughtLevel(st.acpID, effort); err != nil {
		return fmt.Errorf("pig: set thinking level %q: %w", effort, err)
	}
	return nil
}

// legacySteerSock is the pre-Handoff socket path, kept as the fallback before
// any handle allocates its own.
func legacySteerSock(sessionID string) string {
	id := sessionID
	if len(id) > 16 {
		id = id[:16]
	}
	return filepath.Join(os.TempDir(), "elp-"+id+".sock")
}

// steerSock is the live handle's steer socket. Each handle gets a unique path
// (see allocSock); the map holds the current one. Before any handle exists it
// falls back to the legacy path.
func (b *PigACPBackend) steerSock(sessionID string) string {
	b.sockMu.Lock()
	defer b.sockMu.Unlock()
	if s, ok := b.socks[sessionID]; ok && s != "" {
		return s
	}
	return legacySteerSock(sessionID)
}

// allocSock mints a unique steer socket path for one handle start. The caller
// stores it via claimSock on success; a failed start leaves the current
// mapping untouched so Steer still reaches the old handle.
func (b *PigACPBackend) allocSock(sessionID string) string {
	b.sockMu.Lock()
	defer b.sockMu.Unlock()
	b.sockGen++
	id := sessionID
	if len(id) > 16 {
		id = id[:16]
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("elp-%s-%d.sock", id, b.sockGen))
}

// claimSock makes path the current steer socket for the Session and clears
// any staged pending socket for it.
func (b *PigACPBackend) claimSock(sessionID, path string) {
	b.sockMu.Lock()
	defer b.sockMu.Unlock()
	if b.socks == nil {
		b.socks = map[string]string{}
	}
	b.socks[sessionID] = path
	if b.pendingSocks != nil {
		delete(b.pendingSocks, sessionID)
	}
}

// dropSock forgets the Session's steer socket mapping. The shim removes its
// own socket file on exit; this only drops the pointer.
func (b *PigACPBackend) dropSock(sessionID string) {
	b.sockMu.Lock()
	defer b.sockMu.Unlock()
	delete(b.socks, sessionID)
	if b.pendingSocks != nil {
		delete(b.pendingSocks, sessionID)
	}
}

// takePendingSock takes the pre-allocated socket for one handle start, when
// present.
func (b *PigACPBackend) takePendingSock(sessionID string) (string, bool) {
	b.sockMu.Lock()
	defer b.sockMu.Unlock()
	if b.pendingSocks == nil {
		return "", false
	}
	s, ok := b.pendingSocks[sessionID]
	if ok {
		delete(b.pendingSocks, sessionID)
	}
	return s, ok
}

// setPendingSock stages the socket one handle start will use. connFor takes
// it when the adapter starts.
func (b *PigACPBackend) setPendingSock(sessionID, path string) {
	b.sockMu.Lock()
	defer b.sockMu.Unlock()
	if b.pendingSocks == nil {
		b.pendingSocks = map[string]string{}
	}
	b.pendingSocks[sessionID] = path
}

// Steer, FollowUp, QueuedMessages and ClearQueued make the Backend a
// steerBackend: pig's native steer and follow-up queues, reached through the
// Session's shim. A steer lands after the running tool call, as with the SDK.

// Steer queues text as a native pig steer. It reports whether pig queued it;
// false means the shim was unreachable and the Daemon must hold the message
// itself instead of assuming it queued.
func (b *PigACPBackend) Steer(sessionID, text string) bool {
	return shimQueueCmd(b.steerSock(sessionID), "steer", text, nil)
}

// SteerImages is Steer with image attachments (imageSteerBackend).
func (b *PigACPBackend) SteerImages(sessionID, text string, images []Image) bool {
	return shimQueueCmd(b.steerSock(sessionID), "steer", text, images)
}

// FollowUpImages is FollowUp with image attachments (imageSteerBackend).
func (b *PigACPBackend) FollowUpImages(sessionID, text string, images []Image) bool {
	return shimQueueCmd(b.steerSock(sessionID), "follow_up", text, images)
}

// FollowUp queues text as a native pig follow-up. See Steer for the return.
func (b *PigACPBackend) FollowUp(sessionID, text string) bool {
	return shimQueueCmd(b.steerSock(sessionID), "follow_up", text, nil)
}

// QueuedMessages lists the steers and follow-ups pig has not delivered yet.
func (b *PigACPBackend) QueuedMessages(sessionID string) []QueuedMessage {
	var resp struct {
		Data pigQueue `json:"data"`
	}
	if !shimCall(b.steerSock(sessionID), map[string]any{"op": "queue"}, &resp) {
		return nil
	}
	var out []QueuedMessage
	for _, t := range resp.Data.Steering {
		out = append(out, QueuedMessage{Mode: "steer", Text: t})
	}
	for _, t := range resp.Data.FollowUp {
		out = append(out, QueuedMessage{Mode: "followup", Text: t})
	}
	return out
}

// ClearQueued drops everything pig has queued and reports how many.
func (b *PigACPBackend) ClearQueued(sessionID string) int {
	var resp struct {
		Data pigQueue `json:"data"`
	}
	if !shimCall(b.steerSock(sessionID), map[string]any{"op": "clear_queue"}, &resp) {
		return 0
	}
	return len(resp.Data.Steering) + len(resp.Data.FollowUp)
}

// agentDir is the per-Session pig agent dir root.
func (b *PigACPBackend) agentDir(sessionID string) string {
	return filepath.Join(b.stateDir, "enderbloom", "pig-acp", sessionID)
}

// connFor prepares the Session's agent dir and starts its adapter process.
// Each handle gets its own steer socket: a staged pending socket when
// CreateSession or StartHandle pre-allocated one, else a fresh unique path.
// The socket is claimed as current only after the caller confirms the handle
// started; connFor stages it so a failed start never steals Steer from the
// old handle.
func (b *PigACPBackend) connFor(sessionID, role string, dirs ownerDirs) (acpConn, error) {
	dir, err := b.prepareAgentDir(sessionID, role, dirs)
	if err != nil {
		return nil, err
	}
	sock, ok := b.takePendingSock(sessionID)
	if !ok {
		sock = b.allocSock(sessionID)
	}
	env := append(os.Environ(),
		"PIG_CODING_AGENT_DIR="+filepath.Join(dir, "agent"),
		"PATH="+filepath.Join(dir, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"),
		"PI_SKIP_VERSION_CHECK=1",
		"EL_PIG_STEER_SOCK="+sock,
	)
	conn, err := spawnACPConnEnv(b.runner, env)
	if err != nil {
		return nil, err
	}
	b.setPendingSock(sessionID, sock)
	return conn, nil
}

// prepareAgentDir builds <dir>/agent (links to the owner's pig agent dir,
// except settings.json and mcp.json: settings.json is the owner's copy, mcp.json
// is written fresh) and <dir>/bin/pi, a shim that runs pig. The shim hides the
// version so pi-acp prints no update notice, adds the scout tool allowlist and
// turns off ambient skills and prompt templates, which pig reads from HOME.
func (b *PigACPBackend) prepareAgentDir(sessionID, role string, dirs ownerDirs) (string, error) {
	dir := b.agentDir(sessionID)
	agent := filepath.Join(dir, "agent")
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(agent, 0o700); err != nil {
		return "", err
	}
	if err := os.MkdirAll(bin, 0o700); err != nil {
		return "", err
	}
	home := dirs.pig
	if home == "" {
		home = userPigAgentDir()
	}
	if entries, err := os.ReadDir(home); err == nil {
		for _, e := range entries {
			name := e.Name()
			if name == "mcp.json" || name == "settings.json" {
				continue
			}
			if name == "extensions" {
				linkExtensions(filepath.Join(home, name), filepath.Join(agent, name))
				continue
			}
			link := filepath.Join(agent, name)
			if _, err := os.Lstat(link); err == nil {
				continue
			}
			_ = os.Symlink(filepath.Join(home, name), link)
		}
	}
	if err := os.WriteFile(filepath.Join(agent, "settings.json"), mergedPigSettings(home), 0o600); err != nil {
		return "", err
	}
	mcp, err := b.mergedMCP(sessionID, role)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(agent, "mcp.json"), mcp, 0o600); err != nil {
		return "", err
	}
	// pig runs with its own builtin tools off: the el MCP server (role web,
	// or the read-only scout role) supplies them, as it does for claude, so
	// sudo gating, image reads and the scout limit hold for both harnesses.
	var shim string
	if b.shimBin != "" {
		shim = "#!/bin/sh\n[ \"$1\" = \"--version\" ] && exit 0\nexec " + shellQuote(b.shimBin) + " " + PigShimMode + " " + shellQuote(b.pigBin) + " --no-builtin-tools --no-skills --no-prompt-templates \"$@\"\n"
	} else {
		shim = "#!/bin/sh\n[ \"$1\" = \"--version\" ] && exit 0\nexec " + shellQuote(b.pigBin) + " --no-builtin-tools --no-skills --no-prompt-templates \"$@\"\n"
	}
	if err := os.WriteFile(filepath.Join(bin, "pi"), []byte(shim), 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// herdrOnlyExtensions are pig extensions that drive herdr panes. A Daemon
// Session is not a herdr pane, so they stay out of its agent dir.
var herdrOnlyExtensions = map[string]bool{"herdr-report": true}

// linkExtensions links the user's pig extensions into the Session's agent
// dir, minus the herdr ones.
func linkExtensions(src, dst string) {
	_ = os.MkdirAll(dst, 0o700)
	entries, err := os.ReadDir(src)
	if err != nil {
		return
	}
	for _, e := range entries {
		if herdrOnlyExtensions[e.Name()] {
			continue
		}
		link := filepath.Join(dst, e.Name())
		if _, err := os.Lstat(link); err == nil {
			continue
		}
		_ = os.Symlink(filepath.Join(src, e.Name()), link)
	}
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func userPigAgentDir() string {
	if d := os.Getenv("PIG_CODING_AGENT_DIR"); d != "" {
		return d
	}
	if h := os.Getenv("PIG_HOME"); h != "" {
		return filepath.Join(h, "agent")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".pig", "agent")
}

// mergedPigSettings is the user's settings.json with quietStartup on, so
// pi-acp sends no startup banner into the transcript.
func mergedPigSettings(home string) []byte {
	m := map[string]any{}
	if raw, err := os.ReadFile(filepath.Join(home, "settings.json")); err == nil {
		_ = json.Unmarshal(raw, &m)
	}
	m["quietStartup"] = true
	// Compression is off for pig Sessions over ACP for now: the Daemon has no
	// compaction hook there, and pig must not compact behind its back.
	m["compaction"] = map[string]any{"enabled": false}
	out, _ := json.MarshalIndent(m, "", "  ")
	return out
}

// mergedMCP is the Session's mcp.json: the single el server a claude Session
// gets (the el tools under the Session's role, Daemon tools included), with
// direct exposure so the model sees the tools without codemode. The user's
// own mcp.json is not merged: its el server carries the herdr spawn, tell and
// finish tools, which would run panes instead of Daemon Subagents.
func (b *PigACPBackend) mergedMCP(sessionID, role string) ([]byte, error) {
	b.mu.Lock()
	servers, err := b.mcpServersFor(sessionID, role)
	b.mu.Unlock()
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	for _, sv := range servers {
		env := map[string]string{}
		for _, kv := range sv.Env {
			env[kv[0]] = kv[1]
		}
		out[sv.Name] = map[string]any{
			"command":  sv.Command,
			"args":     sv.Args,
			"env":      env,
			"exposure": "direct",
			"timeout":  600,
		}
	}
	return json.MarshalIndent(map[string]any{"mcpServers": out}, "", "  ")
}
