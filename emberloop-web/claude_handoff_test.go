package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func claudeHandoffDaemon(t *testing.T, hook func(SessionMeta, handleOp) []string) (*Daemon, *ClaudeBackend, *stubConn, *httptest.Server, string, string) {
	return claudeHandoffDaemonWithSettings(t, hook, `{"compress_model":"test-faux/faux-1"}`)
}

func claudeHandoffDaemonWithSettings(t *testing.T, hook func(SessionMeta, handleOp) []string, settingsJSON string) (*Daemon, *ClaudeBackend, *stubConn, *httptest.Server, string, string) {
	t.Helper()
	settingsPath := useSettings(t, settingsJSON)
	if hook != nil {
		prev := memoryInjection
		memoryInjection = func(meta SessionMeta, op handleOp) injection {
			return injection{texts: hook(meta, op)}
		}
		t.Cleanup(func() { memoryInjection = prev })
	}
	// The voice-reminder hook needs an el binary on PATH; a stub one makes
	// the hook survive the swap the same way product code does.
	binDir := tempDir(t)
	if err := os.WriteFile(filepath.Join(binDir, "el"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	stub := &stubConn{updates: []map[string]any{chunk("ok")}}
	be := newStubClaudeBackend(func() (acpConn, error) { return stub, nil })
	d := NewDaemon(tempDir(t), NewFakeBackend()).WithClaudeBackend(be)
	d.SetSettingsPath(settingsPath)
	d.summarizer = &scriptedSummarizer{replies: []string{goodSummary, goodSummary, goodSummary}}
	srv := httptest.NewServer(authedHandler(t, d))
	t.Cleanup(srv.Close)
	id := createClaudeSession(t, srv.URL, "sonnet", "high", tempDir(t))
	return d, be, stub, srv, srv.URL, id
}

// waitChunks waits until the log holds at least n assistant chunks with this
// text and the last Event is idle: one settled turn per chunk.
func waitChunks(t *testing.T, base, id, text string, n int) {
	t.Helper()
	waitSettled(t, base, id, func(evs []Event) bool {
		c := 0
		for _, e := range evs {
			if e.Kind == "assistant_chunk" && e.Text == text {
				c++
			}
		}
		return c >= n
	})
}

// createClaudeSession creates a claude Session with an effort through the API.
func createClaudeSession(t *testing.T, base, model, effort, workDir string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"harness": "claude", "model": model, "effort": effort, "workDir": workDir})
	resp, err := http.Post(base+"/api/sessions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d", resp.StatusCode)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.ID == "" {
		t.Fatal("empty session id")
	}
	return out.ID
}

