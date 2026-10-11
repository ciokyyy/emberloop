package web

// Invariant tests (ticket 31): one file pinning the spec Design rules so a
// later change cannot break them silently. Each test maps to a checkbox in
// .scratch/web-v1/issues/31-invariant-tests.md and runs at the Daemon HTTP
// seam (fake Backend) or against the bank file, asserting external behavior.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"example.com/emberloop-web/memory"
	_ "modernc.org/sqlite"
)

// invariantTables are the bank tables whose DELETE must abort outside purge.
var invariantTables = []string{"observations", "rules", "queue", "profile_versions", "ledger"}

// TestInvariant_NoDeleteOutsidePurge pins "Never delete": every DELETE outside
// the human purge aborts with "use purge", and no non-test Go file outside the
// purge and schema owns a DELETE.
func TestInvariant_NoDeleteOutsidePurge(t *testing.T) {
	// Static: no DELETE FROM outside purge.go and schema.go.
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if path == filepath.Join("memory", "purge.go") || path == filepath.Join("memory", "schema.go") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(raw), "DELETE FROM") {
			t.Errorf("%s owns a DELETE outside purge", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Dynamic: every table refuses DELETE with the purge hint and keeps its row.
	d := NewDaemon(tempDir(t), NewFakeBackend())
	h := d.Handler()
	adminID := userOf(t, h, signedIn(t, h, "admin")).ID
	bank, err := d.profileBankFor(adminID)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := bank.AddObservation(ctx, memory.Observation{SessionID: "inv", EventID: "1", Author: adminID, Quote: "invariant seed quote", Kind: memory.KindFact}); err != nil {
		t.Fatal(err)
	}
	if _, err := bank.AddRule(ctx, memory.Rule{Text: "invariant seed rule", Kind: memory.KindFact, Status: memory.StatusActive, ApprovedBy: adminID}); err != nil {
		t.Fatal(err)
	}
	if _, err := bank.Enqueue(ctx, memory.QueueItem{Kind: memory.QueueRule, Payload: `{}`}); err != nil {
		t.Fatal(err)
	}
	if _, err := bank.AppendProfileVersion(ctx, memory.ProfilePatch{Ops: []memory.ProfileOp{{Op: memory.ProfileAdd, Text: "invariant profile line", Tag: memory.ProfileWorkflow}}}, adminID); err != nil {
		t.Fatal(err)
	}
	if _, err := bank.RecordLedger(ctx, memory.LedgerRow{Stage: "mine", Status: memory.LedgerSuccess}); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(d.memoryRoot(), "users", adminID+".db")
	eng, err := d.memoryEngine()
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	for _, table := range invariantTables {
		if _, err := raw.Exec("DELETE FROM " + table); err == nil || !strings.Contains(err.Error(), "use purge") {
			t.Errorf("DELETE FROM %s err = %v, want abort with %q", table, err, "use purge")
		}
		var n int
		if err := raw.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			t.Errorf("%s lost its row to a refused DELETE", table)
		}
	}
}

