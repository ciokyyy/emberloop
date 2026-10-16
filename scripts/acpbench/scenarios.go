package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// echoPrompt is the plain-English scripted turn. cooperativeEchoPrompt adds
// the fakellm TOOL directive so scripted backends (fake LLM or --cooperate
// agents) invoke the stub deterministically; real models follow the English.
const echoPrompt = "Call the echo tool with text 'hello', then answer with the echoed text."

const cooperativeEchoPrompt = "Call the echo tool with text 'hello', then answer with the echoed text.\n" +
	`TOOL:echo:{"text":"hello"}`

const cooperativeSlowPrompt = "Call the slow tool with ms 15000, then answer done.\n" +
	`TOOL:slow:{"ms":15000}`

// echoPromptFor returns the scripted prompt: cooperative (English + TOOL
// directive) when the run targets a fake LLM or --cooperate is set.
func (b *benchCfg) echoPromptFor(slow bool) string {
	if slow {
		if b.cooperate || b.fakeLLMURL != "" {
			return cooperativeSlowPrompt
		}
		return slowPrompt
	}
	if b.cooperate || b.fakeLLMURL != "" {
		return cooperativeEchoPrompt
	}
	if b.echoDirective != "" {
		return echoPrompt + "\n" + b.echoDirective
	}
	return echoPrompt
}

// deriveBaseToolName strips MCP server namespacing: agents announce
// "acpbench-stub__echo", "mcp__acpbench-stub__echo", "stub/echo" or
// "stub.echo"; the base is the last segment.
func deriveBaseToolName(announced string) string {
	s := strings.TrimSpace(announced)
	if s == "" {
		return ""
	}
	for _, sep := range []string{"__", "/", ":", "."} {
		if i := strings.LastIndex(s, sep); i >= 0 && i+len(sep) < len(s) {
			s = s[i+len(sep):]
		}
	}
	return strings.ToLower(s)
}

// runEchoTurn runs the scripted echo prompt and reports the stop reason
// plus milliseconds to the first streamed chunk.
func runEchoTurn(c *acpConn, sid string, col *collector, timeout time.Duration, prompt string) (stop string, firstChunkMs int64, err error) {
	t0 := time.Now()
	stop, err = c.PromptBlocks(sid, []any{map[string]any{"type": "text", "text": prompt}}, timeout)
	if col != nil {
		_, _, _, _, firstAt, _ := col.snapshot()
		if !firstAt.IsZero() {
			firstChunkMs = ms(firstAt.Sub(t0))
		}
	}
	return stop, firstChunkMs, err
}

// testImage is one image attachment for the images scenario.
type testImage struct {
	Mime string
	Data string // base64
}

type benchCfg struct {
	argv             []string
	env              []string
	cwd              string
	mcpStub          bool
	mcpTransport     string // stdio | http | sse
	stubLog          string // --stub-log (default: temp file per run)
	selfExe          string
	iterations       int
	sessions         int
	cooperate        bool
	fakeLLMURL       string
	echoDirective    string
	authMethod       string // "" none, "auto", or explicit id
	metaJSON         map[string]any
	prePromptWaitMCP bool
	cancelMix        int
	faultRate        string
	fault            string

	mem    *memWatcher
	mu     sync.Mutex
	phases map[string][]memWindow

	marginal *int64

	httpStubURL string
	httpStubLog string
}

type memWindow struct {
	start, end time.Time
}

// markPhase records a memory phase window. It takes a synchronous sample
// first so even sub-sample-interval phases (a fast agent answers in
// milliseconds) contain at least one tick; the window extends past it.
func (b *benchCfg) markPhase(name string, start, end time.Time) {
	b.mem.tick()
	end = time.Now()
	b.mu.Lock()
	b.phases[name] = append(b.phases[name], memWindow{start, end})
	b.mu.Unlock()
}

func (b *benchCfg) spawnConn() (*acpConn, error) {
	c, err := spawnACP(b.argv, b.env, b.cwd)
	if err != nil {
		return nil, err
	}
	b.mem.add(c.pid)
	return c, nil
}

