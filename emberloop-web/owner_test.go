package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Owners, inheritance, hidden ownerless data, prompt senders and the home
// directory at the Daemon HTTP seam. Users sign in through the cookie login,
// as a browser does; the fake Backend (or a stub claude adapter) runs the turns.

// withCookie serves h as the signed-in User behind c.
func withCookie(h http.Handler, c *http.Cookie) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.AddCookie(c)
		h.ServeHTTP(w, r)
	})
}

// withBearer serves h as a machine caller (agents, scripts). The machine
// token passes the gate but is no User.
func withBearer(h http.Handler, token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+token)
		h.ServeHTTP(w, r)
	})
}

// userOf returns the User behind a login cookie.
func userOf(t *testing.T, h http.Handler, c *http.Cookie) User {
	t.Helper()
	return decodeUser(t, authDo(t, h, http.MethodGet, "/api/auth/me", "", c))
}

// metaOf reads one Session's metadata over HTTP.
func metaOf(t *testing.T, base, id string) SessionMeta {
	t.Helper()
	resp, err := http.Get(base + "/api/sessions/" + id)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get session %s = %d", id, resp.StatusCode)
	}
	var out struct {
		Session SessionMeta `json:"session"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.Session
}

// runMapOf reads one Run over HTTP as a generic map, so every field shows.
func runMapOf(t *testing.T, base, id string) map[string]any {
	t.Helper()
	resp, err := http.Get(base + "/api/runs/" + id)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get run %s = %d", id, resp.StatusCode)
	}
	var out struct {
		Run map[string]any `json:"run"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.Run
}

