package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// TestLiveGrokACP drives a real `grok agent stdio` on the user's Grok login:
// an el MCP tool turn through use_tool, a second turn that proves the
// conversation carries, then a Steer that must degrade to a Follow-up. Not in
// the main check: set EL_LIVE_GROK=1 with grok logged in and el-mcp on PATH.
func TestLiveGrokACP(t *testing.T) {
	if os.Getenv("EL_LIVE_GROK") == "" {
		t.Skip("set EL_LIVE_GROK=1 (grok logged in, el-mcp on PATH) for the grok probe")
	}
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	be, err := NewGrokACPBackend(tempDir(t), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	d.RegisterBackend("grok", be)

	model := be.Models()[0]
	wd := tempDir(t)
	if err := os.WriteFile(wd+"/hello.txt", []byte("the magic word is KUMQUAT\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	id := createSession(t, srv.URL, "grok", model, wd)
	sendPrompt(t, srv.URL, id, "Read hello.txt in your working directory with the el read tool and reply with exactly: WORD-IS-<the magic word>")
	es := pollEvents(t, srv.URL, id, 3*time.Minute, func(es []Event) bool {
		return strings.Contains(assistantText(es), "WORD-IS-KUMQUAT")
	})
	if !hasKind(es, "tool_call_started") {
		t.Fatalf("no tool call: %+v", es)
	}
	for _, e := range es {
		if e.Kind == "tool_call_started" && strings.Contains(e.Text, "search_tool") {
			t.Fatalf("catalog should make search_tool unnecessary: %s", e.Text)
		}
	}

	sendPrompt(t, srv.URL, id, "What was the magic word? Reply with exactly: AGAIN-<word>")
	pollEvents(t, srv.URL, id, 3*time.Minute, func(es []Event) bool {
		return strings.Contains(assistantText(es), "AGAIN-KUMQUAT")
	})

	sendPrompt(t, srv.URL, id, "Count from 1 to 60, one number per line, then reply with exactly: COUNT-DONE")
	time.Sleep(3 * time.Second)
	req, _ := http.NewRequest("POST", srv.URL+"/api/sessions/"+id+"/steer", strings.NewReader(`{"text":"Reply with exactly: STEER-LANDED"}`))
	req.Header.Set("Content-Type", "application/json")
	if resp, err := http.DefaultClient.Do(req); err != nil || resp.StatusCode != http.StatusAccepted {
		t.Fatalf("steer: %v %v", err, resp)
	}
	pollEvents(t, srv.URL, id, 4*time.Minute, func(es []Event) bool {
		return strings.Contains(assistantText(es), "STEER-LANDED")
	})

	del, _ := http.NewRequest("DELETE", srv.URL+"/api/sessions/"+id, nil)
	if resp, err := http.DefaultClient.Do(del); err != nil || resp.StatusCode >= 300 {
		t.Fatalf("delete: %v %v", err, resp)
	}
}
