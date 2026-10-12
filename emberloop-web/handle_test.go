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

// promptsSeen records the prompt texts the fake Backend received, in order.
// An injection block shows up in front of the prompt the harness gets, while
// the Event log keeps the prompt alone.
type promptsSeen struct {
	mu    sync.Mutex
	texts []string
}

func (p *promptsSeen) reply(_, text string) (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.texts = append(p.texts, text)
	return "", false
}

func (p *promptsSeen) all() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.texts...)
}

// handleDaemon starts a Daemon on the fake Backend with one Session created by
// the admin User. A non-nil hook replaces the injection hook until the test ends.
func handleDaemon(t *testing.T, hook func(SessionMeta, handleOp) injection) (*Daemon, *FakeBackend, *promptsSeen, string, string) {
	t.Helper()
	useSettings(t, `{"compress_model":"test-faux/faux-1"}`)
	if hook != nil {
		prev := memoryInjection
		memoryInjection = hook
		t.Cleanup(func() { memoryInjection = prev })
	}
	seen := &promptsSeen{}
	be := NewFakeBackend()
	be.ReplyFunc = seen.reply
	d := NewDaemon(tempDir(t), be)
	d.summarizer = &scriptedSummarizer{replies: []string{goodSummary, goodSummary, goodSummary, goodSummary}}
	srv := httptest.NewServer(authedHandler(t, d))
	t.Cleanup(srv.Close)
	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	return d, be, seen, srv.URL, id
}

// runPrompt sends one prompt and waits until the turn it starts settles: the
// log holds the prompt, and the last Event is the closing idle status.
func runPrompt(t *testing.T, base, id, text string) {
	t.Helper()
	sendPrompt(t, base, id, text)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		evs := transcriptOf(t, base, id)
		n := len(evs)
		if n > 0 && evs[n-1].Kind == "status" && evs[n-1].Status == "idle" && promptIndex(evs, text) >= 0 && promptIndex(evs, text) < n-1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("prompt %q did not settle", text)
}

// promptIndex is the index of the last prompt Event with this text, or -1.
func promptIndex(evs []Event, text string) int {
	for i := len(evs) - 1; i >= 0; i-- {
		if evs[i].Kind == "prompt" && evs[i].Text == text {
			return i
		}
	}
	return -1
}

// waitReleased waits until the Session's agent process is released after its
// turn, so the next send takes the resume path.
func waitReleased(t *testing.T, d *Daemon, id string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		d.mu.Lock()
		live := d.sessions[id].live
		d.mu.Unlock()
		if !live {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("session %s stayed live after its turn", id)
}

// compressAndSettle compresses the Session and waits for its n-th compressed Event.
func compressAndSettle(t *testing.T, base, id string, n int) {
	t.Helper()
	if code, body := compressReq(t, base, id, ""); code != 202 {
		t.Fatalf("compress %s = %d %v", id, code, body)
	}
	waitIdleAfter(t, base, id, n)
}

// roleSet lists the distinct el roles the fake Backend got for one Session.
func roleSet(be *FakeBackend, id string) string {
	seen := map[string]bool{}
	var out []string
	for _, s := range be.Starts(id) {
		if !seen[s.handle.role] {
			seen[s.handle.role] = true
			out = append(out, s.handle.role)
		}
	}
	return strings.Join(out, ",")
}

// opSeen reports whether one Session got a start of this operation.
func opSeen(be *FakeBackend, id string, op handleOp) bool {
	for _, s := range be.Starts(id) {
		if s.op == op {
			return true
		}
	}
	return false
}

// spawnScout spawns one scout Subagent of parent and returns its ID.
func spawnScout(t *testing.T, base, parent string) string {
	t.Helper()
	code, out := postSpawn(t, base, parent, map[string]any{
		"agents": []map[string]any{{"prompt": "look around", "type": "scout"}},
	})
	if code != http.StatusCreated {
		t.Fatalf("scout spawn status = %d (%v)", code, out)
	}
	raw, _ := json.Marshal(out["sessions"])
	var kids []SessionMeta
	if err := json.Unmarshal(raw, &kids); err != nil || len(kids) != 1 {
		t.Fatalf("scout spawn sessions = %v, want 1", out["sessions"])
	}
	return kids[0].ID
}

func TestStartHandleDerivesFromRecord(t *testing.T) {
	cases := []struct {
		name string
		meta SessionMeta
		role string
	}{
		{"top-level", SessionMeta{Owner: "u1"}, "web"},
		{"subagent", SessionMeta{Owner: "u1", GroupID: "g1"}, "web-worker"},
		{"run ticket", SessionMeta{Owner: "u1", GroupID: "g1", RunWorker: true}, "run-worker"},
		{"scout", SessionMeta{Owner: "u1", GroupID: "g1", Scout: true}, "scout"},
	}
	for _, c := range cases {
		for _, op := range []handleOp{handleCreate, handleResume, handleCompress} {
			h := startHandle(c.meta, op)
			if h.role != c.role || h.owner != "u1" || !h.inject.empty() {
				t.Fatalf("%s op %d: handle = %+v, want role %q, owner u1, no injections", c.name, op, h, c.role)
			}
		}
	}
}

func TestCreateInjectsOnlyBeforeFirstTurn(t *testing.T) {
	hook := func(_ SessionMeta, op handleOp) injection {
		if op == handleCreate {
			return injection{texts: []string{"PROFILE", "RECALL"}}
		}
		return injection{}
	}
	d, be, seen, base, id := handleDaemon(t, hook)
	runPrompt(t, base, id, "build the thing")
	waitReleased(t, d, id)
	runPrompt(t, base, id, "and more")

	want := []string{"PROFILE\n\nRECALL\n\nbuild the thing", "and more"}
	if got := seen.all(); len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("prompts received = %q, want %q", got, want)
	}
	starts := be.Starts(id)
	if starts[0].op != handleCreate || strings.Join(starts[0].handle.inject.texts, "|") != "PROFILE|RECALL" {
		t.Fatalf("create start = %+v, want PROFILE then RECALL", starts[0])
	}
}

