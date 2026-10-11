package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// A spawn name lands on the child meta, the list, and the parent's
// spawned payload; an empty name falls back to four prompt words.
func TestSpawnNamesAndFallback(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()
	base := srv.URL

	parent := createSession(t, base, "fake", "fake-model", tempDir(t))
	code, out := postSpawn(t, base, parent, map[string]any{
		"agents": []map[string]any{
			{"prompt": "job A does things", "name": "Alpha"},
			{"prompt": "run the full test suite now please"},
		},
	})
	if code != http.StatusCreated {
		t.Fatalf("spawn status = %d (%v)", code, out)
	}
	raw, _ := json.Marshal(out["sessions"])
	var kids []SessionMeta
	if err := json.Unmarshal(raw, &kids); err != nil || len(kids) != 2 {
		t.Fatalf("spawn sessions = %v, want 2", out["sessions"])
	}
	if kids[0].Name != "Alpha" {
		t.Fatalf("child name = %q, want Alpha", kids[0].Name)
	}
	if kids[1].Name != "run the full test" {
		t.Fatalf("child fallback name = %q, want first four words", kids[1].Name)
	}
	// The session list carries the names for the sidebar.
	byID := map[string]SessionMeta{}
	for _, m := range getSessions(t, base) {
		byID[m.ID] = m
	}
	if byID[kids[0].ID].Name != "Alpha" || byID[kids[1].ID].Name != "run the full test" {
		t.Fatalf("listed names = %+v", byID)
	}
	// The parent spawned payloads carry group and name, never a bare id.
	_, events := sessionMeta(t, base, parent)
	names := map[string]bool{}
	for _, e := range events {
		if e.Kind != "subagent_spawned" {
			continue
		}
		p := parseSpawnedPayload(e.Text)
		if p.ID == "" || p.Group == "" || p.Name == "" {
			t.Fatalf("spawned payload = %q, want id, group and name", e.Text)
		}
		names[p.Name] = true
	}
	if !names["Alpha"] || !names["run the full test"] {
		t.Fatalf("spawned names = %v", names)
	}
	waitStatus(t, base, kids[0].ID, "idle", 10*time.Second)
	waitStatus(t, base, kids[1].ID, "idle", 10*time.Second)
	waitParentIdle(t, base, parent)
}

// agentDisplayName trims an explicit name and falls back to four words.
func TestAgentDisplayName(t *testing.T) {
	if got := agentDisplayName("  Alpha ", "job A"); got != "Alpha" {
		t.Fatalf("explicit name = %q", got)
	}
	if got := agentDisplayName("", "one two three four five"); got != "one two three four" {
		t.Fatalf("fallback = %q", got)
	}
	if got := agentDisplayName("", "solo"); got != "solo" {
		t.Fatalf("short fallback = %q", got)
	}
}

// An unknown deliver mode is refused with no half Group.
func TestSpawnUnknownDeliverRefused(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()
	base := srv.URL

	parent := createSession(t, base, "fake", "fake-model", tempDir(t))
	if code, _ := postSpawn(t, base, parent, map[string]any{
		"deliver": "broadcast",
		"agents":  []map[string]any{{"prompt": "job A"}},
	}); code != http.StatusBadRequest {
		t.Fatalf("spawn status = %d, want 400", code)
	}
	if n := len(getSessions(t, base)); n != 1 {
		t.Fatalf("sessions = %d, want parent alone", n)
	}
}

// Each delivery sends one notice per child as it settles.
func TestDeliverEachSendsOneNoticePerChild(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()
	base := srv.URL

	parent := createSession(t, base, "fake", "fake-model", tempDir(t))
	code, out := postSpawn(t, base, parent, map[string]any{
		"deliver": "each",
		"agents": []map[string]any{
			{"prompt": "job A", "name": "Alpha"},
			{"prompt": "job B", "name": "Beta"},
		},
	})
	if code != http.StatusCreated {
		t.Fatalf("spawn status = %d (%v)", code, out)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		_, events := sessionMeta(t, base, parent)
		n, f := 0, 0
		for _, e := range events {
			if e.Kind == "prompt" && e.Mode == "steer" && strings.HasPrefix(e.Text, "Subagent ") {
				n++
			}
			if e.Kind == "subagent_finished" {
				f++
			}
		}
		if n == 2 && f == 2 {
			if !strings.Contains(steerText(events, "Alpha"), "job A") || !strings.Contains(steerText(events, "Beta"), "job B") {
				t.Fatalf("each notices carry the wrong member results")
			}
			waitParentIdle(t, base, parent)
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("each notices = %d, finished = %d, want 2 and 2", n, f)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func steerText(events []Event, want string) string {
	for _, e := range events {
		if e.Kind == "prompt" && strings.Contains(e.Text, want) {
			return e.Text
		}
	}
	return ""
}
