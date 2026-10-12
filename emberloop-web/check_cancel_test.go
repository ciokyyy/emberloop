package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	eltools "example.com/emberloop/tools"
)

func wirePigFamily(d *Daemon) *PigBackend {
	pb := NewPigBackend()
	pb.Spawn = func(parentID string, agents []SpawnAgent, deliver string) (string, []SessionMeta, error) {
		gid, kids, code, msg := d.Spawn(parentID, agents, deliver)
		if code != 0 {
			return "", nil, fmt.Errorf("spawn: %s", msg)
		}
		return gid, kids, nil
	}
	pb.SteerChild = func(parentID, childID, text string) (string, error) {
		outcome, code, msg := d.SteerChild(parentID, childID, text)
		if code != 0 {
			return "", fmt.Errorf("steer: %s", msg)
		}
		return outcome, nil
	}
	pb.CheckChild = func(parentID, childID string) (string, error) {
		var outcome string
		var code int
		var msg string
		if childID == "" {
			outcome, code, msg = d.CheckChildren(parentID)
		} else {
			outcome, code, msg = d.CheckChild(parentID, childID)
		}
		if code != 0 {
			return "", fmt.Errorf("check: %s", msg)
		}
		return outcome, nil
	}
	pb.CancelChild = func(parentID, childID string, kill bool) (string, error) {
		outcome, code, msg := d.CancelChild(parentID, childID, kill)
		if code != 0 {
			return "", fmt.Errorf("cancel: %s", msg)
		}
		return outcome, nil
	}
	return pb
}

func pigCheckTool(t *testing.T, pb *PigBackend, sessionID string) *checkAgentTool {
	t.Helper()
	for _, tl := range pb.sessionTools(SessionMeta{ID: sessionID}) {
		if ct, ok := tl.(*checkAgentTool); ok {
			return ct
		}
	}
	t.Fatalf("check tool missing from pig loadout of %s", sessionID)
	return nil
}

func pigCancelTool(t *testing.T, pb *PigBackend, sessionID string) *cancelAgentTool {
	t.Helper()
	for _, tl := range pb.sessionTools(SessionMeta{ID: sessionID}) {
		if ct, ok := tl.(*cancelAgentTool); ok {
			return ct
		}
	}
	t.Fatalf("cancel tool missing from pig loadout of %s", sessionID)
	return nil
}

func spawnTwo(t *testing.T, base, parent, first, second string) (string, string) {
	t.Helper()
	code, out := postSpawn(t, base, parent, map[string]any{
		"agents": []map[string]any{{"prompt": first}, {"prompt": second}},
	})
	if code != http.StatusCreated {
		t.Fatalf("spawn status = %d (%v)", code, out)
	}
	raw, _ := json.Marshal(out["sessions"])
	var kids []SessionMeta
	if err := json.Unmarshal(raw, &kids); err != nil || len(kids) != 2 {
		t.Fatalf("spawn sessions = %v, want 2", out["sessions"])
	}
	return kids[0].ID, kids[1].ID
}

// proxyCallOne runs one Daemon proxy tool the way the single el MCP server
// serves it: the role's tool with the Session binding in its environment.
func proxyCallOne(t *testing.T, daemonURL, parent, name, args string) (string, error) {
	t.Helper()
	t.Setenv("EL_WEB_PARENT", parent)
	t.Setenv("EL_WEB_DAEMON_URL", daemonURL)
	tool := eltools.WebProxyTool(name)
	if tool == nil {
		t.Fatalf("unknown proxy tool %q", name)
	}
	return tool.Execute(context.Background(), tempDir(t), json.RawMessage(args))
}

// Check with no id lists the caller's own children, worker and scout alike,
// without messaging them.
func TestCheckListsOwnChildren(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	pb := wirePigFamily(d)

	parent := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	code, out := postSpawn(t, srv.URL, parent, map[string]any{
		"agents": []map[string]any{{"prompt": "job A"}, {"prompt": "look only", "type": "scout"}},
	})
	if code != http.StatusCreated {
		t.Fatalf("spawn status = %d (%v)", code, out)
	}
	raw, _ := json.Marshal(out["sessions"])
	var kids []SessionMeta
	_ = json.Unmarshal(raw, &kids)
	waitStatus(t, srv.URL, kids[0].ID, "idle", 10*time.Second)
	waitStatus(t, srv.URL, kids[1].ID, "idle", 10*time.Second)

	tool := pigCheckTool(t, pb, parent)
	res, err := tool.Execute(context.Background(), "c1", json.RawMessage(`{}`), nil)
	if err != nil {
		t.Fatalf("check own children: %v", err)
	}
	got := res.Text()
	for _, want := range []string{kids[0].ID[:8], kids[1].ID[:8], "worker", "scout", "status="} {
		if !strings.Contains(got, want) {
			t.Fatalf("check list = %q, want %q", got, want)
		}
	}
	// A stranger's own list is empty: peeking filters by recorded parent.
	stranger := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	stool := pigCheckTool(t, pb, stranger)
	sres, err := stool.Execute(context.Background(), "c2", json.RawMessage(`{}`), nil)
	if err != nil {
		t.Fatalf("stranger check: %v", err)
	}
	if sres.Text() != "no subagents" {
		t.Fatalf("stranger check = %q, want no subagents", sres.Text())
	}
	_, events := sessionMeta(t, srv.URL, kids[0].ID)
	for _, e := range events {
		if e.Kind == "prompt" && strings.Contains(e.Text, "check") {
			t.Fatalf("check messaged the child: %+v", e)
		}
	}
	waitParentIdle(t, srv.URL, parent)
}