// mcpServers builds the session/new mcpServers value: nil (omitted
// entirely, never null) when no stub, a stdio command entry by default, or
// an http/sse URL entry when --mcp-transport demands it.
func (b *benchCfg) mcpServers() []map[string]any {
	if !b.mcpStub {
		return nil
	}
	switch b.mcpTransport {
	case "http", "sse":
		url := b.httpStubURL
		if url == "" {
			return []map[string]any{{
				"name": "acpbench-stub", "type": b.mcpTransport,
				"url": "http://127.0.0.1:9/mcp",
			}}
		}
		return []map[string]any{{
			"name": "acpbench-stub", "type": b.mcpTransport, "url": url,
		}}
	default:
		args := []string{"mcp-stub"}
		if b.stubLog != "" {
			args = append(args, "--stub-log", b.stubLog)
		}
		return []map[string]any{{
			"name": "acpbench-stub", "command": b.selfExe,
			"args": args, "env": []any{},
		}}
	}
}

// sessionMeta mirrors the Daemon's client posture in generic form: a system
// prompt plus auto-approve judgment, merged with --meta-json. Agents ignore
// unknown _meta keys.
func (b *benchCfg) sessionMeta() map[string]any {
	m := map[string]any{
		"systemPrompt": "You are a test agent driven by the acpbench probe. Follow instructions literally.",
		"acpbench":     map[string]any{"version": version, "autoApprove": true},
	}
	for k, v := range b.metaJSON {
		m[k] = v
	}
	return m
}

// stubEchoCalls counts stub-side echo invocations (server-side proof).
func (b *benchCfg) stubEchoCalls() (listSeen bool, echoCalls, totalCalls int) {
	if !b.mcpStub {
		return false, 0, 0
	}
	listSeen, calls := readStubLog(b.stubLogPath())
	return listSeen, calls["echo"], calls["*"]
}

func (b *benchCfg) stubLogPath() string {
	if b.mcpTransport == "http" || b.mcpTransport == "sse" {
		if b.httpStubLog != "" {
			return b.httpStubLog
		}
	}
	return b.stubLog
}

type collector struct {
	mu         sync.Mutex
	updates    int
	sawTool    bool
	sawChunk   bool
	tools      []string
	text       strings.Builder
	firstAt    time.Time
	perms      []string
	chunkCount int
}

func (c *collector) onUpdate(u Update) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.updates++
	if c.firstAt.IsZero() {
		c.firstAt = u.At
	}
	switch u.Kind {
	case "tool_call":
		c.sawTool = true
		if name := updateToolName(u.Raw); name != "" {
			c.tools = append(c.tools, name)
		}
	case "tool_call_update":
		c.sawTool = true
		if name := updateToolName(u.Raw); name != "" {
			c.tools = append(c.tools, name)
		}
	case "agent_message_chunk", "agent_message":
		c.sawChunk = true
		c.chunkCount++
		c.text.WriteString(updateText(u.Raw))
	}
}

func (c *collector) snapshot() (updates int, sawTool, sawChunk bool, text string, firstAt time.Time, tools []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.updates, c.sawTool, c.sawChunk, c.text.String(), c.firstAt, append([]string(nil), c.tools...)
}

func updateToolName(u map[string]any) string {
	for _, k := range []string{"title", "name", "toolName"} {
		if s, _ := u[k].(string); s != "" {
			return s
		}
	}
	return ""
}

// updateText pulls text out of an agent_message_chunk payload, tolerating
// several content shapes.
func updateText(u map[string]any) string {
	var sb strings.Builder
	var walk func(v any)
	walk = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			if t["type"] == "text" {
				if s, _ := t["text"].(string); s != "" {
					sb.WriteString(s)
				}
			}
			for _, vv := range t {
				walk(vv)
			}
		case []any:
			for _, vv := range t {
				walk(vv)
			}
		}
	}
	if c, ok := u["content"]; ok {
		walk(c)
	} else {
		walk(any(u))
	}
	return sb.String()
}

