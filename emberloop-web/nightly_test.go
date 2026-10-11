package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"example.com/emberloop-web/memory"
)

// nightlyTestDaemon starts a Daemon with the fake clock and the nightly LLM
// seams scripted. A nil reply keeps the seam unset.
func nightlyTestDaemon(t *testing.T, distillReply *string, profileReply *string) (*Daemon, *fakeClock) {
	t.Helper()
	t.Setenv("PIG_TEST_FAUX", "1")
	useSettings(t, `{"decider_mode":"heuristic","memory_model":"test-faux/distill"}`)
	d := NewDaemon(tempDir(t), NewFakeBackend())
	clock := newFakeClock()
	d.clock = clock.get
	if distillReply != nil {
		reply := *distillReply
		d.distillOneShot = func(model, system, user string) (string, error) {
			return reply, nil
		}
	}
	if profileReply != nil {
		reply := *profileReply
		d.profileOneShot = func(model, system, user string) (string, error) {
			return reply, nil
		}
	}
	return d, clock
}

// nightlySettle advances the fake clock past seeded Session timestamps:
// seedEndedSession stamps last with the real clock, so the fake clock must
// move forward or Idle(0) trails it by milliseconds.
func nightlySettle(clock *fakeClock) {
	clock.advance(2 * time.Second)
}

