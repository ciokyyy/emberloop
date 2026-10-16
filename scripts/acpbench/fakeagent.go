package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// fakeModels are the real ids the fake advertises: discovery reads these.
var fakeModels = []string{"fake-default", "fake-alt", "fake-pro"}

// fakeFlags configures the in-repo fake ACP agent (subcommand fake-agent).
type fakeFlags struct {
	hangPrompt      bool
	crashPrompt     bool
	empty           bool
	wrong           bool
	slowMs          int
	slowMatch       string
	slowCancelMs    int
	askPermission   bool
	imageBehavior   string
	allowConcurrent bool
	stateDir        string
	toolPrefix      string
	requireAuth     bool
	noModels        bool
	noConfigOptions bool
}

func runFakeAgent(args []string) int {
	fs := flag.NewFlagSet("fake-agent", flag.ContinueOnError)
	var fl fakeFlags
	fs.BoolVar(&fl.hangPrompt, "hang-prompt", false, "never answer session/prompt, ignore cancel")
	fs.BoolVar(&fl.crashPrompt, "crash-prompt", false, "exit(1) mid-turn after one chunk")
	fs.BoolVar(&fl.empty, "empty", false, "end_turn with zero updates")
	fs.BoolVar(&fl.wrong, "wrong", false, "end_turn with wrong text and no tool call")
	fs.IntVar(&fl.slowMs, "slow-ms", 0, "delay before answering a prompt")
	fs.StringVar(&fl.slowMatch, "slow-match", "", "only slow prompts containing this text")
	fs.IntVar(&fl.slowCancelMs, "slow-cancel-ms", 0, "delay before answering cancelled after session/cancel")
	fs.BoolVar(&fl.askPermission, "ask-permission", false, "request permission mid-turn")
	fs.StringVar(&fl.imageBehavior, "image-behavior", "accept", "accept|drop|error for image blocks")
	fs.BoolVar(&fl.allowConcurrent, "allow-concurrent", false, "allow overlapping prompts")
	fs.StringVar(&fl.stateDir, "state-dir", "", "persist sessions here for load-after-respawn")
	fs.StringVar(&fl.toolPrefix, "tool-prefix", "", "namespace announced tool names (e.g. acpbench-stub__)")
	fs.BoolVar(&fl.requireAuth, "require-auth", false, "fail session/new with AUTH_REQUIRED until authenticate")
	fs.BoolVar(&fl.noModels, "no-models", false, "advertise no models (discovery fallback path)")
	fs.BoolVar(&fl.noConfigOptions, "no-config-options", false, "advertise no configOptions")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	f := &fakeAgent{flags: fl, sessions: map[string]*fakeSession{}, permAns: map[string]string{}}
	if fl.stateDir != "" {
		_ = os.MkdirAll(fl.stateDir, 0o755)
	}
	return f.serveLoop()
}

type fakeSession struct {
	mu       sync.Mutex
	id       string
	cwd      string
	model    string
	config   map[string]string
	history  []string
	busy     bool
	cancelCh chan struct{}
}

type fakeAgent struct {
	flags    fakeFlags
	mu       sync.Mutex
	sessions map[string]*fakeSession
	permAns  map[string]string
	nextID   int
	authed   bool
	// stubCmd/stubURL remember the offered MCP stub so echo turns really
	// invoke it (server-side proof), like a cooperating real agent.
	stubCmd []string
	stubURL string

	wmu sync.Mutex
	out *bufio.Writer
}