// Check with an id shows the child's summary plus its recent events, and the
// tool description tells callers not to poll.
func TestCheckChildShowsSummaryAndEvents(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	pb := wirePigFamily(d)

	parent := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	child := spawnOne(t, srv.URL, parent, "slow job")
	waitStatus(t, srv.URL, child, "idle", 10*time.Second)

	outcome, code, msg := d.CheckChild(parent, child)
	if code != 0 {
		t.Fatalf("check code = %d (%s)", code, msg)
	}
	for _, want := range []string{"status=", "fake reply to: slow job", "#"} {
		if !strings.Contains(outcome, want) {
			t.Fatalf("check = %q, want %q", outcome, want)
		}
	}
	tool := pigCheckTool(t, pb, parent)
	res, err := tool.Execute(context.Background(), "c1", json.RawMessage(`{"id":"`+child+`"}`), nil)
	if err != nil {
		t.Fatalf("pig check own child: %v", err)
	}
	if !strings.Contains(res.Text(), "status=") || !strings.Contains(res.Text(), "#") {
		t.Fatalf("pig check = %q, want summary plus events", res.Text())
	}
	for _, tl := range pb.sessionTools(SessionMeta{ID: parent}) {
		if ct, ok := tl.(*checkAgentTool); ok {
			desc := ct.Schema().Description
			if !strings.Contains(desc, "do not poll") || !strings.Contains(desc, "grouped result") {
				t.Fatalf("check description = %q, want no-poll plus grouped result", desc)
			}
		}
	}
	// The settled child delivers the grouped result to the parent as a
	// steer, so the parent works briefly before going idle again: wait it
	// out before the state dir is cleaned up, or its writes race TempDir
	// removal.
	waitStatus(t, srv.URL, parent, "idle", 20*time.Second)
}

// A stranger's id is refused on every check path.
func TestCheckNonChildRefused(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	pb := wirePigFamily(d)

	owner := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	stranger := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	child := spawnOne(t, srv.URL, owner, "job A")
	waitStatus(t, srv.URL, child, "idle", 10*time.Second)

	if _, code, _ := d.CheckChild(stranger, child); code != http.StatusForbidden {
		t.Fatalf("CheckChild non-child code = %d, want 403", code)
	}
	if _, code, _ := d.CheckChild(owner, "missing"); code != http.StatusNotFound {
		t.Fatalf("CheckChild unknown code = %d, want 404", code)
	}
	tool := pigCheckTool(t, pb, stranger)
	if _, err := tool.Execute(context.Background(), "c1",
		json.RawMessage(`{"id":"`+child+`"}`), nil); err == nil ||
		!strings.Contains(err.Error(), "not a child") {
		t.Fatalf("pig check non-child err = %v, want not-a-child", err)
	}
	if _, err := proxyCallOne(t, srv.URL, stranger, "check", `{"id":"`+child+`"}`); err == nil ||
		!strings.Contains(err.Error(), "not a child") {
		t.Fatalf("proxy check non-child err = %v, want not-a-child", err)
	}
	waitParentIdle(t, srv.URL, owner)
}

// Both proxy check forms read the bound parent's own children through the
// same session routes the UI uses.
func TestProxyCheckReadsOwnChildren(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	parent := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	child := spawnOne(t, srv.URL, parent, "job A")
	waitStatus(t, srv.URL, child, "idle", 10*time.Second)

	list, err := proxyCallOne(t, srv.URL, parent, "check", `{}`)
	if err != nil {
		t.Fatalf("proxy check list: %v", err)
	}
	if !strings.Contains(list, child[:8]) {
		t.Fatalf("proxy check list = %q, want child %s", list, child[:8])
	}
	one, err := proxyCallOne(t, srv.URL, parent, "check", `{"id":"`+child+`"}`)
	if err != nil {
		t.Fatalf("proxy check child: %v", err)
	}
	if !strings.Contains(one, "status=") || !strings.Contains(one, "#") {
		t.Fatalf("proxy check child = %q, want summary plus events", one)
	}
	waitParentIdle(t, srv.URL, parent)
}

