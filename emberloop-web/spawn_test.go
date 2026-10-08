package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func postSpawn(t *testing.T, base, parent string, body any) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	resp, err := http.Post(base+"/api/sessions/"+parent+"/spawn", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// Slice 1: a spawn call creates one Subagent Session per requested agent,
// each recording its parent and Group.
func TestSpawnCreatesSubagentsWithParentAndGroup(t *testing.T) {
	d := NewDaemon(t.TempDir(), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	parent := createSession(t, srv.URL, "fake", "fake-model", t.TempDir())
	code, out := postSpawn(t, srv.URL, parent, map[string]any{
		"agents": []map[string]any{
			{"prompt": "job A"},
			{"prompt": "job B", "type": "scout"},
		},
	})
	if code != http.StatusCreated {
		t.Fatalf("spawn status = %d, want 201 (%v)", code, out)
	}
	group, _ := out["group"].(string)
	if group == "" {
		t.Fatalf("spawn group empty in %v", out)
	}
	raw, _ := json.Marshal(out["sessions"])
	var kids []SessionMeta
	if err := json.Unmarshal(raw, &kids); err != nil || len(kids) != 2 {
		t.Fatalf("spawn sessions = %v, want 2", out["sessions"])
	}
	for _, k := range kids {
		if k.ParentID != parent {
			t.Fatalf("child %s parentId = %q, want %q", k.ID, k.ParentID, parent)
		}
		if k.GroupID != group {
			t.Fatalf("child %s groupId = %q, want %q", k.ID, k.GroupID, group)
		}
	}
	if !kids[1].Scout {
		t.Fatalf("second child scout = false, want true")
	}

	// Each Subagent opens its own transcript with its own prompt.
	for i, want := range []string{"job A", "job B"} {
		_, events := sessionMeta(t, srv.URL, kids[i].ID)
		found := false
		for _, e := range events {
			if e.Kind == "prompt" && strings.Contains(e.Text, want) {
				found = true
			}
		}
		if !found {
			t.Fatalf("child %d transcript missing prompt %q", i, want)
		}
	}
}

// Slice 1: the session list links Subagents to their parent, so the tree
// nests them underneath it.
func TestSpawnedSessionsNestUnderParent(t *testing.T) {
	d := NewDaemon(t.TempDir(), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	parent := createSession(t, srv.URL, "fake", "fake-model", t.TempDir())
	_, out := postSpawn(t, srv.URL, parent, map[string]any{
		"agents": []map[string]any{{"prompt": "job A"}, {"prompt": "job B"}},
	})
	raw, _ := json.Marshal(out["sessions"])
	var kids []SessionMeta
	_ = json.Unmarshal(raw, &kids)

	resp, err := http.Get(srv.URL + "/api/sessions")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var all []SessionMeta
	if err := json.NewDecoder(resp.Body).Decode(&all); err != nil {
		t.Fatal(err)
	}
	byID := map[string]SessionMeta{}
	for _, m := range all {
		byID[m.ID] = m
	}
	for _, k := range kids {
		got, ok := byID[k.ID]
		if !ok {
			t.Fatalf("child %s missing from list", k.ID)
		}
		if _, ok := byID[got.ParentID]; !ok {
			t.Fatalf("child %s parent %q missing from list", k.ID, got.ParentID)
		}
	}
	if _, ok := byID[parent]; !ok {
		t.Fatalf("parent %s missing from list", parent)
	}
}

// Slice 2: when the last Group member finishes, the parent receives exactly
// one grouped result as a Steer.
func TestGroupDeliversOneSteerWhenLastFinishes(t *testing.T) {
	d := NewDaemon(t.TempDir(), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	parent := createSession(t, srv.URL, "fake", "fake-model", t.TempDir())
	code, out := postSpawn(t, srv.URL, parent, map[string]any{
		"agents": []map[string]any{{"prompt": "job A"}, {"prompt": "job B"}},
	})
	if code != http.StatusCreated {
		t.Fatalf("spawn status = %d (%v)", code, out)
	}
	group, _ := out["group"].(string)

	// Parent transcript: initial prompt + working + grouped steer prompt +
	// its working + reply + idle, then no more.
	deadline := time.Now().Add(10 * time.Second)
	var steers []Event
	for time.Now().Before(deadline) {
		_, events := sessionMeta(t, srv.URL, parent)
		steers = nil
		for _, e := range events {
			if e.Kind == "prompt" && e.Mode == "steer" && strings.Contains(e.Text, group) {
				steers = append(steers, e)
			}
		}
		if len(steers) >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(steers) != 1 {
		_, events := sessionMeta(t, srv.URL, parent)
		t.Fatalf("grouped steers = %d, want exactly 1 (events %+v)", len(steers), events)
	}
	if !strings.Contains(steers[0].Text, "job A") || !strings.Contains(steers[0].Text, "job B") {
		t.Fatalf("grouped steer missing member results: %q", steers[0].Text)
	}
	// Exactly-once: settle, then confirm no second grouped result arrives.
	time.Sleep(300 * time.Millisecond)
	_, events := sessionMeta(t, srv.URL, parent)
	n := 0
	for _, e := range events {
		if e.Kind == "prompt" && e.Mode == "steer" && strings.Contains(e.Text, group) {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("grouped steers after settle = %d, want exactly 1", n)
	}
}

// Slice 3: a scout Subagent is read-only: user sends are refused.
func TestScoutSubagentCannotWrite(t *testing.T) {
	d := NewDaemon(t.TempDir(), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	parent := createSession(t, srv.URL, "fake", "fake-model", t.TempDir())
	code, out := postSpawn(t, srv.URL, parent, map[string]any{
		"agents": []map[string]any{{"prompt": "look only", "type": "scout"}},
	})
	if code != http.StatusCreated {
		t.Fatalf("spawn status = %d (%v)", code, out)
	}
	raw, _ := json.Marshal(out["sessions"])
	var kids []SessionMeta
	_ = json.Unmarshal(raw, &kids)
	if len(kids) != 1 {
		t.Fatalf("spawn sessions = %v, want 1", out["sessions"])
	}

	for _, mode := range []string{"prompt", "steer", "followup"} {
		body, _ := json.Marshal(map[string]string{"text": "write this"})
		resp, err := http.Post(srv.URL+"/api/sessions/"+kids[0].ID+"/"+mode, "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("%s on scout status = %d, want 403", mode, resp.StatusCode)
		}
	}
	// A scout cannot spawn either: it cannot write, anywhere.
	if code, _ := postSpawn(t, srv.URL, kids[0].ID, map[string]any{
		"agents": []map[string]any{{"prompt": "nested"}},
	}); code != http.StatusForbidden {
		t.Fatalf("spawn on scout status = %d, want 403", code)
	}
}

func countGroupSteers(events []Event, group string) int {
	n := 0
	for _, e := range events {
		if e.Kind == "prompt" && e.Mode == "steer" && strings.Contains(e.Text, group) {
			n++
		}
	}
	return n
}

func countKind(events []Event, kind string) int {
	n := 0
	for _, e := range events {
		if e.Kind == kind {
			n++
		}
	}
	return n
}

// The parent transcript names every spawn and the one grouped finish.
func TestSpawnEmitsParentNotifications(t *testing.T) {
	d := NewDaemon(t.TempDir(), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	parent := createSession(t, srv.URL, "fake", "fake-model", t.TempDir())
	if code, _ := postSpawn(t, srv.URL, parent, map[string]any{
		"agents": []map[string]any{{"prompt": "job A"}, {"prompt": "job B"}},
	}); code != http.StatusCreated {
		t.Fatalf("spawn status = %d", code)
	}
	_, events := sessionMeta(t, srv.URL, parent)
	if n := countKind(events, "subagent_spawned"); n != 2 {
		t.Fatalf("subagent_spawned = %d, want 2", n)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		_, events = sessionMeta(t, srv.URL, parent)
		if countKind(events, "subagent_finished") >= 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no subagent_finished in %+v", events)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if n := countKind(events, "subagent_finished"); n != 1 {
		t.Fatalf("subagent_finished = %d, want exactly 1", n)
	}
}

// Over the cap the whole spawn call is refused with no half Group.
func TestSpawnOverCapRefusesWithoutHalfGroup(t *testing.T) {
	d := NewDaemon(t.TempDir(), NewFakeBackend())
	d.SetMaxRunning(1)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	parent := createSession(t, srv.URL, "fake", "fake-model", t.TempDir())
	code, _ := postSpawn(t, srv.URL, parent, map[string]any{
		"agents": []map[string]any{{"prompt": "job A"}},
	})
	if code != http.StatusTooManyRequests {
		t.Fatalf("spawn status = %d, want 429", code)
	}
	if n := len(getSessions(t, srv.URL)); n != 1 {
		t.Fatalf("sessions = %d, want parent alone", n)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.groups) != 0 {
		t.Fatalf("groups = %d, want none", len(d.groups))
	}
}

// flakyBackend fails one CreateSession, so spawn must roll its siblings back.
type flakyBackend struct {
	*FakeBackend
	calls  int
	failOn int
}

func (b *flakyBackend) CreateSession(meta SessionMeta, emit func(kind, text, status string)) error {
	b.calls++
	if b.calls == b.failOn {
		return errors.New("boom")
	}
	return b.FakeBackend.CreateSession(meta, emit)
}

// A child that fails to start rolls the whole Group back: no half Group,
// no stranded child.
func TestSpawnFailureRollsBackGroup(t *testing.T) {
	be := &flakyBackend{FakeBackend: NewFakeBackend()}
	d := NewDaemon(t.TempDir(), be)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	parent := createSession(t, srv.URL, "fake", "fake-model", t.TempDir())
	be.failOn = be.calls + 2
	code, _ := postSpawn(t, srv.URL, parent, map[string]any{
		"agents": []map[string]any{{"prompt": "job A"}, {"prompt": "job B"}},
	})
	if code != http.StatusBadRequest {
		t.Fatalf("spawn status = %d, want 400", code)
	}
	if n := len(getSessions(t, srv.URL)); n != 1 {
		t.Fatalf("sessions = %d, want parent alone", n)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.groups) != 0 {
		t.Fatalf("groups = %d, want none", len(d.groups))
	}
}

// A restart after delivery never resends the grouped result.
func TestRestartAfterDeliveryNeverResends(t *testing.T) {
	stateDir := t.TempDir()
	d := NewDaemon(stateDir, NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	parent := createSession(t, srv.URL, "fake", "fake-model", t.TempDir())
	code, out := postSpawn(t, srv.URL, parent, map[string]any{
		"agents": []map[string]any{{"prompt": "job A"}, {"prompt": "job B"}},
	})
	if code != http.StatusCreated {
		t.Fatalf("spawn status = %d", code)
	}
	group, _ := out["group"].(string)
	deadline := time.Now().Add(10 * time.Second)
	for {
		_, events := sessionMeta(t, srv.URL, parent)
		if countGroupSteers(events, group) >= 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("grouped steer never delivered")
		}
		time.Sleep(20 * time.Millisecond)
	}
	raw, err := os.ReadFile(filepath.Join(stateDir, "enderbloom", "groups", group+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var g spawnGroup
	if err := json.Unmarshal(raw, &g); err != nil || !g.Delivered {
		t.Fatalf("group file = %s, want delivered", raw)
	}
	// Restart: the reloaded Daemon must not deliver again.
	d2 := NewDaemon(stateDir, NewFakeBackend())
	srv2 := httptest.NewServer(d2.Handler())
	defer srv2.Close()
	time.Sleep(300 * time.Millisecond)
	_, events := sessionMeta(t, srv2.URL, parent)
	if n := countGroupSteers(events, group); n != 1 {
		t.Fatalf("grouped steers after restart = %d, want exactly 1", n)
	}
}

// An ended parent keeps its finish marker but gets no grouped Steer, and
// the Group still completes exactly once.
func TestEndedParentKeepsMarkerWithoutSteer(t *testing.T) {
	fake := NewFakeBackend()
	fake.Delay = 400 * time.Millisecond
	d := NewDaemon(t.TempDir(), fake)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	parent := createSession(t, srv.URL, "fake", "fake-model", t.TempDir())
	code, out := postSpawn(t, srv.URL, parent, map[string]any{
		"agents": []map[string]any{{"prompt": "job A"}, {"prompt": "job B"}},
	})
	if code != http.StatusCreated {
		t.Fatalf("spawn status = %d", code)
	}
	group, _ := out["group"].(string)
	req, _ := http.NewRequest("DELETE", srv.URL+"/api/sessions/"+parent, nil)
	if resp, err := http.DefaultClient.Do(req); err != nil {
		t.Fatal(err)
	} else {
		resp.Body.Close()
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		_, events := sessionMeta(t, srv.URL, parent)
		if countKind(events, "subagent_finished") >= 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no subagent_finished in %+v", events)
		}
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(300 * time.Millisecond)
	_, events := sessionMeta(t, srv.URL, parent)
	if n := countKind(events, "subagent_finished"); n != 1 {
		t.Fatalf("subagent_finished = %d, want exactly 1", n)
	}
	if n := countGroupSteers(events, group); n != 0 {
		t.Fatalf("grouped steers on ended parent = %d, want 0", n)
	}
}
