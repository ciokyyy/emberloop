package web

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The ACP wire layer (dispatch, permission auto-approve, timeouts) runs
// below the stub seam, so it gets its own test: the test binary replays a
// fake adapter over stdio, no real agent involved.

// TestHelperACPAdapter is not a real test: spawnHelperConn runs it as the
// adapter subprocess when GO_TEST_ACP_ADAPTER is set.
func TestHelperACPAdapter(t *testing.T) {
	mode := os.Getenv("GO_TEST_ACP_ADAPTER")
	if mode == "" {
		return
	}
	if mode == "deaf" {
		select {} // never read stdin or exit: Close must kill
	}
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	out := bufio.NewWriter(os.Stdout)
	send := func(v any) {
		b, err := json.Marshal(v)
		if err != nil {
			fmt.Fprintf(os.Stderr, "marshal: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(out, "%s\n", b)
		_ = out.Flush()
	}
	next := func() acpWireMsg {
		if !sc.Scan() {
			os.Exit(0) // stdin closed: clean shutdown
		}
		var m acpWireMsg
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			fmt.Fprintf(os.Stderr, "bad json: %v\n", err)
			os.Exit(1)
		}
		return m
	}
	for {
		m := next()
		if mode == "silent" {
			continue // never answer anything: the parent must time out
		}
		if m.ID == nil {
			continue
		}
		switch m.Method {
		case "initialize":
			res := map[string]any{}
			if mode == "steer" {
				res["_meta"] = map[string]any{"steering": map[string]any{"supported": true}}
			}
			send(map[string]any{"jsonrpc": "2.0", "id": *m.ID, "result": res})
		case "_session/steering":
			meta, _ := m.Params["_meta"].(map[string]any)
			st, _ := meta["steering"].(map[string]any)
			outcome := "promptRequired"
			if m.Params["sessionId"] == "acp-1" && st["idleBehavior"] == "promptRequired" {
				outcome = "injected"
			}
			send(map[string]any{"jsonrpc": "2.0", "id": *m.ID, "result": map[string]any{"outcome": outcome}})
		case "session/new":
			send(map[string]any{"jsonrpc": "2.0", "id": *m.ID, "result": map[string]any{"sessionId": "acp-1"}})
		case "session/load":
			send(map[string]any{"jsonrpc": "2.0", "id": *m.ID, "result": map[string]any{}})
		case "session/prompt":
			promptID := *m.ID
			if mode == "slowturn" {
				// A healthy long turn: traffic every 100ms for 700ms, longer than the idle bound.
				for i := 0; i < 7; i++ {
					time.Sleep(100 * time.Millisecond)
					send(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{
						"sessionId": "acp-1",
						"update":    map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "."}},
					}})
				}
				send(map[string]any{"jsonrpc": "2.0", "id": promptID, "result": map[string]any{"stopReason": "end_turn"}})
				continue
			}
			send(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{
				"sessionId": "acp-1",
				"update": map[string]any{
					"sessionUpdate": "agent_message_chunk",
					"content":       map[string]any{"type": "text", "text": "wire-hello"},
				},
			}})
			send(map[string]any{"jsonrpc": "2.0", "id": 100, "method": "session/request_permission", "params": map[string]any{
				"sessionId": "acp-1",
				"toolCall":  map[string]any{"toolCallId": "t1", "name": "mcp__x__y", "status": "pending"},
				"options": []any{
					map[string]any{"optionId": "allow-once", "name": "Yes", "kind": "allow_once"},
					map[string]any{"optionId": "reject", "name": "No", "kind": "reject_once"},
				},
			}})
			ans := next()
			var ar struct {
				Outcome  string `json:"outcome"`
				OptionID string `json:"optionId"`
			}
			_ = json.Unmarshal(ans.Result, &ar)
			if ar.Outcome != "selected" || ar.OptionID != "allow-once" {
				fmt.Fprintf(os.Stderr, "permission answer = %+v\n", ar)
				os.Exit(1)
			}
			send(map[string]any{"jsonrpc": "2.0", "id": 101, "method": "fs/readTextFile", "params": map[string]any{"path": "/x"}})
			uns := next()
			if uns.Error == nil || uns.Error.Code != -32601 {
				fmt.Fprintf(os.Stderr, "unsupported answer = %+v\n", uns)
				os.Exit(1)
			}
			send(map[string]any{"jsonrpc": "2.0", "id": promptID, "result": map[string]any{"stopReason": "end_turn"}})
		default:
			fmt.Fprintf(os.Stderr, "unknown method %q\n", m.Method)
			os.Exit(1)
		}
	}
}