// Cancelling a working child interrupts its turn and drops its queue while
// the parent and the sibling keep going.
func TestCancelInterruptsWorkingChild(t *testing.T) {
	t.Parallel()
	fake := NewFakeBackend()
	fake.Delay = 5 * time.Second
	d := NewDaemon(tempDir(t), fake)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	pb := wirePigFamily(d)

	parent := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	a, b := spawnTwo(t, srv.URL, parent, "slow A", "slow B")
	waitStatus(t, srv.URL, a, "working", 5*time.Second)
	waitStatus(t, srv.URL, b, "working", 5*time.Second)

	tool := pigCancelTool(t, pb, parent)
	res, err := tool.Execute(context.Background(), "c1", json.RawMessage(`{"id":"`+a+`"}`), nil)
	if err != nil {
		t.Fatalf("cancel working child: %v", err)
	}
	if !strings.Contains(res.Text(), "interrupted") || !strings.Contains(res.Text(), a) {
		t.Fatalf("cancel result = %q, want interrupted plus child id", res.Text())
	}
	waitStatus(t, srv.URL, a, "idle", 10*time.Second)
	if meta, _ := sessionMeta(t, srv.URL, parent); meta.Status != "idle" {
		t.Fatalf("parent status = %q, want idle (cancel must not touch it)", meta.Status)
	}
	if meta, _ := sessionMeta(t, srv.URL, b); meta.Status != "working" {
		t.Fatalf("sibling status = %q, want working (cancel must not touch it)", meta.Status)
	}
	outcome, code, _ := d.CheckChild(parent, a)
	if code != 0 || !strings.Contains(outcome, "status=") {
		t.Fatalf("check after cancel = %q (%d), want a summary", outcome, code)
	}
	// Stop the sibling too; the grouped result still delivers once both settle.
	if _, err := tool.Execute(context.Background(), "c2", json.RawMessage(`{"id":"`+b+`"}`), nil); err != nil {
		t.Fatalf("cancel sibling: %v", err)
	}
	waitStatus(t, srv.URL, b, "idle", 10*time.Second)
	waitStatus(t, srv.URL, parent, "idle", 20*time.Second)
}

// Cancelling an idle child is a no-op that says so.
func TestCancelIdleReportsAlreadyIdle(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	pb := wirePigFamily(d)

	parent := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	child := spawnOne(t, srv.URL, parent, "quick job")
	waitStatus(t, srv.URL, child, "idle", 10*time.Second)

	outcome, code, msg := d.CancelChild(parent, child, false)
	if code != 0 {
		t.Fatalf("cancel idle code = %d (%s)", code, msg)
	}
	if !strings.Contains(outcome, "already idle") || !strings.Contains(outcome, child) {
		t.Fatalf("cancel idle = %q, want already-idle plus id", outcome)
	}
	tool := pigCancelTool(t, pb, parent)
	res, err := tool.Execute(context.Background(), "c1", json.RawMessage(`{"id":"`+child+`"}`), nil)
	if err != nil {
		t.Fatalf("pig cancel idle: %v", err)
	}
	if !strings.Contains(res.Text(), "already idle") {
		t.Fatalf("pig cancel idle = %q, want already idle", res.Text())
	}
	if meta, _ := sessionMeta(t, srv.URL, child); meta.Status != "idle" {
		t.Fatalf("child status = %q, want idle", meta.Status)
	}
	waitParentIdle(t, srv.URL, parent)
}

// Kill ends the child Session like DELETE does: it reads ended and refuses
// further sends, while the parent keeps going.
func TestCancelKillEndsChild(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	pb := wirePigFamily(d)

	parent := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	child := spawnOne(t, srv.URL, parent, "doomed job")
	waitStatus(t, srv.URL, child, "idle", 10*time.Second)

	tool := pigCancelTool(t, pb, parent)
	res, err := tool.Execute(context.Background(), "c1",
		json.RawMessage(`{"id":"`+child+`","kill":true}`), nil)
	if err != nil {
		t.Fatalf("cancel kill: %v", err)
	}
	if !strings.Contains(res.Text(), "killed") || !strings.Contains(res.Text(), child) {
		t.Fatalf("cancel kill = %q, want killed plus id", res.Text())
	}
	if meta, _ := sessionMeta(t, srv.URL, child); meta.Status != "ended" {
		t.Fatalf("killed status = %q, want ended", meta.Status)
	}
	if _, code, _ := d.SteerChild(parent, child, "late"); code != http.StatusGone {
		t.Fatalf("steer killed code = %d, want 410", code)
	}
	outcome, code, _ := d.CheckChild(parent, child)
	if code != 0 || !strings.Contains(outcome, "status=ended") {
		t.Fatalf("check killed = %q (%d), want status=ended", outcome, code)
	}
	// The kill can land while the child's turn still runs, so
	// the Group's single delivery (and the parent's brief turn on it) starts
	// at kill time: wait it out the way the proxy cancel test does.
	waitStatus(t, srv.URL, parent, "idle", 20*time.Second)
}

