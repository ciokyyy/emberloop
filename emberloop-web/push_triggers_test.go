package web

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

// pushCapture collects encrypted push bodies sent to one subscription.
type pushCapture struct {
	mu     sync.Mutex
	bodies [][]byte
}

func (c *pushCapture) handler(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	c.mu.Lock()
	c.bodies = append(c.bodies, body)
	c.mu.Unlock()
	w.WriteHeader(http.StatusCreated)
}

func (c *pushCapture) all() [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([][]byte(nil), c.bodies...)
}

// capturePush points one Daemon's push store at a test server and returns
// the capture plus the subscription keys for decrypting.
func capturePush(t *testing.T, d *Daemon) (*pushCapture, func([]byte) []byte) {
	t.Helper()
	cap := &pushCapture{}
	srv := httptest.NewTLSServer(http.HandlerFunc(cap.handler))
	t.Cleanup(srv.Close)
	d.push.client = srv.Client()
	sub, ua, auth := newTestSub(t, srv.URL+"/sub")
	d.push.add(sub)
	return cap, func(body []byte) []byte { return decryptPush(t, ua, auth, body) }
}

func waitPush(t *testing.T, cap *pushCapture, n int) [][]byte {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if got := cap.all(); len(got) >= n {
			return got
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d pushes, got %d", n, len(cap.all()))
	return nil
}

func decodePushMessage(t *testing.T, decrypt func([]byte) []byte, body []byte) pushMessage {
	t.Helper()
	var m pushMessage
	if err := json.Unmarshal(decrypt(body), &m); err != nil {
		t.Fatalf("push body is not a pushMessage: %v", err)
	}
	return m
}

func TestPushTurnDoneCueKeyAndPath(t *testing.T) {
	d := NewDaemon(t.TempDir(), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()
	id := createSession(t, srv.URL, "fake", "fake-model", t.TempDir())
	cap, decrypt := capturePush(t, d)

	d.appendEvent(id, "status", "", "working")
	d.appendEvent(id, "status", "", "idle")

	m := decodePushMessage(t, decrypt, waitPush(t, cap, 1)[0])
	if m.Cue != "notification" || m.Key == "" || !strings.HasPrefix(m.Key, "turn:"+id+":") {
		t.Fatalf("turn-done push = %+v, want cue notification with turn key", m)
	}
	if m.Path != "/s/"+id || m.SessionID != id || m.Tag != id {
		t.Fatalf("turn-done push = %+v, want session path", m)
	}
	if m.Title == "" || m.Body != "Turn finished." {
		t.Fatalf("turn-done push = %+v, want titled finished body", m)
	}
}

func TestPushTurnFailedUsesErrorCue(t *testing.T) {
	d := NewDaemon(t.TempDir(), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()
	id := createSession(t, srv.URL, "fake", "fake-model", t.TempDir())
	cap, decrypt := capturePush(t, d)

	d.appendEvent(id, "status", "", "working")
	d.appendEvent(id, "status", "", "failed")

	m := decodePushMessage(t, decrypt, waitPush(t, cap, 1)[0])
	if m.Cue != "error" || m.Body != "Turn failed." {
		t.Fatalf("failed push = %+v, want error cue", m)
	}
}

func TestPushSudoRequestUsesMentionCue(t *testing.T) {
	d := NewDaemon(t.TempDir(), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()
	id := createSession(t, srv.URL, "fake", "fake-model", t.TempDir())
	cap, decrypt := capturePush(t, d)

	body, _ := json.Marshal(map[string]string{"command": "sudo apt-get update"})
	res, err := http.Post(srv.URL+"/api/sessions/"+id+"/sudo", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("sudo create status = %d", res.StatusCode)
	}

	m := decodePushMessage(t, decrypt, waitPush(t, cap, 1)[0])
	if m.Cue != "mention" || !strings.HasPrefix(m.Key, "sudo:") {
		t.Fatalf("sudo push = %+v, want mention cue with sudo key", m)
	}
	if m.Path != "/s/"+id || m.SessionID != id {
		t.Fatalf("sudo push = %+v, want session path", m)
	}
	if !strings.Contains(m.Body, "sudo apt-get update") {
		t.Fatalf("sudo push body = %q, want the command", m.Body)
	}
}

func TestPushRunNeedsYouUsesBlockedCueAndTicketPath(t *testing.T) {
	d := NewDaemon(t.TempDir(), NewFakeBackend())
	cap, decrypt := capturePush(t, d)

	reason := "same refusal 3 times: boom"
	d.pushRunNeedsYou("run-1", "01-first", 1, "First ticket", reason)

	m := decodePushMessage(t, decrypt, waitPush(t, cap, 1)[0])
	if m.Cue != "blocked" || m.Key != needsYouKey("run-1", "01-first", reason) {
		t.Fatalf("needs-you push = %+v, want blocked cue with hashed needs-you key", m)
	}
	if m.Path != "/runs/run-1/01-first" {
		t.Fatalf("needs-you push = %+v, want ticket path", m)
	}
	if m.Title != "Ticket 1 needs you" || !strings.Contains(m.Body, "same refusal 3 times") {
		t.Fatalf("needs-you push = %+v, want ticket title and reason", m)
	}
}

func TestNoteNeedsYouPushDedupsPerReason(t *testing.T) {
	d := NewDaemon(t.TempDir(), NewFakeBackend())
	rs := &runState{ID: "run-1"}

	if !d.noteNeedsYouPushLocked(rs, "t1", "same refusal 3 times") {
		t.Fatal("first reason should push")
	}
	if d.noteNeedsYouPushLocked(rs, "t1", "same refusal 3 times") {
		t.Fatal("same reason should not push twice")
	}
	if !d.noteNeedsYouPushLocked(rs, "t1", "idle 10m with no output") {
		t.Fatal("changed reason should push again")
	}
	if !d.noteNeedsYouPushLocked(rs, "t2", "same refusal 3 times") {
		t.Fatal("another ticket should push")
	}
}

func TestTruncatePushCollapsesWhitespace(t *testing.T) {
	if got := truncatePush("a\n  b\tc", 100); got != "a b c" {
		t.Fatalf("truncate = %q", got)
	}
	if got := truncatePush(strings.Repeat("x", 200), 100); len(got) > 110 || !strings.HasSuffix(got, "…") {
		t.Fatalf("long truncate = %q", got)
	}
}

func TestTruncatePushSlicesRunes(t *testing.T) {
	got := truncatePush(strings.Repeat("é", 200), 100)
	if n := len([]rune(got)); n != 101 {
		t.Fatalf("runes = %d, want 100 kept plus the ellipsis", n)
	}
	if !utf8.ValidString(got) || !strings.HasSuffix(got, "…") {
		t.Fatalf("truncate split a rune: %q", got)
	}
}

func TestNeedsYouKeyIncludesReasonHash(t *testing.T) {
	same := "same refusal 3 times: boom"
	if a, b := needsYouKey("run-1", "t1", same), needsYouKey("run-1", "t1", same); a != b {
		t.Fatalf("same reason gave different keys %q and %q", a, b)
	}
	if a, b := needsYouKey("run-1", "t1", same), needsYouKey("run-1", "t1", "idle 10m with no output"); a == b {
		t.Fatalf("changed reason kept key %q, want a new key so the tab sounds again", a)
	}
	if a, b := needsYouKey("run-1", "t1", same), needsYouKey("run-1", "t2", same); a == b {
		t.Fatalf("another ticket kept key %q, want one key per ticket", a)
	}
	// The empty-reason hash pins the Go/JS contract: sound.js reasonHash('')
	// must give the same suffix, so the Run SSE and the push name one event.
	if got := needsYouKey("run-1", "t1", ""); got != "needsyou:run-1:t1:811c9dc5" {
		t.Fatalf("empty-reason key = %q, want the shared FNV-1a vector", got)
	}
}

// Two identical driver Notifies send one push; a changed reason sends again
// with a new key. This drives the same per-reason gate the driver Notify
// holds before pushRunNeedsYou, through the real encrypted push path.
func TestPushDuplicateNotifySendsOnce(t *testing.T) {
	d := NewDaemon(t.TempDir(), NewFakeBackend())
	cap, decrypt := capturePush(t, d)
	rs := &runState{ID: "run-1", Tickets: []staticTicket{{ID: "01-first", Num: 1, Title: "First"}}}
	notify := func(ticketID, reason string) {
		d.mu.Lock()
		push := d.noteNeedsYouPushLocked(rs, ticketID, reason)
		num, title := ticketNumTitleLocked(rs, ticketID)
		d.mu.Unlock()
		if push {
			d.pushRunNeedsYou(rs.ID, ticketID, num, title, reason)
		}
	}
	firstReason := "same refusal 3 times: boom"
	secondReason := "idle 10m with no output"
	notify("01-first", firstReason)
	notify("01-first", firstReason)
	notify("01-first", secondReason)
	got := waitPush(t, cap, 2)
	keys := map[string]bool{}
	for _, b := range got {
		keys[decodePushMessage(t, decrypt, b).Key] = true
	}
	want := map[string]bool{
		needsYouKey("run-1", "01-first", firstReason):  true,
		needsYouKey("run-1", "01-first", secondReason): true,
	}
	for k := range want {
		if !keys[k] {
			t.Fatalf("push keys = %v, want %v", keys, want)
		}
	}
	if len(keys) != 2 {
		t.Fatalf("push keys = %v, want exactly the two reasons", keys)
	}
	time.Sleep(300 * time.Millisecond)
	if n := len(cap.all()); n != 2 {
		t.Fatalf("pushes = %d, want 2: the duplicate Notify must stay silent", n)
	}
}

// A settling child or scout sends no push of its own: only the parent's
// grouped-result turn notifies. Driven through spawn and real turn settles.
func TestPushChildAndScoutTurnsStaySilent(t *testing.T) {
	d := NewDaemon(t.TempDir(), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()
	parent := createSession(t, srv.URL, "fake", "fake-model", t.TempDir())
	cap, decrypt := capturePush(t, d)

	code, out := postSpawn(t, srv.URL, parent, map[string]any{
		"agents": []map[string]any{{"prompt": "job A"}, {"prompt": "look only", "type": "scout"}},
	})
	if code != http.StatusCreated {
		t.Fatalf("spawn status = %d (%v)", code, out)
	}
	raw, _ := json.Marshal(out["sessions"])
	var kids []SessionMeta
	_ = json.Unmarshal(raw, &kids)
	if len(kids) != 2 {
		t.Fatalf("spawn sessions = %v, want 2", out["sessions"])
	}
	for _, k := range kids {
		waitStatus(t, srv.URL, k.ID, "idle", 10*time.Second)
	}
	// The last child to settle steers the parent, whose turn then notifies.
	waitParentIdle(t, srv.URL, parent)
	m := decodePushMessage(t, decrypt, waitPush(t, cap, 1)[0])
	if m.SessionID != parent || !strings.HasPrefix(m.Key, "turn:"+parent+":") {
		t.Fatalf("push = %+v, want the parent's turn-done only", m)
	}
	time.Sleep(300 * time.Millisecond)
	if n := len(cap.all()); n != 1 {
		t.Fatalf("pushes = %d, want exactly 1: child and scout settles stay silent", n)
	}
}

// Ending a session mid-turn stays silent: the settle lands on an ended
// session, which notifies nothing. Driven through send, DELETE and the real
// turn end.
func TestPushEndedSessionStaysSilent(t *testing.T) {
	fake := NewFakeBackend()
	fake.Delay = time.Second
	d := NewDaemon(t.TempDir(), fake)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()
	id := createSession(t, srv.URL, "fake", "fake-model", t.TempDir())
	cap, _ := capturePush(t, d)

	sendPrompt(t, srv.URL, id, "slow job")
	waitStatus(t, srv.URL, id, "working", 5*time.Second)
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/sessions/"+id, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete status = %d", resp.StatusCode)
	}
	// The turn still settles after the end; its events must send no push.
	_, events := sessionMeta(t, srv.URL, id)
	n := len(events)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_, events = sessionMeta(t, srv.URL, id)
		if len(events) >= n+2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(300 * time.Millisecond)
	if got := cap.all(); len(got) != 0 {
		t.Fatalf("pushes = %d, want 0: an ended session stays silent", len(got))
	}
}

// Gone subscriptions drop: a 404 or 410 answer removes the endpoint, so a
// later notify sends nowhere.
func TestPushNotifyDropsGoneSubscriptions(t *testing.T) {
	var code int
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(code)
	}))
	defer srv.Close()
	p := newPushStore(t.TempDir())
	p.client = srv.Client()
	for _, want := range []int{http.StatusNotFound, http.StatusGone} {
		sub, _, _ := newTestSub(t, srv.URL+"/sub")
		p.add(sub)
		code = want
		p.notify([]byte("ping"))
		if len(p.subs) != 0 {
			t.Fatalf("status %d left %d subscriptions, want none", want, len(p.subs))
		}
	}
}
