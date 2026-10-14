// Package mcp serves tools.Tool values over MCP's stdio transport
// (newline-delimited JSON-RPC), so any harness that speaks MCP gets the same
// tools. Protocol handling follows el's apps/backend/mcpfacade/mcp.go.
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"

	"example.com/emberloop-web/el/tools"
)

// staleNotice is the line a server adds to a tool reply when the installed
// binary changed since the server started. It names the cause and the fix:
// the install gate is what prevents this, and a restart picks up the new
// binary. The server never restarts itself: that would drop session state.
const staleNotice = "note: this el server is stale: its binary changed since it started; restart the session to pick up the installed binary."

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
	// exePath is the installed binary this server compares against, captured
	// once at start so a later replace of the file reads as a change.
	// startedID is its content hash then; startedOK is false when the hash
	// could not be read, in which case no reply is ever marked stale.
	exePath   string
	startedID string
	startedOK bool
}

// Serve answers initialize, ping, tools/list and tools/call until in ends.
// root is the workspace every tool call runs in. At start it records its own
// binary's identity; a tools/call that finds the installed binary changed
// since adds the stale line to its reply.
func Serve(ctx context.Context, in io.Reader, out io.Writer, root string, served []tools.Tool) error {
	s := &server{byName: map[string]tools.Tool{}, order: served, root: root, out: out}
	s.exePath = binaryPath()
	if id, err := hashFile(s.exePath); err == nil {
		s.startedID, s.startedOK = id, true
	}
	for _, t := range served {
		s.byName[t.Name()] = t
	}

	var calls sync.WaitGroup
	defer calls.Wait()
	inflight := &inflightCalls{cancels: map[string]context.CancelFunc{}}

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
			// A notification is never answered. A cancellation ends the call
			// it names, so a tool waiting on an agent stops waiting and hands
			// that agent's result to the notice path instead of to a caller
			// that has already gone.
			if req.Method == "notifications/cancelled" {
				var p struct {
					RequestID json.RawMessage `json:"requestId"`
				}
				if json.Unmarshal(req.Params, &p) == nil {
					inflight.cancel(string(p.RequestID))
				}
			}
			continue
		}
		switch req.Method {
		case "tools/call":
			// A call can block for minutes (a blocking spawn or tell);
			// ping and list must still be answered meanwhile.
			calls.Add(1)
			callCtx, cancel := context.WithCancel(ctx)
			key := string(req.ID)
			inflight.add(key, cancel)
			go func() {
				defer calls.Done()
				defer inflight.done(key)
				s.call(callCtx, req)
			}()
		default:
			s.handle(req)
		}
	}
	return frames.Err()
}

// inflightCalls maps a request id to the cancel of its running tool call.
type inflightCalls struct {
	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

func (f *inflightCalls) add(id string, cancel context.CancelFunc) {
	f.mu.Lock()
	f.cancels[id] = cancel
	f.mu.Unlock()
}

func (f *inflightCalls) cancel(id string) {
	f.mu.Lock()
	c := f.cancels[id]
	f.mu.Unlock()
	if c != nil {
		c()
	}
}

func (f *inflightCalls) done(id string) {
	f.mu.Lock()
	c := f.cancels[id]
	delete(f.cancels, id)
	f.mu.Unlock()
	if c != nil {
		c()
	}
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
			"serverInfo":      map[string]any{"name": "el", "version": "1"},
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
	text, images, err := tools.ExecuteMulti(ctx, t, s.root, p.Arguments)
	isError := err != nil
	if isError {
		text = err.Error()
	}
	if s.isStale() {
		if text == "" {
			text = staleNotice
		} else {
			text += "\n" + staleNotice
		}
	}
	content := []map[string]any{{"type": "text", "text": text}}
	for _, img := range images {
		content = append(content, map[string]any{"type": "image", "data": img.Data, "mimeType": img.MimeType})
	}
	s.reply(req.ID, map[string]any{
		"content": content,
		"isError": isError,
	}, nil)
}

// isStale reports whether the installed binary's content differs from what
// this server recorded at start. An unreadable file is not stale: without
// both identities there is nothing to compare.
func (s *server) isStale() bool {
	if !s.startedOK {
		return false
	}
	cur, err := hashFile(s.exePath)
	if err != nil {
		return false
	}
	return cur != s.startedID
}

// binaryPath is the installed binary the staleness check compares against.
// EL_BINARY_PATH overrides it, so a test can point it at a temp file and
// replace that file mid-call; otherwise it is this process's executable.
func binaryPath() string {
	if p := os.Getenv("EL_BINARY_PATH"); p != "" {
		return p
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return exe
}

// hashFile is the content identity of the file at path. An empty path or an
// unreadable file is an error, and the caller treats it as unknown, never
// as changed.
func hashFile(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("no binary path")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (s *server) reply(id json.RawMessage, result any, e *rpcError) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = json.NewEncoder(s.out).Encode(response{JSONRPC: "2.0", ID: id, Result: result, Error: e})
}