// statusOf sends one request and returns its status code.
func statusOf(t *testing.T, method, url, body string) int {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

// settle waits until the Session is idle with nothing queued and returns its
// transcript.
func settle(t *testing.T, base, id string) []Event {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(getQueue(t, base, id)) == 0 {
			evs := transcriptOf(t, base, id)
			if n := len(evs); n > 0 && evs[n-1].Kind == "status" && evs[n-1].Status == "idle" {
				return evs
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("session %s did not settle: %+v", id, transcriptOf(t, base, id))
	return nil
}

// waitUntil polls cond until it holds, failing the test after five seconds.
func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// firstPromptWhere polls the Session until a prompt Event matches pred.
func firstPromptWhere(t *testing.T, base, id string, pred func(Event) bool) Event {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, e := range transcriptOf(t, base, id) {
			if e.Kind == "prompt" && pred(e) {
				return e
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no matching prompt in session %s: %+v", id, transcriptOf(t, base, id))
	return Event{}
}

// promptNamed returns the prompt Event carrying text.
func promptNamed(t *testing.T, evs []Event, text string) Event {
	t.Helper()
	for _, e := range evs {
		if e.Kind == "prompt" && e.Text == text {
			return e
		}
	}
	t.Fatalf("no prompt %q in %+v", text, evs)
	return Event{}
}

// wantSender asserts a prompt Event's sender: "user" with a User ID, or
// "system" with no User.
func wantSender(t *testing.T, e Event, sender, user string) {
	t.Helper()
	if e.Sender != sender || e.User != user {
		t.Fatalf("prompt %q: sender=%q user=%q, want sender=%q user=%q", e.Text, e.Sender, e.User, sender, user)
	}
}

// waitChildren polls until at least n Sessions name parent as their parent.
func waitChildren(t *testing.T, base, parent string, n int) []SessionMeta {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var kids []SessionMeta
		for _, s := range getSessions(t, base) {
			if s.ParentID == parent {
				kids = append(kids, s)
			}
		}
		if len(kids) >= n {
			return kids
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("run %s has fewer than %d Sessions", parent, n)
	return nil
}

// stripOwner rewrites a meta.json without its owner: the shape of Session
// data written before owners existed.
func stripOwner(t *testing.T, path string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var meta map[string]any
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatal(err)
	}
	if _, ok := meta["owner"]; !ok {
		t.Fatalf("no owner in %s", path)
	}
	delete(meta, "owner")
	out, _ := json.Marshal(meta)
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
}

// Creating a Session records the signed-in User who sent the create.
func TestCreateRecordsCookieUserAsOwner(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	h := d.Handler()
	signedIn(t, h, "admin")
	bob := signedIn(t, h, "bob")
	bobID := userOf(t, h, bob).ID
	srv := httptest.NewServer(withCookie(h, bob))
	defer srv.Close()

	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	if got := metaOf(t, srv.URL, id).Owner; got != bobID {
		t.Fatalf("owner = %q, want bob %q", got, bobID)
	}
}

// The machine token is no User: it cannot create a Session or a Run.
func TestMachineTokenCannotCreateSessionOrRun(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	h := d.Handler()
	signedIn(t, h, "admin")
	agent := httptest.NewServer(withBearer(h, d.MachineToken()))
	defer agent.Close()

	body, _ := json.Marshal(map[string]string{"harness": "fake", "model": "fake-model", "workDir": tempDir(t)})
	if code := statusOf(t, http.MethodPost, agent.URL+"/api/sessions", string(body)); code != http.StatusForbidden {
		t.Fatalf("machine create session = %d, want 403", code)
	}
	if code := statusOf(t, http.MethodPost, agent.URL+"/api/runs", `{"spec":"/repo/.scratch/demo/spec.md"}`); code != http.StatusForbidden {
		t.Fatalf("machine create run = %d, want 403", code)
	}
	if got := getSessions(t, agent.URL); len(got) != 0 {
		t.Fatalf("sessions after refused create = %+v, want none", got)
	}
}

// Subagents take the owner of their parent, not the owner of the User who
// spawned them. Each first prompt is typed by the sender of the spawn request.
func TestSubagentsInheritParentOwner(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	h := d.Handler()
	admin := signedIn(t, h, "admin")
	adminID := userOf(t, h, admin).ID
	bob := signedIn(t, h, "bob")
	bobID := userOf(t, h, bob).ID
	asAdmin := httptest.NewServer(withCookie(h, admin))
	defer asAdmin.Close()
	asBob := httptest.NewServer(withCookie(h, bob))
	defer asBob.Close()
	asAgent := httptest.NewServer(withBearer(h, d.MachineToken()))
	defer asAgent.Close()

	parent := createSession(t, asAdmin.URL, "fake", "fake-model", tempDir(t))
	byBob := spawnOne(t, asBob.URL, parent, "bob job")
	byAgent := spawnOne(t, asAgent.URL, parent, "agent job")
	for _, kid := range []string{byBob, byAgent} {
		if got := metaOf(t, asAdmin.URL, kid).Owner; got != adminID {
			t.Fatalf("subagent %s owner = %q, want parent owner %q", kid, got, adminID)
		}
	}
	wantSender(t, firstPromptWhere(t, asAdmin.URL, byBob, func(e Event) bool { return e.Text == "bob job" }), "user", bobID)
	wantSender(t, firstPromptWhere(t, asAdmin.URL, byAgent, func(e Event) bool { return e.Text == "agent job" }), "system", "")

	// The grouped result a Group delivers to the parent is a Daemon prompt.
	group := firstPromptWhere(t, asAdmin.URL, parent, func(e Event) bool { return e.Mode == "steer" })
	wantSender(t, group, "system", "")
}

// A Run records the User who started it; its Worker and Reviewer Sessions
// inherit that owner, and their driver prompts are system prompts.
func TestRunOwnerReachesWorkerAndReviewers(t *testing.T) {
	repo, spec := initRunRepo(t, demoTickets())
	d := NewDaemon(tempDir(t), NewFakeBackend())
	h := d.Handler()
	admin := signedIn(t, h, "admin")
	adminID := userOf(t, h, admin).ID
	srv := httptest.NewServer(withCookie(h, admin))
	defer srv.Close()

	run := startRun(t, srv.URL, spec, repo, "fake", "fake-model")
	if got := runMapOf(t, srv.URL, run.ID)["owner"]; got != adminID {
		t.Fatalf("run owner = %v, want %q", got, adminID)
	}
	worker := waitChildren(t, srv.URL, run.ID, 1)[0]
	commitTicketWork(t, repo, "01-first")
	submitSession(t, srv.URL, worker.ID, "done", []string{"alpha"})
	kids := waitChildren(t, srv.URL, run.ID, 3)
	for _, kid := range kids {
		if kid.Owner != adminID {
			t.Fatalf("run session %s owner = %q, want %q", kid.ID, kid.Owner, adminID)
		}
	}
	wantSender(t, firstPromptWhere(t, srv.URL, worker.ID, func(Event) bool { return true }), "system", "")
	for _, kid := range kids {
		if kid.ID != worker.ID {
			wantSender(t, firstPromptWhere(t, srv.URL, kid.ID, func(Event) bool { return true }), "system", "")
		}
	}
}

// Sessions and Runs with no owner predate owners: lists skip them and every
// route answers 404, so no User reaches them. Data is not migrated.
func TestOwnerlessSessionsAndRunsAreHidden(t *testing.T) {
	stateDir := tempDir(t)
	d1 := NewDaemon(stateDir, NewFakeBackend())
	srv1 := httptest.NewServer(authedHandler(t, d1))
	legacy := createSession(t, srv1.URL, "fake", "fake-model", tempDir(t))
	srv1.Close()
	stripOwner(t, filepath.Join(d1.sessionDir(legacy), "meta.json"))
	runDir := filepath.Join(stateDir, "enderbloom", "runs")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	run := `{"id":"run-legacy","spec":"/repo/.scratch/demo/spec.md","repo":"/repo","status":"paused","harness":"fake","model":"m","tickets":[]}`
	if err := os.WriteFile(filepath.Join(runDir, "run-legacy.json"), []byte(run), 0o644); err != nil {
		t.Fatal(err)
	}

	d2 := NewDaemon(stateDir, NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d2))
	defer srv.Close()
	owned := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))

	if listed := getSessions(t, srv.URL); len(listed) != 1 || listed[0].ID != owned {
		t.Fatalf("session list = %+v, want only %s", listed, owned)
	}
	if runs := listRunsJSON(t, srv.URL); len(runs) != 0 {
		t.Fatalf("run list = %+v, want none", runs)
	}
	for _, c := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/sessions/" + legacy, ""},
		{http.MethodPost, "/api/sessions/" + legacy + "/prompt", `{"text":"hi"}`},
		{http.MethodPost, "/api/sessions/" + legacy + "/interrupt", ""},
		{http.MethodGet, "/api/sessions/" + legacy + "/events", ""},
		{http.MethodDelete, "/api/sessions/" + legacy, ""},
		{http.MethodGet, "/api/runs/run-legacy", ""},
		{http.MethodGet, "/api/runs/run-legacy/events", ""},
	} {
		if code := statusOf(t, c.method, srv.URL+c.path, c.body); code != http.StatusNotFound {
			t.Fatalf("%s %s = %d, want 404", c.method, c.path, code)
		}
	}
	d2.mu.Lock()
	ended := d2.sessions[legacy].meta.Status == "ended"
	d2.mu.Unlock()
	if ended {
		t.Fatal("hidden session was ended by a refused DELETE")
	}
}

// Any signed-in User prompts, interrupts and ends any Session: no ACLs.
func TestAnyUserCanPromptInterruptAndEndAnySession(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	h := d.Handler()
	admin := signedIn(t, h, "admin")
	bob := signedIn(t, h, "bob")
	asAdmin := httptest.NewServer(withCookie(h, admin))
	defer asAdmin.Close()
	asBob := httptest.NewServer(withCookie(h, bob))
	defer asBob.Close()

	id := createSession(t, asAdmin.URL, "fake", "fake-model", tempDir(t))
	sendPrompt(t, asBob.URL, id, "from bob")
	settle(t, asAdmin.URL, id)
	if code := statusOf(t, http.MethodPost, asBob.URL+"/api/sessions/"+id+"/interrupt", ""); code != http.StatusOK {
		t.Fatalf("bob interrupt = %d, want 200", code)
	}
	if code := statusOf(t, http.MethodDelete, asBob.URL+"/api/sessions/"+id, ""); code != http.StatusOK {
		t.Fatalf("bob end = %d, want 200", code)
	}
	if got := metaOf(t, asAdmin.URL, id).Status; got != "ended" {
		t.Fatalf("status after bob's end = %q, want ended", got)
	}
}

// A prompt, steer or follow-up is typed by its request: a signed-in User's
// message is "user" with that User's ID in any mode; a machine caller's
// message is "system".
func TestPromptSteerAndFollowupCarrySender(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	h := d.Handler()
	admin := signedIn(t, h, "admin")
	adminID := userOf(t, h, admin).ID
	bob := signedIn(t, h, "bob")
	bobID := userOf(t, h, bob).ID
	asAdmin := httptest.NewServer(withCookie(h, admin))
	defer asAdmin.Close()
	asBob := httptest.NewServer(withCookie(h, bob))
	defer asBob.Close()
	asAgent := httptest.NewServer(withBearer(h, d.MachineToken()))
	defer asAgent.Close()

	id := createSession(t, asAdmin.URL, "fake", "fake-model", tempDir(t))
	sendPrompt(t, asAdmin.URL, id, "admin prompt")
	settle(t, asAdmin.URL, id)
	postMode(t, asAdmin.URL, id, "steer", "admin steer")
	settle(t, asAdmin.URL, id)
	postMode(t, asAdmin.URL, id, "followup", "admin followup")
	settle(t, asAdmin.URL, id)
	postMode(t, asAgent.URL, id, "steer", "agent steer")
	settle(t, asAdmin.URL, id)
	sendPrompt(t, asBob.URL, id, "bob prompt")
	evs := settle(t, asAdmin.URL, id)

	wantSender(t, promptNamed(t, evs, "admin prompt"), "user", adminID)
	wantSender(t, promptNamed(t, evs, "admin steer"), "user", adminID)
	wantSender(t, promptNamed(t, evs, "admin followup"), "user", adminID)
	wantSender(t, promptNamed(t, evs, "agent steer"), "system", "")
	wantSender(t, promptNamed(t, evs, "bob prompt"), "user", bobID)
	if got := promptNamed(t, evs, "admin steer").Mode; got != "steer" {
		t.Fatalf("admin steer mode = %q, want steer", got)
	}
	if got := promptNamed(t, evs, "admin followup").Mode; got != "followup" {
		t.Fatalf("admin followup mode = %q, want followup", got)
	}
	for _, e := range eventsOfKind(evs, "prompt") {
		if e.Sender == "" {
			t.Fatalf("prompt without sender: %+v", e)
		}
	}
}

// Messages queued behind a turn keep their own sender: a delivered prompt
// Event never joins one User's text with another's.
func TestQueuedMessagesNeverMixSenders(t *testing.T) {
	stub := &stubConn{blockPrompt: make(chan struct{})}
	be := newStubClaudeBackend(func() (acpConn, error) { return stub, nil })
	d := NewDaemon(tempDir(t), NewFakeBackend()).WithClaudeBackend(be)
	h := d.Handler()
	admin := signedIn(t, h, "admin")
	adminID := userOf(t, h, admin).ID
	bob := signedIn(t, h, "bob")
	bobID := userOf(t, h, bob).ID
	asAdmin := httptest.NewServer(withCookie(h, admin))
	defer asAdmin.Close()
	asBob := httptest.NewServer(withCookie(h, bob))
	defer asBob.Close()

	id := createSession(t, asAdmin.URL, "claude", "sonnet", tempDir(t))
	sendPrompt(t, asAdmin.URL, id, "slow job")
	waitStatus(t, asAdmin.URL, id, "working", 5*time.Second)
	postMode(t, asBob.URL, id, "followup", "bob one")
	postMode(t, asAdmin.URL, id, "followup", "admin two")
	postMode(t, asBob.URL, id, "followup", "bob three")
	if q := getQueue(t, asAdmin.URL, id); len(q) != 3 {
		t.Fatalf("queue = %+v, want 3 entries before delivery", q)
	}
	close(stub.blockPrompt)
	// Every queued message reaches the adapter once its prompt Event is logged;
	// wait for all four before reading the transcript.
	waitUntil(t, "four adapter prompts", func() bool {
		stub.mu.Lock()
		defer stub.mu.Unlock()
		return len(stub.prompts) == 4
	})
	evs := settle(t, asAdmin.URL, id)

	wantSender(t, promptNamed(t, evs, "bob one"), "user", bobID)
	wantSender(t, promptNamed(t, evs, "admin two"), "user", adminID)
	wantSender(t, promptNamed(t, evs, "bob three"), "user", bobID)
	stub.mu.Lock()
	prompts := append([]string{}, stub.prompts...)
	stub.mu.Unlock()
	if got, want := strings.Join(prompts, "|"), "slow job|bob one|admin two|bob three"; got != want {
		t.Fatalf("adapter prompts = %q, want %q", got, want)
	}
}

// A create with no Project and no folder opens in the User's home directory,
// which is made on first use under the user store.
func TestSessionWithNoProjectOpensInUserHome(t *testing.T) {
	stateDir := tempDir(t)
	d := NewDaemon(stateDir, NewFakeBackend())
	h := d.Handler()
	signedIn(t, h, "admin")
	bob := signedIn(t, h, "bob")
	srv := httptest.NewServer(withCookie(h, bob))
	defer srv.Close()

	home := filepath.Join(stateDir, "enderbloom", "users", "bob", "home")
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatalf("home exists before first use: %v", err)
	}
	raw, _ := json.Marshal(map[string]string{"harness": "fake", "model": "fake-model"})
	resp, err := http.Post(srv.URL+"/api/sessions", "application/json", strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create without folder = %d, want 201", resp.StatusCode)
	}
	var made SessionMeta
	if err := json.NewDecoder(resp.Body).Decode(&made); err != nil {
		t.Fatal(err)
	}
	if made.WorkDir != absWorkDir(home) || made.ProjectID != "" {
		t.Fatalf("workDir = %q projectId = %q, want home %q", made.WorkDir, made.ProjectID, home)
	}
	if info, err := os.Stat(home); err != nil || !info.IsDir() {
		t.Fatalf("home not created on first use: %v", err)
	}

	// An explicit folder still sets the working directory.
	folder := tempDir(t)
	id := createSession(t, srv.URL, "fake", "fake-model", folder)
	if got := metaOf(t, srv.URL, id).WorkDir; got != absWorkDir(folder) {
		t.Fatalf("explicit workDir = %q, want %q", got, folder)
	}
}