func (f *fakeAgent) serveLoop() int {
	f.out = bufio.NewWriter(os.Stdout)
	defer f.out.Flush()
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 64*1024), maxACPLine)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var msg struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
			Result json.RawMessage `json:"result"`
		}
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		// Client answer to our permission request: a result carrying a
		// string id we issued. Record it for the waiting turn.
		if msg.ID != nil && msg.Method == "" {
			var sid string
			if json.Unmarshal(msg.ID, &sid) == nil && strings.HasPrefix(sid, "perm-") {
				var ans struct {
					Outcome  string `json:"outcome"`
					OptionID string `json:"optionId"`
				}
				_ = json.Unmarshal(msg.Result, &ans)
				got := ans.OptionID
				if ans.Outcome == "cancelled" {
					got = "denied"
				}
				f.mu.Lock()
				f.permAns[sid] = got
				f.mu.Unlock()
			}
			continue
		}
		if msg.ID == nil {
			if msg.Method == "session/cancel" {
				f.cancelSession(msg.Params)
			}
			continue
		}
		f.handleCall(msg.ID, msg.Method, msg.Params)
	}
	return 0
}

func (f *fakeAgent) reply(id json.RawMessage, result any) {
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id), "result": result})
	f.wmu.Lock()
	f.out.Write(append(b, '\n'))
	f.out.Flush()
	f.wmu.Unlock()
}

func (f *fakeAgent) replyErr(id json.RawMessage, code int, msg string) {
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id),
		"error": map[string]any{"code": code, "message": msg}})
	f.wmu.Lock()
	f.out.Write(append(b, '\n'))
	f.out.Flush()
	f.wmu.Unlock()
}

func (f *fakeAgent) notify(method string, params any) {
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
	f.wmu.Lock()
	f.out.Write(append(b, '\n'))
	f.out.Flush()
	f.wmu.Unlock()
}

func (f *fakeAgent) update(sessionID string, u map[string]any) {
	f.notify("session/update", map[string]any{"sessionId": sessionID, "update": u})
}

func (f *fakeAgent) getSession(id string) *fakeSession {
	f.mu.Lock()
	s := f.sessions[id]
	f.mu.Unlock()
	if s != nil {
		return s
	}
	if f.flags.stateDir != "" {
		if s := f.loadPersisted(id); s != nil {
			f.mu.Lock()
			f.sessions[id] = s
			f.mu.Unlock()
			return s
		}
	}
	return nil
}

func (f *fakeAgent) persist(s *fakeSession) {
	if f.flags.stateDir == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	b, _ := json.Marshal(map[string]any{
		"sessionId": s.id, "cwd": s.cwd, "model": s.model, "history": s.history,
		"config": s.config,
	})
	_ = os.WriteFile(filepath.Join(f.flags.stateDir, "session-"+s.id+".json"), b, 0o644)
}

func (f *fakeAgent) loadPersisted(id string) *fakeSession {
	if f.flags.stateDir == "" {
		return nil
	}
	raw, err := os.ReadFile(filepath.Join(f.flags.stateDir, "session-"+id+".json"))
	if err != nil {
		return nil
	}
	var rec struct {
		SessionID string            `json:"sessionId"`
		Cwd       string            `json:"cwd"`
		Model     string            `json:"model"`
		History   []string          `json:"history"`
		Config    map[string]string `json:"config"`
	}
	if json.Unmarshal(raw, &rec) != nil || rec.SessionID == "" {
		return nil
	}
	if rec.Config == nil {
		rec.Config = map[string]string{}
	}
	return &fakeSession{id: rec.SessionID, cwd: rec.Cwd, model: rec.Model,
		history: rec.History, config: rec.Config}
}

// cancelSession closes the turn's cancel channel.
func (f *fakeAgent) cancelSession(raw json.RawMessage) {
	var p struct {
		SessionID string `json:"sessionId"`
	}
	_ = json.Unmarshal(raw, &p)
	s := f.getSession(p.SessionID)
	if s == nil {
		return
	}
	s.mu.Lock()
	ch := s.cancelCh
	s.mu.Unlock()
	if ch != nil {
		select {
		case <-ch:
		default:
			close(ch)
		}
	}
}

