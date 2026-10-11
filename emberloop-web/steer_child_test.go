package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func wirePigSteer(d *Daemon) *PigBackend {
	pb := NewPigBackend()
	pb.Spawn = func(parentID string, agents []SpawnAgent, deliver string) (string, []SessionMeta, error) {
		gid, kids, code, msg := d.Spawn(parentID, agents, "")
		if code != 0 {
			return "", nil, fmt.Errorf("spawn: %s", msg)
		}
		return gid, kids, nil
	}
	pb.SteerChild = func(parentID, childID, text string) (string, error) {
		outcome, code, msg := d.SteerChild(parentID, childID, text)
		if code != 0 {
			return "", fmt.Errorf("steer: %s", msg)
		}
		return outcome, nil
	}
	return pb
}

func pigSteerTool(t *testing.T, pb *PigBackend, sessionID string) *steerAgentTool {
	t.Helper()
	for _, tl := range pb.sessionTools(SessionMeta{ID: sessionID}) {
		if st, ok := tl.(*steerAgentTool); ok {
			return st
		}
	}
	t.Fatalf("steer tool missing from pig loadout of %s", sessionID)
	return nil
}

func spawnOne(t *testing.T, base, parent, prompt string) string {
	t.Helper()
	code, out := postSpawn(t, base, parent, map[string]any{
		"agents": []map[string]any{{"prompt": prompt}},
	})
	if code != http.StatusCreated {
		t.Fatalf("spawn status = %d (%v)", code, out)
	}
	raw, _ := json.Marshal(out["sessions"])
	var kids []SessionMeta
	if err := json.Unmarshal(raw, &kids); err != nil || len(kids) != 1 {
		t.Fatalf("spawn sessions = %v, want 1", out["sessions"])
	}
	return kids[0].ID
}