func TestCompressInjectsProfileRecallThenHandoff(t *testing.T) {
	hook := func(_ SessionMeta, op handleOp) injection {
		if op == handleCompress {
			return injection{texts: []string{"PROFILE", "RECALL"}}
		}
		return injection{}
	}
	_, be, seen, base, id := handleDaemon(t, hook)
	runPrompt(t, base, id, "build the thing")
	compressAndSettle(t, base, id, 1)

	evs := transcriptOf(t, base, id)
	handoff := handoffText(evs)
	if !strings.HasPrefix(handoff, handoffHeader) {
		t.Fatalf("Handoff prompt Event = %q, want the Handoff alone", handoff)
	}
	got := seen.all()
	if want := "PROFILE\n\nRECALL\n\n" + handoff; got[len(got)-1] != want {
		t.Fatalf("compress turn = %q, want %q", got[len(got)-1], want)
	}
	starts := be.Starts(id)
	last := starts[len(starts)-1]
	if last.op != handleCompress || strings.Join(last.handle.inject.texts, "|") != "PROFILE|RECALL" {
		t.Fatalf("compress start = %+v, want PROFILE then RECALL", last)
	}
}

func TestCompressWithoutMemorySendsHandoffAlone(t *testing.T) {
	_, be, seen, base, id := handleDaemon(t, nil)
	runPrompt(t, base, id, "build the thing")
	compressAndSettle(t, base, id, 1)

	handoff := handoffText(transcriptOf(t, base, id))
	got := seen.all()
	if got[len(got)-1] != handoff {
		t.Fatalf("compress turn = %q, want the Handoff alone", got[len(got)-1])
	}
	for _, s := range be.Starts(id) {
		if !s.handle.inject.empty() {
			t.Fatalf("start %+v injects with no memory", s)
		}
	}
}

func TestResumeInjectsOnEachResumedTurn(t *testing.T) {
	hook := func(_ SessionMeta, op handleOp) injection {
		if op == handleResume {
			return injection{texts: []string{"PROFILE"}}
		}
		return injection{}
	}
	d, be, seen, base, id := handleDaemon(t, hook)
	for _, p := range []string{"one", "two", "three"} {
		runPrompt(t, base, id, p)
		waitReleased(t, d, id)
	}
	want := []string{"one", "PROFILE\n\ntwo", "PROFILE\n\nthree"}
	got := seen.all()
	if len(got) != len(want) {
		t.Fatalf("prompts received = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("prompt %d = %q, want %q", i, got[i], want[i])
		}
	}
	ops := be.Starts(id)
	if len(ops) != 3 || ops[0].op != handleCreate || ops[1].op != handleResume || ops[2].op != handleResume {
		t.Fatalf("starts = %+v, want create, resume, resume", ops)
	}
}

func TestCompressKeepsOwnerAcrossTokenRotation(t *testing.T) {
	d, be, _, base, id := handleDaemon(t, nil)
	runPrompt(t, base, id, "build the thing")
	waitReleased(t, d, id)
	before, _ := sessionMeta(t, base, id)
	if before.Owner == "" {
		t.Fatal("Session has no owner")
	}
	compressAndSettle(t, base, id, 1)
	after, _ := sessionMeta(t, base, id)
	if after.ResumeToken == before.ResumeToken {
		t.Fatalf("resume token did not rotate: %q", after.ResumeToken)
	}
	if after.Owner != before.Owner {
		t.Fatalf("owner after compress = %q, want %q", after.Owner, before.Owner)
	}
	for _, s := range be.Starts(id) {
		if s.handle.owner != before.Owner {
			t.Fatalf("start %+v has owner %q, want %q", s, s.handle.owner, before.Owner)
		}
	}
}