func (f *fakeAgent) handleCall(id json.RawMessage, method string, raw json.RawMessage) {
	switch method {
	case "initialize":
		f.reply(id, map[string]any{
			"protocolVersion": 1,
			"agentCapabilities": map[string]any{
				"loadSession": true, "mcp": true,
				"promptCapabilities": map[string]any{"image": true, "audio": false},
			},
			"authMethods": []any{
				map[string]any{"id": "env-api-key", "name": "API key from environment", "type": "api-key"},
				map[string]any{"id": "oauth", "name": "OAuth"},
			},
			"promptCapabilities": map[string]any{"image": true, "audio": false},
			"mcpCapabilities":    map[string]any{"tools": true},
			"_meta":              map[string]any{"steering": map[string]any{"supported": true}},
		})
	case "authenticate":
		var p struct {
			MethodID string `json:"methodId"`
			Method   string `json:"method"`
		}
		_ = json.Unmarshal(raw, &p)
		got := p.MethodID
		if got == "" {
			got = p.Method
		}
		if got == "" {
			var s string
			_ = json.Unmarshal(raw, &s)
			got = s
		}
		if got == "" {
			f.replyErr(id, -32602, "authenticate: missing methodId")
			return
		}
		f.mu.Lock()
		f.authed = true
		f.mu.Unlock()
		f.reply(id, map[string]any{"methodId": got})
	case "session/new":
		if f.flags.requireAuth {
			f.mu.Lock()
			ok := f.authed
			f.mu.Unlock()
			if !ok {
				f.replyErr(id, -32000, "AUTH_REQUIRED: call authenticate first")
				return
			}
		}
		var pnew struct {
			Cwd        string           `json:"cwd"`
			McpServers []map[string]any `json:"mcpServers"`
		}
		_ = json.Unmarshal(raw, &pnew)
		f.rememberStub(pnew.McpServers)
		f.discoverStubTools() // best-effort tools/list, like a real agent
		f.mu.Lock()
		f.nextID++
		sid := fmt.Sprintf("sess-%d", f.nextID)
		f.mu.Unlock()
		s := &fakeSession{id: sid, cwd: pnew.Cwd, model: "fake-default", config: map[string]string{}}
		f.mu.Lock()
		f.sessions[sid] = s
		f.mu.Unlock()
		f.reply(id, f.newResult(sid))
		// Let clients exercise the MCP-ready wait.
		go func() {
			time.Sleep(100 * time.Millisecond)
			f.notify("_x.ai/mcp_initialized", map[string]any{"sessionId": sid})
		}()
	case "session/load":
		var p struct {
			SessionID string `json:"sessionId"`
			Cwd       string `json:"cwd"`
		}
		_ = json.Unmarshal(raw, &p)
		s := f.getSession(p.SessionID)
		if s == nil {
			f.replyErr(id, -32602, "unknown session "+p.SessionID)
			return
		}
		if p.Cwd != "" {
			s.mu.Lock()
			s.cwd = p.Cwd
			s.mu.Unlock()
		}
		f.persist(s)
		res := map[string]any{}
		for k, v := range f.newResult(p.SessionID) {
			if k == "sessionId" {
				continue
			}
			res[k] = v
		}
		f.reply(id, res)
	case "session/resume":
		var p struct {
			SessionID string `json:"sessionId"`
			Cwd       string `json:"cwd"`
		}
		_ = json.Unmarshal(raw, &p)
		s := f.getSession(p.SessionID)
		if s == nil {
			f.replyErr(id, -32602, "unknown session "+p.SessionID)
			return
		}
		f.reply(id, map[string]any{"sessionId": p.SessionID})
	case "session/prompt":
		var p struct {
			SessionID string `json:"sessionId"`
			Prompt    []any  `json:"prompt"`
		}
		_ = json.Unmarshal(raw, &p)
		s := f.getSession(p.SessionID)
		if s == nil {
			f.replyErr(id, -32602, "unknown session "+p.SessionID)
			return
		}
		s.mu.Lock()
		if s.busy && !f.flags.allowConcurrent {
			s.mu.Unlock()
			f.replyErr(id, -32600, "a turn is already running")
			return
		}
		s.busy = true
		s.cancelCh = make(chan struct{})
		s.mu.Unlock()
		text, hasImage := splitPrompt(p.Prompt)
		go f.runTurn(id, s, text, hasImage)
	case "_session/steering":
		var p struct {
			SessionID string `json:"sessionId"`
			Prompt    []any  `json:"prompt"`
		}
		_ = json.Unmarshal(raw, &p)
		s := f.getSession(p.SessionID)
		if s == nil {
			f.replyErr(id, -32602, "unknown session "+p.SessionID)
			return
		}
		s.mu.Lock()
		running := s.busy
		s.mu.Unlock()
		if running {
			text, _ := splitPrompt(p.Prompt)
			f.update(p.SessionID, map[string]any{"sessionUpdate": "agent_message_chunk",
				"content": map[string]any{"type": "text", "text": "steered:" + text}})
			f.reply(id, map[string]any{"outcome": "injected"})
		} else {
			f.reply(id, map[string]any{"outcome": "promptRequired"})
		}
	case "session/set_model":
		var p struct {
			SessionID string `json:"sessionId"`
			ModelID   string `json:"modelId"`
		}
		_ = json.Unmarshal(raw, &p)
		if p.ModelID == "" {
			f.replyErr(id, -32602, "session/set_model: missing modelId (spec key is modelId, not model)")
			return
		}
		if !slicesContains(fakeModels, p.ModelID) {
			f.replyErr(id, -32602, "unknown modelId "+p.ModelID)
			return
		}
		if s := f.getSession(p.SessionID); s != nil {
			s.mu.Lock()
			s.model = p.ModelID
			s.mu.Unlock()
			f.persist(s)
		}
		f.reply(id, map[string]any{})
	case "session/set_config_option":
		var p struct {
			SessionID string `json:"sessionId"`
			ConfigID  string `json:"configId"`
			Value     string `json:"value"`
		}
		_ = json.Unmarshal(raw, &p)
		switch p.ConfigID {
		case "model":
			if !slicesContains(fakeModels, p.Value) {
				f.replyErr(id, -32602, "unknown model value "+p.Value)
				return
			}
			if s := f.getSession(p.SessionID); s != nil {
				s.mu.Lock()
				s.model = p.Value
				s.config[p.ConfigID] = p.Value
				s.mu.Unlock()
				f.persist(s)
			}
		case "thought_level", "effort":
			if p.Value == "" {
				f.replyErr(id, -32602, "empty value for "+p.ConfigID)
				return
			}
			if p.Value == "bogus" || strings.HasPrefix(p.Value, "acpbench-probe") || strings.HasPrefix(p.Value, "acpbench_probe") {
				f.replyErr(id, -32602, "unknown value "+p.Value+" for "+p.ConfigID)
				return
			}
			if s := f.getSession(p.SessionID); s != nil {
				s.mu.Lock()
				s.config[p.ConfigID] = p.Value
				s.mu.Unlock()
				f.persist(s)
			}
		default:
			f.replyErr(id, -32602, "unknown configId "+p.ConfigID)
			return
		}
		f.reply(id, map[string]any{})
	default:
		f.replyErr(id, -32601, "Method not found")
	}
}

