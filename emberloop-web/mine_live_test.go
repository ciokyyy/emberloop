package web

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"example.com/emberloop-web/memory"
)

// activeSession seeds an unsettled Session: mid-run, last Event just now, so
// the 10-minute settle window has not passed.
func activeSession(t *testing.T, d *Daemon, id, owner string, evs []Event) {
	t.Helper()
	seedEndedSession(t, d, id, owner, evs)
	d.mu.Lock()
	d.sessions[id].meta.Status = "active"
	d.mu.Unlock()
}

func appendEventLine(t *testing.T, d *Daemon, id string, e Event) {
	t.Helper()
	e.SessionID = id
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(d.sessionDir(id), "events.jsonl"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write(append(b, '\n')); err != nil {
		t.Fatal(err)
	}
}

func TestLiveMiningOffByDefault(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	activeSession(t, d, "s1", "alice", []Event{userPrompt(1, "alice", "No, use tabs here")})
	d.maybeMineLive("s1")
	d.waitMining()
	if obs := bankQuotes(t, d, "alice", "tabs"); len(obs) != 0 {
		t.Fatalf("live mining ran while disabled: %+v", obs)
	}
}

func TestLiveMiningMinesUnsettledSessionAtTurnEnd(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.EnableLiveMining()
	activeSession(t, d, "s1", "alice", []Event{userPrompt(1, "alice", "No, use tabs here")})
	// The nightly path still honors the settle window.
	if res, err := d.MineSession("s1"); err != nil || res.Scanned != 0 {
		t.Fatalf("settle gate: res=%+v err=%v, want nothing scanned", res, err)
	}
	d.maybeMineLive("s1")
	d.waitMining()
	obs := bankQuotes(t, d, "alice", "tabs")
	if len(obs) != 1 || obs[0].Quote != "No, use tabs here" {
		t.Fatalf("observations = %+v, want the turn-end correction", obs)
	}
}

func TestLiveMiningReadsOnlyTheDelta(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.EnableLiveMining()
	activeSession(t, d, "s1", "alice", []Event{userPrompt(1, "alice", "No, use tabs here")})
	d.maybeMineLive("s1")
	d.waitMining()
	appendEventLine(t, d, "s1", userPrompt(2, "alice", "Never add comments unless asked"))
	d.maybeMineLive("s1")
	d.waitMining()
	if obs := bankQuotes(t, d, "alice", "tabs"); len(obs) != 1 || obs[0].EvidenceCount != 1 {
		t.Fatalf("first correction rescanned: %+v", obs)
	}
	if obs := bankQuotes(t, d, "alice", "comments"); len(obs) != 1 {
		t.Fatalf("second correction missing: %+v", obs)
	}
	// A turn end with no new bytes scans nothing and stores nothing.
	d.maybeMineLive("s1")
	d.waitMining()
	if obs := bankQuotes(t, d, "alice", "tabs"); len(obs) != 1 || obs[0].EvidenceCount != 1 {
		t.Fatalf("idle trigger changed evidence: %+v", obs)
	}
}

func TestLiveMiningCoalescesTriggersAndNightlyShareCursor(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.EnableLiveMining()
	activeSession(t, d, "s1", "alice", []Event{userPrompt(1, "alice", "No, use tabs here")})
	for i := 0; i < 20; i++ {
		d.maybeMineLive("s1")
	}
	d.waitMining()
	// The session ends; the nightly catch-up reads the same cursor, so the
	// correction is not counted twice.
	d.mu.Lock()
	d.sessions["s1"].meta.Status = "ended"
	d.mu.Unlock()
	res, err := d.MineSession("s1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Scanned != 0 {
		t.Fatalf("nightly rescanned %d events the live scan already read", res.Scanned)
	}
	if obs := bankQuotes(t, d, "alice", "tabs"); len(obs) != 1 || obs[0].EvidenceCount != 1 {
		t.Fatalf("observations = %+v, want one with evidence 1", obs)
	}
}

// A real turn through the API mines its own correction at turn end, with no
// settle wait and no nightly run.
func TestLiveMiningFiresAtRealTurnEnd(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.EnableLiveMining()
	srv := httptest.NewServer(authedHandler(t, d))
	t.Cleanup(srv.Close)
	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	sendPrompt(t, srv.URL, id, "No, use tabs here")
	user := userIDOf(t, srv.URL)
	var obs []memory.Observation
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		d.waitMining()
		if obs = bankQuotes(t, d, user, "tabs"); len(obs) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(obs) != 1 {
		t.Fatalf("observations = %+v, want the turn-end correction", obs)
	}
}
