package web

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// The spawn tool is served by the Daemon, not by the existing emberloop MCP
// server: groups, delivery and scout policy all live in Daemon state, and a
// second owner calling back would split them. Pig takes the tool in-process
// (spawnAgentTool, bound per Session); claude reaches the same implementation
// through this stdio MCP server, spawned per Session with the parent in its
// environment. The framing (newline JSON-RPC, tools/list, tools/call) matches
// what the ACP adapter expects of an MCP stdio server.

// SpawnServerEnvs names the environment carrying one spawn server's binding:
// the spawning Session and the loopback Daemon it calls back.
const (
	SpawnParentEnv = "EMBERLOOP_WEB_PARENT"
	SpawnDaemonEnv = "EMBERLOOP_WEB_DAEMON_URL"
)

type mcpRequest struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ServeSpawnMCP serves the spawn tool over MCP stdio until in ends.
// daemonURL is the loopback Daemon (no trailing slash); parentID is the
// spawning Session every call spawns under.
func ServeSpawnMCP(in io.Reader, out io.Writer, daemonURL, parentID string) error {
	if parentID == "" {
		return fmt.Errorf("spawn server: parent session is required")
	}
	daemonURL = strings.TrimSuffix(daemonURL, "/")
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 64*1024), 8<<20)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var req mcpRequest
		if err := json.Unmarshal(line, &req); err != nil {
			writeMCP(out, mcpResponse{JSONRPC: "2.0", Error: &mcpError{Code: -32700, Message: "the request could not be parsed"}})
			continue
		}
		if req.ID == nil {
			continue
		}
		switch req.Method {
		case "initialize":
			writeMCP(out, mcpResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "emberloop-spawn", "version": "1"},
			}})
		case "ping":
			writeMCP(out, mcpResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}})
		case "tools/list":
			writeMCP(out, mcpResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{
				"tools": []any{spawnMCPTool()},
			}})
		case "tools/call":
			result, mcpErr := callSpawn(daemonURL, parentID, req.Params)
			resp := mcpResponse{JSONRPC: "2.0", ID: req.ID}
			if mcpErr != nil {
				resp.Error = mcpErr
			} else {
				resp.Result = result
			}
			writeMCP(out, resp)
		default:
			writeMCP(out, mcpResponse{JSONRPC: "2.0", ID: req.ID, Error: &mcpError{Code: -32601, Message: "unknown method"}})
		}
	}
	return sc.Err()
}

func spawnMCPTool() map[string]any {
	return map[string]any{
		"name":        "spawn",
		"description": "Spawn Subagents that work in parallel and report back as one grouped result. Each agent takes a prompt and a type (worker or scout); scouts are read-only.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"agents": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":       "object",
						"properties": map[string]any{"prompt": map[string]any{"type": "string"}, "type": map[string]any{"type": "string"}},
						"required":   []string{"prompt"},
					},
				},
			},
			"required": []string{"agents"},
		},
	}
}

func callSpawn(daemonURL, parentID string, params json.RawMessage) (any, *mcpError) {
	var req struct {
		Name      string `json:"name"`
		Arguments struct {
			Agents []SpawnAgent `json:"agents"`
		} `json:"arguments"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &mcpError{Code: -32602, Message: "bad arguments"}
	}
	if req.Name != "spawn" {
		return nil, &mcpError{Code: -32602, Message: "unknown tool"}
	}
	if len(req.Arguments.Agents) == 0 {
		return nil, &mcpError{Code: -32602, Message: "agents is required"}
	}
	body, _ := json.Marshal(map[string]any{"agents": req.Arguments.Agents})
	resp, err := http.Post(daemonURL+"/api/sessions/"+parentID+"/spawn", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, &mcpError{Code: -32000, Message: err.Error()}
	}
	defer resp.Body.Close()
	var out struct {
		Group    string        `json:"group"`
		Sessions []SessionMeta `json:"sessions"`
		Err      string        `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode != http.StatusCreated {
		msg := out.Err
		if msg == "" {
			msg = fmt.Sprintf("spawn refused: %d", resp.StatusCode)
		}
		return nil, &mcpError{Code: -32000, Message: msg}
	}
	ids := make([]string, 0, len(out.Sessions))
	for _, s := range out.Sessions {
		ids = append(ids, s.ID)
	}
	return map[string]any{
		"content": []any{map[string]any{
			"type": "text",
			"text": "spawned group " + out.Group + ": " + strings.Join(ids, " "),
		}},
	}, nil
}

func writeMCP(out io.Writer, resp mcpResponse) {
	b, _ := json.Marshal(resp)
	_, _ = out.Write(append(b, '\n'))
}