// newResult advertises real model ids + configOptions for discovery.
func (f *fakeAgent) newResult(sid string) map[string]any {
	res := map[string]any{"sessionId": sid}
	if f.flags.noModels && f.flags.noConfigOptions {
		return res
	}
	avail := []any{}
	for _, m := range fakeModels {
		avail = append(avail, map[string]any{"modelId": m, "name": m})
	}
	if !f.flags.noModels {
		res["models"] = map[string]any{"currentModelId": "fake-default", "availableModels": avail}
	}
	if !f.flags.noConfigOptions {
		res["configOptions"] = []any{
			map[string]any{"id": "model", "category": "model", "values": fakeModels},
			map[string]any{"id": "thought_level", "category": "model", "values": []string{"low", "high"}},
			map[string]any{"id": "effort", "category": "model", "values": []string{"low", "max"}},
		}
	}
	return res
}

func slicesContains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// rememberStub records the offered stub (stdio command or http url) from
// session/new so echo turns really invoke it, like a cooperating agent.
func (f *fakeAgent) rememberStub(servers []map[string]any) {
	for _, sv := range servers {
		if cmd, _ := sv["command"].(string); cmd != "" {
			var args []string
			if a, ok := sv["args"].([]any); ok {
				for _, e := range a {
					if s, ok := e.(string); ok {
						args = append(args, s)
					}
				}
			}
			f.mu.Lock()
			f.stubCmd = append([]string{cmd}, args...)
			f.stubURL = ""
			f.mu.Unlock()
			return
		}
		if url, _ := sv["url"].(string); url != "" {
			f.mu.Lock()
			f.stubURL = url
			f.stubCmd = nil
			f.mu.Unlock()
			return
		}
	}
}

