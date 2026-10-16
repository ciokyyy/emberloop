package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"example.com/emberloop-web/memory"
)

// cannedPrompt builds a miner-readable user prompt Event.
func cannedPrompt(seq int64, text string) Event {
	return Event{Kind: "prompt", Sender: senderUser, User: "u1", Text: text, Seq: seq}
}

func fakeBankFactory() func() memory.Memory {
	return func() memory.Memory { return memory.NewFake(memory.Options{}) }
}

func TestReplayEvaluateCannedLogs(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	sessions := []ReplaySession{
		{ID: "s1", CreatedAt: base, Events: []Event{
			cannedPrompt(1, "remember: always return wrapped errors"),
		}},
		{ID: "s2", CreatedAt: base.Add(time.Hour), Events: []Event{
			cannedPrompt(1, "remember: always return wrapped errors in handlers"),
		}},
		{ID: "s3", CreatedAt: base.Add(2 * time.Hour), Events: []Event{
			cannedPrompt(1, "What time is lunch?"),
			cannedPrompt(2, "remember: use tabs for indentation"),
		}},
	}
	res, cases, err := ReplayEvaluate(sessions, fakeBankFactory())
	if err != nil {
		t.Fatal(err)
	}
	// s1's correction has no prior (miss), s2 refines s1 (hit), s3's tabs
	// note is novel (miss). The lunch question is never a case.
	if res.Cases != 3 {
		t.Fatalf("cases = %d, want 3 (lunch is not a correction)", res.Cases)
	}
	if res.Covered != 1 || res.RecallAt5 != 1.0/3.0 {
		t.Fatalf("Rules arm = %+v, want one of three covered", res)
	}
	if res.MemoryCovered != 1 || res.MemoryRecallAt5 != 1.0/3.0 {
		t.Fatalf("Memories arm = %+v, want one of three covered", res)
	}
	if res.RepeatTotal != 3 || res.RepeatCovered != 1 || res.RepeatRate != 1.0/3.0 {
		t.Fatalf("repeat = %+v, want total 3 covered 1 rate 1/3", res)
	}
	if len(cases) != 3 || !cases[1].Covered || cases[0].Covered || cases[2].Covered || !cases[1].MemoryCovered || cases[0].MemoryCovered || cases[2].MemoryCovered {
		t.Fatalf("same-case arms = %+v, want only middle case covered in Rules and Memories", cases)
	}
	report := FormatReplayReport(res, 3, 0, 0, 0, false)
	if !strings.Contains(report, "recall@5: 0.333") || !strings.Contains(report, "recall@5 (Memories): 0.333") {
		t.Fatalf("replay report omits an arm: %q", report)
	}
}

func TestReplayCompareGapOnSameCases(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	covering := "remember: always return wrapped errors"
	distractors := []string{
		"remember: always return early from handlers with context",
		"remember: always return cached values for dashboard widgets",
		"remember: always return copies of request buffers upstream",
		"remember: always return fallback profiles for guest sessions",
		"remember: always return paginated rows for admin tables",
	}
	var priors []Event
	priors = append(priors, cannedPrompt(1, covering))
	for i, d := range distractors {
		priors = append(priors, cannedPrompt(int64(i+2), d))
	}
	sessions := []ReplaySession{
		{ID: "s1", CreatedAt: base, Events: priors},
		{ID: "s2", CreatedAt: base.Add(time.Hour), Events: []Event{
			cannedPrompt(1, covering),
		}},
	}
	// Sanity: distractors must not cover the query, or the gap is meaningless.
	for _, d := range distractors {
		if memory.CoversRule(d, covering) {
			t.Fatalf("distractor %q covers the query", d)
		}
	}
	// The embedder demotes the covering Rule below five distractors, so the
	// same FTS top-20 gives FTS cover and embedder miss.
	scores := map[string]map[string]float64{"s2:1": {}}
	for _, d := range distractors {
		scores["s2:1"][d] = 1.0
	}
	scores["s2:1"][covering] = 0.0
	fts, emb, gap, res, err := ReplayCompare(sessions, fakeBankFactory(), scores)
	if err != nil {
		t.Fatal(err)
	}
	// Seven cases: the six s1 priors (first with no memory, the rest novel)
	// plus the s2 duplicate. Only the s2 duplicate is FTS-covered; the
	// embedder demotes it out of its top 5 on the same top-20.
	if res.Cases != 7 {
		t.Fatalf("cases = %d, want 7", res.Cases)
	}
	if fts != 1.0/7.0 || emb != 0 || gap != -1.0/7.0 {
		t.Fatalf("fts=%v emb=%v gap=%v, want %v 0 %v", fts, emb, gap, 1.0/7.0, -1.0/7.0)
	}
}

