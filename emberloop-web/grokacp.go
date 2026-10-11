package web

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	el "example.com/emberloop"
)

// grokDefaultModel is the model the Grok CLI offers when `grok models` cannot
// be read.
const grokDefaultModel = "grok-4.7"

// grokEfforts are the effort levels a Grok Session accepts. The agent's model
// advertises low to xhigh; the UI's max maps to xhigh.
var grokEfforts = []string{"low", "medium", "high", "xhigh", "max"}

func validGrokEffort(e string) bool {
	if e == "" {
		return true
	}
	for _, v := range grokEfforts {
		if v == e {
			return true
		}
	}
	return false
}

// grokConfigTOML is the per-Session config.toml of the isolated GROK_HOME. It
// turns off everything that is not an el MCP tool: subagents, memory, media
// and web tools, the question tool, and every Claude-compat scan, so no
// user skills, hooks, rules, agents or MCP servers leak in.
const grokConfigTOML = `[subagents]
enabled = false

[memory]
enabled = false

[memory_v2]
enabled = false

[features]
image_gen = false
web_fetch = false
ask_user_question = false
backend_tools = false

[compat.claude]
agents = false
hooks = false
mcps = false
rules = false
skills = false
`

// grokProfile is the agent profile that strips the built-in toolset. Grok
// reaches MCP tools only through use_tool (search_tool finds them), and
// `agent stdio` ignores the --tools flag, so the profile's tools list is the
// allowlist. search_tool stays only when no catalog was rendered.
func grokProfile(withSearch bool) string {
	tools := "  - use_tool\n"
	if withSearch {
		tools += "  - search_tool\n"
	}
	return "---\nname: el\ndescription: el stripped profile\nagents_md: false\ntools:\n" + tools + "---\nEl stripped profile.\n"
}