// callStubEcho invokes the real stub echo tool; ok=false means no stub was
// offered or the call failed (caller falls back to simulated updates).
func (f *fakeAgent) callStubEcho(text string) (string, bool) {
	f.mu.Lock()
	cmd := append([]string(nil), f.stubCmd...)
	url := f.stubURL
	f.mu.Unlock()
	if len(cmd) == 0 && url == "" {
		return "", false
	}
	if url != "" {
		return callStubEchoHTTP(url, text)
	}
	return callStubEchoStdio(cmd, text)
}

// discoverStubTools runs MCP initialize + tools/list against the offered
// stub at session/new, like a real agent discovering tools. Best effort:
// failures are ignored so sessions still open without a stub.
func (f *fakeAgent) discoverStubTools() {
	f.mu.Lock()
	cmd := append([]string(nil), f.stubCmd...)
	url := f.stubURL
	f.mu.Unlock()
	if len(cmd) == 0 && url == "" {
		return
	}
	if url != "" {
		post := func(method string) {
			b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": map[string]any{}})
			req, err := http.NewRequest("POST", url, bytes.NewReader(b))
			if err != nil {
				return
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return
			}
			resp.Body.Close()
		}
		post("initialize")
		post("tools/list")
		return
	}
	c := exec.Command(cmd[0], cmd[1:]...)
	stdin, err := c.StdinPipe()
	if err != nil {
		return
	}
	stdout, err := c.StdoutPipe()
	if err != nil {
		return
	}
	if err := c.Start(); err != nil {
		return
	}
	defer func() { _ = stdin.Close(); _ = c.Wait() }()
	rd := bufio.NewReader(stdout)
	for _, method := range []string{"initialize", "tools/list"} {
		stdin.Write(mcpRPC(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": map[string]any{}}))
		for {
			line, err := rd.ReadBytes('\n')
			if err != nil {
				return
			}
			var m map[string]any
			if json.Unmarshal(line, &m) != nil {
				continue
			}
			if m["id"] == float64(1) {
				break
			}
		}
	}
}

func mcpRPC(payload map[string]any) []byte {
	b, _ := json.Marshal(payload)
	return append(b, '\n')
}

// callStubEchoStdio spawns the stub and runs MCP initialize + tools/call.
func callStubEchoStdio(cmd []string, text string) (string, bool) {
	if len(cmd) == 0 {
		return "", false
	}
	c := exec.Command(cmd[0], cmd[1:]...)
	stdin, err := c.StdinPipe()
	if err != nil {
		return "", false
	}
	stdout, err := c.StdoutPipe()
	if err != nil {
		return "", false
	}
	c.Stderr = nil
	if err := c.Start(); err != nil {
		return "", false
	}
	defer func() { _ = stdin.Close(); _ = c.Wait() }()
	rd := bufio.NewReader(stdout)
	call := func(id int, method string, params any) (map[string]any, bool) {
		stdin.Write(mcpRPC(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}))
		for {
			line, err := rd.ReadBytes('\n')
			if err != nil {
				return nil, false
			}
			var m map[string]any
			if json.Unmarshal(line, &m) != nil {
				continue
			}
			if m["id"] == float64(id) {
				return m, true
			}
		}
	}
	if _, ok := call(1, "initialize", map[string]any{"protocolVersion": "2024-11-05"}); !ok {
		return "", false
	}
	_, _ = call(2, "tools/list", map[string]any{})
	m, ok := call(3, "tools/call", map[string]any{"name": "echo", "arguments": map[string]any{"text": text}})
	if !ok {
		return "", false
	}
	if _, hasErr := m["error"]; hasErr {
		return "", false
	}
	b, _ := json.Marshal(m["result"])
	var res struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(b, &res) != nil || len(res.Content) == 0 {
		return "", false
	}
	return res.Content[0].Text, true
}

// callStubEchoHTTP POSTs MCP tools/call to an http/sse stub URL.
func callStubEchoHTTP(url, text string) (string, bool) {
	post := func(method string, params any) (map[string]any, bool) {
		b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
		req, err := http.NewRequest("POST", url, bytes.NewReader(b))
		if err != nil {
			return nil, false
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, false
		}
		defer resp.Body.Close()
		var m map[string]any
		if json.NewDecoder(resp.Body).Decode(&m) != nil {
			return nil, false
		}
		return m, true
	}
	if _, ok := post("initialize", map[string]any{"protocolVersion": "2024-11-05"}); !ok {
		return "", false
	}
	_, _ = post("tools/list", map[string]any{})
	m, ok := post("tools/call", map[string]any{"name": "echo", "arguments": map[string]any{"text": text}})
	if !ok {
		return "", false
	}
	if _, hasErr := m["error"]; hasErr {
		return "", false
	}
	b, _ := json.Marshal(m["result"])
	var res struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(b, &res) != nil || len(res.Content) == 0 {
		return "", false
	}
	return res.Content[0].Text, true
}

func splitPrompt(blocks []any) (text string, hasImage bool) {
	for _, b := range blocks {
		m, ok := b.(map[string]any)
		if !ok {
			continue
		}
		switch m["type"] {
		case "text":
			if t, _ := m["text"].(string); t != "" {
				if text != "" {
					text += "\n"
				}
				text += t
			}
		case "image":
			hasImage = true
		}
	}
	return text, hasImage
}

// runTurn executes one prompt turn per the configured fault flags.
func (f *fakeAgent) runTurn(id json.RawMessage, s *fakeSession, text string, hasImage bool) {
	finish := func(result any) {
		s.mu.Lock()
		s.busy = false
		s.history = append(s.history, text)
		s.cancelCh = nil
		s.mu.Unlock()
		f.persist(s)
		f.reply(id, result)
	}
	fail := func(code int, msg string) {
		s.mu.Lock()
		s.busy = false
		s.cancelCh = nil
		s.mu.Unlock()
		f.replyErr(id, code, msg)
	}
	cancelled := func() bool {
		s.mu.Lock()
		ch := s.cancelCh
		s.mu.Unlock()
		if ch == nil {
			return false
		}
		select {
		case <-ch:
			return true
		default:
			return false
		}
	}

	if hasImage {
		switch f.flags.imageBehavior {
		case "error":
			fail(-32602, "images not supported")
			return
		case "drop":
			f.update(s.id, map[string]any{"sessionUpdate": "agent_message_chunk",
				"content": map[string]any{"type": "text", "text": "dropped:image (policy)"}})
			finish(map[string]any{"stopReason": "end_turn"})
			return
		}
	}

	if f.flags.hangPrompt {
		return // never answer; ignore cancel too
	}
	if f.flags.crashPrompt {
		f.update(s.id, map[string]any{"sessionUpdate": "agent_message_chunk",
			"content": map[string]any{"type": "text", "text": "before crash"}})
		time.Sleep(100 * time.Millisecond)
		os.Exit(1)
	}
	if f.flags.empty {
		finish(map[string]any{"stopReason": "end_turn"})
		return
	}
	if f.flags.wrong {
		f.update(s.id, map[string]any{"sessionUpdate": "agent_message_chunk",
			"content": map[string]any{"type": "text", "text": "the moon is cheese"}})
		finish(map[string]any{"stopReason": "end_turn"})
		return
	}
	if strings.Contains(text, "history-check") {
		s.mu.Lock()
		n := len(s.history)
		s.mu.Unlock()
		f.update(s.id, map[string]any{"sessionUpdate": "agent_message_chunk",
			"content": map[string]any{"type": "text", "text": fmt.Sprintf("history:%d", n)}})
		finish(map[string]any{"stopReason": "end_turn"})
		return
	}
	// Provider-fault contract: prompts containing FAULT:<class> simulate how
	// a real agent's LLM backend fails. fakellm2 honors the same directive
	// text; the fake agent mirrors it so the probe classifies without it.
	if class := faultClass(text); class != "" {
		f.runFault(id, s, class, finish, fail)
		return
	}
	if ms := f.flags.slowMs; ms > 0 && (f.flags.slowMatch == "" || strings.Contains(text, f.flags.slowMatch)) {
		s.mu.Lock()
		ch := s.cancelCh
		s.mu.Unlock()
		select {
		case <-ch:
			if f.flags.slowCancelMs > 0 {
				time.Sleep(time.Duration(f.flags.slowCancelMs) * time.Millisecond)
			}
			finish(map[string]any{"stopReason": "cancelled"})
			return
		case <-time.After(time.Duration(ms) * time.Millisecond):
		}
	} else if cancelled() {
		finish(map[string]any{"stopReason": "cancelled"})
		return
	}

	if f.flags.askPermission {
		permID := fmt.Sprintf("perm-%d", time.Now().UnixNano())
		b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": permID,
			"method": "session/request_permission",
			"params": map[string]any{
				"toolCall": map[string]any{"name": "echo", "title": "echo"},
				"options": []any{
					map[string]any{"optionId": "allow-once", "kind": "allow_once", "label": "Allow once"},
					map[string]any{"optionId": "reject", "kind": "reject_once", "label": "Reject"},
				}}})
		f.wmu.Lock()
		f.out.Write(append(b, '\n'))
		f.out.Flush()
		f.wmu.Unlock()
		if answer := f.waitPermAnswer(permID, 15*time.Second); answer != "allow-once" {
			f.update(s.id, map[string]any{"sessionUpdate": "agent_message_chunk",
				"content": map[string]any{"type": "text", "text": "permission denied for echo"}})
			finish(map[string]any{"stopReason": "end_turn"})
			return
		}
	}

	if cancelled() {
		if f.flags.slowCancelMs > 0 {
			time.Sleep(time.Duration(f.flags.slowCancelMs) * time.Millisecond)
		}
		finish(map[string]any{"stopReason": "cancelled"})
		return
	}
	s.mu.Lock()
	model := s.model
	cfg := map[string]string{}
	for k, v := range s.config {
		cfg[k] = v
	}
	s.mu.Unlock()
	echoName := "echo"
	if f.flags.toolPrefix != "" {
		echoName = f.flags.toolPrefix + "echo"
	}
	// When the client offered a real stub, invoke it for server-side proof.
	// Deny-leg turns return before this point, so a denied tool never runs.
	stubText, stubOK := f.callStubEcho("hello")
	f.update(s.id, map[string]any{"sessionUpdate": "tool_call",
		"toolCallId": "t1", "title": echoName, "name": echoName, "kind": "other", "status": "in_progress"})
	echoContent := "echo:hello"
	if stubOK {
		echoContent = stubText
	}
	f.update(s.id, map[string]any{"sessionUpdate": "tool_call_update",
		"toolCallId": "t1", "status": "completed", "content": echoContent})
	txt := echoContent + " model:" + model
	if v := cfg["thought_level"]; v != "" {
		txt += " thought:" + v
	}
	if v := cfg["effort"]; v != "" {
		txt += " effort:" + v
	}
	f.update(s.id, map[string]any{"sessionUpdate": "agent_message_chunk",
		"content": map[string]any{"type": "text", "text": txt}})
	finish(map[string]any{"stopReason": "end_turn"})
}

