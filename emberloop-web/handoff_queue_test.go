package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// promptRecord keeps the prompt texts a fake Backend received, in order.
type promptRecord struct {
	mu   sync.Mutex
	seen []string
}

// reply records one prompt and keeps the fake's default reply.
func (p *promptRecord) reply(_, text string) (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.seen = append(p.seen, text)
	return "", false
}

func (p *promptRecord) texts() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.seen...)
}

// queueDaemon serves one fresh fake Session whose turns last delay. It
// returns the Daemon, the prompts its Backend received, the server URL and
// the Session ID.
func queueDaemon(t *testing.T, delay time.Duration) (*Daemon, *promptRecord, string, string) {
	t.Helper()
	settingsPath := useSettings(t, `{"compress_model":"test-faux/faux-1"}`)
	be := NewFakeBackend()
	be.Delay = delay
	rec := &promptRecord{}
	be.ReplyFunc = rec.reply
	d := NewDaemon(tempDir(t), be)
	d.SetSettingsPath(settingsPath)
	d.summarizer = &scriptedSummarizer{replies: []string{goodSummary}}
	srv := httptest.NewServer(authedHandler(t, d))
	t.Cleanup(srv.Close)
	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	return d, rec, srv.URL, id
}

// promptWith returns the first prompt Event whose text holds substr. An empty
// mode matches any prompt; otherwise the Event mode must equal it.
func promptWith(evs []Event, substr, mode string) (Event, bool) {
	for _, e := range evs {
		if e.Kind == "prompt" && strings.Contains(e.Text, substr) && (mode == "" || e.Mode == mode) {
			return e, true
		}
	}
	return Event{}, false
}

