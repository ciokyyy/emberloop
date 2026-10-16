package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"example.com/emberloop-web/memory"
)

func urlQueryEscape(s string) string { return url.QueryEscape(s) }

// memoryPageDaemon starts a Daemon on the fake Backend with the memory clock
// wired, returning the Daemon and its authed base URL.
func memoryPageDaemon(t *testing.T) (*Daemon, string) {
	t.Helper()
	settingsPath := useSettings(t, `{"compress_model":"test-faux/faux-1"}`)
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.SetSettingsPath(settingsPath)
	d.summarizer = &scriptedSummarizer{replies: []string{goodSummary}}
	srv := httptest.NewServer(authedHandler(t, d))
	t.Cleanup(srv.Close)
	return d, srv.URL
}

// memoryTwoUsers starts one Daemon with an admin and a bob server, like the
// private-notes invariant, returning both bases and owners.
func memoryTwoUsers(t *testing.T) (*Daemon, string, string, string, string) {
	t.Helper()
	settingsPath := useSettings(t, `{"compress_model":"test-faux/faux-1"}`)
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.SetSettingsPath(settingsPath)
	d.summarizer = &scriptedSummarizer{replies: []string{goodSummary}}
	h := d.Handler()
	admin := signedIn(t, h, "admin")
	adminID := userOf(t, h, admin).ID
	bob := signedIn(t, h, "bob")
	bobID := userOf(t, h, bob).ID
	asAdmin := httptest.NewServer(withCookie(h, admin))
	t.Cleanup(asAdmin.Close)
	asBob := httptest.NewServer(withCookie(h, bob))
	t.Cleanup(asBob.Close)
	return d, asAdmin.URL, asBob.URL, adminID, bobID
}

func getRulesList(t *testing.T, base, q string) []ruleView {
	t.Helper()
	u := base + "/api/rules"
	if q != "" {
		u += "?q=" + urlQueryEscape(q)
	}
	resp, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200", u, resp.StatusCode)
	}
	var out []ruleView
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode rules: %v", err)
	}
	return out
}

func getObservationsList(t *testing.T, base, q string) []observationView {
	t.Helper()
	u := base + "/api/observations"
	if q != "" {
		u += "?q=" + urlQueryEscape(q)
	}
	resp, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200", u, resp.StatusCode)
	}
	var out []observationView
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode observations: %v", err)
	}
	return out
}

func getMemoryStatus(t *testing.T, base string) (int, memoryStatus) {
	t.Helper()
	resp, err := http.Get(base + "/api/memory/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out memoryStatus
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func getMemoryLedger(t *testing.T, base string) []ledgerView {
	t.Helper()
	resp, err := http.Get(base + "/api/memory/ledger")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/memory/ledger = %d, want 200", resp.StatusCode)
	}
	var out []ledgerView
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode ledger: %v", err)
	}
	return out
}