func waitPrompt(t *testing.T, base, id, substr string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		_, events := sessionMeta(t, base, id)
		for _, e := range events {
			if e.Kind == "prompt" && strings.Contains(e.Text, substr) {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	_, events := sessionMeta(t, base, id)
	t.Fatalf("prompt %q never delivered to %s in %+v", substr, id, events)
}

// A parent steers its own working child through the pig tool: the message
// lands as a steer without interrupting the parent turn.
func TestSteerChildParentSteersOwnWorkingChild(t *testing.T) {
	fake := NewFakeBackend()
	fake.Delay = 400 * time.Millisecond
	d := NewDaemon(tempDir(t), fake)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()
	pb := wirePigSteer(d)

	parent := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	child := spawnOne(t, srv.URL, parent, "slow job")
	waitStatus(t, srv.URL, child, "working", 5*time.Second)

	tool := pigSteerTool(t, pb, parent)
	res, err := tool.Execute(context.Background(), "call-1",
		json.RawMessage(`{"id":"`+child+`","message":"go left"}`), nil)
	if err != nil {
		t.Fatalf("steer own child: %v", err)
	}
	if !strings.Contains(res.Text(), "steered") || !strings.Contains(res.Text(), child) {
		t.Fatalf("steer result = %q, want steered plus child id", res.Text())
	}
	if meta, _ := sessionMeta(t, srv.URL, parent); meta.Status == "working" {
		t.Fatalf("parent status = working, want idle (steer must not interrupt it)")
	}
	waitPrompt(t, srv.URL, child, "go left", 10*time.Second)
	waitStatus(t, srv.URL, child, "idle", 15*time.Second)
	waitParentIdle(t, srv.URL, parent)
}

// A parent steers its idle child through the Daemon: it starts as a
// follow-up turn.
func TestSteerChildIdleStartsFollowup(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	parent := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	child := spawnOne(t, srv.URL, parent, "quick job")
	waitStatus(t, srv.URL, child, "idle", 10*time.Second)

	outcome, code, msg := d.SteerChild(parent, child, "after that")
	if code != 0 {
		t.Fatalf("steer idle child code = %d (%s)", code, msg)
	}
	if !strings.Contains(outcome, "started follow-up") || !strings.Contains(outcome, child) {
		t.Fatalf("steer idle outcome = %q, want started follow-up plus id", outcome)
	}
	waitPrompt(t, srv.URL, child, "after that", 10*time.Second)
	waitStatus(t, srv.URL, child, "idle", 10*time.Second)
	waitParentIdle(t, srv.URL, parent)
}

// A stranger's child is refused on every path: no cross-session messaging.
func TestSteerChildNonChildRefused(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()
	pb := wirePigSteer(d)

	owner := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	stranger := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	child := spawnOne(t, srv.URL, owner, "job A")
	waitStatus(t, srv.URL, child, "idle", 10*time.Second)

	if _, code, _ := d.SteerChild(stranger, child, "hijack"); code != http.StatusForbidden {
		t.Fatalf("SteerChild non-child code = %d, want 403", code)
	}
	tool := pigSteerTool(t, pb, stranger)
	if _, err := tool.Execute(context.Background(), "c1",
		json.RawMessage(`{"id":"`+child+`","message":"hijack"}`), nil); err == nil ||
		!strings.Contains(err.Error(), "not a child") {
		t.Fatalf("pig steer non-child err = %v, want not-a-child", err)
	}
	if _, err := tool.Execute(context.Background(), "c2",
		json.RawMessage(`{"id":"missing","message":"hijack"}`), nil); err == nil {
		t.Fatalf("pig steer unknown child err = nil, want error")
	}

	if _, err := proxyCallOne(t, srv.URL, stranger, "steer", `{"id":"`+child+`","message":"hijack"}`); err == nil ||
		!strings.Contains(err.Error(), "not a child") {
		t.Fatalf("proxy steer non-child err = %v, want not-a-child", err)
	}
	time.Sleep(200 * time.Millisecond)
	_, events := sessionMeta(t, srv.URL, child)
	for _, e := range events {
		if e.Kind == "prompt" && strings.Contains(e.Text, "hijack") {
			t.Fatalf("refused steer delivered: %+v", e)
		}
	}
	waitParentIdle(t, srv.URL, owner)
}

// Scout targets stay read-only on every path.
func TestSteerChildScoutRefused(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()
	pb := wirePigSteer(d)

	parent := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	code, out := postSpawn(t, srv.URL, parent, map[string]any{
		"agents": []map[string]any{{"prompt": "look only", "type": "scout"}},
	})
	if code != http.StatusCreated {
		t.Fatalf("spawn status = %d (%v)", code, out)
	}
	raw, _ := json.Marshal(out["sessions"])
	var kids []SessionMeta
	_ = json.Unmarshal(raw, &kids)

	if _, code, _ := d.SteerChild(parent, kids[0].ID, "write this"); code != http.StatusForbidden {
		t.Fatalf("SteerChild scout code = %d, want 403", code)
	}
	tool := pigSteerTool(t, pb, parent)
	if _, err := tool.Execute(context.Background(), "c1",
		json.RawMessage(`{"id":"`+kids[0].ID+`","message":"write this"}`), nil); err == nil ||
		!strings.Contains(err.Error(), "read-only") {
		t.Fatalf("pig steer scout err = %v, want read-only", err)
	}
	if _, err := proxyCallOne(t, srv.URL, parent, "steer", `{"id":"`+kids[0].ID+`","message":"write this"}`); err == nil ||
		!strings.Contains(err.Error(), "read-only") {
		t.Fatalf("proxy steer scout err = %v, want read-only", err)
	}
	waitStatus(t, srv.URL, kids[0].ID, "idle", 10*time.Second)
	waitParentIdle(t, srv.URL, parent)
}

// The proxy steer tool steers the bound parent's own child through the same
// HTTP route the UI uses.
func TestProxySteerSteersOwnChild(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	parent := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	child := spawnOne(t, srv.URL, parent, "job A")
	waitStatus(t, srv.URL, child, "idle", 10*time.Second)

	out, err := proxyCallOne(t, srv.URL, parent, "steer", `{"id":"`+child+`","message":"go left"}`)
	if err != nil {
		t.Fatalf("proxy steer own child: %v", err)
	}
	if !strings.Contains(out, child) ||
		(!strings.Contains(out, "steered") && !strings.Contains(out, "started") && !strings.Contains(out, "follow-up")) {
		t.Fatalf("proxy steer result = %q, want outcome plus child id", out)
	}
	waitPrompt(t, srv.URL, child, "go left", 10*time.Second)
	waitStatus(t, srv.URL, child, "idle", 10*time.Second)
	waitParentIdle(t, srv.URL, parent)
}

// Workers gain spawn, steer, check and cancel; scouts gain none of the four:
// they have no spawn tool, so no children to message, peek at or stop.
func TestPigLoadoutWorkerGainsSubagentTools(t *testing.T) {
	pb := NewPigBackend()
	pb.Spawn = func(string, []SpawnAgent, string) (string, []SessionMeta, error) { return "", nil, nil }
	pb.SteerChild = func(string, string, string) (string, error) { return "", nil }
	pb.CheckChild = func(string, string) (string, error) { return "", nil }
	pb.CancelChild = func(string, string, bool) (string, error) { return "", nil }
	names := map[string]bool{}
	for _, tl := range pb.sessionTools(SessionMeta{ID: "w"}) {
		names[tl.Name()] = true
	}
	for _, want := range []string{"spawn", "steer", "check", "cancel"} {
		if !names[want] {
			t.Fatalf("worker loadout = %v, want %q", names, want)
		}
	}
	for _, tl := range pb.sessionTools(SessionMeta{ID: "s", Scout: true}) {
		switch tl.Name() {
		case "spawn", "steer", "check", "cancel":
			t.Fatalf("scout loadout contains %q", tl.Name())
		}
	}
}
