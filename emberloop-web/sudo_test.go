package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"example.com/emberloop/tools"
)

func postJSON(t *testing.T, url string, body any) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	resp, err := http.Post(url, "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func waitEvent(t *testing.T, d *Daemon, id string, pred func(Event) bool) Event {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		d.mu.Lock()
		evs := append([]Event(nil), d.sessions[id].events...)
		d.mu.Unlock()
		for _, e := range evs {
			if pred(e) {
				return e
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("event not seen")
	return Event{}
}

func TestUsesSudo(t *testing.T) {
	for cmd, want := range map[string]bool{
		"sudo apt update":       true,
		"cd x && sudo rm -rf y": true,
		"echo hi | sudo tee f":  true,
		"echo pseudo":           false,
		"ls sudoers.d":          false,
		"grep sudo notes.txt":   true, // word match; the user simply denies
	} {
		if got := tools.UsesSudo(cmd); got != want {
			t.Errorf("UsesSudo(%q) = %v, want %v", cmd, got, want)
		}
	}
}

// A sudo request waits for the user; an approved one runs and its TOON result
// reaches the Session.
func TestSudoRequestApproveDeliversTOON(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))

	// Gate through the real HTTP client the MCP server uses.
	gate := tools.HTTPSudoGate(srv.URL, id, "")
	msg, taken := gate(t.Context(), tempDir(t), "sudo echo hello-from-root")
	if !taken || !strings.Contains(msg, "sent to the user") {
		t.Fatalf("gate = %q, %v", msg, taken)
	}
	req := waitEvent(t, d, id, func(e Event) bool { return e.Kind == "sudo_request" })
	var rq map[string]any
	_ = json.Unmarshal([]byte(req.Text), &rq)
	rid, _ := rq["id"].(string)
	if rid == "" || rq["state"] != "pending" {
		t.Fatalf("request = %v", rq)
	}

	// No password and no passwordless sudo: the run fails, the result says so.
	code, _ := postJSON(t, srv.URL+"/api/sessions/"+id+"/sudo/"+rid, map[string]any{"action": "approve"})
	if code != http.StatusAccepted {
		t.Fatalf("approve = %d", code)
	}
	if code, _ := postJSON(t, srv.URL+"/api/sessions/"+id+"/sudo/"+rid, map[string]any{"action": "deny"}); code != http.StatusConflict {
		t.Fatalf("second decision = %d, want 409", code)
	}
	res := waitEvent(t, d, id, func(e Event) bool { return e.Kind == "prompt" && strings.HasPrefix(e.Text, "sudo_result:") })
	for _, want := range []string{"  request: " + rid, "  status: ", "  exit: ", `  command: "sudo echo hello-from-root"`, "  output: "} {
		if !strings.Contains(res.Text, want) {
			t.Errorf("result missing %q in\n%s", want, res.Text)
		}
	}
	if strings.Contains(res.Text, "password") && strings.Contains(res.Text, "hunter2") {
		t.Fatal("password leaked")
	}
}

func TestSudoDenyDeliversDenied(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	code, out := postJSON(t, srv.URL+"/api/sessions/"+id+"/sudo", map[string]any{"command": "sudo reboot"})
	if code != http.StatusCreated {
		t.Fatalf("create = %d", code)
	}
	rid, _ := out["id"].(string)
	postJSON(t, srv.URL+"/api/sessions/"+id+"/sudo/"+rid, map[string]any{"action": "deny"})
	res := waitEvent(t, d, id, func(e Event) bool { return e.Kind == "prompt" && strings.HasPrefix(e.Text, "sudo_result:") })
	if !strings.Contains(res.Text, "status: denied") {
		t.Fatalf("result = %s", res.Text)
	}
	if code, _ := postJSON(t, srv.URL+"/api/sessions/"+id+"/sudo/nope", map[string]any{"action": "approve"}); code != http.StatusNotFound {
		t.Fatalf("unknown request = %d, want 404", code)
	}
}

func TestSudoShellWrapsSudo(t *testing.T) {
	if got := sudoShell("sudo ls", false); !strings.Contains(got, "sudo -n") {
		t.Fatal(got)
	}
	if got := sudoShell("sudo ls", true); !strings.Contains(got, "-S -p ''") {
		t.Fatal(got)
	}
}