func spawnHelperConn(t *testing.T, mode string) acpConn {
	t.Helper()
	abs, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GO_TEST_ACP_ADAPTER", mode)
	conn, err := spawnACPConn([]string{abs, "-test.run=TestHelperACPAdapter"})
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

// The wire delivers updates, auto-approves allow-once, and answers unknown
// client requests as unsupported (the helper exits nonzero otherwise).
func TestACPWireStreamsAndAutoApproves(t *testing.T) {
	conn := spawnHelperConn(t, "speak")
	if err := conn.Initialize(); err != nil {
		t.Fatal(err)
	}
	sid, err := conn.NewSession(tempDir(t), []acpMCPServer{{Name: "m", Command: "/abs/mcp"}}, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if sid != "acp-1" {
		t.Fatalf("session id = %q", sid)
	}
	var chunks []string
	var perms [][2]string
	stop, err := conn.Prompt(sid, "hi", acpCallbacks{
		OnUpdate: func(u map[string]any) {
			if s, _ := u["sessionUpdate"].(string); s == "agent_message_chunk" {
				chunks = append(chunks, acpText(u["content"]))
			}
		},
		OnPermission: func(tool, option string) {
			perms = append(perms, [2]string{tool, option})
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if stop != "end_turn" {
		t.Fatalf("stop = %q", stop)
	}
	if len(chunks) != 1 || chunks[0] != "wire-hello" {
		t.Fatalf("chunks = %v", chunks)
	}
	if len(perms) != 1 || perms[0][0] != "mcp__x__y" || perms[0][1] != "allow-once" {
		t.Fatalf("perms = %v", perms)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("helper failed: %v", err)
	}
}

// A hung adapter fails the call instead of hanging the Daemon.
func TestACPCallTimesOut(t *testing.T) {
	conn := spawnHelperConn(t, "silent")
	pc, ok := conn.(*acpProcessConn)
	if !ok {
		t.Fatal("not a process conn")
	}
	pc.ctlTimeout = 200 * time.Millisecond
	start := time.Now()
	err := conn.Initialize()
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("initialize err = %v, want timeout", err)
	}
	if time.Since(start) > 10*time.Second {
		t.Fatalf("timeout took %v", time.Since(start))
	}
	_ = conn.Close()
}

// Close kills an adapter that ignores stdin EOF instead of hanging.
func TestACPCloseKillsDeafAdapter(t *testing.T) {
	conn := spawnHelperConn(t, "deaf")
	pc, ok := conn.(*acpProcessConn)
	if !ok {
		t.Fatal("not a process conn")
	}
	pc.closeTimeout = 200 * time.Millisecond
	done := make(chan error, 1)
	go func() { done <- conn.Close() }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("close hung on deaf adapter")
	}
}

// Initialize reads the advertised steering capability and Steer speaks
// _session/steering with the promptRequired opt-in.
func TestACPWireSteering(t *testing.T) {
	conn := spawnHelperConn(t, "steer")
	defer conn.Close()
	if err := conn.Initialize(); err != nil {
		t.Fatal(err)
	}
	sc, ok := conn.(acpSteerer)
	if !ok || !sc.SteeringSupported() {
		t.Fatal("steering capability not read from initialize")
	}
	injected, err := sc.Steer("acp-1", "go left")
	if err != nil || !injected {
		t.Fatalf("Steer = %v, %v; want injected", injected, err)
	}
}

// An adapter that does not advertise steering reports it unsupported.
func TestACPWireNoSteeringByDefault(t *testing.T) {
	conn := spawnHelperConn(t, "speak")
	defer conn.Close()
	if err := conn.Initialize(); err != nil {
		t.Fatal(err)
	}
	if conn.(acpSteerer).SteeringSupported() {
		t.Fatal("steering reported without the capability")
	}
}

// The prompt timeout bounds silence, not wall clock: a turn that keeps
// streaming outlives the bound.
func TestACPPromptTimeoutIsIdleBound(t *testing.T) {
	conn := spawnHelperConn(t, "slowturn")
	pc, ok := conn.(*acpProcessConn)
	if !ok {
		t.Fatal("not a process conn")
	}
	pc.promptTimeout = 300 * time.Millisecond
	stop, err := conn.Prompt("acp-1", "go", acpCallbacks{})
	if err != nil || stop != "end_turn" {
		t.Fatalf("prompt = %q, %v; want end_turn after 700ms of traffic", stop, err)
	}
	_ = conn.Close()
}