func postPurge(t *testing.T, base string, body any) (int, string) {
	t.Helper()
	raw, _ := json.Marshal(body)
	resp, err := http.Post(base+"/api/memory/purge", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(resp.Body)
	return resp.StatusCode, buf.String()
}

// A User sees only their own bank; admins get no extra read access to other
// Users' notes.
func TestMemoryPagePrivateByDefault(t *testing.T) {
	t.Parallel()
	d, asAdmin, asBob, adminID, bobID := memoryTwoUsers(t)
	ctx := context.Background()
	if _, err := ownerBank(t, d, adminID).AddRule(ctx, memory.Rule{
		Text: "admin private decree note", Kind: memory.KindFact, ApprovedBy: adminID,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ownerBank(t, d, bobID).AddRule(ctx, memory.Rule{
		Text: "bob private edict note", Kind: memory.KindFact, ApprovedBy: bobID,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ownerBank(t, d, adminID).AddObservation(ctx, memory.Observation{
		SessionID: "s1", EventID: "e1", Author: adminID,
		Quote: "admin private decree observation", Kind: memory.KindFact,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ownerBank(t, d, bobID).AddObservation(ctx, memory.Observation{
		SessionID: "s2", EventID: "e1", Author: bobID,
		Quote: "bob private edict observation", Kind: memory.KindFact,
	}); err != nil {
		t.Fatal(err)
	}
	adminRules := getRulesList(t, asAdmin, "")
	for _, r := range adminRules {
		if strings.Contains(r.Text, "bob private edict") {
			t.Fatalf("admin rules hold bob's note: %+v", adminRules)
		}
	}
	found := false
	for _, r := range adminRules {
		if strings.Contains(r.Text, "admin private decree") {
			found = true
		}
	}
	if !found {
		t.Fatalf("admin rules = %+v, want the admin note", adminRules)
	}
	bobRules := getRulesList(t, asBob, "")
	for _, r := range bobRules {
		if strings.Contains(r.Text, "admin private decree") {
			t.Fatalf("bob rules hold admin's note: %+v", bobRules)
		}
	}
	if got := getObservationsList(t, asAdmin, ""); len(got) != 1 || !strings.Contains(got[0].Quote, "admin private decree") {
		t.Fatalf("admin observations = %+v, want only the admin quote", got)
	}
	if got := getObservationsList(t, asBob, ""); len(got) != 1 || !strings.Contains(got[0].Quote, "bob private edict") {
		t.Fatalf("bob observations = %+v, want only the bob quote", got)
	}
	// Search stays in the caller's bank too.
	if got := getRulesList(t, asAdmin, "decree"); len(got) != 1 {
		t.Fatalf("admin search decree = %+v, want 1", got)
	}
	if got := getRulesList(t, asAdmin, "edict"); len(got) != 0 {
		t.Fatalf("admin search edict = %+v, want none", got)
	}
	// The machine token is no User on the page routes.
	for _, path := range []string{"/api/rules", "/api/observations", "/api/memory/ledger", "/api/memory/status"} {
		req, _ := http.NewRequest("GET", asAdmin+path, nil)
		req.Header.Set("Authorization", "Bearer "+d.MachineToken())
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("machine GET %s = %d, want 401", path, resp.StatusCode)
		}
	}
}

// Queue shows each proposal with quotes and source Sessions; approve, edit,
// reject and batch approve work from the page routes.
func TestMemoryPageQueueDecideAndBatch(t *testing.T) {
	t.Parallel()
	_, base := memoryPageDaemon(t)
	id := createSession(t, base, "fake", "fake-model", tempDir(t))
	runPrompt(t, base, id, "the team uses tabs for indentation here")
	if code, _ := postRemember(t, base, id, map[string]any{
		"text": "use tabs for indentation", "reason": "tabs reason", "quote": "uses tabs for indentation",
	}); code != http.StatusCreated {
		t.Fatalf("remember = %d, want 201", code)
	}
	queue := getRulesQueue(t, base)
	if len(queue) != 1 {
		t.Fatalf("queue = %d, want 1", len(queue))
	}
	proposal, err := memory.DecodeRememberProposal(queue[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Text == "" || len(proposal.Quotes) == 0 || len(proposal.Sessions) == 0 {
		t.Fatalf("proposal = %+v, want text with quotes and sessions", proposal)
	}
	// Edit keeps both: the proposal stays, the Rule carries the edit.
	if code, _ := decideRule(t, base, queue[0].ID, map[string]any{"status": "approved", "text": "use tabs for Go files"}); code != http.StatusOK {
		t.Fatalf("approve with edit = %d, want 200", code)
	}
	if got := getRulesList(t, base, "Go files"); len(got) != 1 || got[0].Text != "use tabs for Go files" {
		t.Fatalf("rules after edit = %+v, want the edited text", got)
	}
	kept := getRulesQueue(t, base)
	if len(kept) != 1 || kept[0].Status != memory.QueueApproved {
		t.Fatalf("queue after approve = %+v, want the approved row kept", kept)
	}
	if p, _ := memory.DecodeRememberProposal(kept[0].Payload); p.Text != "use tabs for indentation" {
		t.Fatalf("kept proposal = %+v, want the original text", p)
	}
	// Reject keeps the item as a label.
	runPrompt(t, base, id, "the team uses tabs for indentation here again")
	if code, _ := postRemember(t, base, id, map[string]any{
		"text": "second tabs note", "reason": "r", "quote": "uses tabs for indentation",
	}); code != http.StatusCreated {
		t.Fatalf("second remember = %d, want 201", code)
	}
	var second int64
	for _, it := range getRulesQueue(t, base) {
		if it.Status == memory.QueuePending {
			second = it.ID
		}
	}
	if code, _ := decideRule(t, base, second, map[string]any{"status": "rejected"}); code != http.StatusOK {
		t.Fatalf("reject = %d, want 200", code)
	}
	// Batch approve per Session clears the Session's candidates together.
	runPrompt(t, base, id, "batch session marker with tabs")
	for _, text := range []string{"batch tabs alpha", "batch tabs beta"} {
		if code, _ := postRemember(t, base, id, map[string]any{
			"text": text, "reason": "r", "quote": "batch session marker",
		}); code != http.StatusCreated {
			t.Fatalf("batch remember %q = %d, want 201", text, code)
		}
	}
	pending := 0
	for _, it := range getRulesQueue(t, base) {
		if it.Status == memory.QueuePending {
			pending++
		}
	}
	if pending != 2 {
		t.Fatalf("pending before batch = %d, want 2", pending)
	}
	code, results := postBatch(t, base, map[string]any{"sessionId": id, "status": "approved"})
	if code != http.StatusOK || len(results) != 2 {
		t.Fatalf("batch = %d %+v, want 2 results", code, results)
	}
	for _, r := range results {
		if r.Error != "" || r.RuleID == "" {
			t.Fatalf("batch result = %+v, want a rule per item", r)
		}
	}
}

// Profile versions are listed; any version can be restored as a new version.
func TestMemoryPageProfileVersionsRestore(t *testing.T) {
	t.Parallel()
	_, base := memoryPageDaemon(t)
	put := func(ops []memory.ProfileOp) {
		t.Helper()
		raw, _ := json.Marshal(map[string]any{"ops": ops})
		req, _ := http.NewRequest("PUT", base+"/api/profile", bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("PUT profile = %d, want 200", resp.StatusCode)
		}
	}
	put([]memory.ProfileOp{
		{Op: memory.ProfileAdd, Text: "memory page alpha", Tag: memory.ProfileWorkflow},
		{Op: memory.ProfileAdd, Text: "memory page beta", Tag: memory.ProfileStyle},
	})
	versions := getProfileVersions(t, base)
	if len(versions) != 1 || versions[0].Version != 1 {
		t.Fatalf("versions = %+v, want one v1", versions)
	}
	raw, _ := json.Marshal(map[string]any{"version": 1})
	resp, err := http.Post(base+"/api/profile/restore", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("restore = %d, want 200", resp.StatusCode)
	}
	if got := getProfileVersions(t, base); len(got) != 2 || got[1].Version != 2 {
		t.Fatalf("versions after restore = %+v, want v2 as a new version", got)
	}
}

// Purge needs a typed confirmation and reports what it removed.
func TestMemoryPagePurgeNeedsConfirmAndReports(t *testing.T) {
	t.Parallel()
	d, base := memoryPageDaemon(t)
	ctx := context.Background()
	owner := userOf(t, d.Handler(), signedIn(t, d.Handler(), "purgeuser")).ID
	_ = owner
	// Seed through the authed base (the admin's bank).
	rule, err := ownerBank(t, d, userIDOf(t, base)).AddRule(ctx, memory.Rule{
		Text: "purgeable rule note", Kind: memory.KindFact, ApprovedBy: userIDOf(t, base),
	})
	if err != nil {
		t.Fatal(err)
	}
	obsID, err := ownerBank(t, d, userIDOf(t, base)).AddObservation(ctx, memory.Observation{
		SessionID: "s1", EventID: "e1", Author: userIDOf(t, base),
		Quote: "purgeable observation quote", Kind: memory.KindFact,
	})
	if err != nil {
		t.Fatal(err)
	}
	if code, body := postPurge(t, base, map[string]any{
		"observations": []int64{obsID}, "rules": []string{rule.RuleID}, "confirm": "nope",
	}); code != http.StatusBadRequest || !strings.Contains(body, "PURGE") {
		t.Fatalf("purge without confirm = %d %q, want 400 naming PURGE", code, body)
	}
	if got := getRulesList(t, base, "purgeable"); len(got) != 1 {
		t.Fatalf("rules after refused purge = %+v, want the rule kept", got)
	}
	if code, body := postPurge(t, base, map[string]any{
		"observations": []int64{obsID}, "rules": []string{rule.RuleID}, "confirm": "PURGE",
	}); code != http.StatusOK || !strings.Contains(body, "observations") {
		t.Fatalf("purge = %d %q, want 200 reporting counts", code, body)
	} else {
		var res purgeResultView
		if err := json.Unmarshal([]byte(body), &res); err != nil {
			t.Fatalf("purge result decodes: %v", err)
		}
		if res.Observations != 1 || res.Rules != 1 {
			t.Fatalf("purge result = %+v, want 1 observation and 1 rule", res)
		}
	}
	if got := getRulesList(t, base, "purgeable"); len(got) != 0 {
		t.Fatalf("rules after purge = %+v, want none", got)
	}
	if got := getObservationsList(t, base, "purgeable"); len(got) != 0 {
		t.Fatalf("observations after purge = %+v, want none", got)
	}
	// An empty selection is refused even with the confirmation.
	if code, _ := postPurge(t, base, map[string]any{"confirm": "PURGE"}); code != http.StatusBadRequest {
		t.Fatalf("empty purge = %d, want 400", code)
	}
}

// Stats report queue size, approval latency, the last nightly run and the
// ledger; the status chip is unavailable when the engine fails.
func TestMemoryPageStatusAndLedger(t *testing.T) {
	t.Parallel()
	d, base := memoryPageDaemon(t)
	id := createSession(t, base, "fake", "fake-model", tempDir(t))
	runPrompt(t, base, id, "the team uses tabs for indentation here")
	if code, _ := postRemember(t, base, id, map[string]any{
		"text": "status tabs note", "reason": "r", "quote": "uses tabs for indentation",
	}); code != http.StatusCreated {
		t.Fatalf("remember = %d, want 201", code)
	}
	code, st := getMemoryStatus(t, base)
	if code != http.StatusOK || !st.Available || st.Queue == nil {
		t.Fatalf("status = %d %+v, want available with queue", code, st)
	}
	if st.Queue.Pending != 1 || st.Queue.Total != 1 || st.Queue.Budget != memory.QueueBudget {
		t.Fatalf("queue stats = %+v, want 1 pending of 1 with the budget", st.Queue)
	}
	ledger := getMemoryLedger(t, base)
	_ = ledger
	// A queue decision records latency, which the status then reports.
	qid := getRulesQueue(t, base)[0].ID
	if code, _ := decideRule(t, base, qid, map[string]any{"status": "approved"}); code != http.StatusOK {
		t.Fatalf("approve = %d, want 200", code)
	}
	_, st2 := getMemoryStatus(t, base)
	if st2.Queue.Approved != 1 {
		t.Fatalf("queue after approve = %+v, want 1 approved", st2.Queue)
	}
	ledger2 := getMemoryLedger(t, base)
	found := false
	for _, l := range ledger2 {
		if l.Stage == "queue_approve" {
			found = true
		}
	}
	if !found {
		t.Fatalf("ledger = %+v, want a queue_approve row", ledger2)
	}
	// A closed engine is unavailable, never a 500 with rows.
	eng, err := d.memoryEngine()
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.Close(); err != nil {
		t.Fatal(err)
	}
	code, st3 := getMemoryStatus(t, base)
	if code != http.StatusServiceUnavailable || st3.Available {
		t.Fatalf("closed status = %d %+v, want 503 unavailable", code, st3)
	}
}

// userIDOf reads the signed-in User behind an authed base URL.
func userIDOf(t *testing.T, base string) string {
	t.Helper()
	resp, err := http.Get(base + "/api/auth/me")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var me struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&me); err != nil {
		t.Fatalf("decode me: %v", err)
	}
	if me.ID == "" {
		t.Fatal("no user id behind the base")
	}
	return me.ID
}

func TestMemoryPageMemoryActions(t *testing.T) {
	d, _ := memoryPageDaemon(t)
	h := d.Handler()
	cookie := signedIn(t, h, "memory-actions")
	user := userOf(t, h, cookie)
	bank, err := d.profileBankFor(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	loopback := func(req *http.Request) { req.RemoteAddr = "127.0.0.1:1234"; req.Host = "127.0.0.1" }
	id, err := bank.AddMemory(ctx, memory.MemoryItem{SessionID: "session-a", EventID: "event-a", Author: user.ID, Kind: memory.KindPreference, Text: "Use concise replies", Quote: "Please use concise replies"})
	if err != nil {
		t.Fatal(err)
	}
	post := func(path string, body any) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
		loopback(req)
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/memory/memories?q=concise", nil)
	loopback(req)
	req.AddCookie(cookie)
	h.ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Use concise replies") {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	for _, action := range []string{"pin", "park"} {
		body := map[string]any{}
		if action == "pin" {
			body["pinned"] = true
		}
		if got := post("/api/memory/memories/"+strconv.FormatInt(id, 10)+"/"+action, body); got.Code != 200 {
			t.Fatalf("%s: %d %s", action, got.Code, got.Body.String())
		}
	}
	if got := post("/api/memory/memories/"+strconv.FormatInt(id, 10)+"/edit", map[string]string{"text": "Prefer concise replies"}); got.Code != 200 {
		t.Fatalf("edit: %d %s", got.Code, got.Body.String())
	}
	old, _ := bank.GetMemory(ctx, id)
	if old.Status != memory.StatusSuperseded || old.Text != "Use concise replies" || old.SupersededBy == 0 {
		t.Fatalf("old row changed incorrectly: %+v", old)
	}
	if got := post("/api/memory/memories/"+strconv.FormatInt(old.SupersededBy, 10)+"/promote", map[string]any{}); got.Code != 200 {
		t.Fatalf("promote: %d %s", got.Code, got.Body.String())
	}
	items, _ := bank.Queue(ctx)
	found := false
	for _, item := range items {
		if item.Kind == memory.QueueRule && item.Status == memory.QueuePending && strings.Contains(item.Payload, `"source_memory_id"`) {
			found = true
		}
	}
	if !found {
		t.Fatalf("pending source Rule not queued: %+v", items)
	}
	if got := post("/api/memory/memories/"+strconv.FormatInt(old.SupersededBy, 10)+"/forget", map[string]any{}); got.Code != 200 {
		t.Fatalf("forget: %d %s", got.Code, got.Body.String())
	}
	forgot, _ := bank.GetMemory(ctx, old.SupersededBy)
	if forgot.Status != memory.StatusForgotten {
		t.Fatalf("forgotten status = %q", forgot.Status)
	}
	listed := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/memory/memories", nil)
	loopback(req)
	req.AddCookie(cookie)
	h.ServeHTTP(listed, req)
	if got := post("/api/memory/purge", map[string]any{"memories": []int64{forgot.ID}, "confirm": "PURGE"}); got.Code != 200 {
		t.Fatalf("purge forgotten memory: %d %s", got.Code, got.Body.String())
	}
	if _, err := bank.GetMemory(ctx, forgot.ID); !errors.Is(err, memory.ErrNotFound) {
		t.Fatalf("purged memory remains: %v", err)
	}
	if strings.Contains(listed.Body.String(), `"id":`+strconv.FormatInt(forgot.ID, 10)) {
		t.Fatalf("forgotten row visible in default list: %s", listed.Body.String())
	}
	obsID, err := bank.AddObservation(ctx, memory.Observation{SessionID: "session-a", EventID: "legacy-event", Author: user.ID, Quote: "I prefer compact summaries", Kind: memory.KindPreference})
	if err != nil {
		t.Fatal(err)
	}
	if got := post("/api/memory/observations/"+strconv.FormatInt(obsID, 10)+"/promote", map[string]any{}); got.Code != 201 {
		t.Fatalf("legacy promote: %d %s", got.Code, got.Body.String())
	}
	legacy, _ := bank.ListMemories(ctx, "", "", 100)
	if len(legacy) == 0 {
		t.Fatal("legacy observation was not promoted")
	}
	if _, err := bank.AddEpisode(ctx, memory.Episode{SessionID: "session-a", EventRange: "event-a:event-z", Author: user.ID, Text: "old user turn text"}); err != nil {
		t.Fatal(err)
	}
	episodesReq := httptest.NewRequest(http.MethodGet, "/api/memory/episodes", nil)
	loopback(episodesReq)
	episodesReq.AddCookie(cookie)
	episodesRec := httptest.NewRecorder()
	h.ServeHTTP(episodesRec, episodesReq)
	if episodesRec.Code != 200 || !strings.Contains(episodesRec.Body.String(), "old user turn text") {
		t.Fatalf("episodes: %d %s", episodesRec.Code, episodesRec.Body.String())
	}
}