func nightlyQueueLen(t *testing.T, d *Daemon, owner string) int {
	t.Helper()
	bank, err := d.profileBankFor(owner)
	if err != nil {
		t.Fatal(err)
	}
	items, err := bank.Queue(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return len(items)
}

func nightlyLedgerLen(t *testing.T, d *Daemon, owner string) int {
	t.Helper()
	bank, err := d.profileBankFor(owner)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := bank.Ledger(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return len(rows)
}

// TestNightlyRunsOnlyWhenIdleAndLogsPerStage pins the idle gate and the
// ledger-row-per-stage contract.
func TestNightlyRunsOnlyWhenIdleAndLogsPerStage(t *testing.T) {
	flush := "FLUSH_OK"
	none := "none"
	d, clock := nightlyTestDaemon(t, &flush, &none)
	seedEndedSession(t, d, "idle-s1", "alice", []Event{userPrompt(1, "alice", "No, use tabs here")})
	nightlySettle(clock)

	res, err := d.RunNightly(context.Background())
	if err != nil {
		t.Fatalf("RunNightly = %v, want success on an idle Daemon", err)
	}
	if len(res.Stages) != len(nightlyStages) {
		t.Fatalf("stages = %d, want %d (%v)", len(res.Stages), len(nightlyStages), res.Stages)
	}
	for i, want := range nightlyStages {
		if res.Stages[i].Stage != want {
			t.Fatalf("stage %d = %q, want %q", i, res.Stages[i].Stage, want)
		}
		if res.Stages[i].Status == "" {
			t.Fatalf("stage %q has no ledger status", want)
		}
	}
	bank, err := d.profileBankFor("alice")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := bank.Ledger(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]int)
	for _, r := range rows {
		if r.RunID == res.RunID {
			seen[r.Stage]++
			if r.Status != memory.LedgerSuccess && r.Status != memory.LedgerNoData && r.Status != memory.LedgerFailed {
				t.Fatalf("ledger stage %q status = %q, want a ledger status", r.Stage, r.Status)
			}
			if !json.Valid([]byte(r.CountsJSON)) {
				t.Fatalf("ledger stage %q counts = %q, want JSON", r.Stage, r.CountsJSON)
			}
		}
	}
	for _, want := range nightlyStages {
		if seen[want] != 1 {
			t.Fatalf("ledger rows for stage %q = %d, want 1 (have %v)", want, seen[want], seen)
		}
	}

	// A busy Daemon aborts with no ledger write.
	d2, _ := nightlyTestDaemon(t, &flush, &none)
	d2.sessions["busy"] = &session{live: true, busy: true, notify: make(chan struct{}), last: time.Now().UTC()}
	if _, err := d2.RunNightly(context.Background()); err == nil {
		t.Fatal("RunNightly on a busy Daemon = nil, want an error")
	} else if !strings.Contains(err.Error(), "busy") {
		t.Fatalf("busy error = %q, want a busy message", err.Error())
	}
}

// TestNightlyFailingStageLeavesLaterStagesRunning pins continue-on-failure
// with visibility on the ledger.
func TestNightlyFailingStageLeavesLaterStagesRunning(t *testing.T) {
	flush := "FLUSH_OK"
	none := "none"
	d, clock := nightlyTestDaemon(t, &flush, &none)
	d.nightlyFailStage = nightlyStagePair
	seedEndedSession(t, d, "pair-s1", "alice", []Event{userPrompt(1, "alice", "No, use tabs here")})
	nightlySettle(clock)

	res, err := d.RunNightly(context.Background())
	if err != nil {
		t.Fatalf("RunNightly with a failing pair stage = %v, want success with later stages running", err)
	}
	byStage := make(map[string]NightlyStageResult)
	for _, s := range res.Stages {
		byStage[s.Stage] = s
	}
	if got := byStage[nightlyStagePair].Status; got != memory.LedgerFailed {
		t.Fatalf("pair stage = %q, want FAILED", got)
	}
	for _, want := range []string{nightlyStageProfile, nightlyStageAnchors, nightlyStageSummary} {
		s, ok := byStage[want]
		if !ok {
			t.Fatalf("stage %q missing after a pair failure: %+v", want, res.Stages)
		}
		if s.Status == "" {
			t.Fatalf("stage %q has no status after a pair failure", want)
		}
	}
	bank, _ := d.profileBankFor("alice")
	rows, _ := bank.Ledger(context.Background())
	var pairRow, summaryRow *memory.LedgerRow
	for i := range rows {
		if rows[i].RunID != res.RunID {
			continue
		}
		if rows[i].Stage == nightlyStagePair {
			pairRow = &rows[i]
		}
		if rows[i].Stage == nightlyStageSummary {
			summaryRow = &rows[i]
		}
	}
	if pairRow == nil || pairRow.Status != memory.LedgerFailed {
		t.Fatalf("pair ledger row = %+v, want FAILED", pairRow)
	}
	if summaryRow == nil || summaryRow.Status != memory.LedgerSuccess {
		t.Fatalf("summary ledger row = %+v, want SUCCESS after a pair failure", summaryRow)
	}
}

// TestNightlySnapshotFailureAbortsBeforeAnyWrite pins the abort: no mine
// writes and no later ledger rows.
func TestNightlySnapshotFailureAbortsBeforeAnyWrite(t *testing.T) {
	flush := "FLUSH_OK"
	none := "none"
	d, clock := nightlyTestDaemon(t, &flush, &none)
	d.nightlyFailStage = nightlyStageSnapshot
	seedEndedSession(t, d, "snap-s1", "alice", []Event{userPrompt(1, "alice", "No, use tabs here for snap")})
	nightlySettle(clock)

	bank, _ := d.profileBankFor("alice")
	liveBefore, _ := bank.LiveRules(context.Background())
	queueBefore := nightlyQueueLen(t, d, "alice")
	ledgerBefore := nightlyLedgerLen(t, d, "alice")
	obsBefore := len(bankQuotes(t, d, "alice", "snap"))

	res, err := d.RunNightly(context.Background())
	if err == nil || !res.Aborted {
		t.Fatalf("snapshot-failed run = %+v %v, want an aborted error", res, err)
	}
	if len(res.Stages) != 1 || res.Stages[0].Stage != nightlyStageSnapshot {
		t.Fatalf("aborted stages = %+v, want only the snapshot stage", res.Stages)
	}
	if obsAfter := len(bankQuotes(t, d, "alice", "snap")); obsAfter != obsBefore {
		t.Fatalf("observations = %d, want %d: a snapshot failure must abort before any mine write", obsAfter, obsBefore)
	}
	liveAfter, _ := bank.LiveRules(context.Background())
	if len(liveAfter) != len(liveBefore) || nightlyQueueLen(t, d, "alice") != queueBefore {
		t.Fatalf("rules/queue changed after an aborted run")
	}
	rows, _ := bank.Ledger(context.Background())
	if len(rows) <= ledgerBefore {
		t.Fatalf("ledger rows = %d, want more than %d: the failed snapshot stays visible", len(rows), ledgerBefore)
	}
	for _, r := range rows {
		if r.RunID == res.RunID && r.Stage != nightlyStageSnapshot {
			t.Fatalf("run %s has a %q row after a snapshot abort, want snapshot only", res.RunID, r.Stage)
		}
	}
}

// TestNightlyFlushOKYieldsNoQueueItem pins the distill sentinel: a Session
// with new observations but nothing durable enqueues nothing.
func TestNightlyFlushOKYieldsNoQueueItem(t *testing.T) {
	flush := "FLUSH_OK"
	none := "none"
	d, clock := nightlyTestDaemon(t, &flush, &none)
	seedEndedSession(t, d, "flush-s1", "alice", []Event{userPrompt(1, "alice", "No, use tabs here for flush")})
	nightlySettle(clock)

	nQueueBefore := nightlyQueueLen(t, d, "alice")

	res, err := d.RunNightly(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var distill *NightlyStageResult
	for i := range res.Stages {
		if res.Stages[i].Stage == nightlyStageDistill {
			distill = &res.Stages[i]
		}
	}
	if distill == nil {
		t.Fatal("no distill stage in the run")
	}
	flushN := 0
	if n, ok := distill.Counts["flush_ok"].(int); ok {
		flushN = n
	} else if f, ok := distill.Counts["flush_ok"].(float64); ok {
		flushN = int(f)
	}
	if flushN < 1 {
		t.Fatalf("distill counts = %v, want at least one FLUSH_OK", distill.Counts)
	}
	if got := nightlyQueueLen(t, d, "alice"); got != nQueueBefore {
		t.Fatalf("queue items = %d, want %d: FLUSH_OK must enqueue nothing", got, nQueueBefore)
	}
}

// TestNightlyDistillInputIsRedactedAndCapped pins the 15 KB redacted cap at
// the distill egress.
func TestNightlyDistillInputIsRedactedAndCapped(t *testing.T) {
	var got string
	d, clock := nightlyTestDaemon(t, nil, nil)
	d.distillOneShot = func(model, system, user string) (string, error) {
		got = user
		return "FLUSH_OK", nil
	}
	none := "none"
	d.profileOneShot = func(model, system, user string) (string, error) { return none, nil }
	long := "remember: the password is hunter2-2026, always use tabs in this repo. " + strings.Repeat("filler notes about tabs ", 800)
	seedEndedSession(t, d, "cap-s1", "alice", []Event{userPrompt(1, "alice", long)})
	nightlySettle(clock)

	if _, err := d.RunNightly(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got == "" {
		t.Fatal("distill was never called for a Session with new observations")
	}
	if strings.Contains(got, "hunter2") {
		t.Fatalf("distill input holds the raw secret: %.120q", got)
	}
	if !strings.Contains(got, "[redacted:") {
		t.Fatalf("distill input lacks a redaction marker: %.120q", got)
	}
	if len(got) > nightlyDistillCap {
		t.Fatalf("distill input is %d bytes, want at most %d", len(got), nightlyDistillCap)
	}
}

// TestNightlyDeletesNothing asserts row counts before and after: Rules,
// queue and Profile versions never shrink; only ledger grows.
func TestNightlyDeletesNothing(t *testing.T) {
	brief := "Prefer tabs for indentation."
	ops, _ := json.Marshal(map[string]any{"ops": []any{map[string]any{"op": "add", "text": "nightly keeps tabs", "tag": "workflow"}}})
	profileReply := string(ops)
	d, clock := nightlyTestDaemon(t, &brief, &profileReply)
	seedEndedSession(t, d, "keep-s1", "alice", []Event{userPrompt(1, "alice", "No, use tabs here for keeps")})
	seedEndedSession(t, d, "keep-s2", "alice", []Event{userPrompt(1, "alice", "No, use tabs here for keeps again")})
	nightlySettle(clock)

	bank, _ := d.profileBankFor("alice")
	liveBefore, _ := bank.LiveRules(context.Background())
	queueBefore := nightlyQueueLen(t, d, "alice")
	ledgerBefore := nightlyLedgerLen(t, d, "alice")
	histBefore, _ := bank.ProfileHistory(context.Background())
	obsBefore := len(bankQuotes(t, d, "alice", "tabs"))

	if _, err := d.RunNightly(context.Background()); err != nil {
		t.Fatal(err)
	}
	obsAfter := len(bankQuotes(t, d, "alice", "tabs"))
	liveAfter, _ := bank.LiveRules(context.Background())
	histAfter, _ := bank.ProfileHistory(context.Background())
	if obsAfter < obsBefore {
		t.Fatalf("observations shrank %d -> %d", obsBefore, obsAfter)
	}
	if len(liveAfter) < len(liveBefore) {
		t.Fatalf("rules shrank %d -> %d", len(liveBefore), len(liveAfter))
	}
	if got := nightlyQueueLen(t, d, "alice"); got < queueBefore {
		t.Fatalf("queue shrank %d -> %d", queueBefore, got)
	}
	if len(histAfter) < len(histBefore) {
		t.Fatalf("profile versions shrank %d -> %d", len(histBefore), len(histAfter))
	}
	if got := nightlyLedgerLen(t, d, "alice"); got <= ledgerBefore {
		t.Fatalf("ledger rows = %d, want more than %d", got, ledgerBefore)
	}
}

// TestNightlyWithFakeMemoryDeciderAndClock drives the pairing helper with the
// in-memory fake, the scripted Decider and the fake clock.
func TestNightlyWithFakeMemoryDeciderAndClock(t *testing.T) {
	clock := newFakeClock()
	fake := memory.NewFake(memory.Options{Now: clock.get})
	ctx := context.Background()
	rule, err := fake.AddRule(ctx, memory.Rule{Text: "always use tabs", Kind: memory.KindPreference, Status: memory.StatusActive, SubjectKey: "use.tabs", ApprovedBy: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	proposal := memory.EncodeRuleProposal(memory.RuleProposal{Text: "always use tabs here", Kind: memory.KindPreference, SubjectKey: "use.tabs", Quotes: []string{"always use tabs here"}, Sessions: []string{"s1"}, Author: "alice", Origin: "mined"})
	if _, err := fake.Enqueue(ctx, memory.QueueItem{Kind: memory.QueueRule, Payload: proposal}); err != nil {
		t.Fatal(err)
	}
	dec := &memory.ScriptedDecider{Durable: 0.9, Kind: memory.KindPreference, Scope: "user_rule"}
	qs := []memory.DeciderQuestion{memory.Noul(memory.QuestionDurable)}
	if _, err := dec.Decide(ctx, memory.DeciderState{Statement: "always use tabs here"}, qs); err != nil {
		t.Fatalf("scripted decider = %v, want an answer", err)
	}
	items, err := fake.Queue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	paired := 0
	for _, it := range items {
		p, err := memory.DecodeRememberProposal(it.Payload)
		if err != nil {
			t.Fatal(err)
		}
		same, err := fake.RulesBySubjectKey(ctx, p.SubjectKey)
		if err != nil {
			t.Fatal(err)
		}
		if related, _ := memory.PairCandidate(p.Text, p.SubjectKey, append(same, rule)); related != "" {
			paired++
		}
	}
	if paired != 1 {
		t.Fatalf("paired = %d, want 1 on the fake bank", paired)
	}
	live, err := fake.LiveRules(ctx)
	if err != nil || len(live) != 1 || live[0].RuleID != rule.RuleID {
		t.Fatalf("live rules = %+v %v, want the one stored rule", live, err)
	}
	clock.advance(7 * time.Hour)
	if _, err := fake.Snapshot(ctx); err != nil {
		t.Fatalf("fake snapshot = %v", err)
	}
}

// TestNightlyOnDemandRoutes covers the Memory-page seams: POST runs the job,
// GET serves the own-bank ledger, and both need a User.
func TestNightlyOnDemandRoutes(t *testing.T) {
	flush := "FLUSH_OK"
	none := "none"
	d, clock := nightlyTestDaemon(t, &flush, &none)
	seedEndedSession(t, d, "route-s1", "alice", []Event{userPrompt(1, "alice", "No, use tabs here for routes")})
	nightlySettle(clock)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/memory/nightly", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/memory/nightly = %d, want 200", resp.StatusCode)
	}
	var res NightlyResult
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatal(err)
	}
	if len(res.Stages) != len(nightlyStages) {
		t.Fatalf("on-demand stages = %d, want %d", len(res.Stages), len(nightlyStages))
	}
	led, err := http.Get(srv.URL + "/api/memory/ledger")
	if err != nil {
		t.Fatal(err)
	}
	defer led.Body.Close()
	if led.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/memory/ledger = %d, want 200", led.StatusCode)
	}
	var rows []memory.LedgerRow
	if err := json.NewDecoder(led.Body).Decode(&rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("ledger rows = 0, want the nightly rows")
	}
	// Without a cookie both routes answer 401 like every other /api route.
	plain := httptest.NewServer(d.Handler())
	defer plain.Close()
	if r, _ := http.Post(plain.URL+"/api/memory/nightly", "application/json", nil); r.StatusCode != http.StatusUnauthorized {
		r.Body.Close()
		t.Fatalf("unauth POST nightly = %d, want 401", r.StatusCode)
	} else {
		r.Body.Close()
	}
	if r, _ := http.Get(plain.URL + "/api/memory/ledger"); r.StatusCode != http.StatusUnauthorized {
		r.Body.Close()
		t.Fatalf("unauth GET ledger = %d, want 401", r.StatusCode)
	} else {
		r.Body.Close()
	}
}

// TestNightlyDue fires once per local day at 03:00.
func TestNightlyDue(t *testing.T) {
	day := time.Date(2026, 10, 11, 3, 0, 0, 0, time.Local)
	if due, _ := nightlyDue(day, ""); !due {
		t.Fatal("03:00 with no run today is not due")
	}
	if due, _ := nightlyDue(day, day.Local().Format("2006-01-02")); due {
		t.Fatal("03:00 after today's run is due again")
	}
	if due, _ := nightlyDue(time.Date(2026, 10, 11, 4, 0, 0, 0, time.Local), ""); due {
		t.Fatal("04:00 is due")
	}
}

// TestNightlyPairAnnotatesNewCandidates pins the spec pairing step: a pending
// rule proposal that arrived without a relation gets the FTS5 relation to its
// overlapping stored Rule written back onto its queue payload. A second run
// leaves it alone.
func TestNightlyPairAnnotatesNewCandidates(t *testing.T) {
	flush := "FLUSH_OK"
	none := "none"
	d, clock := nightlyTestDaemon(t, &flush, &none)
	nightlySettle(clock)
	ctx := context.Background()
	bank, err := d.profileBankFor("alice")
	if err != nil {
		t.Fatal(err)
	}
	rule, err := bank.AddRule(ctx, memory.Rule{Text: "always use tabs", Kind: memory.KindPreference, Status: memory.StatusActive, SubjectKey: "use.tabs", ApprovedBy: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	pairable, err := bank.Enqueue(ctx, memory.QueueItem{Kind: memory.QueueRule, Payload: memory.EncodeRuleProposal(memory.RuleProposal{
		Text: "always use tabs here", Kind: memory.KindPreference, SubjectKey: "use.tabs",
		Quotes: []string{"always use tabs here"}, Sessions: []string{"s1"}, Author: "alice", Origin: "mined",
	})})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bank.Enqueue(ctx, memory.QueueItem{Kind: memory.QueueRule, Payload: memory.EncodeRuleProposal(memory.RuleProposal{
		Text: "purple giraffes orbit mars at dawn", Kind: memory.KindFact, SubjectKey: "purple.giraffe.orbit",
		Quotes: []string{"purple giraffes orbit mars at dawn"}, Sessions: []string{"s9"}, Author: "alice", Origin: "mined",
	})}); err != nil {
		t.Fatal(err)
	}
	res := d.nightlyPair(ctx, "pair-test-run", []string{"alice"})
	if res.Status != memory.LedgerSuccess {
		t.Fatalf("pair stage = %+v, want SUCCESS", res)
	}
	if n, _ := res.Counts["annotated"].(int); n != 1 {
		t.Fatalf("annotated = %v, want 1", res.Counts)
	}
	items, err := bank.Queue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.ID != pairable {
			continue
		}
		got, err := memory.DecodeRememberProposal(it.Payload)
		if err != nil {
			t.Fatal(err)
		}
		if got.Relation == "" || got.RelatedRuleID != rule.RuleID {
			t.Fatalf("pairable payload relation = %q %q, want a relation to %s", got.Relation, got.RelatedRuleID, rule.RuleID)
		}
	}
	again := d.nightlyPair(ctx, "pair-test-rerun", []string{"alice"})
	if n, _ := again.Counts["annotated"].(int); n != 0 {
		t.Fatalf("rerun annotated = %v, want 0: paired proposals are left untouched", again.Counts)
	}
	if n, _ := again.Counts["paired"].(int); n != 1 {
		t.Fatalf("rerun paired = %v, want 1", again.Counts)
	}
}

// TestNightlyDistillEmptyTextIsFailure pins that only the cheap LLM may
// return FLUSH_OK: a Session with new observations but no miner-readable text
// counts as a failure, never as a sentinel.
func TestNightlyDistillEmptyTextIsFailure(t *testing.T) {
	flush := "FLUSH_OK"
	none := "none"
	d, _ := nightlyTestDaemon(t, &flush, &none)
	_, _, res := d.nightlyDistill(context.Background(), "distill-empty-run", []string{"alice"},
		[]MineResult{{SessionID: "ghost-session", Stored: 1}})
	if res.Status != memory.LedgerFailed {
		t.Fatalf("distill stage = %+v, want FAILED", res)
	}
	flushN := 0
	if n, ok := res.Counts["flush_ok"].(int); ok {
		flushN = n
	}
	if flushN != 0 {
		t.Fatalf("flush_ok = %v, want 0: no LLM call means no sentinel", res.Counts)
	}
	if n, _ := res.Counts["failed"].(int); n != 1 {
		t.Fatalf("failed = %v, want 1", res.Counts)
	}
}