// TestInvariant_EveryRuleGroundedInUserEvent pins "Grounding": every Rule the
// Daemon creates cites a sender=user Event whose quote verifies, and anything
// else is rejected without a model.
func TestInvariant_EveryRuleGroundedInUserEvent(t *testing.T) {
	d, base, id, owner := rememberDaemon(t)
	runPrompt(t, base, id, "the team always wraps errors with context in this repo")
	body, _ := json.Marshal(map[string]string{"text": "/remember always wrap errors with context"})
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
	if resp.StatusCode != http.StatusCreated || out.RuleID == "" {
		t.Fatalf("slash remember = %d %+v, want 201 with a ruleId", resp.StatusCode, out)
	}
	rules, err := ownerBank(t, d, owner).Recall(context.Background(), "wrap errors", 10)
	if err != nil || len(rules) != 1 || rules[0].RuleID != out.RuleID {
		t.Fatalf("recall = %+v %v, want the direct Rule %s", rules, err, out.RuleID)
	}
	evs := transcriptOf(t, base, id)
	var ground *Event
	for i := range evs {
		if evs[i].Kind == "prompt" && evs[i].Sender == senderUser && strings.Contains(evs[i].Text, rules[0].Text) {
			ground = &evs[i]
			break
		}
	}
	if ground == nil {
		t.Fatalf("no sender=user Event holds Rule %q in %+v", rules[0].Text, evs)
	}
	if err := memory.CheckGrounding("evt", rules[0].Text, memory.GroundEvent{Found: true, Sender: ground.Sender, Text: ground.Text}); err != nil {
		t.Fatalf("Rule is not grounded in its Event: %v", err)
	}
	// The verifier itself rejects the three non-grounded shapes.
	if err := memory.CheckGrounding("1", "wrap", memory.GroundEvent{}); err == nil {
		t.Error("CheckGrounding accepted a missing Event")
	}
	if err := memory.CheckGrounding("1", "wrap errors", memory.GroundEvent{Found: true, Sender: senderSystem, Text: "wrap errors here"}); err == nil {
		t.Error("CheckGrounding accepted a sender=system Event")
	}
	if err := memory.CheckGrounding("1", "words nobody typed", memory.GroundEvent{Found: true, Sender: senderUser, Text: "the team wraps errors"}); err == nil {
		t.Error("CheckGrounding accepted a non-substring quote")
	}
	// An ungrounded remember proposal is refused without a model call.
	if code, _ := postRemember(t, base, id, map[string]any{"text": "something else", "reason": "r", "quote": "words nobody typed"}); code != http.StatusBadRequest {
		t.Fatalf("ungrounded remember = %d, want 400", code)
	}
}

