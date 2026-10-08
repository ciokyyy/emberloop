package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
