// Package mcpfake runs a fake MCP server over stdio for tests. It answers
// initialize, tools/list and tools/call from a Config, and can hang, exit or
// fail calls on demand, so the client tests cover a server that never
// answers, one that exits, and one that returns protocol errors.
//
// A test spawns it with Helper, which re-executes the test binary: the test
// binary must define a test named HelperTestName that calls RunIfHelper.
// Tickets 17 and 20 reuse this helper to stand in for their servers.
package mcpfake

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"testing"
)

// HelperTestName is the test every test binary that spawns the fake defines:
//
//	func TestMcpFakeHelperProcess(t *testing.T) { mcpfake.RunIfHelper(t) }
const HelperTestName = "TestMcpFakeHelperProcess"

// Mode and config travel to the child in the environment, so the re-executed
// test binary needs no new flags.
const (
	envMode   = "EMBERLOOP_MCPFAKE"
	envConfig = "EMBERLOOP_MCPFAKE_CONFIG"
)

// ToolDef is one tool the fake lists.
type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Schema      map[string]any `json:"schema,omitempty"`
	ReadOnly    bool           `json:"read_only,omitempty"`
}

// Result is one scripted call outcome. A non-empty Message answers the call
// with a JSON-RPC error of Code (defaulting to -32603); otherwise the call
// answers Text, marked isError when IsError is set.
type Result struct {
	Text    string `json:"text,omitempty"`
	IsError bool   `json:"is_error,omitempty"`
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// Config drives one fake server process.
type Config struct {
	Tools []ToolDef `json:"tools,omitempty"`
	// Calls scripts per-tool results by original name. A tool with no entry
	// echoes its name and arguments.
	Calls map[string]Result `json:"calls,omitempty"`
	// HangOn never answers one method: "initialize", "list" or "call".
	HangOn string `json:"hang_on,omitempty"`
	// ExitOn exits the process on "start", "initialize" or "call".
	ExitOn   string `json:"exit_on,omitempty"`
	ExitCode int    `json:"exit_code,omitempty"`
	// LogPath appends every received method name, one per line, so a test
	// can assert the handshake happened.
	LogPath string `json:"log_path,omitempty"`
}

// Helper returns the command, args and extra environment that spawn the fake
// for cfg. Merge env into the server setting's Env; the client passes its
// whole environment to the child plus those entries.
func Helper(t *testing.T, cfg Config) (command string, args []string, env map[string]string) {
	t.Helper()
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return os.Args[0],
		[]string{"-test.run", HelperTestName, "-test.count=1"},
		map[string]string{envMode: "1", envConfig: string(raw)}
}

// RunIfHelper serves the fake when this process is a re-executed helper, and
// skips the test otherwise, the way the repo's other helper processes do.
func RunIfHelper(t *testing.T) {
	t.Helper()
	if os.Getenv(envMode) == "" {
		t.Skip("not a fake MCP server process")
	}
	var cfg Config
	if err := json.Unmarshal([]byte(os.Getenv(envConfig)), &cfg); err != nil {
		fmt.Fprintln(os.Stderr, "mcpfake: bad config:", err)
		os.Exit(2)
	}
	run(cfg)
}

type fakeRequest struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

func run(cfg Config) {
	if cfg.ExitOn == "start" {
		code := cfg.ExitCode
		if code == 0 {
			code = 1
		}
		os.Exit(code)
	}
	logMethod := func(method string) {
		if cfg.LogPath == "" {
			return
		}
		f, err := os.OpenFile(cfg.LogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return
		}
		fmt.Fprintln(f, method)
		f.Close()
	}
	out := bufio.NewWriter(os.Stdout)
	reply := func(id json.RawMessage, result any, code int, message string) {
		m := map[string]any{"jsonrpc": "2.0", "id": id}
		if message != "" {
			m["error"] = map[string]any{"code": code, "message": message}
		} else {
			m["result"] = result
		}
		raw, _ := json.Marshal(m)
		out.Write(raw)
		out.WriteString("\n")
		out.Flush()
	}
	hang := func() { select {} }
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 64*1024), 8<<20)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var req fakeRequest
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}
		logMethod(req.Method)
		if req.ID == nil {
			continue
		}
		switch req.Method {
		case "initialize":
			if cfg.ExitOn == "initialize" {
				code := cfg.ExitCode
				if code == 0 {
					code = 1
				}
				os.Exit(code)
			}
			if cfg.HangOn == "initialize" {
				hang()
			}
			reply(req.ID, map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "mcpfake", "version": "1"},
			}, 0, "")
		case "ping":
			reply(req.ID, map[string]any{}, 0, "")
		case "tools/list":
			if cfg.HangOn == "list" {
				hang()
			}
			listed := make([]map[string]any, 0, len(cfg.Tools))
			for _, tl := range cfg.Tools {
				schema := tl.Schema
				if schema == nil {
					schema = map[string]any{"type": "object"}
				}
				entry := map[string]any{
					"name": tl.Name, "description": tl.Description, "inputSchema": schema,
				}
				if tl.ReadOnly {
					entry["annotations"] = map[string]any{"readOnlyHint": true}
				}
				listed = append(listed, entry)
			}
			reply(req.ID, map[string]any{"tools": listed}, 0, "")
		case "tools/call":
			if cfg.ExitOn == "call" {
				code := cfg.ExitCode
				if code == 0 {
					code = 1
				}
				os.Exit(code)
			}
			if cfg.HangOn == "call" {
				hang()
			}
			var p struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			}
			if err := json.Unmarshal(req.Params, &p); err != nil || p.Name == "" {
				reply(req.ID, nil, -32602, "a tool call must name a tool")
				continue
			}
			known := false
			for _, tl := range cfg.Tools {
				if tl.Name == p.Name {
					known = true
				}
			}
			if !known {
				reply(req.ID, nil, -32602, fmt.Sprintf("unknown tool %q", p.Name))
				continue
			}
			if scripted, ok := cfg.Calls[p.Name]; ok {
				if scripted.Message != "" {
					code := scripted.Code
					if code == 0 {
						code = -32603
					}
					reply(req.ID, nil, code, scripted.Message)
					continue
				}
				reply(req.ID, map[string]any{
					"content": []map[string]any{{"type": "text", "text": scripted.Text}},
					"isError": scripted.IsError,
				}, 0, "")
				continue
			}
			args := string(bytes.TrimSpace(p.Arguments))
			if args == "" {
				args = "{}"
			}
			reply(req.ID, map[string]any{
				"content": []map[string]any{{"type": "text", "text": p.Name + "(" + args + ")"}},
				"isError": false,
			}, 0, "")
		default:
			reply(req.ID, nil, -32601, fmt.Sprintf("unknown method %q", req.Method))
		}
	}
}

// CommandForTest builds the *exec.Cmd a test spawns for cfg. It is a thin
// wrapper over Helper for tests that spawn the fake directly instead of
// through a client.
func CommandForTest(t *testing.T, cfg Config) *exec.Cmd {
	t.Helper()
	command, args, env := Helper(t, cfg)
	cmd := exec.Command(command, args...)
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	return cmd
}