func TestAssignRecallOff(t *testing.T) {
	t.Parallel()
	if AssignRecallOff(0, 0) || AssignRecallOff(5, 0) || AssignRecallOff(0, -1) {
		t.Fatalf("non-positive N must never assign")
	}
	// Every 2nd worker: indices 1, 3 off; 0, 2 on.
	if AssignRecallOff(0, 2) || !AssignRecallOff(1, 2) || AssignRecallOff(2, 2) || !AssignRecallOff(3, 2) {
		t.Fatalf("every-2 assignment wrong")
	}
	if !AssignRecallOff(2, 3) || AssignRecallOff(0, 3) {
		t.Fatalf("every-3 assignment wrong")
	}
}

func TestNoRecallSkipsRecallBlock(t *testing.T) {
	t.Parallel()
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	meta := SessionMeta{Owner: "u1", NoRecall: true}
	rb, refs, err := d.recallBlockFor(context.Background(), meta, "remember: always wrap errors")
	if err != nil {
		t.Fatal(err)
	}
	if len(rb) != 0 || len(refs) != 0 {
		t.Fatalf("NoRecall block = %v %v, want no Rules (Profile only)", rb, refs)
	}
}

func TestRecallBlockInjectsOnlyActiveMemoriesAndTracksIDs(t *testing.T) {
	t.Parallel()
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	bank, err := d.profileBankFor("u1")
	if err != nil {
		t.Fatal(err)
	}
	add := func(text, status string) {
		_, e := bank.AddMemory(context.Background(), memory.MemoryItem{SessionID: "s", EventID: text, Author: "u1", Kind: memory.KindPreference, Text: text, Quote: text, Status: status, Veracity: memory.VeracityStated})
		if e != nil {
			t.Fatal(e)
		}
	}
	add("Prefer blue lanterns", memory.StatusActive)
	add("Forgot blue lanterns", memory.StatusForgotten)
	add("Parked blue lanterns", memory.StatusParked)
	add("Superseded blue lanterns", memory.StatusSuperseded)
	if _, err := bank.AddEpisode(context.Background(), memory.Episode{SessionID: "s", Author: "u1", Text: "Episode-only blue lantern detail"}); err != nil {
		t.Fatal(err)
	}
	rb, refs, err := d.recallBlockFor(context.Background(), SessionMeta{Owner: "u1"}, "blue lanterns")
	if err != nil {
		t.Fatal(err)
	}
	if len(rb) != 1 || !strings.Contains(rb[0], "Prefer blue lanterns") || strings.Contains(rb[0], "Forgot blue lanterns") || strings.Contains(rb[0], "Parked blue lanterns") || strings.Contains(rb[0], "Superseded blue lanterns") || strings.Contains(rb[0], "Episode-only") {
		t.Fatalf("block=%v", rb)
	}
	if len(refs) != 1 || refs[0].MemoryID == 0 {
		t.Fatalf("memory refs=%+v", refs)
	}
}