// A stranger cannot cancel or kill another parent's child on any path, while
// a parent may stop its own scout: a stop is not a message.
func TestCancelNonChildRefusedScoutStoppable(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	pb := wirePigFamily(d)

	owner := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	stranger := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	child := spawnOne(t, srv.URL, owner, "job A")
	waitStatus(t, srv.URL, child, "idle", 10*time.Second)

	if _, code, _ := d.CancelChild(stranger, child, false); code != http.StatusForbidden {
		t.Fatalf("CancelChild non-child code = %d, want 403", code)
	}
	if _, code, _ := d.CancelChild(stranger, child, true); code != http.StatusForbidden {
		t.Fatalf("CancelChild kill non-child code = %d, want 403", code)
	}
	tool := pigCancelTool(t, pb, stranger)
	if _, err := tool.Execute(context.Background(), "c1",
		json.RawMessage(`{"id":"`+child+`"}`), nil); err == nil ||
		!strings.Contains(err.Error(), "not a child") {
		t.Fatalf("pig cancel non-child err = %v, want not-a-child", err)
	}
	if _, err := proxyCallOne(t, srv.URL, stranger, "cancel", `{"id":"`+child+`"}`); err == nil ||
		!strings.Contains(err.Error(), "not a child") {
		t.Fatalf("proxy cancel non-child err = %v, want not-a-child", err)
	}
	if meta, _ := sessionMeta(t, srv.URL, child); meta.Status == "ended" {
		t.Fatalf("refused kill ended the child")
	}

	code, out := postSpawn(t, srv.URL, owner, map[string]any{
		"agents": []map[string]any{{"prompt": "look only", "type": "scout"}},
	})
	if code != http.StatusCreated {
		t.Fatalf("spawn status = %d (%v)", code, out)
	}
	raw, _ := json.Marshal(out["sessions"])
	var scouts []SessionMeta
	_ = json.Unmarshal(raw, &scouts)
	waitStatus(t, srv.URL, scouts[0].ID, "idle", 10*time.Second)
	if _, code, _ := d.CancelChild(owner, scouts[0].ID, false); code != 0 {
		t.Fatalf("CancelChild own scout code = %d, want 0", code)
	}
	if outcome, code, _ := d.CancelChild(owner, scouts[0].ID, true); code != 0 ||
		!strings.Contains(outcome, "killed") {
		t.Fatalf("CancelChild kill own scout = %q (%d), want killed", outcome, code)
	}
	waitParentIdle(t, srv.URL, owner)
}

// The proxy cancel path interrupts a working child through the interrupt route
// and kills through DELETE.
func TestProxyCancelInterruptsAndKills(t *testing.T) {
	fake := NewFakeBackend()
	fake.Delay = 5 * time.Second
	d := NewDaemon(tempDir(t), fake)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	parent := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	child := spawnOne(t, srv.URL, parent, "slow job")
	waitStatus(t, srv.URL, child, "working", 5*time.Second)

	out, err := proxyCallOne(t, srv.URL, parent, "cancel", `{"id":"`+child+`"}`)
	if err != nil {
		t.Fatalf("proxy cancel: %v", err)
	}
	if !strings.Contains(out, "interrupted") || !strings.Contains(out, child) {
		t.Fatalf("proxy cancel = %q, want interrupted plus id", out)
	}
	waitStatus(t, srv.URL, child, "idle", 10*time.Second)
	// The single child's settle delivers the grouped result to the parent as
	// a steer, so the parent works briefly before going idle again.
	waitStatus(t, srv.URL, parent, "idle", 20*time.Second)
	kill, err := proxyCallOne(t, srv.URL, parent, "cancel", `{"id":"`+child+`","kill":true}`)
	if err != nil {
		t.Fatalf("proxy cancel kill: %v", err)
	}
	if !strings.Contains(kill, "killed") {
		t.Fatalf("proxy cancel kill = %q, want killed", kill)
	}
	if meta, _ := sessionMeta(t, srv.URL, child); meta.Status != "ended" {
		t.Fatalf("proxy killed status = %q, want ended", meta.Status)
	}
}