// startSession spawns, initializes (+authenticate when asked) and news a
// session, wiring col into updates. With --auth-method it sends authenticate
// once after initialize and retries session/new after AUTH_REQUIRED.
func (b *benchCfg) startSession(col *collector, permMode PermissionMode) (*acpConn, string, error) {
	c, err := b.spawnConn()
	if err != nil {
		return nil, "", err
	}
	c.permMode = permMode
	if col != nil {
		c.onUpdate = col.onUpdate
		c.onPerm = func(tool, opt string) {
			col.mu.Lock()
			col.perms = append(col.perms, tool+"="+opt)
			col.mu.Unlock()
		}
	}
	if err := c.Initialize(); err != nil {
		c.Close()
		return nil, "", err
	}
	if b.authMethod != "" {
		if mid, ok := pickAuthMethod(c.rawInit, b.authMethod); ok {
			_ = c.Authenticate(mid)
		}
	}
	cwd := b.cwd
	sid, _, err := c.NewSessionRaw(cwd, b.mcpServers(), b.sessionMeta())
	if err != nil && isAuthRequired(err) && b.authMethod != "" {
		if mid, ok := pickAuthMethod(c.rawInit, b.authMethod); ok {
			if aerr := c.Authenticate(mid); aerr == nil {
				sid, _, err = c.NewSessionRaw(cwd, b.mcpServers(), b.sessionMeta())
			} else {
				c.Close()
				return nil, "", aerr
			}
		}
	}
	if err != nil {
		c.Close()
		return nil, "", err
	}
	if b.prePromptWaitMCP {
		_ = c.WaitMCPReady(mcpReadyWait)
	}
	return c, sid, nil
}

// colSawBaseTool reports whether the collector saw a tool_call whose
// namespaced name derives to base (e.g. "acpbench-stub__echo" -> "echo").
func colSawBaseTool(col *collector, base string) bool {
	if col == nil {
		return false
	}
	_, _, _, _, _, tools := col.snapshot()
	for _, t := range tools {
		if deriveBaseToolName(t) == base {
			return true
		}
	}
	return false
}

func ms(d time.Duration) int64 { return d.Milliseconds() }

func failResult(name string, start time.Time, verdict string, err error, details map[string]any) ScenarioResult {
	if details == nil {
		details = map[string]any{}
	}
	r := ScenarioResult{Name: name, Verdict: verdict, LatencyMs: ms(time.Since(start)), Details: details}
	if err != nil {
		r.Error = err.Error()
	}
	return r
}

// --- scenario 1: initialize ---

func scInitialize(b *benchCfg) ScenarioResult {
	name := "initialize"
	start := time.Now()
	c, err := b.spawnConn()
	if err != nil {
		return failResult(name, start, VerdictCrash, err, nil)
	}
	defer c.Close()
	t0 := time.Now()
	if err := c.Initialize(); err != nil {
		v := VerdictProtocolError
		if isTimeout(err) {
			v = VerdictTimeoutHang
		}
		if strings.Contains(err.Error(), "adapter closed") {
			v = VerdictCrash
		}
		return failResult(name, start, v, err, nil)
	}
	initMs := ms(time.Since(t0))
	b.markPhase("startup-init", start, time.Now())
	details := map[string]any{
		"timeToInitializeMs": initMs,
		"steeringAdvertised": c.steeringSupported(),
	}
	var raw map[string]any
	if len(c.rawInit) > 0 && jsonUnmarshal(c.rawInit, &raw) == nil {
		for _, k := range []string{"protocolVersion", "agentCapabilities", "authMethods", "promptCapabilities", "mcpCapabilities"} {
			if v, ok := raw[k]; ok {
				details[k] = v
			} else {
				details[k] = nil
			}
		}
		details["rawKeys"] = keysOf(raw)
	}
	if b.authMethod != "" {
		if mid, ok := pickAuthMethod(c.rawInit, b.authMethod); ok {
			details["authSelected"] = mid
			if aerr := c.Authenticate(mid); aerr != nil {
				details["authenticate"] = "failed: " + aerr.Error()
			} else {
				details["authenticate"] = "ok"
			}
		} else {
			details["authenticate"] = "no-method-advertised"
		}
	}
	// Idle settle so idle-after-initialize has real samples.
	s0 := time.Now()
	time.Sleep(settleWait)
	b.markPhase("idle-after-initialize", s0, time.Now())
	return ScenarioResult{Name: name, Verdict: VerdictOK, LatencyMs: ms(time.Since(start)), Details: details}
}