// TestInvariant_PrivateNotesNeverLeak pins "Private by default": User A's
// Rules, observations, Profile and queue never appear in B's recall,
// injection or Memory page.
func TestInvariant_PrivateNotesNeverLeak(t *testing.T) {
	t.Setenv("PIG_TEST_FAUX", "1")
	useSettings(t, `{"compress_model":"test-faux/faux-1"}`)
	seen := &promptsSeen{}
	be := NewFakeBackend()
	be.ReplyFunc = seen.reply
	d := NewDaemon(tempDir(t), be)
	d.summarizer = &scriptedSummarizer{replies: []string{goodSummary, goodSummary, goodSummary, goodSummary}}
	h := d.Handler()
	admin := signedIn(t, h, "admin")
	adminID := userOf(t, h, admin).ID
	bob := signedIn(t, h, "bob")
	bobID := userOf(t, h, bob).ID
	asAdmin := httptest.NewServer(withCookie(h, admin))
	defer asAdmin.Close()
	asBob := httptest.NewServer(withCookie(h, bob))
	defer asBob.Close()
	adminSession := createSession(t, asAdmin.URL, "fake", "fake-model", tempDir(t))
	bobSession := createSession(t, asBob.URL, "fake", "fake-model", tempDir(t))

	for _, tc := range []struct{ base, sid, text string }{
		{asAdmin.URL, adminSession, "/remember admin tabs decree for indentation"},
		{asBob.URL, bobSession, "/remember bob spaces edict for indentation"},
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
	ctx := context.Background()
	if _, err := ownerBank(t, d, adminID).AddObservation(ctx, memory.Observation{SessionID: adminSession, EventID: "1", Author: adminID, Quote: "admin tabs observation", Kind: memory.KindFact}); err != nil {
		t.Fatal(err)
	}
	if _, err := ownerBank(t, d, bobID).AddObservation(ctx, memory.Observation{SessionID: bobSession, EventID: "1", Author: bobID, Quote: "bob spaces observation", Kind: memory.KindFact}); err != nil {
		t.Fatal(err)
	}
	putAs := func(base, line string) {
		t.Helper()
		raw, _ := json.Marshal(map[string]any{"ops": []memory.ProfileOp{{Op: memory.ProfileAdd, Text: line, Tag: memory.ProfileWorkflow}}})
		req, _ := http.NewRequest("PUT", base+"/api/profile", bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("PUT profile %q = %d", line, resp.StatusCode)
		}
	}
	putAs(asAdmin.URL, "admin profile alpha line")
	putAs(asBob.URL, "bob profile beta line")

	// Recall stays in the caller's bank.
	for _, tc := range []struct{ base, sid, want, notWant string }{
		{asAdmin.URL, adminSession, "admin tabs decree", "bob spaces edict"},
		{asBob.URL, bobSession, "bob spaces edict", "admin tabs decree"},
	} {
		if code, text, _ := getRecall(t, tc.base, tc.sid, "decree edict"); code != http.StatusOK {
			t.Fatalf("recall = %d, want 200", code)
		} else if !strings.Contains(text, tc.want) || strings.Contains(text, tc.notWant) {
			t.Fatalf("recall = %q, want %q without %q", text, tc.want, tc.notWant)
		}
	}
	for _, tc := range []struct{ base, sid, want, notWant string }{
		{asAdmin.URL, adminSession, "admin tabs observation", "bob spaces observation"},
		{asBob.URL, bobSession, "bob spaces observation", "admin tabs observation"},
	} {
		if code, text, _ := getRecall(t, tc.base, tc.sid, "observation"); code != http.StatusOK {
			t.Fatalf("recall observations = %d, want 200", code)
		} else if !strings.Contains(text, tc.want) || strings.Contains(text, tc.notWant) {
			t.Fatalf("recall observations = %q, want %q without %q", text, tc.want, tc.notWant)
		}
	}

	// Injection carries only the owner's Rule and Profile.
	fresh := createSession(t, asAdmin.URL, "fake", "fake-model", tempDir(t))
	runPrompt(t, asAdmin.URL, fresh, "admin tabs decree task")
	got := seen.all()
	if len(got) == 0 {
		t.Fatal("no backend prompts")
	}
	last := got[len(got)-1]
	if !strings.Contains(last, "admin tabs decree") || strings.Contains(last, "bob spaces edict") {
		t.Fatalf("injected prompt = %q, want admin Rule without bob's", last)
	}
	if !strings.Contains(last, "admin profile alpha line") || strings.Contains(last, "bob profile beta line") {
		t.Fatalf("injected prompt = %q, want admin Profile without bob's", last)
	}
	ies := injectionEventsOf(t, transcriptOf(t, asAdmin.URL, fresh))
	if len(ies) == 0 {
		t.Fatal("no injection Events")
	}
	// The injection Event must resolve to the owner's bank only.
	adminRules, _ := ownerBank(t, d, adminID).Recall(ctx, "decree", 20)
	bobRules, _ := ownerBank(t, d, bobID).Recall(ctx, "edict", 20)
	if len(adminRules) != 1 || len(bobRules) != 1 {
		t.Fatalf("banks hold %d/%d Rules, want 1 each", len(adminRules), len(bobRules))
	}
	lastIE := ies[len(ies)-1]
	foundAdmin, foundBob := false, false
	for _, r := range lastIE.Rules {
		if r.ID == adminRules[0].RuleID {
			foundAdmin = true
		}
		if r.ID == bobRules[0].RuleID {
			foundBob = true
		}
	}
	if !foundAdmin || foundBob {
		t.Fatalf("injection Event %+v, want admin Rule %s without bob %s", lastIE, adminRules[0].RuleID, bobRules[0].RuleID)
	}

	// The Memory page (Profile and queue) is per-User.
	getAs := func(base, path string) string {
		t.Helper()
		resp, err := http.Get(base + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(resp.Body)
		return buf.String()
	}
	if body := getAs(asAdmin.URL, "/api/profile"); !strings.Contains(body, "admin profile alpha line") || strings.Contains(body, "bob profile beta line") {
		t.Fatalf("admin Memory profile = %s, want only admin lines", body)
	}
	if body := getAs(asBob.URL, "/api/profile"); !strings.Contains(body, "bob profile beta line") || strings.Contains(body, "admin profile alpha line") {
		t.Fatalf("bob Memory profile = %s, want only bob lines", body)
	}
	if body := getAs(asAdmin.URL, "/api/profile/queue"); strings.Contains(body, "bob") && strings.Contains(body, "edict") {
		t.Fatalf("admin queue leaks bob: %s", body)
	}
}

// TestInvariant_RecallOnlyRolesHaveNoRemember pins "Roles and tools": scouts,
// spawned workers, Run Sessions and Reviewers get recall only, before and
// after a compress.
func TestInvariant_RecallOnlyRolesHaveNoRemember(t *testing.T) {
	d, base, parent, _ := rememberDaemon(t)
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
	recallOnly := map[string]string{"worker": worker, "scout": scout, "run-worker": runWorker.ID, "reviewer": reviewer.ID}
	for name, sid := range recallOnly {
		if code, _ := postRemember(t, base, sid, map[string]any{"text": "x", "reason": "r"}); code != http.StatusForbidden {
			t.Fatalf("%s remember before compress = %d, want 403", name, code)
		}
		if code, _, _ := getRecall(t, base, sid, "task"); code != http.StatusOK {
			t.Fatalf("%s recall before compress = %d, want 200", name, code)
		}
	}
	// Compress keeps the tool set: the same roles still refuse remember.
	compressAndSettle(t, base, worker, 1)
	compressAndSettle(t, base, runWorker.ID, 1)
	compressAndSettle(t, base, reviewer.ID, 1)
	if code, body := compressReq(t, base, scout, ""); code != http.StatusForbidden {
		t.Fatalf("scout compress = %d %v, want 403", code, body)
	}
	for name, sid := range recallOnly {
		if code, _ := postRemember(t, base, sid, map[string]any{"text": "x", "reason": "r"}); code != http.StatusForbidden {
			t.Fatalf("%s remember after compress = %d, want 403", name, code)
		}
		if code, _, _ := getRecall(t, base, sid, "task"); code != http.StatusOK {
			t.Fatalf("%s recall after compress = %d, want 200", name, code)
		}
	}
	// The top level still proposes after its own compress.
	compressAndSettle(t, base, parent, 1)
	runPrompt(t, base, parent, "parent task again uses tabs")
	if code, _ := postRemember(t, base, parent, remember); code != http.StatusCreated {
		t.Fatalf("top-level remember after compress = %d, want 201", code)
	}
}

// TestInvariant_OwnerToolsetBankBindingsIdenticalAcrossCompress pins "No hidden
// state in a handle": startHandle derives owner, tool set and bank bindings
// from the Session record on every create, resume and compress.
func TestInvariant_OwnerToolsetBankBindingsIdenticalAcrossCompress(t *testing.T) {
	d, be, seen, base, id := handleDaemon(t, nil)
	runPrompt(t, base, id, "build the thing")
	waitReleased(t, d, id)
	before, _ := sessionMeta(t, base, id)
	if before.Owner == "" {
		t.Fatal("Session has no owner")
	}
	// Seed memory so the bank binding is observable across the swap.
	putProfileLines(t, base, "compress binding profile line")
	addRule(t, d, before.Owner, "compress binding tabs rule")
	eng, err := d.memoryEngine()
	if err != nil {
		t.Fatal(err)
	}
	beforeBank, err := eng.Bank(before.Owner)
	if err != nil {
		t.Fatal(err)
	}
	beforeStarts := be.Starts(id)
	if len(beforeStarts) == 0 {
		t.Fatal("no starts before compress")
	}
	beforeEnv := beforeStarts[0].Env()
	compressAndSettle(t, base, id, 1)
	after, _ := sessionMeta(t, base, id)
	if after.Owner != before.Owner {
		t.Fatalf("owner after compress = %q, want %q", after.Owner, before.Owner)
	}
	if after.ResumeToken == before.ResumeToken {
		t.Fatal("resume token did not rotate")
	}
	afterBank, err := eng.Bank(after.Owner)
	if err != nil {
		t.Fatal(err)
	}
	if afterBank != beforeBank {
		t.Fatal("bank binding changed across compress")
	}
	for _, s := range be.Starts(id) {
		if s.Owner() != before.Owner || s.handle.owner != before.Owner {
			t.Fatalf("start %+v has owner %q, want %q", s, s.Owner(), before.Owner)
		}
		if s.handle.role != "web" {
			t.Fatalf("start %+v has role %q, want web before and after compress", s, s.handle.role)
		}
		if strings.Join(s.Env(), "|") != strings.Join(beforeEnv, "|") {
			t.Fatalf("start env = %q, want %q", s.Env(), beforeEnv)
		}
	}
	if !opSeen(be, id, handleCompress) {
		t.Fatal("no compress start")
	}
	// The same bank still serves the same Rule after the swap.
	got, err := afterBank.Recall(context.Background(), "binding tabs", 10)
	if err != nil || len(got) != 1 {
		t.Fatalf("recall after compress = %+v %v, want the seeded Rule", got, err)
	}
	last := seen.all()[len(seen.all())-1]
	if !strings.Contains(last, "## Task") {
		t.Fatalf("compress turn = %q, want the Handoff last", last)
	}
	_ = seen
}

// TestInvariant_NothingInjectedIsUserAndMinerReadsOnlyUser pins "Sender
// typing": every injection is sender=system (never a user message), and the
// miner reads only sender=user prompts through the filter.
func TestInvariant_NothingInjectedIsUserAndMinerReadsOnlyUser(t *testing.T) {
	// Injections log as system and leave the user prompt alone.
	injectHook(t, createOnly(injection{texts: []string{"PROFILE", "RECALL"}, profile: 4, refs: []InjectedRule{{ID: "r1", Version: 1}}}))
	d, _, seen, base, id := handleDaemon(t, nil)
	runPrompt(t, base, id, "build the thing")
	waitReleased(t, d, id)
	evs := transcriptOf(t, base, id)
	var prompt *Event
	for i := range evs {
		if evs[i].Kind == "prompt" && evs[i].Text == "build the thing" {
			prompt = &evs[i]
		}
		if evs[i].Kind == "injection" {
			if evs[i].Sender != senderSystem || evs[i].User != "" {
				t.Fatalf("injection Event sender=%q user=%q, want system with no user", evs[i].Sender, evs[i].User)
			}
		}
		if evs[i].Kind == "prompt" && evs[i].Sender == senderUser && strings.Contains(evs[i].Text, "PROFILE") {
			t.Fatalf("injected block recorded as user message: %+v", evs[i])
		}
	}
	if prompt == nil {
		t.Fatalf("no user prompt in %+v", evs)
	}
	if prompt.Sender != senderUser {
		t.Fatalf("prompt sender = %q, want user", prompt.Sender)
	}
	if got := seen.all(); len(got) != 1 || got[0] != "PROFILE\n\nRECALL\n\nbuild the thing" {
		t.Fatalf("backend prompt = %q, want the block ahead of the user text", got)
	}

	// The miner keeps only sender=user prompts; system text never mines.
	d2 := NewDaemon(tempDir(t), NewFakeBackend())
	mixed := []Event{
		{Kind: "tool_call_started", Text: `{"name":"write","input":"x"}`, Seq: 1},
		userPrompt(2, "alice", "No, use tabs here"),
		{Kind: "prompt", Sender: senderSystem, Text: "grouped result", Seq: 3},
		{Kind: "prompt", Sender: senderSystem, Handoff: true, Text: "# Handoff", Seq: 4},
		{Kind: "injection", Sender: senderSystem, Text: InjectionEvent{Rules: []InjectedRule{{ID: "r", Version: 1}}}.text(), Seq: 5},
		{Kind: "assistant_chunk", Text: "ok", Seq: 6},
	}
	seedEndedSession(t, d2, "s1", "alice", mixed)
	res, err := d2.MineSession("s1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Scanned != len(mixed) || res.Kept != 1 || res.Stored != 1 {
		t.Fatalf("mine = %+v, want 6 scanned, 1 kept, 1 stored", res)
	}
	if got := FilterMinerEvents(mixed); len(got) != 1 || got[0].Text != "No, use tabs here" {
		t.Fatalf("FilterMinerEvents kept %+v, want the one user prompt", got)
	}
	if MinerReadable(Event{Kind: "prompt", Sender: senderSystem, Text: "grouped"}) {
		t.Fatal("MinerReadable accepted a system prompt")
	}
	if MinerReadable(Event{Kind: "injection", Sender: senderSystem}) {
		t.Fatal("MinerReadable accepted an injection Event")
	}

	// Static: any file that mines the log filters through the predicate.
	err = filepath.WalkDir(".", func(path string, dEnt os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if dEnt.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !minerGuarded(string(raw)) {
			t.Errorf("%s mines the event log without MinerReadable/FilterMinerEvents", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestInvariant_PastedRememberAndToolOutputCreateNothing pins the "Human
// gate": pasted remember: text and /remember inside tool output never act;
// only a structural /remember first line of a typed user message does.
func TestInvariant_PastedRememberAndToolOutputCreateNothing(t *testing.T) {
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
	// Tool output never mines: a /remember inside a tool result stores nothing.
	d2 := NewDaemon(tempDir(t), NewFakeBackend())
	toolEvs := []Event{
		{Kind: "tool_call_started", Text: "/remember tool says tabs", Seq: 1},
		userPrompt(2, "alice", "thanks, that helps a lot for now"),
	}
	seedEndedSession(t, d2, "tools", "alice", toolEvs)
	res, err := d2.MineSession("tools")
	if err != nil {
		t.Fatal(err)
	}
	if res.Stored != 0 {
		t.Fatalf("tool-output mine stored %d, want 0", res.Stored)
	}
	if obs := bankQuotes(t, d2, "alice", "tabs"); len(obs) != 0 {
		t.Fatalf("tool-output observations = %+v, want none", obs)
	}
	// The structural first line still acts, proving the gate is positional.
	body, _ = json.Marshal(map[string]string{"text": "/remember typed tabs decree"})
	resp, err = http.Post(base+"/api/sessions/"+id+"/prompt", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("typed /remember = %d, want 201", resp.StatusCode)
	}
	if rules, _ := ownerBank(t, d, owner).Recall(context.Background(), "typed tabs decree", 10); len(rules) != 1 {
		t.Fatalf("typed rules = %+v, want 1", rules)
	}
}

// TestInvariant_RedactorOnEveryEgress pins "Redact at every egress": the
// redactor runs before anything is stored or sent to the distill LLM, a
// Decider or the Jev API.
func TestInvariant_RedactorOnEveryEgress(t *testing.T) {
	secret := "key AKIAIOSFODNN7EXAMPLE here"
	if got := memory.Redact(secret); !strings.Contains(got, "[redacted:aws-access-key]") || strings.Contains(got, "AKIAIOSFODNN7EXAMPLE") {
		t.Fatalf("Redact(%q) = %q, want the marker without the secret", secret, got)
	}
	for name, fn := range map[string]func(string) string{
		"ForDistill": memory.ForDistill,
		"ForDecider": memory.ForDecider,
		"ForJev":     memory.ForJev,
	} {
		if got, want := fn(secret), memory.Redact(secret); got != want {
			t.Fatalf("%s = %q, want %q", name, got, want)
		}
	}
	// The miner stores redacted text, never the raw secret.
	d := NewDaemon(tempDir(t), NewFakeBackend())
	seedEndedSession(t, d, "s1", "alice", []Event{userPrompt(1, "alice", "remember: the password is hunter2-2026, always use tabs")})
	if _, err := d.MineSession("s1"); err != nil {
		t.Fatal(err)
	}
	obs := bankQuotes(t, d, "alice", "tabs")
	if len(obs) != 1 {
		t.Fatalf("observations = %+v, want 1", obs)
	}
	if strings.Contains(obs[0].Quote, "hunter2") || !strings.Contains(obs[0].Quote, "[redacted:password]") {
		t.Fatalf("miner stored raw secret: %q", obs[0].Quote)
	}
	// The remember tool stores redacted text, never the raw secret.
	rd, rbase, rid, rowner := rememberDaemon(t)
	runPrompt(t, rbase, rid, "the password is hunter2-2026 for tabs work")
	if code, _ := postRemember(t, rbase, rid, map[string]any{"text": "use tabs", "reason": "r", "quote": "password is hunter2-2026"}); code != http.StatusCreated {
		t.Fatalf("remember with secret quote = %d, want 201", code)
	}
	for _, it := range queueRules(t, rd, rowner) {
		if strings.Contains(it.Payload, "hunter2") {
			t.Fatalf("remember queue holds raw secret: %s", it.Payload)
		}
		if !strings.Contains(it.Payload, "[redacted:password]") {
			t.Fatalf("remember queue lacks redaction marker: %s", it.Payload)
		}
	}
	// Static: the three Daemon call sites name the redactor in the same file.
	for path, want := range map[string]string{
		"miner.go": "memory.Redact",
		"remember.go": "memory.Redact",
		"recall_inject.go": "ForDecider",
	} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), want) {
			t.Errorf("%s has no %s: the egress path bypasses the redactor", path, want)
		}
	}
	// The egress wrappers stay thin over Redact.
	in := "token ghp_aBcDeFgHiJkLmNoPqRsT0123456789ABCD with passwort: hunter2supersecretvalue123"
	if want := memory.Redact(in); memory.ForDistill(in) != want || memory.ForDecider(in) != want || memory.ForJev(in) != want {
		t.Fatalf("egress wrappers diverge from Redact for %q", in)
	}
}