// waitSettled polls until the transcript holds ok and its last Event is an
// idle status, then returns the transcript.
func waitSettled(t *testing.T, base, id string, ok func([]Event) bool) []Event {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		evs := transcriptOf(t, base, id)
		if n := len(evs); n > 0 && evs[n-1].Kind == "status" && evs[n-1].Status == "idle" && ok(evs) {
			return evs
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("session %s never settled with the wanted Events: %+v", id, transcriptOf(t, base, id))
	return nil
}

// Follow-ups queued during a Handoff wait for it. They reach the new handle
// after the Handoff prompt, in the order they were sent.
func TestHandoffQueuedFollowupsRunAfterHandoffInOrder(t *testing.T) {
	t.Parallel()
	_, rec, base, id := queueDaemon(t, time.Second)
	sendPrompt(t, base, id, "build the thing")
	waitStatus(t, base, id, "idle", 5*time.Second)
	if code, body := compressReq(t, base, id, ""); code != http.StatusAccepted {
		t.Fatalf("compress = %d %v", code, body)
	}
	waitPrompt(t, base, id, "# Handoff", 5*time.Second)
	for _, text := range []string{"first", "second"} {
		if code, out := postMode(t, base, id, "followup", text); code != http.StatusAccepted {
			t.Fatalf("followup %q = %d %v", text, code, out)
		}
	}
	evs := waitSettled(t, base, id, func(evs []Event) bool {
		_, ok := promptWith(evs, "first", "followup")
		return ok
	})
	compressed := eventsOfKind(evs, "compressed")
	handoff, hok := promptWith(evs, "# Handoff", "")
	queued, qok := promptWith(evs, "first", "followup")
	if len(compressed) != 1 || !hok || !qok || compressed[0].Seq > handoff.Seq || handoff.Seq > queued.Seq {
		t.Fatalf("order wrong: compressed %+v, handoff seq %d, queued seq %d", compressed, handoff.Seq, queued.Seq)
	}
	if queued.Text != "first\n\nsecond" {
		t.Fatalf("queued prompt = %q, want both follow-ups in order", queued.Text)
	}
	if got := rec.texts(); len(got) != 3 || got[0] != "build the thing" || !strings.HasPrefix(got[1], "# Handoff") || got[2] != "first\n\nsecond" {
		t.Fatalf("prompts received = %q", got)
	}
}

// A held Steer waits for a tool call on claude only, so the fake seam seeds
// one into pending the way the claude path leaves it. Across a Handoff it
// degrades to a Follow-up, lands after the Handoff and is not lost.
func TestHandoffHeldSteerDegradesToFollowupAndIsNotLost(t *testing.T) {
	t.Parallel()
	d, rec, base, id := queueDaemon(t, time.Second)
	sendPrompt(t, base, id, "build the thing")
	waitStatus(t, base, id, "idle", 5*time.Second)
	if code, body := compressReq(t, base, id, ""); code != http.StatusAccepted {
		t.Fatalf("compress = %d %v", code, body)
	}
	waitPrompt(t, base, id, "# Handoff", 5*time.Second)
	d.mu.Lock()
	s := d.sessions[id]
	s.pending = append(s.pending, queuedMsg{mode: "steer", text: "go left", held: true})
	d.mu.Unlock()
	evs := waitSettled(t, base, id, func(evs []Event) bool {
		_, ok := promptWith(evs, "go left", "followup")
		return ok
	})
	handoff, hok := promptWith(evs, "# Handoff", "")
	held, lok := promptWith(evs, "go left", "followup")
	if !hok || !lok || handoff.Seq > held.Seq || held.Text != "go left" {
		t.Fatalf("held steer = %+v after handoff seq %d, want unlabeled followup after it", held, handoff.Seq)
	}
	if _, stale := promptWith(evs, "go left", "steer"); stale {
		t.Fatal("held steer kept its steer label across the swap")
	}
	if got := rec.texts(); len(got) != 3 || got[2] != "go left" {
		t.Fatalf("prompts received = %q", got)
	}
}

// A claude Steer that arrives while a Handoff runs must not wait as a held
// Steer for a tool call: the Handoff leads, so the Steer queues as a Follow-up.
func TestClaudeSteerDuringHandoffQueuesFollowup(t *testing.T) {
	t.Parallel()
	stub := &stubConn{blockPrompt: make(chan struct{})}
	be := newStubClaudeBackend(func() (acpConn, error) { return stub, nil })
	d := NewDaemon(tempDir(t), NewFakeBackend()).WithClaudeBackend(be)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	id := createSession(t, srv.URL, "claude", "sonnet", tempDir(t))
	sendPrompt(t, srv.URL, id, "slow job")
	waitStatus(t, srv.URL, id, "working", 5*time.Second)
	// A Handoff claim sets swapping; the blocked stub turn stands in for it.
	d.mu.Lock()
	d.sessions[id].swapping = true
	d.mu.Unlock()
	if code, out := postMode(t, srv.URL, id, "steer", "go left"); code != http.StatusAccepted {
		t.Fatalf("steer = %d %v", code, out)
	}
	q := getQueue(t, srv.URL, id)
	if len(q) != 1 || q[0].Mode != "followup" || !q[0].Degraded {
		t.Fatalf("queue = %+v, want one degraded followup and no held steer", q)
	}
	close(stub.blockPrompt)
	waitSettled(t, srv.URL, id, func(evs []Event) bool {
		_, ok := promptWith(evs, "go left", "followup")
		return ok
	})
}

// Compress waits while a Group of the Session is outstanding. The Group's
// result reaches the parent first; the compress runs after that turn.
func TestCompressWaitsWhileGroupOutstanding(t *testing.T) {
	t.Parallel()
	for _, deliver := range []string{"group", "each"} {
		deliver := deliver
		t.Run(deliver, func(t *testing.T) {
			t.Parallel()
			_, _, base, parent := queueDaemon(t, time.Second)
			sendPrompt(t, base, parent, "plan the work")
			waitStatus(t, base, parent, "idle", 5*time.Second)
			if code, out := postSpawn(t, base, parent, map[string]any{
				"agents":  []map[string]any{{"prompt": "job A"}},
				"deliver": deliver,
			}); code != http.StatusCreated {
				t.Fatalf("spawn = %d %v", code, out)
			}
			if code, body := compressReq(t, base, parent, ""); code != http.StatusAccepted || body["waiting"] != "group" {
				t.Fatalf("compress while the Group runs = %d %v, want 202 waiting", code, body)
			}
			if n := len(eventsOfKind(transcriptOf(t, base, parent), "compressed")); n != 0 {
				t.Fatalf("compressed %d times while the Group runs", n)
			}
			evs := waitSettled(t, base, parent, func(evs []Event) bool {
				return len(eventsOfKind(evs, "compressed")) == 1
			})
			compressed := eventsOfKind(evs, "compressed")[0]
			result, ok := promptWith(evs, "fake reply to: job A", "steer")
			if !ok || result.Seq > compressed.Seq {
				t.Fatalf("group result %+v must reach the parent before compressed seq %d", result, compressed.Seq)
			}
		})
	}
}

// A Group spawned after a compress reports to the parent on the new handle.
func TestGroupResultReachesParentAfterCompress(t *testing.T) {
	t.Parallel()
	_, rec, base, parent := queueDaemon(t, 20*time.Millisecond)
	sendPrompt(t, base, parent, "plan the work")
	waitStatus(t, base, parent, "idle", 5*time.Second)
	if code, body := compressReq(t, base, parent, ""); code != http.StatusAccepted {
		t.Fatalf("compress = %d %v", code, body)
	}
	waitSettled(t, base, parent, func(evs []Event) bool {
		return len(eventsOfKind(evs, "compressed")) == 1
	})
	if code, out := postSpawn(t, base, parent, map[string]any{"agents": []map[string]any{{"prompt": "job B"}}}); code != http.StatusCreated {
		t.Fatalf("spawn = %d %v", code, out)
	}
	evs := waitSettled(t, base, parent, func(evs []Event) bool {
		_, ok := promptWith(evs, "fake reply to: job B", "steer")
		return ok
	})
	result, _ := promptWith(evs, "fake reply to: job B", "steer")
	if compressed := eventsOfKind(evs, "compressed"); result.Seq < compressed[0].Seq {
		t.Fatalf("grouped result seq %d before the compress", result.Seq)
	}
	n := 0
	for _, text := range rec.texts() {
		if strings.Contains(text, "fake reply to: job B") {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("grouped result reached the Backend %d times, want 1", n)
	}
}

// A Group stranded by a restart never delivers. Its persisted member must
// not hold a compress of the parent.
func TestStrandedGroupAfterRestartDoesNotBlockCompress(t *testing.T) {
	t.Parallel()
	stateDir := tempDir(t)
	settingsPath := useSettings(t, `{"compress_model":"test-faux/faux-1"}`)
	stuck := make(chan struct{}) // never closed: the old Daemon's child turn stays open
	old := NewFakeBackend()
	old.ReplyFunc = func(_, text string) (string, bool) {
		if strings.HasPrefix(text, "job") {
			<-stuck
		}
		return "", false
	}
	d1 := NewDaemon(stateDir, old)
	d1.SetSettingsPath(settingsPath)
	srv1 := httptest.NewServer(authedHandler(t, d1))
	defer srv1.Close()
	parent := createSession(t, srv1.URL, "fake", "fake-model", tempDir(t))
	sendPrompt(t, srv1.URL, parent, "plan the work")
	waitStatus(t, srv1.URL, parent, "idle", 5*time.Second)
	code, out := postSpawn(t, srv1.URL, parent, map[string]any{"agents": []map[string]any{{"prompt": "job A"}}})
	if code != http.StatusCreated {
		t.Fatalf("spawn = %d %v", code, out)
	}
	var kids []SessionMeta
	raw, _ := json.Marshal(out["sessions"])
	if err := json.Unmarshal(raw, &kids); err != nil || len(kids) != 1 {
		t.Fatalf("spawn sessions = %v", out["sessions"])
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if m, _ := sessionMeta(t, srv1.URL, kids[0].ID); m.Status == "working" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("child never started its turn")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Restart: the reloaded Daemon sees the Group and its member mid-turn on disk.
	d2 := NewDaemon(stateDir, NewFakeBackend())
	d2.SetSettingsPath(settingsPath)
	d2.summarizer = &scriptedSummarizer{replies: []string{goodSummary}}
	srv2 := httptest.NewServer(authedHandler(t, d2))
	defer srv2.Close()
	if code, body := compressReq(t, srv2.URL, parent, ""); code != http.StatusAccepted || body["waiting"] != nil {
		t.Fatalf("compress after restart = %d %v, want 202 with no wait", code, body)
	}
	waitSettled(t, srv2.URL, parent, func(evs []Event) bool {
		return len(eventsOfKind(evs, "compressed")) == 1
	})
}

// A compress that runs while another compress still waits on a Group settles
// that wait. A held compress must not run a second Handoff after it.
func TestCompressNowSettlesHeldCompress(t *testing.T) {
	t.Parallel()
	d, _, base, id := queueDaemon(t, 20*time.Millisecond)
	sendPrompt(t, base, id, "build the thing")
	waitStatus(t, base, id, "idle", 5*time.Second)
	// The window after a Group settles and before its trigger runs: the held
	// compress is still flagged when a compress is asked for now.
	d.mu.Lock()
	d.sessions[id].compressWait = true
	d.sessions[id].compressInstr = "keep the plan"
	d.mu.Unlock()
	if code, body := compressReq(t, base, id, ""); code != http.StatusAccepted {
		t.Fatalf("compress = %d %v", code, body)
	}
	waitSettled(t, base, id, func(evs []Event) bool {
		return len(eventsOfKind(evs, "compressed")) >= 1
	})
	time.Sleep(100 * time.Millisecond)
	if n := len(eventsOfKind(transcriptOf(t, base, id), "compressed")); n != 1 {
		t.Fatalf("compressed %d times, want 1: the held compress ran again", n)
	}
}