// --- scenario 2: new_session ---

func scNewSession(b *benchCfg) ScenarioResult {
	name := "new_session"
	start := time.Now()
	col := &collector{}
	c, _, err := b.startSession(col, PermAutoApprove)
	if err != nil {
		v := VerdictProtocolError
		if isTimeout(err) {
			v = VerdictTimeoutHang
		}
		return failResult(name, start, v, err, nil)
	}
	defer c.Close()
	details := map[string]any{"mcpStub": b.mcpStub, "mcpTransport": b.mcpTransport}
	// Column 1: was the MCP-initialized notification seen? Column 2: did the
	// stub server itself see tools/list? Never skip for a missing
	// _x.ai/mcp_initialized-style notification; report both honestly.
	t0 := time.Now()
	notifSeen := c.WaitMCPReady(mcpReadyWait) == nil
	details["notificationSeen"] = notifSeen
	details["mcpReadyLatencyMs"] = ms(time.Since(t0))
	if notifSeen {
		details["mcpReady"] = "ready"
	} else {
		details["mcpReady"] = "no-notification"
	}
	// Give a stdio-spawned stub a beat to serve tools/list before reading.
	time.Sleep(300 * time.Millisecond)
	listSeen, calls := readStubLog(b.stubLogPath())
	details["toolListSeen"] = listSeen
	details["stubCalls"] = calls
	s0 := time.Now()
	time.Sleep(settleWait)
	b.markPhase("idle-after-new-session", s0, time.Now())
	return ScenarioResult{Name: name, Verdict: VerdictOK, LatencyMs: ms(time.Since(start)), Details: details}
}

func scPromptText(b *benchCfg) ScenarioResult {
	name := "prompt_text"
	start := time.Now()
	col := &collector{}
	c, sid, err := b.startSession(col, PermAutoApprove)
	if err != nil {
		return failResult(name, start, VerdictProtocolError, err, nil)
	}
	defer c.Close()
	prompt := b.echoPromptFor(false)
	p0 := time.Now()
	stop, firstChunkMs, perr := runEchoTurn(c, sid, col, promptTimeout, prompt)
	b.markPhase("prompt-turn", p0, time.Now())
	post0 := time.Now()
	time.Sleep(300 * time.Millisecond)
	b.markPhase("post-turn", post0, time.Now())
	updates, sawTool, sawChunk, text, _, tools := col.snapshot()
	sawEcho := colSawBaseTool(col, "echo")
	_, echoCalls, totalCalls := b.stubEchoCalls()
	details := map[string]any{
		"stopReason": stop, "updates": updates, "sawToolCall": sawTool,
		"sawEchoTool": sawEcho, "sawChunk": sawChunk, "tools": tools,
		"timeToFirstChunkMs": firstChunkMs, "textBytes": len(text),
		"cooperative": b.cooperate || b.fakeLLMURL != "",
	}
	if b.mcpStub {
		details["stubEchoCalls"] = echoCalls
		details["stubTotalCalls"] = totalCalls
		details["stubVerified"] = echoCalls > 0
	}
	o := turnOutcome{err: perr, stopReason: stop, updates: updates,
		sawToolCall: sawEcho || sawTool, sawChunk: sawChunk, expectTool: true}
	if perr != nil {
		o.timedOut = isTimeout(perr)
		o.crashed = c.exited()
		r := failResult(name, start, classifyTurn(o), perr, details)
		return r
	}
	if stop != "end_turn" {
		details["unexpectedStop"] = true
		return failResult(name, start, VerdictWrongResult, fmt.Errorf("stopReason %q", stop), details)
	}
	// Server-side proof: with the stub offered, the turn only counts when
	// the stub actually ran echo. Updates alone can be faked.
	if b.mcpStub && echoCalls == 0 {
		details["note"] = "agent announced a tool call but the stub never ran echo"
		return failResult(name, start, VerdictWrongResult, fmt.Errorf("stub never invoked echo"), details)
	}
	return ScenarioResult{Name: name, Verdict: classifyTurn(o), LatencyMs: ms(time.Since(start)), Details: details}
}
