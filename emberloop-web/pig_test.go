package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
)

func newPigTestBackend(t *testing.T) *PigBackend {
	t.Helper()
	return newPigTestBackendAt(t, t.TempDir())
}

func newPigTestBackendAt(t *testing.T, agentDir string) *PigBackend {
	t.Helper()
	t.Setenv("PIG_TEST_FAUX", "1")
	t.Setenv("PIG_TEST_FAUX_SCENARIO", "parity-basic")
	config := `{"providers":{"test-faux":{"baseUrl":"http://localhost:0","api":"test-faux","authHeader":false,"models":[{"id":"faux-1","name":"Test Faux"}]}}}`
	if err := os.WriteFile(filepath.Join(agentDir, "models.json"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	pb := NewPigBackend()
	pb.AgentDir = agentDir
	return pb
}

func newPigDaemon(t *testing.T) (*Daemon, *PigBackend) {
	t.Helper()
	pb := newPigTestBackend(t)
	d := NewDaemon(t.TempDir(), NewFakeBackend())
	d.RegisterBackend("pig", pb)
	return d, pb
}

const pigFauxModel = "test-faux/faux-1"

// The pig picker lists only providers with usable auth (fake openrouter key
// in auth.json here), not the whole SDK catalog.
func TestPigModelsFilteredToAuthedProviders(t *testing.T) {
	d, pb := newPigDaemon(t)
	writeAuthJSON(t, pb.AgentDir, `{"openrouter":{"type":"api_key","key":"fake-key"}}`)
	t.Setenv("ANTHROPIC_API_KEY", "")
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/models?harness=pig")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("models status = %d", resp.StatusCode)
	}
	var out struct {
		Models []string `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(out.Models, pb.Models()) {
		t.Fatalf("endpoint models != backend models (%d vs %d)", len(out.Models), len(pb.Models()))
	}
	full := map[string]bool{}
	for _, m := range PigModels() {
		full[m] = true
	}
	for _, m := range out.Models {
		if !full[m] {
			t.Fatalf("listed model %q not in SDK catalog", m)
		}
	}
	openrouter := ai.ListModels("openrouter")
	if len(openrouter) == 0 {
		t.Fatal("no openrouter models in catalog")
	}
	if !slices.Contains(out.Models, openrouter[0].Provider+"/"+openrouter[0].ID) {
		t.Fatalf("authed openrouter model missing (len=%d)", len(out.Models))
	}
	for _, m := range ai.ListModels("anthropic") {
		if slices.Contains(out.Models, m.Provider+"/"+m.ID) {
			t.Fatalf("unauthed model %q listed", m.Provider+"/"+m.ID)
		}
	}
	if len(out.Models) >= len(PigModels()) {
		t.Fatalf("filtered list not smaller than catalog (%d vs %d)", len(out.Models), len(PigModels()))
	}
}

// An environment key alone (no auth.json entry) authorizes its provider.
func TestPigModelsEnvKeyAuth(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "fake-key")
	t.Setenv("ANTHROPIC_API_KEY", "")
	pb := NewPigBackend()
	pb.AgentDir = t.TempDir()
	models := pb.Models()
	openrouter := ai.ListModels("openrouter")
	if len(openrouter) == 0 {
		t.Fatal("no openrouter models in catalog")
	}
	if !slices.Contains(models, openrouter[0].Provider+"/"+openrouter[0].ID) {
		t.Fatalf("env-authed openrouter model missing (len=%d)", len(models))
	}
	for _, m := range ai.ListModels("anthropic") {
		if slices.Contains(models, m.Provider+"/"+m.ID) {
			t.Fatalf("unauthed model %q listed", m.Provider+"/"+m.ID)
		}
	}
}

// ?current= keeps the selected model visible even when its provider holds
// no auth, so a stored choice never vanishes from the picker.
func TestModelsEndpointKeepsCurrent(t *testing.T) {
	d, pb := newPigDaemon(t)
	writeAuthJSON(t, pb.AgentDir, `{"openrouter":{"type":"api_key","key":"fake-key"}}`)
	t.Setenv("ANTHROPIC_API_KEY", "")
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	anthropic := ai.ListModels("anthropic")
	if len(anthropic) == 0 {
		t.Fatal("no anthropic models in catalog")
	}
	current := anthropic[0].Provider + "/" + anthropic[0].ID
	if slices.Contains(pb.Models(), current) {
		t.Fatalf("test needs %q unauthed", current)
	}
	resp, err := http.Get(srv.URL + "/api/models?harness=pig&current=" + current)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Models []string `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(out.Models, current) {
		t.Fatalf("current model %q missing (len=%d)", current, len(out.Models))
	}
}

func writeAuthJSON(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "auth.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Slice 2: the Session offers no native tools, only the el MCP tools
// (empty until ticket 06 serves spawn).
func TestPigSessionStrippedToMCPOnly(t *testing.T) {
	d, pb := newPigDaemon(t)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	id := createSession(t, srv.URL, "pig", pigFauxModel, t.TempDir())
	if names := pb.ToolNames(id); len(names) != 0 {
		t.Fatalf("stripped session tools = %q, want none", names)
	}
}

// Slice 2: a pig Session answers a prompt and streams normalized Events.
func TestPigPromptStreamsNormalizedEvents(t *testing.T) {
	d, _ := newPigDaemon(t)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	id := createSession(t, srv.URL, "pig", pigFauxModel, t.TempDir())
	sendPrompt(t, srv.URL, id, "What is 20+22?")

	events := readSSE(t, srv.URL, id, 0, 5, 30*time.Second)
	for _, e := range events {
		if e.Version != 1 {
			t.Fatalf("event version = %d, want 1: %+v", e.Version, e)
		}
		if e.SessionID != id {
			t.Fatalf("event session = %q, want %q", e.SessionID, id)
		}
	}
	kinds := map[string]bool{}
	for _, e := range events {
		kinds[e.Kind] = true
	}
	for _, want := range []string{"prompt", "status", "assistant_chunk"} {
		if !kinds[want] {
			t.Fatalf("kind %q missing in %+v", want, events)
		}
	}
	found := false
	for _, e := range events {
		if e.Kind == "assistant_chunk" && strings.Contains(e.Text, "42") {
			found = true
		}
	}
	if !found {
		t.Fatalf("assistant answer 42 missing in %+v", events)
	}
}

// Slice 3: an unknown model rejects the create instead of a broken Session.
func TestPigUnknownModelRejected(t *testing.T) {
	d, _ := newPigDaemon(t)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{"harness": "pig", "model": "nope/nonexistent-1", "workDir": t.TempDir()})
	resp, err := http.Post(srv.URL+"/api/sessions", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("create status = %d, want 400", resp.StatusCode)
	}
}

// Slice 3: ending the Session closes the SDK session and runtime; the log
// stays readable and a prompt after the end is refused.
func TestPigEndClosesSession(t *testing.T) {
	d, pb := newPigDaemon(t)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	id := createSession(t, srv.URL, "pig", pigFauxModel, t.TempDir())
	sendPrompt(t, srv.URL, id, "What is 20+22?")
	readSSE(t, srv.URL, id, 0, 5, 30*time.Second)

	req, _ := http.NewRequest("DELETE", srv.URL+"/api/sessions/"+id, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete status = %d, want 200", resp.StatusCode)
	}
	if names := pb.ToolNames(id); names != nil {
		t.Fatalf("closed session tools = %q, want none", names)
	}
	// A second close stays safe.
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("second delete status = %d, want 200", resp2.StatusCode)
	}
	// Prompting an ended Session is gone, and its history still reads.
	body, _ := json.Marshal(map[string]string{"text": "again"})
	resp3, err := http.Post(srv.URL+"/api/sessions/"+id+"/prompt", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	resp3.Body.Close()
	if resp3.StatusCode != http.StatusGone {
		t.Fatalf("prompt after end status = %d, want 410", resp3.StatusCode)
	}
	get, err := http.Get(srv.URL + "/api/sessions/" + id)
	if err != nil {
		t.Fatal(err)
	}
	defer get.Body.Close()
	var out struct {
		Session SessionMeta `json:"session"`
		Events  []Event     `json:"events"`
	}
	if err := json.NewDecoder(get.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Session.Status != "ended" {
		t.Fatalf("status = %q, want ended", out.Session.Status)
	}
	if len(out.Events) == 0 {
		t.Fatal("history lost after end")
	}
}

// Slice 4: SDK stream and execution events normalize to the Event schema.
func TestMapAgentEventToolCalls(t *testing.T) {
	msg := func(ev ai.AssistantMessageEvent) agent.MessageUpdateEvent {
		return agent.MessageUpdateEvent{AssistantMessageEvent: ev}
	}
	cases := []struct {
		name string
		in   agent.AgentEvent
		want []normEvent
	}{
		{"text", msg(ai.TextDeltaEvent{Delta: "hi"}), []normEvent{{kind: "assistant_chunk", text: "hi"}}},
		{"thinking", msg(ai.ThinkingDeltaEvent{Delta: "hmm"}), []normEvent{{kind: "thinking_chunk", text: "hmm"}}},
		{"exec start", agent.ToolExecutionStartEvent{ToolName: "echo"}, []normEvent{{kind: "tool_call_started", text: `{"id":"","name":"echo","input":"","output":"","status":"running"}`}}},
		{"exec end", agent.ToolExecutionEndEvent{
			ToolName: "echo",
			Result:   agent.AgentToolResult{Content: []ai.ToolResultMessageContent{ai.TextContent{Text: "done"}}},
		}, []normEvent{{kind: "tool_call_updated", text: `{"id":"","name":"echo","input":"","output":"done","status":"completed"}`}}},
		{"exec error", agent.ToolExecutionEndEvent{
			ToolName: "echo",
			IsError:  true,
			Result:   agent.AgentToolResult{Content: []ai.ToolResultMessageContent{ai.TextContent{Text: "boom"}}},
		}, []normEvent{{kind: "tool_call_updated", text: `{"id":"","name":"echo","input":"","output":"boom","status":"failed"}`}}},
		{"turn noise ignored", agent.TurnStartEvent{}, nil},
		{"queue owned by ticket 05", agent.QueueUpdateEvent{}, nil},
	}
	for _, c := range cases {
		if got := mapAgentEvent(c.in); !normEqual(got, c.want) {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}

func normEqual(a, b []normEvent) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// An idle pig Session releases its SDK stack; the next prompt and a Daemon
// restart resume it through Runtime.Resume with the same answer.
func TestPigIdleReleaseAndRestartResume(t *testing.T) {
	t.Helper()
	stateDir := t.TempDir()
	agentDir := t.TempDir()
	pb := newPigTestBackendAt(t, agentDir)
	d := NewDaemon(stateDir, NewFakeBackend())
	d.RegisterBackend("pig", pb)
	srv := httptest.NewServer(d.Handler())

	id := createSession(t, srv.URL, "pig", pigFauxModel, t.TempDir())
	sendPrompt(t, srv.URL, id, "What is 20+22?")
	_ = readSSE(t, srv.URL, id, 0, 5, 30*time.Second)
	waitForLive(t, srv.URL, id, false, 10*time.Second)
	if names := pb.ToolNames(id); names != nil {
		t.Fatalf("released session tools = %q, want none", names)
	}
	seq1 := lastSeqOf(sessionEvents(t, srv.URL, id))

	sendPrompt(t, srv.URL, id, "What is 20+22?")
	events := readSSE(t, srv.URL, id, seq1, 4, 30*time.Second)
	if !hasChunkWith(events, "42") {
		t.Fatalf("resumed answer 42 missing in %+v", events)
	}
	waitForLive(t, srv.URL, id, false, 10*time.Second)
	seq2 := lastSeqOf(sessionEvents(t, srv.URL, id))
	srv.Close()

	pb2 := newPigTestBackendAt(t, agentDir)
	d2 := NewDaemon(stateDir, NewFakeBackend())
	d2.RegisterBackend("pig", pb2)
	srv2 := httptest.NewServer(d2.Handler())
	defer srv2.Close()
	if view := sessionView(t, srv2.URL, id); !view.Resumable {
		t.Fatalf("resumable = false after restart, note %q", view.ResumeNote)
	}
	sendPrompt(t, srv2.URL, id, "What is 20+22?")
	after := readSSE(t, srv2.URL, id, seq2, 4, 30*time.Second)
	if !hasChunkWith(after, "42") {
		t.Fatalf("post-restart answer 42 missing in %+v", after)
	}
}

func hasChunkWith(events []Event, want string) bool {
	for _, e := range events {
		if e.Kind == "assistant_chunk" && strings.Contains(e.Text, want) {
			return true
		}
	}
	return false
}

// Review: a repeat DELETE returns the Session without another ended Event.
func TestPigDeleteEmitsSingleEnded(t *testing.T) {
	d, _ := newPigDaemon(t)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	id := createSession(t, srv.URL, "pig", pigFauxModel, t.TempDir())
	deleteSession(t, srv.URL, id)
	deleteSession(t, srv.URL, id)

	events := sessionEvents(t, srv.URL, id)
	ended := 0
	for _, e := range events {
		if e.Kind == "status" && e.Status == "ended" {
			ended++
		}
	}
	if ended != 1 {
		t.Fatalf("ended events = %d, want 1", ended)
	}
}

// Review: the test-faux provider passes validation only with PIG_TEST_FAUX.
func TestPigFauxModelRejectedWithoutEnv(t *testing.T) {
	pb := newPigTestBackend(t)
	if err := os.Unsetenv("PIG_TEST_FAUX"); err != nil {
		t.Fatal(err)
	}
	err := pb.CreateSession(SessionMeta{ID: "ghost", Model: pigFauxModel, WorkDir: t.TempDir()}, func(kind, text, status string) {})
	if err == nil || !strings.Contains(err.Error(), "unknown model") {
		t.Fatalf("create err = %v, want unknown model", err)
	}
}

// Review: OnPrompt after CloseSession emits no status edges, so a late
// failed can never regress an ended Session.
func TestPigPromptAfterCloseEmitsNoStatus(t *testing.T) {
	pb := newPigTestBackend(t)
	meta := SessionMeta{ID: "late", Model: pigFauxModel, WorkDir: t.TempDir()}
	noop := func(kind, text, status string) {}
	if err := pb.CreateSession(meta, noop); err != nil {
		t.Fatal(err)
	}
	pb.CloseSession(meta.ID)
	var kinds []string
	pb.OnPrompt(meta.ID, "hi", func(kind, text, status string) { kinds = append(kinds, kind) })
	for _, k := range kinds {
		if k == "status" {
			t.Fatalf("status edge after close: %q", kinds)
		}
	}
}

// Review: ended is terminal against late status edges at the Daemon.
func TestEndedTerminalAgainstLateEdges(t *testing.T) {
	d, _ := newPigDaemon(t)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	id := createSession(t, srv.URL, "pig", pigFauxModel, t.TempDir())
	deleteSession(t, srv.URL, id)
	d.appendEvent(id, "status", "", "failed")

	meta, _ := sessionMeta(t, srv.URL, id)
	if meta.Status != "ended" {
		t.Fatalf("status = %q, want ended", meta.Status)
	}
}

func deleteSession(t *testing.T, base, id string) {
	t.Helper()
	req, _ := http.NewRequest("DELETE", base+"/api/sessions/"+id, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete status = %d, want 200", resp.StatusCode)
	}
}

func sessionEvents(t *testing.T, base, id string) []Event {
	t.Helper()
	get, err := http.Get(base + "/api/sessions/" + id)
	if err != nil {
		t.Fatal(err)
	}
	defer get.Body.Close()
	var out struct {
		Events []Event `json:"events"`
	}
	if err := json.NewDecoder(get.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.Events
}

func sessionMeta(t *testing.T, base, id string) (SessionMeta, []Event) {
	t.Helper()
	get, err := http.Get(base + "/api/sessions/" + id)
	if err != nil {
		t.Fatal(err)
	}
	defer get.Body.Close()
	var out struct {
		Session SessionMeta `json:"session"`
		Events  []Event     `json:"events"`
	}
	if err := json.NewDecoder(get.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.Session, out.Events
}

// lastSeqOf is the newest Event seq, so a test reads only what follows.
func lastSeqOf(events []Event) int64 {
	var m int64
	for _, e := range events {
		if e.Seq > m {
			m = e.Seq
		}
	}
	return m
}

func TestPigSpawnToolSpawnsUnderItsSession(t *testing.T) {
	d := NewDaemon(t.TempDir(), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	parent := createSession(t, srv.URL, "fake", "fake-model", t.TempDir())
	var gotParent string
	var gotAgents []SpawnAgent
	pb := NewPigBackend()
	pb.Spawn = func(parentID string, agents []SpawnAgent, deliver string) (string, []SessionMeta, error) {
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
	pb.Spawn = func(string, []SpawnAgent, string) (string, []SessionMeta, error) {
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
