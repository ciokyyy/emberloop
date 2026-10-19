package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeDaemon is the Daemon HTTP contract the proxies call back: spawn,
// session reads and list, steer/follow-up, queue, interrupt, end and submit.
type fakeDaemon struct {
	t                 *testing.T
	spawned           []map[string]any
	spawnDeliver      string
	steered           []string
	ended             []string
	submits           []map[string]any
	sessions          map[string]fakeSession
	order             []string
	includeSuperseded bool
	// bgStarts and bgStops record the background commands the fake saw.
	bgStarts []string
	bgStops  []string
}

type fakeSession struct {
	meta   map[string]any
	events []map[string]any
	live   bool
}

func newFakeDaemon(t *testing.T) *fakeDaemon {
	t.Helper()
	d := &fakeDaemon{t: t, sessions: map[string]fakeSession{}}
	d.sessions["parent"] = fakeSession{meta: map[string]any{"id": "parent", "status": "idle"}}
	d.sessions["child"] = fakeSession{
		meta: map[string]any{"id": "child", "status": "working", "parentId": "parent"},
		events: []map[string]any{
			{"seq": 1, "kind": "prompt", "text": "job A"},
			{"seq": 2, "kind": "assistant_chunk", "text": "working on it"},
		},
		live: true,
	}
	d.sessions["scout"] = fakeSession{
		meta: map[string]any{"id": "scout", "status": "idle", "parentId": "parent", "scout": true},
	}
	d.sessions["stranger"] = fakeSession{meta: map[string]any{"id": "stranger", "status": "idle"}}
	d.sessions["ended"] = fakeSession{meta: map[string]any{"id": "ended", "status": "ended", "parentId": "parent"}}
	d.order = []string{"parent", "child", "scout", "stranger", "ended"}
	return d
}