// faultClass extracts the FAULT:<class> directive from prompt text,
// stripping trailing punctuation (the probe writes "FAULT:<class>. ...").
func faultClass(text string) string {
	up := strings.ToUpper(text)
	idx := strings.Index(up, "FAULT:")
	if idx < 0 {
		return ""
	}
	rest := text[idx+len("FAULT:"):]
	end := len(rest)
	for i, r := range rest {
		if r == ' ' || r == '\n' || r == '\t' || r == '"' || r == '\'' || r == ',' || r == '}' {
			end = i
			break
		}
	}
	return strings.ToLower(strings.Trim(strings.TrimSpace(rest[:end]), ".,;:!?"))
}

// faultStallMs honors ACPBENCH_FAULT_STALL_MS to keep tests fast.
func faultStallMs(def time.Duration) time.Duration {
	if v := os.Getenv("ACPBENCH_FAULT_STALL_MS"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return def
}

// runFault mirrors provider failures for FAULT:<class> prompts.
func (f *fakeAgent) runFault(id json.RawMessage, s *fakeSession, class string, finish func(any), fail func(int, string)) {
	switch class {
	case "http429", "429":
		f.update(s.id, map[string]any{"sessionUpdate": "agent_message_chunk",
			"content": map[string]any{"type": "text", "text": "provider error 429 rate limited"}})
		fail(-32000, "provider 429 rate limited")
	case "http500", "500":
		f.update(s.id, map[string]any{"sessionUpdate": "agent_message_chunk",
			"content": map[string]any{"type": "text", "text": "provider error 500 internal"}})
		fail(-32000, "provider 500 internal error")
	case "http529", "529":
		f.update(s.id, map[string]any{"sessionUpdate": "agent_message_chunk",
			"content": map[string]any{"type": "text", "text": "provider error 529 overloaded"}})
		fail(-32000, "provider 529 overloaded")
	case "midstream-error", "midstream", "mid-stream-error":
		f.update(s.id, map[string]any{"sessionUpdate": "agent_message_chunk",
			"content": map[string]any{"type": "text", "text": "partial response before failure"}})
		time.Sleep(50 * time.Millisecond)
		fail(-32000, "provider mid-stream error: connection reset")
	case "stall-5s", "stall":
		select {
		case <-s.cancelCh:
			finish(map[string]any{"stopReason": "cancelled"})
			return
		case <-time.After(faultStallMs(5 * time.Second)):
		}
		f.update(s.id, map[string]any{"sessionUpdate": "agent_message_chunk",
			"content": map[string]any{"type": "text", "text": "recovered after stall retries:1"}})
		finish(map[string]any{"stopReason": "end_turn"})
	case "stall-forever", "stallforever", "hang":
		return // never answer: the probe must time out, not hang forever
	case "truncate", "truncated":
		f.update(s.id, map[string]any{"sessionUpdate": "agent_message_chunk",
			"content": map[string]any{"type": "text", "text": "trunc"}})
		finish(map[string]any{"stopReason": "end_turn"})
	case "reset":
		fail(-32000, "provider connection reset")
	case "empty":
		finish(map[string]any{"stopReason": "end_turn"})
	default:
		fail(-32000, "provider fault "+class)
	}
}

// waitPermAnswer polls the answer table fed by serveLoop.
func (f *fakeAgent) waitPermAnswer(permID string, timeout time.Duration) string {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		ans, ok := f.permAns[permID]
		if ok {
			delete(f.permAns, permID)
		}
		f.mu.Unlock()
		if ok {
			return ans
		}
		time.Sleep(10 * time.Millisecond)
	}
	return ""
}
