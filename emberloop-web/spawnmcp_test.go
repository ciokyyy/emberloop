package web

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPigSpawnToolSpawnsUnderItsSession(t *testing.T) {
	d := NewDaemon(t.TempDir(), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	parent := createSession(t, srv.URL, "fake", "fake-model", t.TempDir())
	var gotParent string
	var gotAgents []SpawnAgent
	pb := NewPigBackend()
	pb.Spawn = func(parentID string, agents []SpawnAgent) (string, []SessionMeta, error) {
		gotParent, gotAgents = parentID, agents
		return "g1", []SessionMeta{{ID: "c1"}, {ID: "c2"}}, nil
	}
	tools := pb.sessionTools(SessionMeta{ID: parent})
	var tool *spawnAgentTool
	for _, tl := range tools {
		if st, ok := tl.(*spawnAgentTool); ok {
			tool = st
		}
	}
	if tool == nil {
		t.Fatal("spawn tool missing from pig loadout")
	}
	res, err := tool.Execute(context.Background(), "call-1",
		json.RawMessage(`{"agents":[{"prompt":"job A"},{"prompt":"job B","type":"scout"}]}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if gotParent != parent {
		t.Fatalf("spawn parent = %q, want %q", gotParent, parent)
	}
	if len(gotAgents) != 2 || gotAgents[0].Prompt != "job A" || gotAgents[1].Type != "scout" {
		t.Fatalf("spawn agents = %+v", gotAgents)
	}
	if !strings.Contains(res.Text(), "g1") || !strings.Contains(res.Text(), "c1") {
		t.Fatalf("spawn result = %q, want group and ids", res.Text())
	}
}

// Scouts keep a tool-free loadout: no spawn tool, nothing writable through it.
func TestPigScoutHasNoSpawnTool(t *testing.T) {
	pb := NewPigBackend()
	pb.Spawn = func(string, []SpawnAgent) (string, []SessionMeta, error) {
		return "", nil, nil
	}
	for _, tl := range pb.sessionTools(SessionMeta{ID: "s", Scout: true}) {
		if tl.Name() == "spawn" {
			t.Fatal("scout loadout contains spawn")
		}
	}
	if names := len(pb.sessionTools(SessionMeta{ID: "s", Scout: true})); names != 0 {
		t.Fatalf("scout loadout has %d tools, want 0", names)
	}
}

// Claude Sessions get the Daemon-hosted spawn server bound to their id;
// without it the listing is the base server alone. Scouts get neither
// spawn nor write tools: the base server runs under the read-only role.
func TestClaudeMCPServersIncludeSpawnServer(t *testing.T) {
	b := NewClaudeBackend()
	servers, err := b.mcpServersFor("sess-1", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 1 {
		t.Fatalf("servers = %d, want 1 without spawn config", len(servers))
	}
	b.SetSpawnServer([]string{"/abs/emberloop-web", "spawn-server"}, "http://127.0.0.1:9")
	servers, err = b.mcpServersFor("sess-1", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 2 {
		t.Fatalf("servers = %d, want base plus spawn", len(servers))
	}
	sp := servers[1]
	if sp.Name != "emberloop-spawn" || sp.Command != "/abs/emberloop-web" {
		t.Fatalf("spawn server = %+v", sp)
	}
	env := map[string]string{}
	for _, kv := range sp.Env {
		env[kv[0]] = kv[1]
	}
	if env[SpawnParentEnv] != "sess-1" || env[SpawnDaemonEnv] != "http://127.0.0.1:9" {
		t.Fatalf("spawn env = %v", env)
	}
	scouts, err := b.mcpServersFor("sess-9", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(scouts) != 1 {
		t.Fatalf("scout servers = %d, want base alone", len(scouts))
	}
	if len(scouts[0].Args) != 2 || scouts[0].Args[0] != "--role" || scouts[0].Args[1] != "scout" {
		t.Fatalf("scout base args = %v, want --role scout", scouts[0].Args)
	}
}

// The stdio server speaks initialize, tools/list and tools/call, and a call
// creates real Subagents under the bound parent.
func TestSpawnMCPServerSpawnsUnderBoundParent(t *testing.T) {
	d := NewDaemon(t.TempDir(), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	parent := createSession(t, srv.URL, "fake", "fake-model", t.TempDir())
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"spawn","arguments":{"agents":[{"prompt":"job A"}]}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"nope","arguments":{}}}`,
	}, "\n") + "\n"
	var out bytes.Buffer
	if err := ServeSpawnMCP(strings.NewReader(in), &out, srv.URL, parent); err != nil {
		t.Fatal(err)
	}
	replies := map[int]map[string]any{}
	sc := bufio.NewScanner(&out)
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatal(err)
		}
		var id int
		raw, _ := json.Marshal(m["id"])
		_ = json.Unmarshal(raw, &id)
		replies[id] = m
	}
	if _, ok := replies[1]["result"]; !ok {
		t.Fatalf("initialize reply = %v", replies[1])
	}
	list, _ := replies[2]["result"].(map[string]any)
	found := false
	for _, tl := range list["tools"].([]any) {
		if m, _ := tl.(map[string]any); m["name"] == "spawn" {
			found = true
		}
	}
	if !found {
		t.Fatalf("tools/list missing spawn: %v", replies[2])
	}
	call, _ := replies[3]["result"].(map[string]any)
	content, _ := json.Marshal(call["content"])
	if !strings.Contains(string(content), "spawned group") {
		t.Fatalf("tools/call result = %v", replies[3])
	}
	if _, ok := replies[4]["error"]; !ok {
		t.Fatalf("unknown tool reply = %v, want error", replies[4])
	}
	// The call created a real child of the bound parent.
	resp := getSessions(t, srv.URL)
	for _, m := range resp {
		if m.ParentID == parent {
			return
		}
	}
	t.Fatalf("no child of %s in %+v", parent, resp)
}

func getSessions(t *testing.T, base string) []SessionMeta {
	t.Helper()
	resp, err := http.Get(base + "/api/sessions")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out []SessionMeta
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}