func (d *fakeDaemon) writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (d *fakeDaemon) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/sessions", func(w http.ResponseWriter, r *http.Request) {
		var all []map[string]any
		for _, id := range d.order {
			all = append(all, d.sessions[id].meta)
		}
		d.writeJSON(w, http.StatusOK, all)
	})
	mux.HandleFunc("/api/sessions/", func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
		id, sub, _ := strings.Cut(rest, "/")
		s, ok := d.sessions[id]
		if !ok {
			http.NotFound(w, r)
			return
		}
		switch {
		case sub == "spawn" && r.Method == http.MethodPost:
			var req struct {
				Agents  []map[string]any `json:"agents"`
				Deliver string           `json:"deliver"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			if len(req.Agents) == 0 {
				d.writeJSON(w, http.StatusBadRequest, map[string]any{"error": "agents is required"})
				return
			}
			if id == "scout" {
				d.writeJSON(w, http.StatusForbidden, map[string]any{"error": "scout sessions are read-only"})
				return
			}
			d.spawned = append(d.spawned, req.Agents...)
			d.spawnDeliver = req.Deliver
			d.writeJSON(w, http.StatusCreated, map[string]any{
				"group":    "g1",
				"sessions": []map[string]any{{"id": "c1"}, {"id": "c2"}},
			})
		case sub == "" && r.Method == http.MethodGet:
			d.writeJSON(w, http.StatusOK, map[string]any{"session": s.meta, "events": s.events, "live": s.live})
		case (sub == "steer" || sub == "followup") && r.Method == http.MethodPost:
			var req struct {
				Text string `json:"text"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			if sc, _ := s.meta["scout"].(bool); sc {
				d.writeJSON(w, http.StatusForbidden, map[string]any{"error": "scout sessions are read-only"})
				return
			}
			d.steered = append(d.steered, sub+":"+req.Text)
			d.writeJSON(w, http.StatusAccepted, map[string]any{"seq": 7, "queued": false})
		case sub == "recall" && r.Method == http.MethodGet:
			d.includeSuperseded = r.URL.Query().Get("include_superseded") == "true"
			d.writeJSON(w, http.StatusOK, map[string]any{"text": "recalled"})
		case sub == "queue":
			d.writeJSON(w, http.StatusOK, map[string]any{"queue": []any{}})
		case sub == "interrupt" && r.Method == http.MethodPost:
			d.writeJSON(w, http.StatusOK, map[string]any{"interrupted": s.meta["status"] == "working"})
		case sub == "submit" && r.Method == http.MethodPost:
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			status, _ := req["status"].(string)
			verdict, _ := req["verdict"].(string)
			if status == "" && verdict == "" {
				http.Error(w, "status is required", http.StatusBadRequest)
				return
			}
			d.submits = append(d.submits, req)
			resp := map[string]any{"submits": len(d.submits)}
			if verdict != "" {
				resp["verdict"] = verdict
				if tip, _ := req["tip"].(string); tip == "stale-tip" {
					resp["stale"] = true
					resp["currentTip"] = "current-tip"
				}
			}
			d.writeJSON(w, http.StatusAccepted, resp)
		case sub == "" && r.Method == http.MethodDelete:
			d.ended = append(d.ended, id)
			d.writeJSON(w, http.StatusOK, map[string]any{})
		case sub == "background" && r.Method == http.MethodPost:
			var req struct {
				Command string `json:"command"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			if strings.TrimSpace(req.Command) == "" {
				d.writeJSON(w, http.StatusBadRequest, map[string]any{"error": "command is required"})
				return
			}
			d.bgStarts = append(d.bgStarts, req.Command)
			d.writeJSON(w, http.StatusOK, map[string]any{
				"process": map[string]any{"id": "aa11", "pid": 4242, "running": true},
				"output":  "listening on 5173",
			})
		case strings.HasPrefix(sub, "background/") && r.Method == http.MethodGet:
			ref := strings.TrimPrefix(sub, "background/")
			if ref != "aa11" {
				http.NotFound(w, r)
				return
			}
			d.writeJSON(w, http.StatusOK, map[string]any{
				"process": map[string]any{"id": ref, "pid": 4242, "running": false, "exit": "exit 0", "output": "line one\nline two"},
			})
		case strings.HasPrefix(sub, "background/") && r.Method == http.MethodDelete:
			ref := strings.TrimPrefix(sub, "background/")
			if ref != "aa11" {
				http.NotFound(w, r)
				return
			}
			d.bgStops = append(d.bgStops, ref)
			d.writeJSON(w, http.StatusOK, map[string]any{"stopped": 1})
		default:
			http.NotFound(w, r)
		}
	})
	return mux
}

// bindProxy points the proxy binding at the fake Daemon as one Session.
func bindProxy(t *testing.T, srv *httptest.Server, session string) {
	t.Helper()
	t.Setenv(SudoGateDaemonEnv, srv.URL)
	t.Setenv(SudoGateSessionEnv, session)
}

func proxyCall(t *testing.T, name, args string) (string, error) {
	t.Helper()
	tool := WebProxyTool(name)
	if tool == nil {
		t.Fatalf("WebProxyTool(%q) = nil", name)
	}
	return tool.Execute(context.Background(), t.TempDir(), json.RawMessage(args))
}

func TestWebProxySpawnPostsAgents(t *testing.T) {
	d := newFakeDaemon(t)
	srv := httptest.NewServer(d.handler())
	defer srv.Close()
	bindProxy(t, srv, "parent")

	out, err := proxyCall(t, "spawn", `{"agents":[{"prompt":"job A"},{"prompt":"look","type":"scout"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "spawned group g1: c1 c2" {
		t.Fatalf("spawn = %q", out)
	}
	if len(d.spawned) != 2 {
		t.Fatalf("spawned agents = %v", d.spawned)
	}
	if _, err := proxyCall(t, "spawn", `{"agents":[]}`); err == nil {
		t.Fatal("empty agents must fail")
	}
}

func TestWebProxySpawnPassesNameAndDeliver(t *testing.T) {
	d := newFakeDaemon(t)
	srv := httptest.NewServer(d.handler())
	defer srv.Close()
	bindProxy(t, srv, "parent")

	out, err := proxyCall(t, "spawn", `{"deliver":"each","agents":[{"prompt":"job A","name":"Alpha"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "spawned group g1: c1 c2" {
		t.Fatalf("spawn = %q", out)
	}
	if len(d.spawned) != 1 || d.spawned[0]["name"] != "Alpha" {
		t.Fatalf("spawned agents = %v, want the name passed through", d.spawned)
	}
	if d.spawnDeliver != "each" {
		t.Fatalf("spawn deliver = %q, want each", d.spawnDeliver)
	}
}

func TestWebProxySteerRoutesByStatus(t *testing.T) {
	d := newFakeDaemon(t)
	srv := httptest.NewServer(d.handler())
	defer srv.Close()
	bindProxy(t, srv, "parent")

	out, err := proxyCall(t, "steer", `{"id":"child","message":"go left"}`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "started steer on child (seq 7)" {
		t.Fatalf("steer working = %q", out)
	}
	if len(d.steered) != 1 || d.steered[0] != "steer:go left" {
		t.Fatalf("steered = %v", d.steered)
	}

	d.sessions["child"] = fakeSession{meta: map[string]any{"id": "child-1", "status": "idle", "parentId": "parent"}}
	out, err = proxyCall(t, "steer", `{"id":"child","message":"go right"}`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "started follow-up on child (seq 7)" {
		t.Fatalf("steer idle = %q", out)
	}

	for _, tc := range []struct{ id, want string }{
		{"stranger", "not a child"},
		{"scout", "read-only"},
		{"ended", "session ended"},
		{"missing", "unknown session"},
	} {
		if _, err := proxyCall(t, "steer", `{"id":"`+tc.id+`","message":"x"}`); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("steer %s err = %v, want %q", tc.id, err, tc.want)
		}
	}
	if _, err := proxyCall(t, "steer", `{"id":"child"}`); err == nil {
		t.Error("steer without message must fail")
	}
}

func TestWebProxyCheckListsAndPeeks(t *testing.T) {
	d := newFakeDaemon(t)
	srv := httptest.NewServer(d.handler())
	defer srv.Close()
	bindProxy(t, srv, "parent")

	out, err := proxyCall(t, "check", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	// Three children of the bound parent: the worker, the scout and the ended one.
	for _, want := range []string{"worker", "scout", "ended"} {
		if !strings.Contains(out, want) {
			t.Errorf("check list = %q, want %q", out, want)
		}
	}
	if strings.Contains(out, "stranger") {
		t.Errorf("check list leaks a stranger: %q", out)
	}

	out, err = proxyCall(t, "check", `{"id":"child"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "status=working") || !strings.Contains(out, "working on it") {
		t.Errorf("check one = %q", out)
	}
	if _, err := proxyCall(t, "check", `{"id":"stranger"}`); err == nil || !strings.Contains(err.Error(), "not a child") {
		t.Errorf("check stranger err = %v", err)
	}
}

func TestWebProxyCancelInterruptsAndKills(t *testing.T) {
	d := newFakeDaemon(t)
	srv := httptest.NewServer(d.handler())
	defer srv.Close()
	bindProxy(t, srv, "parent")

	out, err := proxyCall(t, "cancel", `{"id":"child"}`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "interrupted child" {
		t.Fatalf("cancel working = %q", out)
	}

	d.sessions["child"] = fakeSession{meta: map[string]any{"id": "child-1", "status": "idle", "parentId": "parent"}}
	out, err = proxyCall(t, "cancel", `{"id":"child"}`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "child already idle" {
		t.Fatalf("cancel idle = %q", out)
	}

	out, err = proxyCall(t, "cancel", `{"id":"child","kill":true}`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "killed child" {
		t.Fatalf("cancel kill = %q", out)
	}
	if len(d.ended) != 1 {
		t.Fatalf("ended = %v", d.ended)
	}

	if _, err := proxyCall(t, "cancel", `{"id":"stranger"}`); err == nil || !strings.Contains(err.Error(), "not a child") {
		t.Errorf("cancel stranger err = %v", err)
	}
	// Scouts stop without messaging: the interrupt lands.
	if _, err := proxyCall(t, "cancel", `{"id":"scout"}`); err != nil {
		t.Errorf("cancel scout err = %v", err)
	}
}

func TestWebProxySubmitPostsOwnResult(t *testing.T) {
	d := newFakeDaemon(t)
	srv := httptest.NewServer(d.handler())
	defer srv.Close()
	bindProxy(t, srv, "child")

	out, err := proxyCall(t, "submit", `{"status":"done","completed":["box one"],"dispositions":[{"findingId":"r1-reviewer-spec-1","disposition":"deferred","ticket":"11"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "submitted done (submit 1)" {
		t.Fatalf("submit = %q", out)
	}
	if len(d.submits) != 1 || d.submits[0]["status"] != "done" {
		t.Fatalf("submits = %v", d.submits)
	}
	if got, ok := d.submits[0]["dispositions"].([]any); !ok || len(got) != 1 {
		t.Fatalf("worker dispositions = %v, want the disposition forwarded", d.submits[0]["dispositions"])
	}
	if _, err := proxyCall(t, "submit", `{}`); err == nil || !strings.Contains(err.Error(), "status is required") {
		t.Errorf("submit without status err = %v", err)
	}
}

// A reviewer verdict posts through the same submit proxy: the verdict route
// answers with the verdict and submit count, and a stale tip names the
// current tip so the reviewer can judge it without waiting for a re-prompt.
func TestWebProxySubmitPostsReviewerVerdict(t *testing.T) {
	d := newFakeDaemon(t)
	srv := httptest.NewServer(d.handler())
	defer srv.Close()
	bindProxy(t, srv, "child")

	out, err := proxyCall(t, "submit", `{"verdict":"pass","tip":"abc","ran":[{"command":"go vet ./...","exitCode":0}],"findings":[]}`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "submitted pass (submit 1)" {
		t.Fatalf("submit verdict = %q", out)
	}
	if len(d.submits) != 1 || d.submits[0]["verdict"] != "pass" {
		t.Fatalf("submits = %v", d.submits)
	}

	out, err = proxyCall(t, "submit", `{"verdict":"pass","tip":"stale-tip","ran":[{"command":"go vet ./...","exitCode":0}],"findings":[]}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "stale") || !strings.Contains(out, "current-tip") {
		t.Fatalf("stale submit = %q, want the stale mark and the current tip", out)
	}
}

// The Daemon-hosted background tool starts a command in the Session, reads its
// output with the exit state, and stops it. A refusal keeps the Daemon's own
// message, so a broken command is not reported as a healthy server.
func TestWebProxyBackgroundStartReadStop(t *testing.T) {
	d := newFakeDaemon(t)
	srv := httptest.NewServer(d.handler())
	defer srv.Close()
	bindProxy(t, srv, "parent")

	out, err := StartBackground(context.Background(), "npm run dev")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.bgStarts) != 1 || d.bgStarts[0] != "npm run dev" {
		t.Fatalf("start sent %v", d.bgStarts)
	}
	for _, want := range []string{"aa11", "4242", "listening on 5173"} {
		if !strings.Contains(out, want) {
			t.Errorf("start = %q, want %q", out, want)
		}
	}

	out, err = proxyCall(t, "background", `{"action":"read","pane":"aa11","lines":5}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "line two") || !strings.Contains(out, "[exit 0]") {
		t.Errorf("read = %q, want the tail and its exit state", out)
	}
	out, err = proxyCall(t, "background", `{"action":"wait","pane":"aa11","timeout":5}`)
	if err != nil || !strings.Contains(out, "[exit 0]") {
		t.Errorf("wait = %q, %v, want the exit state at once", out, err)
	}

	out, err = proxyCall(t, "background", `{"action":"stop","pane":"aa11"}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.bgStops) != 1 || d.bgStops[0] != "aa11" || !strings.Contains(out, "closed aa11") {
		t.Fatalf("stop sent %v, said %q", d.bgStops, out)
	}

	if _, err := StartBackground(context.Background(), "   "); err == nil || !strings.Contains(err.Error(), "required") {
		t.Errorf("empty command err = %v", err)
	}
	if _, err := proxyCall(t, "background", `{"action":"start","command":"x"}`); err == nil || !strings.Contains(err.Error(), "unknown action") {
		t.Errorf("start action err = %v, want it gone", err)
	}
	if _, err := proxyCall(t, "background", `{"action":"read","pane":"nope"}`); err == nil || !strings.Contains(err.Error(), "no process") {
		t.Errorf("unknown pane err = %v", err)
	}
	if _, err := proxyCall(t, "background", `{"action":"dance"}`); err == nil || !strings.Contains(err.Error(), "unknown action") {
		t.Errorf("unknown action err = %v", err)
	}
}

func TestWebProxyNeedsBinding(t *testing.T) {
	t.Setenv(SudoGateDaemonEnv, "")
	t.Setenv(SudoGateSessionEnv, "")
	for _, name := range []string{"spawn", "steer", "check", "cancel", "submit"} {
		if _, err := proxyCall(t, name, `{}`); err == nil || !strings.Contains(err.Error(), "not bound") {
			t.Errorf("%s unbound err = %v", name, err)
		}
	}
	if WebProxyTool("nope") != nil {
		t.Error("WebProxyTool(nope) must be nil")
	}
	if _, _, ok := WebDaemonBinding(); ok {
		t.Error("empty binding must not bind")
	}
}

func TestWebProxyRecallOptsIntoSupersededMemories(t *testing.T) {
	d := newFakeDaemon(t)
	srv := httptest.NewServer(d.handler())
	defer srv.Close()
	bindProxy(t, srv, "parent")
	if _, err := proxyCall(t, "recall", `{"query":"wrapped errors"}`); err != nil {
		t.Fatal(err)
	}
	if d.includeSuperseded {
		t.Fatal("default recall included superseded Memories")
	}
	if _, err := proxyCall(t, "recall", `{"query":"wrapped errors","include_superseded":true}`); err != nil {
		t.Fatal(err)
	}
	if !d.includeSuperseded {
		t.Fatal("recall did not forward include_superseded")
	}
}

func TestWebProxyCheckIsReadOnly(t *testing.T) {
	check := WebProxyTool("check")
	ro, ok := check.(ReadOnly)
	if !ok || !ro.ReadOnly() {
		t.Fatal("check must stay read-only so the MCP server can batch it")
	}
	recall := WebProxyTool("recall")
	if ro, ok := recall.(ReadOnly); !ok || !ro.ReadOnly() {
		t.Fatal("recall must stay read-only so the MCP server can batch it")
	}
	for _, name := range []string{"spawn", "steer", "cancel", "submit", "profile", "remember"} {
		if ro, ok := WebProxyTool(name).(ReadOnly); ok && ro.ReadOnly() {
			t.Errorf("%s must not be read-only", name)
		}
	}
}

// Daemon proxies never reach a script, even renamed: steering, peeking,
// stopping, spawning or submitting from inside execute would bypass the
// harness that owns the turn.
func TestWebProxiesStayOutOfExecute(t *testing.T) {
	for _, name := range []string{"spawn", "steer", "check", "cancel", "submit", "profile", "remember", "recall"} {
		if !ExecuteExcludedTool(WebProxyTool(name)) {
			t.Errorf("%s proxy must stay out of execute", name)
		}
		if !ExecuteExcludedTool(Renamed(WebProxyTool(name), "renamed-"+name)) {
			t.Errorf("renamed %s proxy must stay out of execute", name)
		}
	}
	if ExecuteExcludedTool(readTool{}) {
		t.Error("read must stay script-callable")
	}
}

// Every Daemon proxy call presents the machine token from the environment.
func TestWebProxySendsMachineToken(t *testing.T) {
	t.Setenv(SudoGateTokenEnv, "machine-secret")
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	resp, err := webProxyGet(context.Background(), srv.URL+"/api/sessions/s1/queue")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if auth != "Bearer machine-secret" {
		t.Fatalf("Authorization = %q, want the machine token", auth)
	}
}
