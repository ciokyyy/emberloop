package web

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
}

// Title names a Session through the pig SDK, as the SDK Backend does.
func (b *PigACPBackend) Title(model, prompt string) (string, error) {
	return b.sdk.Title(model, prompt)
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
	if err := b.StartSession(meta); err != nil {
		return err
	}
	if err := b.applyPigEffort(meta.ID, meta.Effort); err != nil {
		b.DropSession(meta.ID)
		return err
	}
	return nil
}

// ResumeSession reloads the ACP session and re-applies its thinking level.
// The role comes from the record, so the new adapter keeps the same
// read-only tool allowlist.
func (b *PigACPBackend) ResumeSession(meta SessionMeta, emit func(kind, text, status string)) error {
	if err := b.ClaudeBackend.ResumeSession(meta, emit); err != nil {
		return err
	}
	if err := b.applyPigEffort(meta.ID, meta.Effort); err != nil {
		b.DropSession(meta.ID)
		return err
	}
	return nil
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

// steerSock is the shim's steer socket. It lives in the temp dir: unix
// socket paths are short, and the state dir path can be long.
func (b *PigACPBackend) steerSock(sessionID string) string {
	id := sessionID
	if len(id) > 16 {
		id = id[:16]
	}
	return filepath.Join(os.TempDir(), "elp-"+id+".sock")
}

// Steer, FollowUp, QueuedMessages and ClearQueued make the Backend a
// steerBackend: pig's native steer and follow-up queues, reached through the
// Session's shim. A steer lands after the running tool call, as with the SDK.

// Steer queues text as a native pig steer.
func (b *PigACPBackend) Steer(sessionID, text string) {
	shimQueueCmd(b.steerSock(sessionID), "steer", text, nil)
}

// SteerImages is Steer with image attachments (imageSteerBackend).
func (b *PigACPBackend) SteerImages(sessionID, text string, images []Image) {
	shimQueueCmd(b.steerSock(sessionID), "steer", text, images)
}

// FollowUpImages is FollowUp with image attachments (imageSteerBackend).
func (b *PigACPBackend) FollowUpImages(sessionID, text string, images []Image) {
	shimQueueCmd(b.steerSock(sessionID), "follow_up", text, images)
}

// FollowUp queues text as a native pig follow-up.
func (b *PigACPBackend) FollowUp(sessionID, text string) {
	shimQueueCmd(b.steerSock(sessionID), "follow_up", text, nil)
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
func (b *PigACPBackend) connFor(sessionID, role string) (acpConn, error) {
	dir, err := b.prepareAgentDir(sessionID, role)
	if err != nil {
		return nil, err
	}
	env := append(os.Environ(),
		"PIG_CODING_AGENT_DIR="+filepath.Join(dir, "agent"),
		"PATH="+filepath.Join(dir, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"),
		"PI_SKIP_VERSION_CHECK=1",
		"EL_PIG_STEER_SOCK="+b.steerSock(sessionID),
	)
	return spawnACPConnEnv(b.runner, env)
}

// prepareAgentDir builds <dir>/agent (the user's pig agent dir by symlink,
// except settings.json and mcp.json, written fresh) and <dir>/bin/pi, a
// shim that runs pig. The shim hides the version so pi-acp prints no update
// notice, and adds the scout tool allowlist.
func (b *PigACPBackend) prepareAgentDir(sessionID, role string) (string, error) {
	dir := b.agentDir(sessionID)
	agent := filepath.Join(dir, "agent")
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(agent, 0o700); err != nil {
		return "", err
	}
	if err := os.MkdirAll(bin, 0o700); err != nil {
		return "", err
	}
	home := userPigAgentDir()
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
		shim = "#!/bin/sh\n[ \"$1\" = \"--version\" ] && exit 0\nexec " + shellQuote(b.shimBin) + " " + PigShimMode + " " + shellQuote(b.pigBin) + " --no-builtin-tools \"$@\"\n"
	} else {
		shim = "#!/bin/sh\n[ \"$1\" = \"--version\" ] && exit 0\nexec " + shellQuote(b.pigBin) + " --no-builtin-tools \"$@\"\n"
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
