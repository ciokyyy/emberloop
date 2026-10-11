package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// TestLivePigACP drives a real pig through pi-acp: a tool turn, then a
// spawned scout Subagent whose grouped result returns to the parent. Not in
// the main check: set EL_LIVE_PIG_ACP=1 and PIG_ACP_SPAWN_BIN to a built
// emberloop-web binary (it runs the pig shim).
func TestLivePigACP(t *testing.T) {
	if os.Getenv("EL_LIVE_PIG_ACP") == "" {
		t.Skip("set EL_LIVE_PIG_ACP=1 and PIG_ACP_SPAWN_BIN for the pi-acp probe")
	}
	bin := os.Getenv("PIG_ACP_SPAWN_BIN")
	model := os.Getenv("PIG_LIVE_MODEL")
	if model == "" {
		model = "opencode-go/muse-spark-1.3-contributor"
	}
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	be, err := NewPigACPBackend(tempDir(t), bin, srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	d.RegisterBackend("pig", be)

	id := createSession(t, srv.URL, "pig", model, tempDir(t))
	sendPrompt(t, srv.URL, id, "Run bash: echo ACP-TOOL-OK. Then reply with exactly: TURN-ONE-DONE")
	es := pollEvents(t, srv.URL, id, 3*time.Minute, func(es []Event) bool {
		return strings.Contains(assistantText(es), "TURN-ONE-DONE")
	})
	if !hasKind(es, "tool_call_started") {
		t.Fatalf("no tool call: %+v", es)
	}
	for _, e := range es {
		if e.Kind == "assistant_chunk" && strings.Contains(e.Text, "New version") {
			t.Fatalf("update notice leaked into transcript")
		}
	}

	sendPrompt(t, srv.URL, id, "Use the spawn tool once with one scout agent whose prompt is: Reply with exactly SCOUT-SAYS-HI. Then wait for its result and reply with exactly: PARENT-GOT-RESULT")
	es = pollEvents(t, srv.URL, id, 5*time.Minute, func(es []Event) bool {
		return hasKind(es, "subagent_finished") && strings.Contains(assistantText(es), "PARENT-GOT-RESULT")
	})
	if !hasKind(es, "subagent_spawned") {
		t.Fatalf("no subagent_spawned: %+v", es)
	}

	sendPrompt(t, srv.URL, id, "Run bash: sleep 12; echo one. After that command, reply with exactly: AFTER-ONE")
	time.Sleep(4 * time.Second)
	steerReq, _ := http.NewRequest("POST", srv.URL+"/api/sessions/"+id+"/steer", strings.NewReader(`{"text":"STEER: after the current command, do not say AFTER-ONE. Reply with exactly: STEER-HONORED"}`))
	steerReq.Header.Set("Content-Type", "application/json")
	if resp, err := http.DefaultClient.Do(steerReq); err != nil || resp.StatusCode != http.StatusAccepted {
		t.Fatalf("steer: %v %v", err, resp)
	}
	es = pollEvents(t, srv.URL, id, 3*time.Minute, func(es []Event) bool {
		return strings.Contains(assistantText(es), "STEER-HONORED")
	})
	if strings.Contains(assistantText(es[lastPromptIndex(es):]), "AFTER-ONE") {
		t.Logf("model still said AFTER-ONE after the steer")
	}

	req, _ := http.NewRequest("DELETE", srv.URL+"/api/sessions/"+id, nil)
	if resp, err := http.DefaultClient.Do(req); err != nil || resp.StatusCode >= 300 {
		t.Fatalf("delete: %v %v", err, resp)
	}
}

// assistantText joins every assistant chunk: the stream splits words across
// chunks, so a marker never sits whole in one.
func assistantText(es []Event) string {
	var b strings.Builder
	for _, e := range es {
		if e.Kind == "assistant_chunk" {
			b.WriteString(e.Text)
		}
	}
	return b.String()
}

func lastPromptIndex(es []Event) int {
	for i := len(es) - 1; i >= 0; i-- {
		if es[i].Kind == "prompt" && es[i].Mode != "steer" {
			return i
		}
	}
	return 0
}