func TestToolSetHoldsAcrossCompress(t *testing.T) {
	d, be, _, base, parent := handleDaemon(t, nil)
	parentMeta, _ := sessionMeta(t, base, parent)

	child := spawnOne(t, base, parent, "slow job")
	waitStatus(t, base, child, "idle", 10*time.Second)
	compressAndSettle(t, base, child, 1)
	if got := roleSet(be, child); got != "web-worker" || !opSeen(be, child, handleCompress) {
		t.Fatalf("worker roles = %q, want web-worker before and after compress", got)
	}

	scout := spawnScout(t, base, parent)
	waitStatus(t, base, scout, "idle", 10*time.Second)
	if code, body := compressReq(t, base, scout, ""); code != http.StatusForbidden {
		t.Fatalf("scout compress = %d %v, want 403", code, body)
	}
	if got := roleSet(be, scout); got != "scout" {
		t.Fatalf("scout roles = %q, want scout alone", got)
	}

	rw := SessionMeta{
		ID: newID(), Harness: "fake", Model: "fake-model", WorkDir: tempDir(t),
		Status: "idle", CreatedAt: time.Now().UTC(), Owner: parentMeta.Owner, RunWorker: true,
	}
	if _, serr := d.startSession(rw); serr != nil {
		t.Fatalf("run worker start: %s", serr.msg)
	}
	runPrompt(t, base, rw.ID, "fix the ticket")
	compressAndSettle(t, base, rw.ID, 1)
	if got := roleSet(be, rw.ID); got != "run-worker" || !opSeen(be, rw.ID, handleCompress) {
		t.Fatalf("run worker roles = %q, want run-worker before and after compress", got)
	}

	rv := SessionMeta{
		ID: newID(), Harness: "fake", Model: "fake-model", WorkDir: tempDir(t),
		Status: "idle", CreatedAt: time.Now().UTC(), Owner: parentMeta.Owner, ParentID: "run-test",
	}
	if _, serr := d.startSession(rv); serr != nil {
		t.Fatalf("reviewer start: %s", serr.msg)
	}
	runPrompt(t, base, rv.ID, "review the ticket")
	compressAndSettle(t, base, rv.ID, 1)
	if got := roleSet(be, rv.ID); got != "run-worker" || !opSeen(be, rv.ID, handleCompress) {
		t.Fatalf("reviewer roles = %q, want run-worker before and after compress", got)
	}
}

func TestParentLinksSurviveCompress(t *testing.T) {
	d, _, _, base, parent := handleDaemon(t, nil)
	child := spawnOne(t, base, parent, "slow job")
	waitStatus(t, base, child, "idle", 10*time.Second)
	waitGroupedDelivery(t, base, parent, child)
	waitParentIdle(t, base, parent)
	compressAndSettle(t, base, parent, 1)

	if meta, _ := sessionMeta(t, base, child); meta.ParentID != parent {
		t.Fatalf("child parent = %q, want %q", meta.ParentID, parent)
	}
	if out, code, msg := d.CheckChildren(parent); code != 0 || !strings.Contains(out, child[:8]) {
		t.Fatalf("check after compress = %q (%d %s), want child %s", out, code, msg, child)
	}
	again := spawnOne(t, base, parent, "after the swap")
	if meta, _ := sessionMeta(t, base, again); meta.ParentID != parent || meta.GroupID == "" {
		t.Fatalf("spawn after compress: parent %q group %q, want parent %s and a group", meta.ParentID, meta.GroupID, parent)
	}
	if _, code, msg := d.SteerChild(parent, child, "go left"); code != 0 {
		t.Fatalf("steer after compress = %d %s", code, msg)
	}
	waitPrompt(t, base, child, "go left", 10*time.Second)
	waitStatus(t, base, child, "idle", 10*time.Second)
	if _, code, msg := d.CancelChild(parent, child, true); code != 0 {
		t.Fatalf("cancel after compress = %d %s", code, msg)
	}
	if meta, _ := sessionMeta(t, base, child); meta.Status != "ended" {
		t.Fatalf("child status after kill = %q, want ended", meta.Status)
	}
}

func TestPigLoadoutGatesDaemonToolsByRole(t *testing.T) {
	pb := NewPigBackend()
	pb.DaemonURL = "http://127.0.0.1:1"
	pb.Spawn = func(string, []SpawnAgent, string) (string, []SessionMeta, error) { return "", nil, nil }
	pb.SteerChild = func(string, string, string) (string, error) { return "", nil }
	pb.CheckChild = func(string, string) (string, error) { return "", nil }
	pb.CancelChild = func(string, string, bool) (string, error) { return "", nil }
	names := func(meta SessionMeta) string {
		var out []string
		for _, tl := range pb.sessionTools(meta) {
			switch n := tl.Name(); n {
			case "spawn", "steer", "check", "cancel", "submit":
				out = append(out, n)
			}
		}
		return strings.Join(out, ",")
	}
	cases := []struct {
		name string
		meta SessionMeta
		want string
	}{
		{"top-level", SessionMeta{ID: "p"}, "spawn,steer,check,cancel"},
		{"subagent", SessionMeta{ID: "w", GroupID: "g"}, "spawn"},
		{"run ticket", SessionMeta{ID: "r", GroupID: "g", RunWorker: true}, "submit,spawn"},
		{"scout", SessionMeta{ID: "s", Scout: true}, ""},
	}
	for _, c := range cases {
		if got := names(c.meta); got != c.want {
			t.Fatalf("%s loadout = %q, want %q", c.name, got, c.want)
		}
	}
}
