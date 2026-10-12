package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"example.com/emberloop-web/memory"
)

// rulesClock is a settable clock for rule queue API tests.
type rulesClock struct {
	mu sync.Mutex
	t  time.Time
}

func newRulesClock() *rulesClock {
	return &rulesClock{t: time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)}
}

func (c *rulesClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *rulesClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// rulesDaemon starts a Daemon on the fake Backend with the fake clock wired
// into the memory engine, returning the Daemon, its authed base URL, one
// Session, its owner and the clock.
func rulesDaemon(t *testing.T) (*Daemon, string, string, string, *rulesClock) {
	t.Helper()
	settingsPath := useSettings(t, `{"compress_model":"test-faux/faux-1"}`)
	clock := newRulesClock()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.SetSettingsPath(settingsPath)
	d.clock = clock.Now
	d.summarizer = &scriptedSummarizer{replies: []string{goodSummary}}
	srv := httptest.NewServer(authedHandler(t, d))
	t.Cleanup(srv.Close)
	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	meta, _ := sessionMeta(t, srv.URL, id)
	return d, srv.URL, id, meta.Owner, clock
}

func rulesBank(t *testing.T, d *Daemon, owner string) memory.Memory {
	t.Helper()
	bank, err := d.profileBankFor(owner)
	if err != nil {
		t.Fatal(err)
	}
	return bank
}

func getRulesQueue(t *testing.T, base string) []memory.QueueItem {
	t.Helper()
	resp, err := http.Get(base + "/api/rules/queue")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/rules/queue = %d, want 200", resp.StatusCode)
	}
	var out []memory.QueueItem
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode rules queue: %v", err)
	}
	return out
}

