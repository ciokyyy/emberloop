// Package mcp serves tools.Tool values over MCP's stdio transport
// (newline-delimited JSON-RPC), so any harness that speaks MCP gets the same
// tools. Protocol handling follows wick's apps/backend/mcpfacade/mcp.go.
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"example.com/wick/tools"
)

// defaultProtocol answers a client that names no version. A client that does
// name one gets it echoed: the tools/list and tools/call shapes this server
// speaks are the same across the revisions in use.
const defaultProtocol = "2024-11-05"

const maxFrameBytes = 8 << 20

type request struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type server struct {
	byName map[string]tools.Tool
	order  []tools.Tool
	root   string
	mu     sync.Mutex
	out    io.Writer
}

// Serve answers initialize, ping, tools/list and tools/call until in ends.
// root is the workspace every tool call runs in.
func Serve(ctx context.Context, in io.Reader, out io.Writer, root string, served []tools.Tool) error {
	s := &server{byName: map[string]tools.Tool{}, order: served, root: root, out: out}
	for _, t := range served {
		s.byName[t.Name()] = t
	}

	var calls sync.WaitGroup
	defer calls.Wait()

	frames := bufio.NewScanner(in)
	frames.Buffer(make([]byte, 0, 64*1024), maxFrameBytes)
	for frames.Scan() {
		line := bytes.TrimSpace(frames.Bytes())
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			s.reply(nil, nil, &rpcError{-32700, "the request could not be parsed"})
			continue
		}
		if req.ID == nil {
			continue // a notification is never answered
		}
		switch req.Method {
		case "tools/call":
			// A call can block for minutes (wait_workers); ping and list must
			// still be answered meanwhile.
			calls.Add(1)
			go func() {
				defer calls.Done()
				s.call(ctx, req)
			}()
		default:
			s.handle(req)
		}
	}
	return frames.Err()
}

func (s *server) handle(req request) {
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		if p.ProtocolVersion == "" {
			p.ProtocolVersion = defaultProtocol
		}
		s.reply(req.ID, map[string]any{
			"protocolVersion": p.ProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "wick", "version": "1"},
		}, nil)
	case "ping":
		s.reply(req.ID, map[string]any{}, nil)
	case "tools/list":
		listed := make([]map[string]any, 0, len(s.order))
		for _, t := range s.order {
			entry := map[string]any{
				"name": t.Name(), "description": t.Description(), "inputSchema": t.Schema(),
			}
			// readOnlyHint is what lets a harness batch a call with others, so
			// it goes on exactly the tools that only look.
			if ro, ok := t.(tools.ReadOnly); ok && ro.ReadOnly() {
				entry["annotations"] = map[string]any{"readOnlyHint": true}
			}
			listed = append(listed, entry)
		}
		s.reply(req.ID, map[string]any{"tools": listed}, nil)
	default:
		s.reply(req.ID, nil, &rpcError{-32601, fmt.Sprintf("unknown method %q", req.Method)})
	}
}

// call runs one tool. A tool that fails is a result with isError set, not a
// protocol error: the model reads it and works around it.
func (s *server) call(ctx context.Context, req request) {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil || p.Name == "" {
		s.reply(req.ID, nil, &rpcError{-32602, "a tool call must name a tool"})
		return
	}
	t, ok := s.byName[p.Name]
	if !ok {
		s.reply(req.ID, nil, &rpcError{-32602, fmt.Sprintf("unknown tool %q", p.Name)})
		return
	}
	if len(p.Arguments) == 0 {
		p.Arguments = json.RawMessage("{}")
	}
	text, err := t.Execute(ctx, s.root, p.Arguments)
	isError := err != nil
	if isError {
		text = err.Error()
	}
	s.reply(req.ID, map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
		"isError": isError,
	}, nil)
}

func (s *server) reply(id json.RawMessage, result any, e *rpcError) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = json.NewEncoder(s.out).Encode(response{JSONRPC: "2.0", ID: id, Result: result, Error: e})
}
