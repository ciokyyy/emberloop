package web

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"example.com/emberloop-web/memory"
)

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
	f, err := os.OpenFile(filepath.Join(d.sessionDir(id), "events.jsonl"), os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err = f.Write(append(b, '\n')); err != nil {
		t.Fatal(err)
	}
}
func liveEpisodes(t *testing.T, d *Daemon, user string) []memory.Episode {
	t.Helper()
	b, err := d.profileBankFor(user)
	if err != nil {
		t.Fatal(err)
	}
	es, err := b.ListEpisodes(context.Background(), 20)
	if err != nil {
		t.Fatal(err)
	}
	return es
}

func TestLiveMiningOffByDefault(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	activeSession(t, d, "s1", "alice", []Event{userPrompt(1, "alice", "No, use tabs here")})
	d.maybeMineLive("s1")
	d.waitMining()
	if got := liveEpisodes(t, d, "alice"); len(got) != 0 {
		t.Fatalf("episodes=%+v", got)
	}
}
func TestLiveMiningMinesUnsettledSessionAtTurnEnd(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.EnableLiveMining()
	activeSession(t, d, "s1", "alice", []Event{userPrompt(1, "alice", "No, use tabs here")})
	if res, err := d.MineSession("s1"); err != nil || res.Scanned != 0 {
		t.Fatalf("settle gate: %+v %v", res, err)
	}
	d.maybeMineLive("s1")
	d.waitMining()
	got := liveEpisodes(t, d, "alice")
	if len(got) != 1 || got[0].Text != "No, use tabs here" {
		t.Fatalf("episodes=%+v", got)
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
	got := liveEpisodes(t, d, "alice")
	if len(got) != 2 {
		t.Fatalf("episodes=%+v", got)
	}
	d.maybeMineLive("s1")
	d.waitMining()
	if after := liveEpisodes(t, d, "alice"); len(after) != 2 {
		t.Fatalf("duplicate scan episodes=%+v", after)
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
	d.mu.Lock()
	d.sessions["s1"].meta.Status = "ended"
	d.mu.Unlock()
	res, err := d.MineSession("s1")
	if err != nil || res.Scanned != 0 {
		t.Fatalf("nightly replay: %+v %v", res, err)
	}
	if got := liveEpisodes(t, d, "alice"); len(got) != 1 {
		t.Fatalf("episodes=%+v", got)
	}
}
func TestLiveMiningFiresAtRealTurnEnd(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.EnableLiveMining()
	srv := httptest.NewServer(authedHandler(t, d))
	t.Cleanup(srv.Close)
	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	sendPrompt(t, srv.URL, id, "No, use tabs here")
	user := userIDOf(t, srv.URL)
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		d.waitMining()
		if got := liveEpisodes(t, d, user); len(got) > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("turn-end episode not stored")
}