func decideRule(t *testing.T, base string, id int64, body any) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	resp, err := http.Post(base+"/api/rules/queue/"+strconv.FormatInt(id, 10)+"/decide", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func postPromote(t *testing.T, base, key, kind string) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(map[string]string{"subject_key": key, "kind": kind})
	resp, err := http.Post(base+"/api/rules/queue/promote", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func postBatch(t *testing.T, base string, body any) (int, []ruleBatchResult) {
	t.Helper()
	raw, _ := json.Marshal(body)
	resp, err := http.Post(base+"/api/rules/queue/batch", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out []ruleBatchResult
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func getQueueStats(t *testing.T, base string) memory.QueueStats {
	t.Helper()
	resp, err := http.Get(base + "/api/rules/queue/stats")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/rules/queue/stats = %d, want 200", resp.StatusCode)
	}
	var st memory.QueueStats
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		t.Fatalf("decode stats: %v", err)
	}
	return st
}

// A single mention never reaches the queue; the same subject key in two
// Sessions on different tasks does.
func TestRuleQueuePromoteNeedsRecurrence(t *testing.T) {
	d, base, _, owner, _ := rulesDaemon(t)
	ctx := context.Background()
	bank := rulesBank(t, d, owner)
	if _, err := bank.AddObservation(ctx, memory.Observation{
		SessionID: "s1", EventID: "e1", Author: owner,
		Quote: "always wrap errors in this repo", Kind: memory.KindPreference, SubjectKey: "wrap.errors",
	}); err != nil {
		t.Fatal(err)
	}
	if code, out := postPromote(t, base, "wrap.errors", ""); code != http.StatusOK || out["promoted"] != false {
		t.Fatalf("promote single = %d %v, want 200 promoted false", code, out)
	}
	if got := getRulesQueue(t, base); len(got) != 0 {
		t.Fatalf("queue after single = %d, want none", len(got))
	}
	if _, err := bank.AddObservation(ctx, memory.Observation{
		SessionID: "s2", EventID: "e2", Author: owner,
		Quote: "always wrap errors in this repo please", Kind: memory.KindPreference, SubjectKey: "wrap.errors",
	}); err != nil {
		t.Fatal(err)
	}
	code, out := postPromote(t, base, "wrap.errors", "")
	if code != http.StatusCreated || out["promoted"] != true {
		t.Fatalf("promote recurring = %d %v, want 201 promoted true", code, out)
	}
	queue := getRulesQueue(t, base)
	if len(queue) != 1 {
		t.Fatalf("queue after recurrence = %d, want 1", len(queue))
	}
	proposal, err := memory.DecodeRememberProposal(queue[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(proposal.Quotes) != 2 || len(proposal.Sessions) != 2 {
		t.Fatalf("proposal = %+v, want both quotes and sessions", proposal)
	}
	if proposal.Text == "" {
		t.Fatalf("proposal text empty, want one generalization")
	}
	// A second promote for the same subject returns the pending item instead
	// of enqueuing twice.
	code, again := postPromote(t, base, "wrap.errors", "")
	if code != http.StatusCreated || again["promoted"] != false {
		t.Fatalf("re-promote = %d %v, want 201 promoted false with the pending id", code, again)
	}
	if got := getRulesQueue(t, base); len(got) != 1 {
		t.Fatalf("queue after re-promote = %d, want still 1", len(got))
	}
}

// Approve creates a new rule version; the queue row stays as the record.
func TestRuleQueueApproveCreatesRule(t *testing.T) {
	d, base, id, owner, _ := rulesDaemon(t)
	runPrompt(t, base, id, "the team uses tabs for indentation here")
	code, _ := postRemember(t, base, id, map[string]any{
		"text": "use tabs for indentation", "reason": "the user prefers tabs", "quote": "uses tabs for indentation",
	})
	if code != http.StatusCreated {
		t.Fatalf("remember = %d, want 201", code)
	}
	queue := getRulesQueue(t, base)
	if len(queue) != 1 {
		t.Fatalf("queue = %d, want 1", len(queue))
	}
	qid := queue[0].ID
	code, out := decideRule(t, base, qid, map[string]any{"status": "approved"})
	if code != http.StatusOK || out["rule_id"] == nil && out["RuleID"] == nil {
		t.Fatalf("approve = %d %v, want 200 with a rule", code, out)
	}
	bank := rulesBank(t, d, owner)
	got, err := bank.Recall(context.Background(), "tabs indentation", 10)
	if err != nil || len(got) != 1 || got[0].Text != "use tabs for indentation" {
		t.Fatalf("recall = %+v %v, want the approved Rule", got, err)
	}
	queue = getRulesQueue(t, base)
	if len(queue) != 1 || queue[0].Status != memory.QueueApproved {
		t.Fatalf("queue after approve = %+v, want the approved row kept", queue)
	}
}

// An edit keeps both the proposal and the final text: the queue row holds
// the proposal, the Rule holds the edit.
func TestRuleQueueEditKeepsBoth(t *testing.T) {
	d, base, id, owner, _ := rulesDaemon(t)
	runPrompt(t, base, id, "the team uses tabs for indentation here")
	if code, _ := postRemember(t, base, id, map[string]any{
		"text": "use tabs for indentation", "reason": "r", "quote": "uses tabs for indentation",
	}); code != http.StatusCreated {
		t.Fatalf("remember = %d, want 201", code)
	}
	qid := getRulesQueue(t, base)[0].ID
	code, _ := decideRule(t, base, qid, map[string]any{"status": "approved", "text": "use tabs for Go files"})
	if code != http.StatusOK {
		t.Fatalf("approve with edit = %d, want 200", code)
	}
	bank := rulesBank(t, d, owner)
	got, err := bank.Recall(context.Background(), "tabs Go files", 10)
	if err != nil || len(got) != 1 || got[0].Text != "use tabs for Go files" {
		t.Fatalf("recall = %+v %v, want the edited Rule text", got, err)
	}
	queue := getRulesQueue(t, base)
	if len(queue) != 1 {
		t.Fatalf("queue = %d, want 1", len(queue))
	}
	if !strings.Contains(queue[0].Payload, "use tabs for indentation") {
		t.Fatalf("queue payload = %s, want the original proposal kept", queue[0].Payload)
	}
	if strings.Contains(queue[0].Payload, "use tabs for Go files") {
		t.Fatalf("queue payload was rewritten with the edit: %s", queue[0].Payload)
	}
}

// Reject keeps the item as a label and writes no Rule.
func TestRuleQueueRejectKeepsLabel(t *testing.T) {
	d, base, id, owner, _ := rulesDaemon(t)
	runPrompt(t, base, id, "the team uses tabs for indentation here")
	if code, _ := postRemember(t, base, id, map[string]any{
		"text": "use tabs for indentation", "reason": "r", "quote": "uses tabs for indentation",
	}); code != http.StatusCreated {
		t.Fatalf("remember = %d, want 201", code)
	}
	qid := getRulesQueue(t, base)[0].ID
	if code, _ := decideRule(t, base, qid, map[string]any{"status": "rejected"}); code != http.StatusOK {
		t.Fatalf("reject = %d, want 200", code)
	}
	queue := getRulesQueue(t, base)
	if len(queue) != 1 || queue[0].Status != memory.QueueRejected {
		t.Fatalf("queue after reject = %+v, want the rejected label kept", queue)
	}
	if got, _ := rulesBank(t, d, owner).Recall(context.Background(), "tabs", 10); len(got) != 0 {
		t.Fatalf("rejected proposal wrote %d Rules, want none", len(got))
	}
	if code, _ := decideRule(t, base, qid, map[string]any{"status": "approved"}); code != http.StatusBadRequest {
		t.Fatalf("second decision = %d, want 400", code)
	}
}

// Same key, same observer, newer observation wins; a different observer is a
// disagreement with both Rules live. Identity is the observer behind the
// source observations (the approver fallback when there are none), never the
// approver alone.
func TestRuleQueueSupersessionAndDisagreement(t *testing.T) {
	d, base, id, owner, clock := rulesDaemon(t)
	ctx := context.Background()
	bank := rulesBank(t, d, owner)
	old, err := bank.AddRule(ctx, memory.Rule{
		Text: "always wrap errors", Kind: memory.KindPreference,
		SubjectKey: "wrap.errors", ApprovedBy: owner,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Same observer supersedes: the proposal is newer than the stored Rule.
	clock.Advance(time.Hour)
	runPrompt(t, base, id, "always wrap errors in this repo please")
	if code, _ := postRemember(t, base, id, map[string]any{
		"text": "always wrap errors with context", "reason": "r", "quote": "always wrap errors in this repo please",
	}); code != http.StatusCreated {
		t.Fatalf("remember = %d, want 201", code)
	}
	var qid int64
	for _, it := range getRulesQueue(t, base) {
		if it.Status == memory.QueuePending {
			qid = it.ID
		}
	}
	// Approve with the same key and observer: the old Rule supersedes.
	code, _ := decideRule(t, base, qid, map[string]any{"status": "approved", "subject_key": "wrap.errors"})
	if code != http.StatusOK {
		t.Fatalf("approve superseding = %d, want 200", code)
	}
	versions, err := bank.RuleVersions(ctx, old.RuleID)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 || versions[0].Status != memory.StatusSuperseded {
		t.Fatalf("old rule versions = %+v, want superseded", versions)
	}
	// An older candidate never clobbers a newer Rule: the newer text wins.
	// The stale proposal enqueues first; the Rule lands an hour later.
	stalePayload := memory.EncodeRuleProposal(memory.RuleProposal{
		Text: "stale wording", Kind: memory.KindPreference, SubjectKey: "newer.key",
		Sessions: []string{"s-old"}, Author: owner, Origin: "mined",
	})
	staleID, err := bank.Enqueue(ctx, memory.QueueItem{Kind: memory.QueueRule, Payload: stalePayload})
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Hour)
	newer, err := bank.AddRule(ctx, memory.Rule{
		Text: "newest wording wins", Kind: memory.KindPreference,
		SubjectKey: "newer.key", ApprovedBy: owner,
	})
	if err != nil {
		t.Fatal(err)
	}
	code, out := decideRule(t, base, staleID, map[string]any{"status": "approved"})
	if code != http.StatusOK {
		t.Fatalf("approve stale = %d, want 200", code)
	}
	if versions, _ := bank.RuleVersions(ctx, newer.RuleID); len(versions) != 1 || versions[0].Status != memory.StatusActive {
		t.Fatalf("newer rule after stale approve = %+v, want still active", versions)
	}
	staleRuleID, _ := out["RuleID"].(string)
	if staleRuleID == "" {
		t.Fatalf("approve response = %v, want the stillborn Rule ID", out)
	}
	if versions, _ := bank.RuleVersions(ctx, staleRuleID); len(versions) != 1 || versions[0].Status != memory.StatusSuperseded {
		t.Fatalf("stale rule = %+v, want stillborn superseded", versions)
	}
	// Different observer is a disagreement: both Rules stay live.
	if _, err := bank.AddRule(ctx, memory.Rule{
		Text: "never wrap errors", Kind: memory.KindPreference,
		SubjectKey: "disagree.key", ApprovedBy: "bob",
	}); err != nil {
		t.Fatal(err)
	}
	runPrompt(t, base, id, "we disagree about this key here")
	if code, _ := postRemember(t, base, id, map[string]any{
		"text": "always handle this key this way", "reason": "r", "quote": "disagree about this key",
	}); code != http.StatusCreated {
		t.Fatalf("remember disagreement = %d, want 201", code)
	}
	var disID int64
	for _, it := range getRulesQueue(t, base) {
		if it.Status == memory.QueuePending {
			disID = it.ID
		}
	}
	if code, _ := decideRule(t, base, disID, map[string]any{"status": "approved", "subject_key": "disagree.key"}); code != http.StatusOK {
		t.Fatalf("approve disagreement = %d, want 200", code)
	}
	// The different observer's Rule is still live: disagreement, not
	// supersession.
	all, _ := bank.RulesBySubjectKey(ctx, "disagree.key")
	seen := map[string]bool{}
	for _, r := range all {
		versions, _ := bank.RuleVersions(ctx, r.RuleID)
		if len(versions) > 0 && versions[len(versions)-1].Status != memory.StatusSuperseded {
			seen[r.RuleID] = true
		}
	}
	if len(seen) < 2 {
		t.Fatalf("disagreement rules = %+v, want both observers live", all)
	}
}

// Rule text over 300 chars is refused at write, not clipped later.
func TestRuleQueueOver300Refused(t *testing.T) {
	_, base, id, _, _ := rulesDaemon(t)
	runPrompt(t, base, id, "the team uses tabs for indentation here")
	if code, _ := postRemember(t, base, id, map[string]any{
		"text": "use tabs", "reason": "r", "quote": "uses tabs for indentation",
	}); code != http.StatusCreated {
		t.Fatalf("remember = %d, want 201", code)
	}
	qid := getRulesQueue(t, base)[0].ID
	long := strings.Repeat("x", memory.MaxRuleChars+1)
	if code, _ := decideRule(t, base, qid, map[string]any{"status": "approved", "text": long}); code != http.StatusBadRequest {
		t.Fatalf("approve over 300 = %d, want 400", code)
	}
	queue := getRulesQueue(t, base)
	if len(queue) != 1 || queue[0].Status != memory.QueuePending {
		t.Fatalf("queue after refused edit = %+v, want still pending", queue)
	}
}

// Batch approval per Session works: one call decides every pending item of
// the Session and leaves other Sessions alone.
func TestRuleQueueBatchPerSession(t *testing.T) {
	d, base, _, owner, _ := rulesDaemon(t)
	ctx := context.Background()
	bank := rulesBank(t, d, owner)
	mk := func(session, text string) {
		t.Helper()
		payload := memory.EncodeRuleProposal(memory.RuleProposal{
			Text: text, Kind: memory.KindPreference, SubjectKey: "batch.key",
			Sessions: []string{session}, Author: owner, Origin: "mined",
		})
		if _, err := bank.Enqueue(ctx, memory.QueueItem{Kind: memory.QueueRule, Payload: payload}); err != nil {
			t.Fatal(err)
		}
	}
	mk("s1", "batch proposal one")
	mk("s1", "batch proposal two")
	mk("s2", "batch proposal three")
	code, results := postBatch(t, base, map[string]any{"sessionId": "s1", "status": "approved"})
	if code != http.StatusOK || len(results) != 2 {
		t.Fatalf("batch s1 = %d %+v, want 2 results", code, results)
	}
	for _, r := range results {
		if r.Error != "" || r.RuleID == "" {
			t.Fatalf("batch result = %+v, want a rule per item", r)
		}
	}
	pending := 0
	for _, it := range getRulesQueue(t, base) {
		if it.Status == memory.QueuePending {
			pending++
			p, _ := memory.DecodeRememberProposal(it.Payload)
			if len(p.Sessions) != 1 || p.Sessions[0] != "s2" {
				t.Fatalf("pending after batch = %+v, want only s2", p)
			}
		}
	}
	if pending != 1 {
		t.Fatalf("pending after batch = %d, want 1", pending)
	}
	// Batch by IDs rejects together.
	var s2id int64
	for _, it := range getRulesQueue(t, base) {
		if it.Status == memory.QueuePending {
			s2id = it.ID
		}
	}
	code, results = postBatch(t, base, map[string]any{"ids": []int64{s2id}, "status": "rejected"})
	if code != http.StatusOK || len(results) != 1 || results[0].Error != "" {
		t.Fatalf("batch reject = %d %+v, want 1 clean reject", code, results)
	}
}

// API tests run on the fake clock: queue timestamps, budget and approval
// latency follow it.
func TestRuleQueueStatsFollowFakeClock(t *testing.T) {
	d, base, _, owner, clock := rulesDaemon(t)
	ctx := context.Background()
	bank := rulesBank(t, d, owner)
	payload := memory.EncodeRuleProposal(memory.RuleProposal{
		Text: "clocked proposal", Kind: memory.KindPreference,
		Sessions: []string{"s1"}, Author: owner, Origin: "mined",
	})
	first, err := bank.Enqueue(ctx, memory.QueueItem{Kind: memory.QueueRule, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(2 * time.Hour)
	second, err := bank.Enqueue(ctx, memory.QueueItem{Kind: memory.QueueRule, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	_ = second
	clock.Advance(time.Hour)
	if code, _ := decideRule(t, base, first, map[string]any{"status": "approved"}); code != http.StatusOK {
		t.Fatalf("approve = %d, want 200", code)
	}
	st := getQueueStats(t, base)
	if st.Pending != 1 || st.Approved != 1 || st.Total != 2 {
		t.Fatalf("stats = %+v, want 1 pending 1 approved 2 total", st)
	}
	if st.Budget != memory.QueueBudget || st.OverBudget {
		t.Fatalf("stats = %+v, want budget %d without overrun", st, memory.QueueBudget)
	}
	if st.AvgApprovalLatency != 3*time.Hour {
		t.Fatalf("avg latency = %v, want 3h on the fake clock", st.AvgApprovalLatency)
	}
	if st.OldestPendingAge != time.Hour {
		t.Fatalf("oldest pending = %v, want 1h on the fake clock", st.OldestPendingAge)
	}
	_ = d
}

// The rule queue never decides a profile_patch item: single and batch
// rejects both refuse it, and it stays pending.
func TestRuleQueueRejectRefusesNonRule(t *testing.T) {
	d, base, _, owner, _ := rulesDaemon(t)
	ctx := context.Background()
	bank := rulesBank(t, d, owner)
	profileID, err := bank.Enqueue(ctx, memory.QueueItem{Kind: memory.QueueProfilePatch, Payload: `{"op":"add"}`})
	if err != nil {
		t.Fatal(err)
	}
	if code, _ := decideRule(t, base, profileID, map[string]any{"status": "rejected"}); code != http.StatusBadRequest {
		t.Fatalf("rule reject of profile item = %d, want 400", code)
	}
	_, results := postBatch(t, base, map[string]any{"ids": []int64{profileID}, "status": "rejected"})
	if len(results) != 1 || results[0].Error == "" {
		t.Fatalf("batch reject of profile item = %+v, want one error", results)
	}
	for _, it := range getRulesQueue(t, base) {
		if it.ID == profileID {
			t.Fatalf("profile item %d leaked into the rule queue", profileID)
		}
	}
	items, _ := bank.Queue(ctx)
	for _, it := range items {
		if it.ID == profileID && it.Status != memory.QueuePending {
			t.Fatalf("profile item after refused rejects = %s, want still pending", it.Status)
		}
	}
	_ = d
}

// A promotion against a different observer's live Rule marks the proposal as
// a disagreement: the relation and the conflicting Rule travel with it for
// the human, instead of superseding.
func TestRuleQueuePromoteMarksDisagreement(t *testing.T) {
	d, base, _, owner, _ := rulesDaemon(t)
	ctx := context.Background()
	bank := rulesBank(t, d, owner)
	bob, err := bank.AddRule(ctx, memory.Rule{
		Text: "never wrap errors here", Kind: memory.KindPreference,
		SubjectKey: "wrap.errors", ApprovedBy: "bob",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range [][2]string{
		{"s1", "always wrap errors in this repo"},
		{"s2", "always wrap errors on every retry"},
	} {
		if _, err := bank.AddObservation(ctx, memory.Observation{
			SessionID: ev[0], EventID: "e1", Author: owner,
			Quote: ev[1], Kind: memory.KindPreference, SubjectKey: "wrap.errors",
		}); err != nil {
			t.Fatal(err)
		}
	}
	code, out := postPromote(t, base, "wrap.errors", "")
	if code != http.StatusCreated || out["promoted"] != true {
		t.Fatalf("promote = %d %v, want 201 promoted true", code, out)
	}
	queue := getRulesQueue(t, base)
	if len(queue) != 1 {
		t.Fatalf("queue = %d, want the disagreement item", len(queue))
	}
	proposal, err := memory.DecodeRememberProposal(queue[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if proposal.RelatedRuleID != bob.RuleID || proposal.Relation == "" {
		t.Fatalf("proposal = %+v, want the disagreement relation to bob's rule", proposal)
	}
}

// Queue budget and approval latency are recorded: every rule-queue mutation
// writes a ledger row with the counts, so the nightly summary reads stored
// rows instead of recomputing them.
func TestRuleQueueLedgerRecorded(t *testing.T) {
	d, base, id, owner, clock := rulesDaemon(t)
	ctx := context.Background()
	bank := rulesBank(t, d, owner)
	runPrompt(t, base, id, "the team uses tabs for indentation here")
	if code, _ := postRemember(t, base, id, map[string]any{
		"text": "use tabs for indentation", "reason": "r", "quote": "uses tabs for indentation",
	}); code != http.StatusCreated {
		t.Fatalf("remember = %d, want 201", code)
	}
	clock.Advance(time.Hour)
	qid := getRulesQueue(t, base)[0].ID
	if code, _ := decideRule(t, base, qid, map[string]any{"status": "approved"}); code != http.StatusOK {
		t.Fatalf("approve = %d, want 200", code)
	}
	ledger, err := bank.Ledger(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var approve *memory.LedgerRow
	for i := range ledger {
		if ledger[i].Stage == "queue_approve" {
			approve = &ledger[i]
		}
	}
	if approve == nil {
		t.Fatalf("ledger = %+v, want a queue_approve row", ledger)
	}
	var counts struct {
		Approved int     `json:"approved"`
		Total    int     `json:"total"`
		Budget   int     `json:"budget"`
		AvgHours float64 `json:"avgApprovalLatencyHours"`
	}
	if err := json.Unmarshal([]byte(approve.CountsJSON), &counts); err != nil {
		t.Fatal(err)
	}
	if counts.Approved != 1 || counts.Total != 1 || counts.Budget != memory.QueueBudget {
		t.Fatalf("approve counts = %+v, want 1 approved of 1 with the budget", counts)
	}
	if counts.AvgHours != 1 {
		t.Fatalf("avg latency = %v hours, want 1 on the fake clock", counts.AvgHours)
	}
	if code, _ := postRemember(t, base, id, map[string]any{
		"text": "use spaces", "reason": "r", "quote": "uses tabs for indentation",
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
	ledger, _ = bank.Ledger(ctx)
	found := false
	for _, l := range ledger {
		if l.Stage == "queue_reject" {
			found = true
		}
	}
	if !found {
		t.Fatalf("ledger = %+v, want a queue_reject row", ledger)
	}
	_ = d
}

// kindPreferenceOf picks the most common valid kind; ties keep the earliest
// leader and anything without a valid kind defaults to preference.
func TestKindPreferenceOf(t *testing.T) {
	if got := kindPreferenceOf(nil); got != memory.KindPreference {
		t.Fatalf("empty group = %q, want preference", got)
	}
	tie := []memory.Observation{
		{Kind: memory.KindFact}, {Kind: memory.KindPreference},
	}
	if got := kindPreferenceOf(tie); got != memory.KindFact {
		t.Fatalf("tie = %q, want the earliest leader fact", got)
	}
	swapped := []memory.Observation{
		{Kind: memory.KindPreference}, {Kind: memory.KindFact},
	}
	if got := kindPreferenceOf(swapped); got != memory.KindPreference {
		t.Fatalf("swapped tie = %q, want the earliest leader preference", got)
	}
	invalid := []memory.Observation{{Kind: "essay"}, {Kind: memory.KindDecision}}
	if got := kindPreferenceOf(invalid); got != memory.KindDecision {
		t.Fatalf("invalid skipped = %q, want decision", got)
	}
}

// Mixed ages converge: with an older and a newer same-observer Rule live, an
// in-between candidate supersedes the older one while landing stillborn
// itself, so exactly the newest text stays live.
func TestRuleQueueMixedAgesConverge(t *testing.T) {
	d, base, _, owner, clock := rulesDaemon(t)
	ctx := context.Background()
	bank := rulesBank(t, d, owner)
	old, err := bank.AddRule(ctx, memory.Rule{
		Text: "oldest wording", Kind: memory.KindPreference,
		SubjectKey: "mixed.key", ApprovedBy: owner,
	})
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Hour)
	midPayload := memory.EncodeRuleProposal(memory.RuleProposal{
		Text: "middle wording", Kind: memory.KindPreference, SubjectKey: "mixed.key",
		Sessions: []string{"s-mid"}, Author: owner, Origin: "mined",
	})
	midID, err := bank.Enqueue(ctx, memory.QueueItem{Kind: memory.QueueRule, Payload: midPayload})
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Hour)
	newer, err := bank.AddRule(ctx, memory.Rule{
		Text: "newest wording", Kind: memory.KindPreference,
		SubjectKey: "mixed.key", ApprovedBy: owner,
	})
	if err != nil {
		t.Fatal(err)
	}
	code, out := decideRule(t, base, midID, map[string]any{"status": "approved"})
	if code != http.StatusOK {
		t.Fatalf("approve middle = %d, want 200", code)
	}
	midRuleID, _ := out["RuleID"].(string)
	if versions, _ := bank.RuleVersions(ctx, old.RuleID); len(versions) != 1 || versions[0].Status != memory.StatusSuperseded {
		t.Fatalf("oldest rule = %+v, want superseded by the middle candidate", versions)
	}
	if versions, _ := bank.RuleVersions(ctx, midRuleID); len(versions) != 1 || versions[0].Status != memory.StatusSuperseded {
		t.Fatalf("middle rule = %+v, want stillborn superseded under the newest", versions)
	}
	if versions, _ := bank.RuleVersions(ctx, newer.RuleID); len(versions) != 1 || versions[0].Status != memory.StatusActive {
		t.Fatalf("newest rule = %+v, want the only live text", versions)
	}
	got, err := bank.Recall(ctx, "wording", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].RuleID != newer.RuleID {
		t.Fatalf("recall = %+v, want only the newest Rule live", got)
	}
}

// A batch that decides nothing writes no ledger row: an empty selection is
// refused up front, and an all-failed batch records nothing.
func TestRuleQueueBatchLedgerOnlyOnSuccess(t *testing.T) {
	d, base, _, owner, _ := rulesDaemon(t)
	ctx := context.Background()
	bank := rulesBank(t, d, owner)
	ledgerCount := func() int {
		ledger, _ := bank.Ledger(ctx)
		n := 0
		for _, l := range ledger {
			if l.Stage == "queue_batch" {
				n++
			}
		}
		return n
	}
	// A session with no pending items selects nothing and writes nothing.
	code, results := postBatch(t, base, map[string]any{"sessionId": "s-empty", "status": "approved"})
	if code != http.StatusOK || len(results) != 0 {
		t.Fatalf("empty batch = %d %+v, want 200 with no results", code, results)
	}
	if n := ledgerCount(); n != 0 {
		t.Fatalf("ledger holds %d batch rows after an empty batch, want none", n)
	}
	// An all-failed batch (a profile item picked by ID) writes nothing.
	profileID, err := bank.Enqueue(ctx, memory.QueueItem{Kind: memory.QueueProfilePatch, Payload: `{"op":"add"}`})
	if err != nil {
		t.Fatal(err)
	}
	code, results = postBatch(t, base, map[string]any{"ids": []int64{profileID}, "status": "rejected"})
	if code != http.StatusOK || len(results) != 1 || results[0].Error == "" {
		t.Fatalf("failed batch = %d %+v, want one error", code, results)
	}
	if n := ledgerCount(); n != 0 {
		t.Fatalf("ledger holds %d batch rows after an all-failed batch, want none", n)
	}
	// One success records the batch.
	payload := memory.EncodeRuleProposal(memory.RuleProposal{
		Text: "batch ledger probe", Kind: memory.KindPreference,
		Sessions: []string{"s1"}, Author: owner, Origin: "mined",
	})
	ruleID, err := bank.Enqueue(ctx, memory.QueueItem{Kind: memory.QueueRule, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	code, results = postBatch(t, base, map[string]any{"ids": []int64{profileID, ruleID}, "status": "approved"})
	if code != http.StatusOK || len(results) != 2 {
		t.Fatalf("mixed batch = %d %+v, want 2 results", code, results)
	}
	if results[0].Error == "" || results[1].RuleID == "" {
		t.Fatalf("mixed batch = %+v, want the profile error plus the rule", results)
	}
	if n := ledgerCount(); n != 1 {
		t.Fatalf("ledger holds %d batch rows after a succeeding batch, want 1", n)
	}
	_ = d
}
