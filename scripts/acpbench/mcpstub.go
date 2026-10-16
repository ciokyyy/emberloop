package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// stubTools is the tool list both transports serve.
func stubTools() []any {
	return []any{
		map[string]any{
			"name": "echo", "description": "Echo back the input text.",
			"inputSchema": map[string]any{"type": "object",
				"properties": map[string]any{"text": map[string]any{"type": "string"}},
				"required":   []string{"text"}},
		},
		map[string]any{
			"name": "slow", "description": "Sleep ms milliseconds, then answer.",
			"inputSchema": map[string]any{"type": "object",
				"properties": map[string]any{"ms": map[string]any{"type": "number"}},
				"required":   []string{}},
		},
	}
}

// appendStubLog records one stub-side event: {"event":"tools/list"} or
// {"event":"tools/call","tool":name}. Agents could fake session/update
// traffic; the stub log is server-side proof the tool really ran.
func appendStubLog(path, event, tool string, args map[string]any) {
	if path == "" {
		return
	}
	entry := map[string]any{"event": event, "at": time.Now().UTC().Format(time.RFC3339Nano)}
	if tool != "" {
		entry["tool"] = tool
	}
	if args != nil {
		entry["arguments"] = args
	}
	b, _ := json.Marshal(entry)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(append(b, '\n'))
}

// stubLogMu serializes log appends within one stub process.
var stubLogMu sync.Mutex

func logStub(path, event, tool string, args map[string]any) {
	stubLogMu.Lock()
	defer stubLogMu.Unlock()
	appendStubLog(path, event, tool, args)
}

// readStubLog summarizes a stub log file: listSeen + per-tool call counts.
func readStubLog(path string) (listSeen bool, calls map[string]int) {
	calls = map[string]int{}
	if path == "" {
		return false, calls
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return false, calls
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var e struct {
			Event string `json:"event"`
			Tool  string `json:"tool"`
		}
		if json.Unmarshal([]byte(line), &e) != nil {
			continue
		}
		switch e.Event {
		case "tools/list":
			listSeen = true
		case "tools/call":
			calls[e.Tool]++
			calls["*"]++
		}
	}
	return listSeen, calls
}

// dispatchStubCall runs one MCP method, logging list/call server-side.
// Returns (result, isError, handled).
func dispatchStubCall(logPath, method string, params json.RawMessage) (any, string, bool) {
	switch method {
	case "initialize":
		return map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "acpbench-stub", "version": version},
		}, "", true
	case "ping":
		return map[string]any{}, "", true
	case "tools/list":
		logStub(logPath, "tools/list", "", nil)
		return map[string]any{"tools": stubTools()}, "", true
	case "tools/call":
		var p struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		_ = json.Unmarshal(params, &p)
		logStub(logPath, "tools/call", p.Name, p.Arguments)
		switch p.Name {
		case "echo":
			text, _ := p.Arguments["text"].(string)
			return map[string]any{"content": []any{
				map[string]any{"type": "text", "text": "echo:" + text}}}, "", true
		case "slow":
			ms := 2000.0
			if v, ok := p.Arguments["ms"].(float64); ok {
				ms = v
			}
			time.Sleep(time.Duration(ms) * time.Millisecond)
			return map[string]any{"content": []any{
				map[string]any{"type": "text", "text": fmt.Sprintf("slept %vms", ms)}}}, "", true
		default:
			return nil, "unknown tool " + p.Name, true
		}
	}
	return nil, "", false
}

// runMCPStub serves the hidden "mcp-stub" subcommand: a minimal MCP server
// exposing tools echo(text) and slow(ms).
//
// Transports:
//
//	stdio (default): agents reach it through session/new
//	  mcpServers=[{command: <this binary>, args: ["mcp-stub"]}].
//	http:  mcp-stub --http 127.0.0.1:0 --stub-log FILE prints
//	  "MCP_URL http://..." on stdout and serves POST /mcp plus GET /sse,
//	  for agents that only accept http/sse server configs.
//
// Both transports append every tools/list + tools/call to --stub-log:
// server-side proof the agent really invoked the tool.
func runMCPStub() int {
	fs := flag.NewFlagSet("mcp-stub", flag.ContinueOnError)
	logPath := fs.String("stub-log", "", "append JSON lines for each tools/list and tools/call")
	httpAddr := fs.String("http", "", "serve HTTP instead of stdio (e.g. 127.0.0.1:0)")
	_ = fs.Parse(os.Args[2:])
	if *httpAddr != "" {
		return runMCPStubHTTP(*httpAddr, *logPath)
	}
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()
	write := func(id json.RawMessage, result any) {
		b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id), "result": result})
		out.Write(append(b, '\n'))
		out.Flush()
	}
	writeErr := func(id json.RawMessage, code int, msg string) {
		b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id),
			"error": map[string]any{"code": code, "message": msg}})
		out.Write(append(b, '\n'))
		out.Flush()
	}
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var msg struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		if msg.ID == nil {
			continue // notification: nothing to answer
		}
		if res, errMsg, ok := dispatchStubCall(*logPath, msg.Method, msg.Params); ok {
			if errMsg != "" {
				writeErr(msg.ID, -32602, errMsg)
			} else {
				write(msg.ID, res)
			}
			continue
		}
		switch msg.Method {
		case "notifications/initialized", "notifications/cancelled":
			// no reply
		default:
			writeErr(msg.ID, -32601, "Method not found")
		}
	}
	return 0
}

// runMCPStubHTTP serves the stub over HTTP for http/sse-only agents.
func runMCPStubHTTP(addr, logPath string) int {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mcp-stub http listen:", err)
		return 1
	}
	mux := http.NewServeMux()
	handleRPC := func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 16*1024*1024))
		var msg struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(body, &msg) != nil {
			w.WriteHeader(400)
			return
		}
		res, errMsg, ok := dispatchStubCall(logPath, msg.Method, msg.Params)
		w.Header().Set("Content-Type", "application/json")
		if !ok {
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": msg.ID,
				"error": map[string]any{"code": -32601, "message": "Method not found"}})
			return
		}
		if errMsg != "" {
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": msg.ID,
				"error": map[string]any{"code": -32602, "message": errMsg}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": res})
	}
	mux.HandleFunc("/mcp", handleRPC)
	mux.HandleFunc("/", handleRPC)
	mux.HandleFunc("/sse", func(w http.ResponseWriter, r *http.Request) {
		logStub(logPath, "tools/list", "", nil) // an SSE connect implies discovery
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		fmt.Fprintf(w, "event: endpoint\ndata: /mcp\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
	})
	fmt.Printf("MCP_URL http://%s/mcp\n", ln.Addr().String())
	_ = http.Serve(ln, mux)
	return 0
}
