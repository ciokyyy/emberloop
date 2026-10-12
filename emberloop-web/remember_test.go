package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"example.com/emberloop-web/memory"
)

// rememberDaemon starts a Daemon on the fake Backend with one admin Session,
// like handleDaemon but returning the pieces the remember tests need.
func rememberDaemon(t *testing.T) (*Daemon, string, string, string) {
	t.Helper()
	settingsPath := useSettings(t, `{"compress_model":"test-faux/faux-1"}`)
	seen := &promptsSeen{}
	be := NewFakeBackend()
	be.ReplyFunc = seen.reply
	d := NewDaemon(tempDir(t), be)
	d.SetSettingsPath(settingsPath)
	d.summarizer = &scriptedSummarizer{replies: []string{goodSummary, goodSummary, goodSummary, goodSummary}}
	srv := httptest.NewServer(authedHandler(t, d))
	t.Cleanup(srv.Close)
	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	meta, _ := sessionMeta(t, srv.URL, id)
	return d, srv.URL, id, meta.Owner
}

func postRemember(t *testing.T, base, id string, body any) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	resp, err := http.Post(base+"/api/sessions/"+id+"/remember", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func postRememberRaw(t *testing.T, base, id, raw string) int {
	t.Helper()
	resp, err := http.Post(base+"/api/sessions/"+id+"/remember", "application/json", strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}
func getRecall(t *testing.T, base, id, query string) (int, string, []map[string]any) {
	t.Helper()
	resp, err := http.Get(base + "/api/sessions/" + id + "/recall?query=" + url.QueryEscape(query))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Results []map[string]any `json:"results"`
		Text    string           `json:"text"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out.Text, out.Results
}

func ownerBank(t *testing.T, d *Daemon, owner string) memory.Memory {
	t.Helper()
	bank, err := d.profileBankFor(owner)
	if err != nil {
		t.Fatal(err)
	}
	return bank
}

func queueRules(t *testing.T, d *Daemon, owner string) []memory.QueueItem {
	t.Helper()
	items, err := ownerBank(t, d, owner).Queue(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var out []memory.QueueItem
	for _, it := range items {
		if it.Kind == memory.QueueRule {
			out = append(out, it)
		}
	}
	return out
}

// An agent remember always creates a queue item in the caller's User bank
// with reason and quote, and never a Rule.
func TestAgentRememberCreatesQueueItemNeverRule(t *testing.T) {
	d, base, id, owner := rememberDaemon(t)
	runPrompt(t, base, id, "the team uses tabs for indentation here")
	code, out := postRemember(t, base, id, map[string]any{
		"text": "use tabs for indentation", "reason": "the user prefers tabs", "quote": "uses tabs for indentation",
	})
	if code != http.StatusCreated {
		t.Fatalf("remember = %d %v, want 201", code, out)
	}
	items := queueRules(t, d, owner)
	if len(items) != 1 {
		t.Fatalf("queue rules = %d, want 1", len(items))
	}
	var payload rememberQueuePayload
	if err := json.Unmarshal([]byte(items[0].Payload), &payload); err != nil {
		t.Fatalf("payload decodes: %v", err)
	}
	if payload.Text != "use tabs for indentation" || payload.Reason != "the user prefers tabs" || payload.Quote != "uses tabs for indentation" {
		t.Fatalf("payload = %+v, want text, reason and quote", payload)
	}
	if payload.SessionID != id || payload.Origin != "remember" {
		t.Fatalf("payload = %+v, want the session and the remember origin", payload)
	}
	if got, err := ownerBank(t, d, owner).Recall(context.Background(), "tabs indentation", 10); err != nil || len(got) != 0 {
		t.Fatalf("recall = %+v %v, want no Rule from a tool propose", got, err)
	}
}

// Remember needs a reason and grounded evidence: missing fields, an
// ungrounded quote and over-long text are all refused.
func TestAgentRememberNeedsReasonAndGrounding(t *testing.T) {
	_, base, id, _ := rememberDaemon(t)
	runPrompt(t, base, id, "keep the build green at all times")
	if code, _ := postRemember(t, base, id, map[string]any{"text": "keep green"}); code != http.StatusBadRequest {
		t.Fatalf("remember without reason = %d, want 400", code)
	}
	if code, _ := postRemember(t, base, id, map[string]any{"reason": "why"}); code != http.StatusBadRequest {
		t.Fatalf("remember without text = %d, want 400", code)
	}
	if code, _ := postRemember(t, base, id, map[string]any{
		"text": "something else", "reason": "r", "quote": "words nobody typed",
	}); code != http.StatusBadRequest {
		t.Fatalf("remember with an ungrounded quote = %d, want 400", code)
	}
	if code := postRememberRaw(t, base, id, `{"text":`); code != http.StatusBadRequest {
		t.Fatalf("remember with bad json = %d, want 400", code)
	}
	long := strings.Repeat("x", memory.MaxRuleChars+1)
	if code, _ := postRemember(t, base, id, map[string]any{"text": long, "reason": "r", "quote": "green"}); code != http.StatusBadRequest {
		t.Fatalf("remember over 300 chars = %d, want 400", code)
	}
}

// Past the per-Session limit a remember is refused with an error.
func TestRememberRateLimitRefused(t *testing.T) {
	t.Setenv("EL_REMEMBER_LIMIT", "2")
	d, base, id, owner := rememberDaemon(t)
	runPrompt(t, base, id, "alpha beta gamma")
	if code, _ := postRemember(t, base, id, map[string]any{"text": "alpha beta", "reason": "first"}); code != http.StatusCreated {
		t.Fatalf("first remember = %d, want 201", code)
	}
	runPrompt(t, base, id, "delta epsilon zeta")
	if code, _ := postRemember(t, base, id, map[string]any{"text": "delta epsilon", "reason": "second"}); code != http.StatusCreated {
		t.Fatalf("second remember = %d, want 201", code)
	}
	runPrompt(t, base, id, "eta theta iota")
	if code, out := postRemember(t, base, id, map[string]any{"text": "eta theta", "reason": "third"}); code != http.StatusTooManyRequests {
		t.Fatalf("third remember = %d %v, want 429", code, out)
	}
	if n := len(queueRules(t, d, owner)); n != 2 {
		t.Fatalf("queue rules = %d, want 2", n)
	}
}

// Only top-level Sessions propose Rules: workers, scouts, Run Sessions and
// Reviewers are refused, while every role keeps recall.
func TestRememberRolesRefusedRecallAllowed(t *testing.T) {
	d, base, parent, _ := rememberDaemon(t)
	// The role check needs five live Sessions at once; the default cap is
	// three, and recall adds a beat before each first turn releases, so lift
	// the cap for the test.
	d.maxRunning = 10
	runPrompt(t, base, parent, "parent task uses tabs everywhere")
	worker := spawnOne(t, base, parent, "worker job")
	waitStatus(t, base, worker, "idle", 10*time.Second)
	scout := spawnScout(t, base, parent)
	waitStatus(t, base, scout, "idle", 10*time.Second)
	meta, _ := sessionMeta(t, base, parent)
	runWorker := SessionMeta{
		ID: newID(), Harness: "fake", Model: "fake-model", WorkDir: tempDir(t),
		Status: "idle", CreatedAt: time.Now().UTC(), Owner: meta.Owner,
		ParentID: "run-1", GroupID: "g-1", RunWorker: true,
	}
	if _, serr := d.startSession(runWorker); serr != nil {
		t.Fatalf("run worker start: %s", serr.msg)
	}
	reviewer := SessionMeta{
		ID: newID(), Harness: "fake", Model: "fake-model", WorkDir: tempDir(t),
		Status: "idle", CreatedAt: time.Now().UTC(), Owner: meta.Owner,
		ParentID: "run-1", Reviewer: "reviewer-spec",
	}
	if _, serr := d.startSession(reviewer); serr != nil {
		t.Fatalf("reviewer start: %s", serr.msg)
	}
	remember := map[string]any{"text": "parent task", "reason": "r", "quote": "parent task"}
	if code, _ := postRemember(t, base, parent, remember); code != http.StatusCreated {
		t.Fatalf("top-level remember = %d, want 201", code)
	}
	for name, sid := range map[string]string{"worker": worker, "scout": scout, "run-worker": runWorker.ID, "reviewer": reviewer.ID} {
		if code, _ := postRemember(t, base, sid, map[string]any{"text": "x", "reason": "r"}); code != http.StatusForbidden {
			t.Fatalf("%s remember = %d, want 403", name, code)
		}
		if code, _, _ := getRecall(t, base, sid, "task"); code != http.StatusOK {
			t.Fatalf("%s recall = %d, want 200", name, code)
		}
	}
}

// Scouts and Run Sessions keep no remember across a compress: the role
// derives from the record on every handle, so the Handoff swap cannot add
// one. Scouts cannot compress themselves, so the parent compresses while the
// scout watches; the Run Session compresses itself.
func TestScoutAndRunHaveNoRememberAfterCompress(t *testing.T) {
	d, base, parent, _ := rememberDaemon(t)
	runPrompt(t, base, parent, "parent work before compress")
	scout := spawnScout(t, base, parent)
	waitStatus(t, base, scout, "idle", 10*time.Second)
	meta, _ := sessionMeta(t, base, parent)
	runWorker := SessionMeta{
		ID: newID(), Harness: "fake", Model: "fake-model", WorkDir: tempDir(t),
		Status: "idle", CreatedAt: time.Now().UTC(), Owner: meta.Owner,
		ParentID: "run-9", GroupID: "g-9", RunWorker: true,
	}
	if _, serr := d.startSession(runWorker); serr != nil {
		t.Fatalf("run worker start: %s", serr.msg)
	}
	runPrompt(t, base, runWorker.ID, "run work before compress")
	remember := map[string]any{"text": "x", "reason": "r"}
	if code, _ := postRemember(t, base, scout, remember); code != http.StatusForbidden {
		t.Fatalf("scout remember before compress = %d, want 403", code)
	}
	if code, _ := postRemember(t, base, runWorker.ID, remember); code != http.StatusForbidden {
		t.Fatalf("run remember before compress = %d, want 403", code)
	}
	compressAndSettle(t, base, parent, 1)
	compressAndSettle(t, base, runWorker.ID, 1)
	if code, _ := postRemember(t, base, scout, remember); code != http.StatusForbidden {
		t.Fatalf("scout remember after compress = %d, want 403", code)
	}
	if code, _ := postRemember(t, base, runWorker.ID, remember); code != http.StatusForbidden {
		t.Fatalf("run remember after compress = %d, want 403", code)
	}
	for _, tc := range []struct {
		name string
		meta SessionMeta
		role string
	}{
		{"scout", SessionMeta{Owner: "u1", GroupID: "g", Scout: true}, "scout"},
		{"run", SessionMeta{Owner: "u1", GroupID: "g", RunWorker: true}, "run-worker"},
	} {
		if got := elRoleFor(tc.meta); got != tc.role {
			t.Fatalf("%s role = %q, want %q", tc.name, got, tc.role)
		}
	}
}

// A structural /remember on the first line of a typed message creates a Rule
// directly after grounding, with no queue item.
func TestSlashRememberCreatesRuleDirectly(t *testing.T) {
	d, base, id, owner := rememberDaemon(t)
	body, _ := json.Marshal(map[string]string{"text": "/remember keep replies short"})
	resp, err := http.Post(base+"/api/sessions/"+id+"/prompt", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("slash remember status = %d, want 201", resp.StatusCode)
	}
	var out struct {
		Seq    int64  `json:"seq"`
		RuleID string `json:"ruleId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || out.RuleID == "" {
		t.Fatalf("slash remember response = %+v %v, want a ruleId", out, err)
	}
	rules, err := ownerBank(t, d, owner).Recall(context.Background(), "replies short", 10)
	if err != nil || len(rules) != 1 || rules[0].Text != "keep replies short" {
		t.Fatalf("rules = %+v %v, want the direct Rule", rules, err)
	}
	if rules[0].ApprovedBy != owner {
		t.Fatalf("rule approved by %q, want the sender %q", rules[0].ApprovedBy, owner)
	}
	if n := len(queueRules(t, d, owner)); n != 0 {
		t.Fatalf("queue rules = %d, want none for a direct Rule", n)
	}
	found := false
	for _, e := range transcriptOf(t, base, id) {
		if e.Kind == "prompt" && strings.Contains(e.Text, "/remember keep replies short") {
			found = true
			if e.Sender != senderUser {
				t.Fatalf("command event sender = %q, want user", e.Sender)
			}
		}
	}
	if !found {
		t.Fatalf("no /remember prompt Event in %+v", transcriptOf(t, base, id))
	}
}

// The same words anywhere else create nothing: pasted blocks (a later line),
// colon forms, and system messages all route as normal prompts.
func TestSlashRememberOnlyFirstLineTyped(t *testing.T) {
	d, base, id, owner := rememberDaemon(t)
	runPrompt(t, base, id, "remember: use tabs in this repo")
	runPrompt(t, base, id, "context first\n/remember use tabs")
	if rules, _ := ownerBank(t, d, owner).Recall(context.Background(), "tabs", 10); len(rules) != 0 {
		t.Fatalf("pasted rules = %+v, want none", rules)
	}
	if n := len(queueRules(t, d, owner)); n != 0 {
		t.Fatalf("queue rules = %d, want none for pasted text", n)
	}
	// A system message (machine token, as agents and tool output send) with
	// the slash on the first line is not a typed action either.
	h := d.Handler()
	agent := httptest.NewServer(withBearer(h, d.MachineToken()))
	defer agent.Close()
	body, _ := json.Marshal(map[string]string{"text": "/remember agent says tabs"})
	req, _ := http.NewRequest("POST", agent.URL+"/api/sessions/"+id+"/prompt", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+d.MachineToken())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if rules, _ := ownerBank(t, d, owner).Recall(context.Background(), "agent says", 10); len(rules) != 0 {
		t.Fatalf("system rules = %+v, want none", rules)
	}
}

// Recall never returns another User's observations or Rules: each bank stays
// private to its User.
func TestRecallNeverCrossesUsers(t *testing.T) {
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
	workA, workB := tempDir(t), tempDir(t)
	adminSession := createSession(t, asAdmin.URL, "fake", "fake-model", workA)
	bobSession := createSession(t, asBob.URL, "fake", "fake-model", workB)
	runPrompt(t, asAdmin.URL, adminSession, "admin keeps tabs for indentation")
	runPrompt(t, asBob.URL, bobSession, "bob keeps spaces for indentation")
	if _, err := ownerBank(t, d, adminID).AddObservation(context.Background(), memory.Observation{
		SessionID: adminSession, EventID: "1", Author: adminID, Quote: "admin tabs observation", Kind: memory.KindFact,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ownerBank(t, d, bobID).AddObservation(context.Background(), memory.Observation{
		SessionID: bobSession, EventID: "1", Author: bobID, Quote: "bob spaces observation", Kind: memory.KindFact,
	}); err != nil {
		t.Fatal(err)
	}
	// Rules cross the same boundary: one direct Rule per bank on disjoint
	// vocabulary, so an OR query cannot match both.
	for _, tc := range []struct{ base, sid, text string }{
		{asAdmin.URL, adminSession, "/remember admin tabs decree"},
		{asBob.URL, bobSession, "/remember bob spaces edict"},
	} {
		body, _ := json.Marshal(map[string]string{"text": tc.text})
		resp, err := http.Post(tc.base+"/api/sessions/"+tc.sid+"/prompt", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("slash remember %q = %d, want 201", tc.text, resp.StatusCode)
		}
	}
	for _, tc := range []struct {
		base, sid, want, notWant string
	}{
		{asAdmin.URL, adminSession, "admin tabs observation", "bob spaces observation"},
		{asBob.URL, bobSession, "bob spaces observation", "admin tabs observation"},
	} {
		if code, text, _ := getRecall(t, tc.base, tc.sid, "observation"); code != http.StatusOK {
			t.Fatalf("recall = %d, want 200", code)
		} else if !strings.Contains(text, tc.want) || strings.Contains(text, tc.notWant) {
			t.Fatalf("recall = %q, want %q without %q", text, tc.want, tc.notWant)
		}
	}
	for _, tc := range []struct {
		base, sid, query, want, notWant string
	}{
		{asAdmin.URL, adminSession, "tabs decree", "admin tabs decree", "bob spaces edict"},
		{asBob.URL, bobSession, "spaces edict", "bob spaces edict", "admin tabs decree"},
	} {
		if code, text, _ := getRecall(t, tc.base, tc.sid, tc.query); code != http.StatusOK {
			t.Fatalf("recall = %d, want 200", code)
		} else if !strings.Contains(text, tc.want) || strings.Contains(text, tc.notWant) {
			t.Fatalf("recall %q = %q, want %q without %q", tc.query, text, tc.want, tc.notWant)
		}
	}
}

// Recall searches Project notes beside the User bank.
func TestRecallIncludesProjectNotes(t *testing.T) {
	_, base, id, _ := rememberDaemon(t)
	meta, _ := sessionMeta(t, base, id)
	notesDir := filepath.Join(meta.WorkDir, ".emberloop", "notes")
	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	note := "---\nid: upload-note\n---\nUse the upload handler for uploads.\n"
	if err := os.WriteFile(filepath.Join(notesDir, "upload.md"), []byte(note), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, text, _ := getRecall(t, base, id, "upload handler"); code != http.StatusOK {
		t.Fatalf("recall = %d, want 200", code)
	} else if !strings.Contains(text, "upload handler") {
		t.Fatalf("recall = %q, want the project note", text)
	}
}

// The pig loadout gates memory tools by role: top-level Sessions propose and
// search, everyone else searches only; unwired backends serve neither.
func TestPigMemoryToolGating(t *testing.T) {
	pb := NewPigBackend()
	pb.Remember = func(string, string, string, string) (int64, error) { return 1, nil }
	pb.Recall = func(string, string) (string, error) { return "rules", nil }
	names := func(meta SessionMeta) map[string]bool {
		out := map[string]bool{}
		for _, tl := range pb.sessionTools(meta) {
			out[tl.Name()] = true
		}
		return out
	}
	if got := names(SessionMeta{ID: "top"}); !got["remember"] || !got["recall"] {
		t.Fatalf("top-level tools = %v, want remember and recall", got)
	}
	for _, meta := range []SessionMeta{
		{ID: "w", GroupID: "g"},
		{ID: "s", Scout: true},
		{ID: "r", GroupID: "g", RunWorker: true},
		{ID: "rev", ParentID: "run-1", Reviewer: "reviewer-spec"},
	} {
		if got := names(meta); got["remember"] || !got["recall"] {
			t.Fatalf("role %+v tools = %v, want recall only", meta, got)
		}
	}
	plain := NewPigBackend()
	for _, tl := range plain.sessionTools(SessionMeta{ID: "top"}) {
		if tl.Name() == "remember" || tl.Name() == "recall" {
			t.Fatalf("unwired backend serves %q", tl.Name())
		}
	}
}

// A quote from User B's steer inside A's Session lands in B's bank,
// never A's: candidates follow the grounded Event's sender.
func TestRememberCrossUserSteerStaysInSenderBank(t *testing.T) {
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
	sess := createSession(t, asAdmin.URL, "fake", "fake-model", tempDir(t))
	body, _ := json.Marshal(map[string]string{"text": "bob prefers spaces for indentation"})
	resp, err := http.Post(asBob.URL+"/api/sessions/"+sess+"/steer", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("bob steer = %d, want 202", resp.StatusCode)
	}
	if code, _ := postRemember(t, asAdmin.URL, sess, map[string]any{
		"text": "bob spaces note", "reason": "r", "quote": "prefers spaces for indentation",
	}); code != http.StatusCreated {
		t.Fatalf("remember = %d, want 201", code)
	}
	if n := len(queueRules(t, d, adminID)); n != 0 {
		t.Fatalf("owner bank queue rules = %d, want none for B's words", n)
	}
	items := queueRules(t, d, bobID)
	if len(items) != 1 {
		t.Fatalf("sender bank queue rules = %d, want 1", len(items))
	}
	var payload rememberQueuePayload
	if err := json.Unmarshal([]byte(items[0].Payload), &payload); err != nil || payload.SessionID != sess {
		t.Fatalf("payload = %+v %v, want the session", payload, err)
	}
}

// The structural command acts on every typed path: steer and follow-up
// create Rules directly, just like prompt. All three route through
// sendMessage, so one hook covers them.
func TestSlashRememberSteerAndFollowupCreateRules(t *testing.T) {
	d, base, id, owner := rememberDaemon(t)
	for _, tc := range []struct{ sub, text, want string }{
		{"steer", "/remember steer rule text", "steer rule text"},
		{"followup", "/remember followup rule text", "followup rule text"},
	} {
		body, _ := json.Marshal(map[string]string{"text": tc.text})
		resp, err := http.Post(base+"/api/sessions/"+id+"/"+tc.sub, "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		var out struct {
			RuleID string `json:"ruleId"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		resp.Body.Close()
		if resp.StatusCode != http.StatusCreated || out.RuleID == "" {
			t.Fatalf("%s %s = %d %+v, want 201 with a ruleId", tc.sub, tc.text, resp.StatusCode, out)
		}
	}
	for _, tc := range []struct{ query, want string }{
		{"steer", "steer rule text"},
		{"followup", "followup rule text"},
	} {
		if got, err := ownerBank(t, d, owner).Recall(context.Background(), tc.query, 10); err != nil || len(got) != 1 || got[0].Text != tc.want {
			t.Fatalf("recall %q = %+v %v, want the direct Rule", tc.query, got, err)
		}
	}
}

// A rejected command logs nothing: an empty /remember and an over-long
// one answer 400 with no prompt Event behind them.
func TestSlashRememberInvalidLogsNothing(t *testing.T) {
	_, base, id, _ := rememberDaemon(t)
	for _, text := range []string{"/remember", "/remember   ", "/remember " + strings.Repeat("x", memory.MaxRuleChars+1)} {
		body, _ := json.Marshal(map[string]string{"text": text})
		resp, err := http.Post(base+"/api/sessions/"+id+"/prompt", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("prompt %q = %d, want 400", text[:min(len(text), 20)], resp.StatusCode)
		}
	}
	for _, e := range transcriptOf(t, base, id) {
		if e.Kind == "prompt" {
			t.Fatalf("transcript holds a prompt Event for a rejected command: %+v", e)
		}
	}
}

// Leftover lines after the /remember first line still route as the turn:
// the Rule is created and the agent sees only the remainder.
func TestSlashRememberWithRemainderRoutesTurn(t *testing.T) {
	settingsPath := useSettings(t, `{"compress_model":"test-faux/faux-1"}`)
	seen := &promptsSeen{}
	be := NewFakeBackend()
	be.ReplyFunc = seen.reply
	d := NewDaemon(tempDir(t), be)
	d.SetSettingsPath(settingsPath)
	d.summarizer = &scriptedSummarizer{replies: []string{goodSummary, goodSummary, goodSummary, goodSummary}}
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	base := srv.URL
	id := createSession(t, base, "fake", "fake-model", tempDir(t))
	meta, _ := sessionMeta(t, base, id)
	body, _ := json.Marshal(map[string]string{"text": "/remember keep replies short\nnow fix the build"})
	resp, err := http.Post(base+"/api/sessions/"+id+"/prompt", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		RuleID string `json:"ruleId"`
		Seq    int64  `json:"seq"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted || out.RuleID == "" {
		t.Fatalf("prompt = %d %+v, want 202 with a ruleId", resp.StatusCode, out)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		evs := transcriptOf(t, base, id)
		n := len(evs)
		if n > 0 && evs[n-1].Kind == "status" && evs[n-1].Status == "idle" && promptIndex(evs, "now fix the build") >= 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("remainder turn did not settle: %+v", evs)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got, err := ownerBank(t, d, meta.Owner).Recall(context.Background(), "replies short", 10); err != nil || len(got) != 1 {
		t.Fatalf("rules = %+v %v, want the direct Rule", got, err)
	}
	got := seen.all()
	if len(got) == 0 || got[len(got)-1] != "now fix the build" {
		t.Fatalf("backend prompts = %q, want only the remainder", got)
	}
}

// Concurrent agent calls cannot exceed the limit together: the check and
// the enqueue are one critical section.
func TestRememberLimitHoldsUnderConcurrency(t *testing.T) {
	t.Setenv("EL_REMEMBER_LIMIT", "3")
	d, base, id, owner := rememberDaemon(t)
	runPrompt(t, base, id, "alpha beta gamma delta")
	var wg sync.WaitGroup
	created := make([]bool, 10)
	for i := range created {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := d.RememberFromSession(id, "concurrent note", "reason", "alpha"); err == nil {
				created[i] = true
			}
		}(i)
	}
	wg.Wait()
	n := 0
	for _, ok := range created {
		if ok {
			n++
		}
	}
	if n != 3 {
		t.Fatalf("concurrent creates = %d, want exactly the limit 3", n)
	}
	if got := len(queueRules(t, d, owner)); got != 3 {
		t.Fatalf("queue rules = %d, want 3", got)
	}
}

// Recall formatting renders Rules and notes with verify flags, appends
// observations, and drops a failing observation search while keeping the
// Rule and note hits.
func TestFormatRecallResults(t *testing.T) {
	flagged := memory.VerifyResult{Status: memory.VerifyChanged, CommitDate: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)}
	hits := []memory.MergedHit{
		{Source: memory.MergeRule, Rule: memory.Rule{RuleID: "r1", Text: "use tabs"}, RuleResult: memory.VerifyResult{Status: memory.VerifyClean}},
		{Source: memory.MergeNote, Note: memory.Note{ID: "n1", Text: "upload note"}, NoteResult: flagged},
	}
	obs := []memory.Observation{{ID: 9, Quote: "tabs observation"}}
	text, out, err := formatRecallResults(hits, obs, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "rule: use tabs") || !strings.Contains(text, "note n1: upload note (code changed since 2026-10-02, verify)") || !strings.Contains(text, "observation: tabs observation") {
		t.Fatalf("formatted = %q", text)
	}
	if len(out) != 3 || out[0].Source != "rule" || out[1].Source != "note" || out[2].Source != "observation" {
		t.Fatalf("results = %+v, want rule, note, observation", out)
	}
	text, out, err = formatRecallResults(hits, nil, errors.New("index gone"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "observation:") || len(out) != 2 {
		t.Fatalf("on observation failure = %q %+v, want only the Rule and note hits", text, out)
	}
}