// compressView reads the compressible flag of one Session.
func compressView(t *testing.T, base, id string) (bool, string) {
	t.Helper()
	resp, err := http.Get(base + "/api/sessions/" + id)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Compressible bool   `json:"compressible"`
		CompressNote string `json:"compressNote"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.Compressible, out.CompressNote
}

// hookCommand reports whether one session/new meta carries the per-turn
// voice-reminder hook.
func hookCommand(meta map[string]any) string {
	cc, _ := meta["claudeCode"].(map[string]any)
	opts, _ := cc["options"].(map[string]any)
	if opts == nil {
		return ""
	}
	raw, _ := json.Marshal(opts["settings"])
	return string(raw)
}

// A claude Session reports compressible, then compresses by Handoff restart:
// history stays, the token rotates, the new ACP session was ready with the
// same model, effort, voice hook and role tools before the Handoff ran, and a
// Daemon restart resumes the new session.
func TestClaudeHandoffSwapCompressesAndResumes(t *testing.T) {
	d, _, stub, _, base, id := claudeHandoffDaemon(t, nil)

	if ok, note := compressView(t, base, id); !ok {
		t.Fatalf("compressible = false, note %q, want true", note)
	}
	sendPrompt(t, base, id, "build the thing")
	waitChunks(t, base, id, "ok", 1)

	d.mu.Lock()
	metaBefore := d.sessions[id].meta
	d.mu.Unlock()

	if code, body := compressReqIdle(t, base, id, ""); code != 202 {
		t.Fatalf("compress = %d %v", code, body)
	}
	evs := waitIdleAfter(t, base, id, 1)

	// Prior Events stay; the Handoff prompt follows them as a system Event.
	hand := eventsOfKind(evs, "prompt")
	if len(hand) != 2 || hand[0].Text != "build the thing" {
		t.Fatalf("history lost: %+v", hand)
	}
	last := hand[len(hand)-1]
	if !last.Handoff || last.Sender != "system" || last.User != "" || last.Mode != "" {
		t.Fatalf("handoff prompt flags = %+v", last)
	}
	if !strings.Contains(last.Text, "## Next steps\nrun the tests") ||
		!strings.HasSuffix(last.Text, "\n## Last user prompt (verbatim)\nbuild the thing\n") {
		t.Fatalf("handoff text = %q", last.Text)
	}

	// compressed Event: number, model, fallback step.
	c := eventsOfKind(evs, "compressed")[0]
	if !strings.Contains(c.Text, `"number":1`) || !strings.Contains(c.Text, `"model":"test-faux/faux-1"`) ||
		!strings.Contains(c.Text, `"fallback":"summary"`) {
		t.Fatalf("compressed text = %s", c.Text)
	}

	// Token rotated; Session ID and Owner unchanged.
	d.mu.Lock()
	metaAfter := d.sessions[id].meta
	d.mu.Unlock()
	if metaBefore.Owner == "" || metaAfter.Owner != metaBefore.Owner {
		t.Fatalf("owner across swap: %q -> %q", metaBefore.Owner, metaAfter.Owner)
	}
	if metaAfter.ResumeToken == metaBefore.ResumeToken || metaAfter.ResumeToken == "" {
		t.Fatalf("token %q -> %q, want rotated", metaBefore.ResumeToken, metaAfter.ResumeToken)
	}
	if metaAfter.ResumeToken != "acp-session-2" {
		t.Fatalf("token after swap = %q, want acp-session-2", metaAfter.ResumeToken)
	}
	// The settled turn releases the process, so the persisted meta token is
	// the rotation proof; the restart test below proves it resumes.

	// The new ACP session started beside the old one: two session/new calls,
	// the second with the same model, effort, voice hook and role tools, and
	// only then did the Handoff turn run against it.
	stub.mu.Lock()
	count := stub.newCount
	metas := append([]map[string]any{}, stub.newMetas...)
	mcps := append([][]acpMCPServer{}, stub.newMCPs...)
	prompts := append([]string{}, stub.prompts...)
	stub.mu.Unlock()
	if count != 2 {
		t.Fatalf("adapter session/new calls = %d, want 2 (create plus Handoff)", count)
	}
	mustStripped(t, metas[1], "sonnet")
	opts := metas[1]["claudeCode"].(map[string]any)["options"].(map[string]any)
	if opts["effort"] != "high" {
		t.Fatalf("handoff effort = %v, want high", opts["effort"])
	}
	if !strings.Contains(hookCommand(metas[0]), "voice-reminder hook") || !strings.Contains(hookCommand(metas[1]), "voice-reminder hook") {
		t.Fatalf("voice hook missing across the swap")
	}
	if len(mcps[1]) != 1 || len(mcps[1][0].Args) != 2 || mcps[1][0].Args[1] != "web" {
		t.Fatalf("handoff mcp servers = %+v, want one el server under --role web", mcps[1])
	}
	if len(prompts) != 2 || prompts[0] != "build the thing" || !strings.HasPrefix(prompts[1], "# Handoff") {
		t.Fatalf("adapter prompts = %q, want the user turn then the Handoff", prompts)
	}

	// Copy stored in the Session directory.
	raw, err := os.ReadFile(filepath.Join(d.sessionDir(id), "handoff-1.md"))
	if err != nil || string(raw) != last.Text {
		t.Fatalf("handoff copy err=%v match=%v", err, string(raw) == last.Text)
	}

}

// A Daemon restart after a claude Handoff resumes the new ACP session.
func TestClaudeHandoffTokenSurvivesRestart(t *testing.T) {
	d, _, stub, srv, base, id := claudeHandoffDaemon(t, nil)
	sendPrompt(t, base, id, "build the thing")
	waitChunks(t, base, id, "ok", 1)
	if code, body := compressReqIdle(t, base, id, ""); code != 202 {
		t.Fatalf("compress = %d %v", code, body)
	}
	waitIdleAfter(t, base, id, 1)

	view := sessionView(t, base, id)
	token := view.Session.ResumeToken
	if token != "acp-session-2" {
		t.Fatalf("token after swap = %q, want acp-session-2", token)
	}

	// Restart the Daemon on the same state dir with a fresh backend around
	// the same adapter double, as the lifecycle test does.
	var dir string
	d.mu.Lock()
	dir = d.stateDir
	d.mu.Unlock()
	srv.Close()
	be2 := newStubClaudeBackend(func() (acpConn, error) { return stub, nil })
	d2 := NewDaemon(dir, NewFakeBackend()).WithClaudeBackend(be2)
	srv2 := httptest.NewServer(authedHandler(t, d2))
	defer srv2.Close()
	if view := sessionView(t, srv2.URL, id); !view.Resumable {
		t.Fatalf("resumable = false after restart, note %q", view.ResumeNote)
	}
	sendPrompt(t, srv2.URL, id, "after restart")
	waitChunks(t, srv2.URL, id, "ok", 3)
	stub.mu.Lock()
	loadID := stub.loadID
	stub.mu.Unlock()
	if loadID != "acp-session-2" {
		t.Fatalf("restart loaded %q, want the Handoff session acp-session-2", loadID)
	}
	waitForLive(t, srv2.URL, id, false, 5*time.Second)
}

// The Handoff turn on the new claude session carries the startHandle
// injections for a compress: Profile, recall, then the Handoff. The Event log
// keeps the Handoff alone.
func TestClaudeHandoffInjectsProfileRecallThenHandoff(t *testing.T) {
	hook := func(_ SessionMeta, op handleOp) []string {
		if op == handleCompress {
			return []string{"PROFILE", "RECALL"}
		}
		return nil
	}
	_, _, stub, _, base, id := claudeHandoffDaemon(t, hook)
	sendPrompt(t, base, id, "build the thing")
	waitChunks(t, base, id, "ok", 1)
	if code, body := compressReqIdle(t, base, id, ""); code != 202 {
		t.Fatalf("compress = %d %v", code, body)
	}
	evs := waitIdleAfter(t, base, id, 1)

	handoff := handoffText(evs)
	if !strings.HasPrefix(handoff, handoffHeader) {
		t.Fatalf("Handoff prompt Event = %q, want the Handoff alone", handoff)
	}
	stub.mu.Lock()
	prompts := append([]string{}, stub.prompts...)
	stub.mu.Unlock()
	if want := "PROFILE\n\nRECALL\n\n" + handoff; prompts[len(prompts)-1] != want {
		t.Fatalf("compress turn = %q, want %q", prompts[len(prompts)-1], want)
	}
}

// A failed Handoff start keeps the old claude handle: an error Event, no
// compressed Event, the token unchanged, and the Session still answers.
func TestClaudeHandoffFailureKeepsOldHandle(t *testing.T) {
	d, _, stub, _, base, id := claudeHandoffDaemon(t, nil)
	sendPrompt(t, base, id, "hello")
	waitChunks(t, base, id, "ok", 1)

	d.mu.Lock()
	tokenBefore := d.sessions[id].meta.ResumeToken
	d.mu.Unlock()
	if tokenBefore != "acp-session-1" {
		t.Fatalf("token before = %q, want acp-session-1", tokenBefore)
	}

	stub.mu.Lock()
	stub.newErr = errNoAdapter
	stub.mu.Unlock()
	if code, body := compressReqIdle(t, base, id, ""); code != 202 {
		t.Fatalf("compress = %d %v", code, body)
	}
	waitEventKind(t, base, id, "error")
	evs := transcriptOf(t, base, id)
	if len(eventsOfKind(evs, "compressed")) != 0 {
		t.Fatalf("compressed without a new handle: %+v", evs)
	}
	d.mu.Lock()
	tokenAfter := d.sessions[id].meta.ResumeToken
	d.mu.Unlock()
	if tokenAfter != tokenBefore {
		t.Fatalf("token %q -> %q across a failed swap", tokenBefore, tokenAfter)
	}
	// No new ACP session started: the failed session/new left no handle.
	stub.mu.Lock()
	count := stub.newCount
	stub.newErr = nil
	stub.mu.Unlock()
	if count != 1 {
		t.Fatalf("adapter session/new calls = %d, want 1 (the failed swap started none)", count)
	}

	// The Session still answers by resuming the old handle, not a new one.
	sendPrompt(t, base, id, "still here")
	waitChunks(t, base, id, "ok", 2)
	stub.mu.Lock()
	loadID := stub.loadID
	stub.mu.Unlock()
	if loadID != "acp-session-1" {
		t.Fatalf("resume loaded %q, want the old handle acp-session-1", loadID)
	}
}

// StartHandle derives the el role from the record, so the new ACP session
// gets the same tool set on every role.
func TestClaudeStartHandleDerivesRoleFromRecord(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		meta SessionMeta
		role string
	}{
		{"top-level", SessionMeta{ID: "c-top", Harness: "claude", Model: "sonnet", WorkDir: tempDir(t)}, "web"},
		{"subagent", SessionMeta{ID: "c-kid", Harness: "claude", Model: "sonnet", WorkDir: tempDir(t), GroupID: "g"}, "web-worker"},
		{"run ticket", SessionMeta{ID: "c-run", Harness: "claude", Model: "sonnet", WorkDir: tempDir(t), GroupID: "g", RunWorker: true}, "run-worker"},
		{"scout", SessionMeta{ID: "c-look", Harness: "claude", Model: "sonnet", WorkDir: tempDir(t), GroupID: "g", Scout: true}, "scout"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := &stubConn{}
			be := newStubClaudeBackend(func() (acpConn, error) { return stub, nil })
			tok, release, err := be.StartHandle(tc.meta, func(kind, text, status string) {})
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			if tok == "" || be.ResumeToken(tc.meta.ID) != tok {
				t.Fatalf("token %q not live in the backend", tok)
			}
			stub.mu.Lock()
			defer stub.mu.Unlock()
			if len(stub.newMCPs) != 1 || len(stub.newMCPs[0]) != 1 || len(stub.newMCPs[0][0].Args) != 2 ||
				stub.newMCPs[0][0].Args[1] != tc.role {
				t.Fatalf("mcp servers = %+v, want one el server under --role %s", stub.newMCPs, tc.role)
			}
			mustStripped(t, stub.newMetas[0], "sonnet")
		})
	}
}

// A claude usage_update at 85% of the 1M window auto-compresses by Handoff:
// the percent gate finally has a reading to work with.
func TestClaudeAutoCompressFiresOnUsageUpdate(t *testing.T) {
	// Cooldown 5: the first compress is never cooled, but the Handoff turn's
	// own replayed usage_update must not start a second one.
	_, _, stub, _, base, id := claudeHandoffDaemonWithSettings(t, nil, `{"compress_model":"test-faux/faux-1","auto_compress":true,"compress_threshold_percent":80,"compress_cooldown":5}`)
	stub.mu.Lock()
	stub.updates = []map[string]any{chunk("ok"), usageUpdate(850000, 1000000)}
	stub.mu.Unlock()
	sendPrompt(t, base, id, "build the thing")
	evs := waitIdleAfter(t, base, id, 1)
	if n := len(eventsOfKind(evs, "compressed")); n != 1 {
		t.Fatalf("compressed = %d, want 1 after the 850k/1M usage_update", n)
	}
	if used, size, ok := usageOf(evs); !ok || size != 1000000 || used < 850000 {
		t.Fatalf("usage = %d/%d ok=%v, want the 850k/1M reading", used, size, ok)
	}
}

// Below the percent gate, the absolute token gate still auto-compresses a
// claude Session: 150k of 1M is 15%, past a 100k token cap.
func TestClaudeAutoCompressFiresOnTokenThreshold(t *testing.T) {
	_, _, stub, _, base, id := claudeHandoffDaemonWithSettings(t, nil, `{"compress_model":"test-faux/faux-1","auto_compress":true,"compress_threshold_percent":80,"compress_cooldown":5,"compress_threshold_tokens":100000}`)
	stub.mu.Lock()
	stub.updates = []map[string]any{chunk("ok"), usageUpdate(150000, 1000000)}
	stub.mu.Unlock()
	sendPrompt(t, base, id, "build the thing")
	evs := waitIdleAfter(t, base, id, 1)
	if n := len(eventsOfKind(evs, "compressed")); n != 1 {
		t.Fatalf("compressed = %d, want 1 past the 100k token cap", n)
	}
}

// Without any adapter usage_update, the ACP fallback still seeds a sized
// reading from the known window, so the meter never reads empty.
func TestClaudeFallbackUsageWithoutAdapterUpdate(t *testing.T) {
	_, _, stub, _, base, id := claudeHandoffDaemonWithSettings(t, nil, `{"compress_model":"test-faux/faux-1","auto_compress":false}`)
	stub.mu.Lock()
	stub.updates = []map[string]any{chunk("ok")}
	stub.mu.Unlock()
	sendPrompt(t, base, id, "build the thing")
	waitChunks(t, base, id, "ok", 1)
	evs := transcriptOf(t, base, id)
	used, size, ok := usageOf(evs)
	if !ok || size != claudeWindow {
		t.Fatalf("usage = %d/%d ok=%v, want a fallback reading sized %d", used, size, ok, claudeWindow)
	}
}