func TestRecallOffNIsARunSettingInTheRunFile(t *testing.T) {
	t.Parallel()
	repo, spec := initRunRepo(t, demoTickets())
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	body, _ := json.Marshal(map[string]any{"spec": spec, "repo": repo, "harness": "fake", "model": "fake-model", "recallOffN": 2})
	resp, err := http.Post(srv.URL+"/api/runs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("start status = %d, want 201", resp.StatusCode)
	}
	var out struct {
		Run map[string]any `json:"run"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Run["recallOffN"] != float64(2) {
		t.Fatalf("run recallOffN = %v, want 2", out.Run["recallOffN"])
	}
	id, _ := out.Run["id"].(string)
	raw, err := os.ReadFile(filepath.Join(tempDirOf(d), "enderbloom", "runs", id+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		RecallOffN int             `json:"recallOffN"`
		RecallOff  map[string]bool `json:"recallOff"`
	}
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.RecallOffN != 2 {
		t.Fatalf("file recallOffN = %d, want 2", saved.RecallOffN)
	}

	// Bounds: -1 and 1 are refused (1 leaves no control arm); large N is
	// allowed and selects nothing in a small run. Each case uses a fresh
	// repo so the active-run refusal cannot mask the bound.
	for _, tc := range []struct {
		n    int
		want int
	}{
		{-1, http.StatusBadRequest},
		{1, http.StatusBadRequest},
		{1000, http.StatusCreated},
	} {
		repo2, spec2 := initRunRepo(t, demoTickets())
		payload, _ := json.Marshal(map[string]any{"spec": spec2, "repo": repo2, "harness": "fake", "model": "fake-model", "recallOffN": tc.n})
		r, err := http.Post(srv.URL+"/api/runs", "application/json", bytes.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		if r.StatusCode != tc.want {
			t.Fatalf("recallOffN=%d status = %d, want %d", tc.n, r.StatusCode, tc.want)
		}
	}
}

// tempDirOf returns the Daemon's state dir for the Run-file read.
func tempDirOf(d *Daemon) string { return d.stateDir }

func TestLoadReplaySessionsFiltersProject(t *testing.T) {
	t.Parallel()
	stateDir := tempDir(t)
	writeReplaySession(t, stateDir, "s1", "proj-a", time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC), []string{
		`{"kind":"prompt","sender":"user","user":"u1","text":"remember: always wrap errors"}`,
		`not json`,
	})
	writeReplaySession(t, stateDir, "s2", "proj-b", time.Date(2026, 10, 11, 10, 0, 0, 0, time.UTC), []string{
		`{"kind":"prompt","sender":"user","user":"u1","text":"remember: use tabs"}`,
	})
	all, err := LoadReplaySessions(stateDir, "", time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("sessions = %d, want 2", len(all))
	}
	if len(all[0].Events) != 1 {
		t.Fatalf("s1 events = %d, want 1 (corrupt line skipped)", len(all[0].Events))
	}
	proj, err := LoadReplaySessions(stateDir, "proj-a", time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(proj) != 1 || proj[0].ID != "s1" {
		t.Fatalf("project filter = %+v, want only s1", proj)
	}
	// The period bounds CreatedAt: since inclusive, until exclusive.
	morning, err := LoadReplaySessions(stateDir, "", time.Date(2026, 10, 11, 9, 30, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(morning) != 1 || morning[0].ID != "s2" {
		t.Fatalf("since filter = %+v, want only s2", morning)
	}
	evening, err := LoadReplaySessions(stateDir, "", time.Time{}, time.Date(2026, 10, 11, 9, 30, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(evening) != 1 || evening[0].ID != "s1" {
		t.Fatalf("until filter = %+v, want only s1", evening)
	}
}

func writeReplaySession(t *testing.T, stateDir, id, project string, at time.Time, lines []string) {
	t.Helper()
	dir := filepath.Join(stateDir, "enderbloom", "sessions", id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	meta, _ := json.Marshal(SessionMeta{ID: id, ProjectID: project, CreatedAt: at, Status: "ended"})
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), meta, 0o644); err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	for _, l := range lines {
		b.WriteString(l + "\n")
	}
	if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDeciderEvalOnCannedLabels(t *testing.T) {
	t.Parallel()
	raw := []byte("{\"id\":\"a\",\"text\":\"remember: always wrap errors\",\"durable\":true}\n" +
		"{\"id\":\"b\",\"text\":\"What time is lunch?\",\"durable\":false}\n" +
		"{\"id\":\"c\",\"text\":\"remember: never wrap errors\",\"durable\":true}\n")
	labels, err := memory.ParseDeciderLabels(raw)
	if err != nil {
		t.Fatal(err)
	}
	// Oversample heuristic hits: two of three labels are corrections.
	scores := map[string]memory.DeciderScores{
		"llm": {"a": 0.9, "b": 0.8, "c": 0.2},
		"jev": {"a": 0.9, "b": 0.1, "c": 0.9},
	}
	evals := memory.ScoreEvalDeciders(labels, scores, memory.DefaultConfirmHigh)
	byName := map[string]memory.DeciderEval{}
	for _, e := range evals {
		byName[e.Decider] = e
	}
	if len(evals) != 4 {
		t.Fatalf("evals = %+v, want heuristic, llm, jev, laya", evals)
	}
	// llm predicts a (TP) and b (FP): precision 1/2.
	if byName["llm"].Precision != 0.5 {
		t.Fatalf("llm = %+v, want precision 0.5", byName["llm"])
	}
	// jev predicts a and c (both TP): precision 1.
	if byName["jev"].Precision != 1 {
		t.Fatalf("jev = %+v, want precision 1", byName["jev"])
	}
	// laya never scored: unavailable, never a yes.
	if byName["laya"].Scored != 0 || byName["laya"].Predicted != 0 {
		t.Fatalf("laya = %+v, want unscored", byName["laya"])
	}
	// Heuristic ran live on every label.
	if byName["heuristic"].Scored != 3 {
		t.Fatalf("heuristic = %+v, want all 3 scored live", byName["heuristic"])
	}
}

func TestRecallOffForTicketFollowsTicketOrder(t *testing.T) {
	t.Parallel()
	tickets := []staticTicket{{ID: "01-first"}, {ID: "02-second"}, {ID: "03-third"}}
	// Every 2nd ticket in ticket order: 02 only, whatever order Workers start.
	if RecallOffForTicket(tickets, "01-first", 2) || !RecallOffForTicket(tickets, "02-second", 2) || RecallOffForTicket(tickets, "03-third", 2) {
		t.Fatalf("every-2 ticket arm wrong")
	}
	if !RecallOffForTicket(tickets, "03-third", 3) || RecallOffForTicket(tickets, "01-first", 3) {
		t.Fatalf("every-3 ticket arm wrong")
	}
	if RecallOffForTicket(tickets, "01-first", 0) || RecallOffForTicket(tickets, "missing", 2) {
		t.Fatalf("disabled N and unknown IDs never match")
	}
}

func TestQueueLabelsForUserReadsDecidedRows(t *testing.T) {
	t.Parallel()
	stateDir := tempDir(t)
	eng, err := memory.Open(filepath.Join(stateDir, "enderbloom", "memory"), memory.Options{})
	if err != nil {
		t.Fatal(err)
	}
	bank, err := eng.Bank("u1")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	approved, err := bank.Enqueue(ctx, memory.QueueItem{Kind: memory.QueueRule, Payload: `{"text":"always wrap errors"}`})
	if err != nil {
		t.Fatal(err)
	}
	rejected, err := bank.Enqueue(ctx, memory.QueueItem{Kind: memory.QueueRule, Payload: `{"quote":"never wrap errors"}`})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bank.Enqueue(ctx, memory.QueueItem{Kind: memory.QueueRule, Payload: `{"text":"unjudged candidate"}`}); err != nil {
		t.Fatal(err)
	}
	if err := bank.Decide(ctx, approved, memory.QueueApproved, "human"); err != nil {
		t.Fatal(err)
	}
	if err := bank.Decide(ctx, rejected, memory.QueueRejected, "human"); err != nil {
		t.Fatal(err)
	}
	if err := eng.Close(); err != nil {
		t.Fatal(err)
	}
	labels, err := QueueLabelsForUser(stateDir, "u1")
	if err != nil {
		t.Fatal(err)
	}
	// The pending row is unjudged and skipped: labels come from decided rows.
	if len(labels) != 2 {
		t.Fatalf("labels = %+v, want the approved and rejected rows", labels)
	}
	if labels[0].Text != "always wrap errors" || !labels[0].Durable {
		t.Errorf("first = %+v, want the approved durable label", labels[0])
	}
	if labels[1].Text != "never wrap errors" || labels[1].Durable {
		t.Errorf("second = %+v, want the rejected non-durable label", labels[1])
	}
	if _, err := QueueLabelsForUser(stateDir, "nobody"); err == nil {
		t.Fatalf("unknown user succeeded, want the missing-bank error")
	}
}

func TestSampleLabelCandidatesOversamplesHits(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	sessions := []ReplaySession{
		{ID: "s1", CreatedAt: base, Events: []Event{
			{Kind: "tool_call_started", Text: "edit app.go"},
			cannedPrompt(1, "remember: always return wrapped errors"),
			cannedPrompt(2, "What time is lunch?"),
		}},
		{ID: "s2", CreatedAt: base.Add(time.Hour), Events: []Event{
			cannedPrompt(1, "remember: never return wrapped errors"),
			cannedPrompt(2, "looks good, thanks"),
		}},
	}
	got := SampleLabelCandidates(sessions, 3)
	// Two hits first in time order, then one miss to fill.
	if len(got) != 3 {
		t.Fatalf("sampled = %d, want 3", len(got))
	}
	if got[0].ID != "s1:1" || got[1].ID != "s2:1" {
		t.Fatalf("hits = %+v, want s1:1 then s2:1", got)
	}
	if got[0].Context != "edit app.go" {
		t.Fatalf("context = %q, want the preceding tool call", got[0].Context)
	}
	if got[0].Heuristic <= 0 || got[2].Heuristic != 0 {
		t.Fatalf("hints = %+v, want positive hints on hits only", got)
	}
	if one := SampleLabelCandidates(sessions, 1); len(one) != 1 || one[0].ID != "s1:1" {
		t.Fatalf("n=1 = %+v, want the first hit", one)
	}
	if all := SampleLabelCandidates(sessions, 0); len(all) != 4 {
		t.Fatalf("n=0 = %d, want every candidate", len(all))
	}
}

func TestFormatDeciderReportMarksUnscoredNA(t *testing.T) {
	t.Parallel()
	labels := []memory.DeciderLabel{
		{ID: "a", Text: "remember: always wrap errors", Durable: true},
		{ID: "b", Text: "What time is lunch?", Durable: false},
	}
	evals := memory.ScoreEvalDeciders(labels, map[string]memory.DeciderScores{
		"llm": {"a": 0.9, "b": 0.1},
	}, memory.DefaultConfirmHigh)
	out := FormatDeciderReport(evals, memory.DefaultConfirmHigh, 1, 1)
	if !strings.Contains(out, "labels: 2 (hand 1, queue 1)") {
		t.Fatalf("report = %q, want the label mix", out)
	}
	// laya scored nothing: n/a, never a measured zero.
	var laya string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "laya:") {
			laya = line
		}
	}
	if !strings.Contains(laya, "precision n/a") || !strings.Contains(laya, "scored 0/2") {
		t.Fatalf("laya = %q, want n/a precision with scored 0/2", laya)
	}
	if !strings.Contains(out, "llm: precision 1.000") {
		t.Fatalf("report = %q, want the measured llm precision", out)
	}
}