// grokMCPTool is one tool from an MCP tools/list reply.
type grokMCPTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// fetchMCPTools starts one MCP server, lists its tools and stops it.
func fetchMCPTools(srv acpMCPServer) ([]grokMCPTool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, srv.Command, srv.Args...)
	cmd.Env = os.Environ()
	for _, kv := range srv.Env {
		cmd.Env = append(cmd.Env, kv[0]+"="+kv[1])
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	defer func() {
		_ = stdin.Close()
		cancel()
		_ = cmd.Wait()
	}()
	rd := bufio.NewReader(stdout)
	call := func(id int, method string, params any) (json.RawMessage, error) {
		b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
		if _, err := stdin.Write(append(b, '\n')); err != nil {
			return nil, err
		}
		for {
			line, err := rd.ReadBytes('\n')
			if err != nil {
				return nil, err
			}
			var m struct {
				ID     *int            `json:"id"`
				Result json.RawMessage `json:"result"`
				Error  *struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			if json.Unmarshal(line, &m) != nil || m.ID == nil || *m.ID != id {
				continue
			}
			if m.Error != nil {
				return nil, errors.New(m.Error.Message)
			}
			return m.Result, nil
		}
	}
	if _, err := call(1, "initialize", map[string]any{
		"protocolVersion": "2024-11-05", "capabilities": map[string]any{},
		"clientInfo": map[string]any{"name": "el-web", "version": "0"},
	}); err != nil {
		return nil, fmt.Errorf("mcp initialize: %w", err)
	}
	note, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	if _, err := stdin.Write(append(note, '\n')); err != nil {
		return nil, err
	}
	raw, err := call(2, "tools/list", map[string]any{})
	if err != nil {
		return nil, fmt.Errorf("mcp tools/list: %w", err)
	}
	var resp struct {
		Tools []grokMCPTool `json:"tools"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	return resp.Tools, nil
}

// schemaTypeName is the short type label of one JSON-schema property.
func schemaTypeName(p map[string]any) string {
	var t string
	switch v := p["type"].(type) {
	case string:
		t = v
	case []any:
		for _, x := range v {
			if s, _ := x.(string); s != "" && s != "null" {
				t = s
				break
			}
		}
	}
	switch t {
	case "string":
		return "str"
	case "integer":
		return "int"
	case "number":
		return "num"
	case "boolean":
		return "bool"
	case "object":
		return "obj"
	case "array":
		if items, ok := p["items"].(map[string]any); ok {
			return schemaTypeName(items) + "[]"
		}
		return "any[]"
	}
	return "any"
}

// firstSentence cuts a tool description to its first sentence, at most 110
// characters.
func firstSentence(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "\n"); i >= 0 {
		s = s[:i]
	}
	if i := strings.Index(s, ". "); i >= 0 {
		s = s[:i+1]
	}
	if r := []rune(s); len(r) > 110 {
		s = string(r[:109]) + "…"
	}
	return s
}

// renderGrokCatalog renders the compact tool catalog the system prompt
// carries: one line per tool, qualified name, typed arguments (optional ones
// marked ?) and the first sentence of the description. Grok then calls
// use_tool directly and never has to search for a schema.
func renderGrokCatalog(server string, tools []grokMCPTool) string {
	var sb strings.Builder
	for _, t := range tools {
		props, _ := t.InputSchema["properties"].(map[string]any)
		required := map[string]bool{}
		var reqOrder []string
		if rs, ok := t.InputSchema["required"].([]any); ok {
			for _, r := range rs {
				if s, _ := r.(string); s != "" {
					required[s] = true
					reqOrder = append(reqOrder, s)
				}
			}
		}
		var optional []string
		for name := range props {
			if !required[name] {
				optional = append(optional, name)
			}
		}
		sort.Strings(optional)
		var args []string
		for _, name := range reqOrder {
			p, _ := props[name].(map[string]any)
			args = append(args, name+":"+schemaTypeName(p))
		}
		for _, name := range optional {
			p, _ := props[name].(map[string]any)
			args = append(args, name+"?:"+schemaTypeName(p))
		}
		sb.WriteString("- " + server + "__" + t.Name + "(" + strings.Join(args, ", ") + ")")
		if d := firstSentence(t.Description); d != "" {
			sb.WriteString(": " + d)
		}
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// grokToolBlock is the tool section of the system prompt.
func grokToolBlock(catalog string) string {
	if catalog == "" {
		return "Your only tool for the el MCP tools is use_tool; find them with search_tool first, then call use_tool with the qualified tool_name and a tool_input object."
	}
	return "Your only tool is use_tool, which calls the el MCP tools below. Call it as use_tool {\"tool_name\": \"<qualified name>\", \"tool_input\": {<arguments>}}. " +
		"Never call search_tool: this catalog is complete (name(arguments): purpose, ? marks an optional argument).\n\n" + catalog
}

// grokPromptWrap appends the per-turn register reminder to a prompt. ACP has
// no hook, so the reminder rides in the message the model reads; the stored
// prompt Event keeps the user's own text.
func grokPromptWrap(text string) string {
	return text + "\n\n<reminder>" + el.VoiceReminder() + "</reminder>"
}

// GrokACPBackend runs Grok Build Sessions through `grok agent stdio`, the
// CLI's own ACP server, on the subscription login. It is the ClaudeBackend
// wire in plain mode. Each Session gets its own adapter process and its own
// temp HOME and GROK_HOME (a copy of auth.json, a stripping config.toml and
// profile), so nothing of the user's Grok or Claude setup reaches the model:
// the one el MCP server is the only tool source. Steer is not native, so a
// Steer degrades to a Follow-up in the Daemon.
type GrokACPBackend struct {
	*ClaudeBackend
	stateDir string
	grokBin  string
	// srcHome is the user's GROK_HOME (default ~/.grok); auth.json is
	// copied from and refreshed tokens are written back to it.
	srcHome string

	gmu      sync.Mutex
	catalogs map[string]string
	models   []string
}

// NewGrokACPBackend finds the grok and el-mcp binaries. It fails when either
// is missing so the Daemon leaves the grok harness unregistered.
func NewGrokACPBackend(stateDir, daemonAddr string) (*GrokACPBackend, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	srcHome := os.Getenv("GROK_HOME")
	if srcHome == "" {
		srcHome = filepath.Join(home, ".grok")
	}
	grokBin, err := exec.LookPath("grok")
	if err != nil {
		cand := filepath.Join(srcHome, "bin", "grok")
		if _, serr := os.Stat(cand); serr != nil {
			return nil, fmt.Errorf("grok not on PATH: %w", err)
		}
		grokBin = cand
	}
	if _, err := exec.LookPath("el-mcp"); err != nil {
		return nil, fmt.Errorf("el-mcp not on PATH: %w", err)
	}
	cb := &ClaudeBackend{
		plain: true, waitMCP: true,
		mcpName: "el", mcpCommand: "el-mcp",
		mcpEnv:   [][2]string{{"HOME", home}},
		sessions: map[string]*claudeState{},
	}
	cb.daemonAddr = daemonAddr
	b := &GrokACPBackend{
		ClaudeBackend: cb, stateDir: stateDir, grokBin: grokBin, srcHome: srcHome,
		catalogs: map[string]string{},
	}
	cb.newConnFor = b.connFor
	cb.metaHook = b.meta
	cb.promptWrap = grokPromptWrap
	return b, nil
}

// catalogFor renders (and caches per role) the tool catalog of the el MCP
// server. A failure returns "" and caches nothing, so the Session falls back
// to search_tool.
func (b *GrokACPBackend) catalogFor(role, sessionID string) string {
	b.gmu.Lock()
	if c, ok := b.catalogs[role]; ok {
		b.gmu.Unlock()
		return c
	}
	b.gmu.Unlock()
	servers, err := b.mcpServersFor(sessionID, role)
	if err != nil || len(servers) == 0 {
		return ""
	}
	tools, err := fetchMCPTools(servers[0])
	if err != nil || len(tools) == 0 {
		return ""
	}
	c := renderGrokCatalog(servers[0].Name, tools)
	b.gmu.Lock()
	b.catalogs[role] = c
	b.gmu.Unlock()
	return c
}

// meta is the session/new and session/load _meta: the el system prompt with
// the project context and tool catalog replaces Grok's own, and the session
// auto-approves tools.
func (b *GrokACPBackend) meta(daemonID, model, effort, workDir, role string) map[string]any {
	prompt := claudeSystemPromptFor(workDir)
	prompt += "\n\n" + grokToolBlock(b.catalogFor(role, daemonID))
	return map[string]any{"systemPromptOverride": prompt, "yoloMode": true}
}

func (b *GrokACPBackend) sessionDir(id string) string {
	return filepath.Join(b.stateDir, "enderbloom", "grok-acp", id)
}

// prepareDir builds the Session's isolated HOME and GROK_HOME and returns
// the profile path. auth.json is copied fresh on every start so the newest
// login wins.
func (b *GrokACPBackend) prepareDir(id, role string) (string, error) {
	dir := b.sessionDir(id)
	grokHome := filepath.Join(dir, "grok")
	for _, d := range []string{filepath.Join(dir, "home"), grokHome} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return "", err
		}
	}
	auth, err := os.ReadFile(filepath.Join(b.srcHome, "auth.json"))
	if err != nil {
		return "", fmt.Errorf("grok is not logged in (run `grok login`): %w", err)
	}
	if err := os.WriteFile(filepath.Join(grokHome, "auth.json"), auth, 0o600); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(grokHome, "config.toml"), []byte(grokConfigTOML), 0o600); err != nil {
		return "", err
	}
	profile := filepath.Join(grokHome, "profile.md")
	withSearch := b.catalogFor(role, id) == ""
	if err := os.WriteFile(profile, []byte(grokProfile(withSearch)), 0o600); err != nil {
		return "", err
	}
	return profile, nil
}

// connFor starts one `grok agent stdio` process under the Session's temp
// HOME and GROK_HOME.
func (b *GrokACPBackend) connFor(sessionID, role string) (acpConn, error) {
	profile, err := b.prepareDir(sessionID, role)
	if err != nil {
		return nil, err
	}
	dir := b.sessionDir(sessionID)
	var env []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "HOME=") || strings.HasPrefix(kv, "GROK_HOME=") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "HOME="+filepath.Join(dir, "home"), "GROK_HOME="+filepath.Join(dir, "grok"))
	return spawnACPConnEnv([]string{b.grokBin, "agent", "--no-leader", "--agent-profile", profile, "stdio"}, env)
}

// syncAuth writes a token the agent refreshed back to the user's auth.json,
// unless that file changed since the copy. Without it a rotated refresh
// token would be lost with the Session's temp dir.
func (b *GrokACPBackend) syncAuth(id string) {
	tmp := filepath.Join(b.sessionDir(id), "grok", "auth.json")
	src := filepath.Join(b.srcHome, "auth.json")
	ti, err := os.Stat(tmp)
	if err != nil {
		return
	}
	si, err := os.Stat(src)
	if err != nil || si.ModTime().After(ti.ModTime()) {
		return
	}
	tb, err := os.ReadFile(tmp)
	if err != nil || !json.Valid(tb) {
		return
	}
	sb, err := os.ReadFile(src)
	if err != nil || string(sb) == string(tb) {
		return
	}
	next := src + ".el-sync"
	if os.WriteFile(next, tb, 0o600) != nil {
		return
	}
	if os.Rename(next, src) != nil {
		_ = os.Remove(next)
	}
}

// Models lists the models `grok models` offers, falling back to the default.
func (b *GrokACPBackend) Models() []string {
	b.gmu.Lock()
	if b.models != nil {
		m := append([]string{}, b.models...)
		b.gmu.Unlock()
		return m
	}
	b.gmu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, b.grokBin, "models").Output()
	models := []string{}
	if err == nil {
		models = parseGrokModels(string(out))
	}
	if len(models) == 0 {
		return []string{grokDefaultModel}
	}
	b.gmu.Lock()
	b.models = models
	b.gmu.Unlock()
	return append([]string{}, models...)
}

// parseGrokModels reads the model ids after "Available models:" in the
// `grok models` output; a leading "*" marks the default.
func parseGrokModels(out string) []string {
	var models []string
	in := false
	for _, line := range strings.Split(out, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "Available models") {
			in = true
			continue
		}
		if !in || t == "" {
			continue
		}
		t = strings.TrimSpace(strings.TrimPrefix(t, "*"))
		if f := strings.Fields(t); len(f) > 0 {
			models = append(models, f[0])
		}
	}
	return models
}

// CreateSession validates the model and effort, then starts the stripped
// agent and applies the effort. A failure leaves no Session behind.
func (b *GrokACPBackend) CreateSession(meta SessionMeta, emit func(kind, text, status string)) error {
	known := false
	for _, m := range b.Models() {
		if m == meta.Model {
			known = true
			break
		}
	}
	if !known {
		return fmt.Errorf("grok: unknown model %q", meta.Model)
	}
	if !validGrokEffort(meta.Effort) {
		return fmt.Errorf("grok: unknown effort %q (want low, medium, high, xhigh or max)", meta.Effort)
	}
	if err := b.StartSession(meta); err != nil {
		return err
	}
	if err := b.applyGrokEffort(meta.ID, meta.Effort); err != nil {
		b.DropSession(meta.ID)
		return err
	}
	return nil
}

// ResumeSession reloads the ACP session in a fresh agent and re-applies the
// effort. The role comes from the record, so the new agent gets the same catalog.
func (b *GrokACPBackend) ResumeSession(meta SessionMeta, emit func(kind, text, status string)) error {
	if err := b.ClaudeBackend.ResumeSession(meta, emit); err != nil {
		return err
	}
	if err := b.applyGrokEffort(meta.ID, meta.Effort); err != nil {
		b.DropSession(meta.ID)
		return err
	}
	return nil
}

// grokConfigSetter is the optional seam to set an ACP config option by id.
type grokConfigSetter interface {
	SetConfigOption(acpID, configID, value string) error
}

// applyGrokEffort sets the Session's reasoning effort. An empty effort keeps
// the model default.
func (b *GrokACPBackend) applyGrokEffort(id, effort string) error {
	if effort == "" {
		return nil
	}
	if effort == "max" {
		effort = "xhigh"
	}
	b.mu.Lock()
	st, ok := b.sessions[id]
	b.mu.Unlock()
	if !ok {
		return nil
	}
	cs, ok := st.conn.(grokConfigSetter)
	if !ok {
		return nil
	}
	if err := cs.SetConfigOption(st.acpID, "reasoning_effort", effort); err != nil {
		return fmt.Errorf("grok set effort %q: %w", effort, err)
	}
	return nil
}

// CloseSession, DropSession and ReleaseSession end the agent process, then
// save a refreshed login back to the user's auth.json.
func (b *GrokACPBackend) CloseSession(sessionID string) { b.DropSession(sessionID) }

func (b *GrokACPBackend) DropSession(id string) {
	b.ClaudeBackend.DropSession(id)
	b.syncAuth(id)
}

func (b *GrokACPBackend) ReleaseSession(sessionID string) string {
	tok := b.ClaudeBackend.ReleaseSession(sessionID)
	b.syncAuth(sessionID)
	return tok
}
