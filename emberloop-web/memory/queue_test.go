package memory

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNormalizeSubjectKey(t *testing.T) {
	cases := []struct{ in, want string }{
		{"go.errors.wrapping", "go.errors.wrapping"},
		{"Go.Errors Wrapping!", "go.errors.wrapping"},
		{"  FOO__bar--baz  ", "foo.bar.baz"},
		{"Go.Errors.Wrapping", "go.errors.wrapping"},
		{"", ""},
		{"   ", ""},
		{"a", "a"},
	}
	for _, c := range cases {
		if got := NormalizeSubjectKey(c.in); got != c.want {
			t.Errorf("NormalizeSubjectKey(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if got := NormalizeSubjectKey("Go Errors Wrapping"); got != "go.errors.wrapping" {
		t.Errorf("model-proposed key not normalized: %q", got)
	}
}

func TestProposeSubjectKeyGroupsPolarity(t *testing.T) {
	a := ProposeSubjectKey("always return wrapped errors in this repo")
	b := ProposeSubjectKey("never return wrapped errors in this repo")
	if a == "" || b == "" {
		t.Fatalf("proposed keys empty: %q %q", a, b)
	}
	if a != b {
		t.Fatalf("polarity forked the key: %q vs %q", a, b)
	}
	if got := ProposeSubjectKey("ok"); got != "" {
		t.Fatalf("vague single token proposed %q, want empty", got)
	}
}

func TestGeneralizationSignals(t *testing.T) {
	if GeneralizationScore("always use tabs in this repo from now on") <= 0 {
		t.Errorf("generalizing language did not count for promotion")
	}
	if GeneralizationScore("fix it here in this function for now") >= 0 {
		t.Errorf("situational language did not count against promotion")
	}
	if !IsGeneralizing("always use tabs") || IsSituational("always use tabs") {
		t.Errorf("IsGeneralizing/IsSituational mismatch on generalizing text")
	}
	if !IsSituational("fix it here") || IsGeneralizing("fix it here") {
		t.Errorf("IsGeneralizing/IsSituational mismatch on situational text")
	}
}

func queueObs(session, event, author, quote, key string) Observation {
	return Observation{SessionID: session, EventID: event, Author: author, Quote: quote, Kind: KindPreference, SubjectKey: key}
}

func TestSingleMentionNeverPromotes(t *testing.T) {
	single := []Observation{queueObs("s1", "e1", "u1", "always wrap errors in this repo", "wrap.errors.repo")}
	if ShouldPromote(single) {
		t.Fatalf("single mention promotes, want no queue item")
	}
	if _, ok := BuildRuleProposal(single, KindPreference, "u1", "mined"); ok {
		t.Fatalf("single mention built a proposal, want none")
	}
}

func TestRecurrenceAcrossSessionsPromotes(t *testing.T) {
	group := []Observation{
		queueObs("s1", "e1", "u1", "always wrap errors in this repo", "wrap.errors.repo"),
		queueObs("s2", "e2", "u1", "always wrap errors in this repo please", "wrap.errors.repo"),
	}
	if !RecursAcrossSessions(group) {
		t.Fatalf("two sessions did not recur")
	}
	if !ShouldPromote(group) {
		t.Fatalf("recurring subject did not promote")
	}
	proposal, ok := BuildRuleProposal(group, KindPreference, "u1", "mined")
	if !ok {
		t.Fatalf("recurring group built no proposal")
	}
	if proposal.SubjectKey != "wrap.errors.repo" {
		t.Fatalf("proposal key = %q, want the subject", proposal.SubjectKey)
	}
	if len(proposal.Quotes) != 2 || len(proposal.Sessions) != 2 {
		t.Fatalf("proposal = %+v, want both quotes and sessions", proposal)
	}
	if proposal.Text == "" {
		t.Fatalf("proposal text empty, want one generalization")
	}
}

func TestSameSessionTwiceDoesNotPromote(t *testing.T) {
	group := []Observation{
		queueObs("s1", "e1", "u1", "always wrap errors", "wrap.errors"),
		queueObs("s1", "e2", "u1", "always wrap errors again", "wrap.errors"),
	}
	if RecursAcrossSessions(group) || ShouldPromote(group) {
		t.Fatalf("same-session repeats promoted, want two sessions on different tasks")
	}
}

func TestSituationalGroupDoesNotPromote(t *testing.T) {
	group := []Observation{
		queueObs("s1", "e1", "u1", "fix it here in this function for now", "fix.function.now"),
		queueObs("s2", "e2", "u1", "fix it here for now please", "fix.function.now"),
	}
	if ShouldPromote(group) {
		t.Fatalf("situational group promoted, want situational language to count against")
	}
}

func TestProposeGeneralizationPicksGeneralizing(t *testing.T) {
	group := []Observation{
		queueObs("s1", "e1", "u1", "fix it here", "fix.tabs"),
		queueObs("s2", "e2", "u1", "always use tabs in this repo", "fix.tabs"),
	}
	text, quotes, err := ProposeGeneralization(group)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(text), "always") {
		t.Fatalf("generalization = %q, want the generalizing quote", text)
	}
	if len(quotes) != 2 {
		t.Fatalf("quotes = %d, want both as evidence", len(quotes))
	}
}

func TestPartitionSubjectRules(t *testing.T) {
	rules := []Rule{
		{RuleID: "r1", Version: 1, Text: "always wrap errors", Kind: KindPreference, SubjectKey: "wrap.errors", Status: StatusActive, ApprovedBy: "u1"},
		{RuleID: "r2", Version: 1, Text: "always wrap errors, bob's take", Kind: KindPreference, SubjectKey: "wrap.errors", Status: StatusActive, ApprovedBy: "bob"},
		{RuleID: "r3", Version: 1, Text: "unrelated note", Kind: KindFact, SubjectKey: "other.key", Status: StatusActive, ApprovedBy: "u1"},
		{RuleID: "r0", Version: 1, Text: "older same-key take", Kind: KindPreference, SubjectKey: "wrap.errors", Status: StatusActive, ApprovedBy: "u1"},
	}
	byApprover := func(r Rule) string { return r.ApprovedBy }
	same, different := PartitionSubjectRules(rules, "Wrap.Errors", "u1", byApprover)
	if len(same) != 2 || same[0].RuleID != "r0" || same[1].RuleID != "r1" {
		t.Fatalf("same = %+v, want r0 r1 ordered by rule ID", same)
	}
	if len(different) != 1 || different[0].RuleID != "r2" {
		t.Fatalf("different = %+v, want r2 (same key, different observer)", different)
	}
	// Unknown or mixed authors never supersede: everything disagrees.
	same, different = PartitionSubjectRules(rules, "wrap.errors", "", byApprover)
	if len(same) != 0 || len(different) != 3 {
		t.Fatalf("unknown candidate: same = %d different = %d, want 0 and 3", len(same), len(different))
	}
	same, different = PartitionSubjectRules(rules, "wrap.errors", "u1", func(Rule) string { return "" })
	if len(same) != 0 || len(different) != 3 {
		t.Fatalf("unknown rule authors: same = %d different = %d, want 0 and 3", len(same), len(different))
	}
	if same, different := PartitionSubjectRules(rules, "", "u1", byApprover); len(same) != 0 || len(different) != 0 {
		t.Fatalf("empty key: same = %d different = %d, want none", len(same), len(different))
	}
}

func TestProposeGeneralizationNeverClips(t *testing.T) {
	long := "always " + strings.Repeat("wrapping errors with context ", 20)
	group := []Observation{
		queueObs("s1", "e1", "u1", "fix it here", "long.key"),
		queueObs("s2", "e2", "u1", long, "long.key"),
	}
	text, quotes, err := ProposeGeneralization(group)
	if err != nil {
		t.Fatal(err)
	}
	if text != strings.TrimSpace(long) {
		t.Fatalf("generalization clipped to %d chars, want the whole %d-char quote", len(text), len(long))
	}
	if len(quotes) != 2 {
		t.Fatalf("quotes = %d, want both as evidence", len(quotes))
	}
}

func TestPairCandidateSameKeyAndOverlap(t *testing.T) {
	rules := []Rule{
		{RuleID: "r1", Version: 1, Text: "always return wrapped errors", Kind: KindPreference, SubjectKey: "return.wrapped.errors", Status: StatusActive, ApprovedBy: "u1"},
	}
	if id, rel := PairCandidate("never return wrapped errors", "return.wrapped.errors", rules); id != "r1" || rel == "" {
		t.Fatalf("pair = %s %s, want r1 with a relation", id, rel)
	}
	if id, _ := PairCandidate("prefer tabs for indentation", "tabs.indentation", rules); id != "" {
		t.Fatalf("unrelated pair = %s, want none", id)
	}
}

func TestSubjectKeyProposers(t *testing.T) {
	ctx := context.Background()
	state := DeciderState{Statement: "always return wrapped errors in this repo"}
	if key, err := (HeuristicDecider{}).ProposeSubjectKey(ctx, state); err != nil || key != ProposeSubjectKey(state.Statement) {
		t.Fatalf("heuristic proposer = %q %v, want the heuristic key", key, err)
	}
	if _, err := (UnavailableDecider{}).ProposeSubjectKey(ctx, state); !errors.Is(err, ErrDeciderUnavailable) {
		t.Fatalf("unavailable proposer err = %v, want ErrDeciderUnavailable", err)
	}
	scripted := &ScriptedDecider{SubjectKey: "Go.Errors.Wrapping"}
	if key, err := scripted.ProposeSubjectKey(ctx, state); err != nil || key != "go.errors.wrapping" {
		t.Fatalf("scripted proposer = %q %v, want normalized go.errors.wrapping", key, err)
	}
	empty := &ScriptedDecider{}
	if _, err := empty.ProposeSubjectKey(ctx, state); !errors.Is(err, ErrDeciderUnavailable) {
		t.Fatalf("empty scripted proposer err = %v, want fallback via ErrDeciderUnavailable", err)
	}
	ensemble := EnsembleDecider{First: HeuristicDecider{}, Second: &ScriptedDecider{SubjectKey: "second.key"}}
	if key, err := ensemble.ProposeSubjectKey(ctx, state); err != nil || key != "second.key" {
		t.Fatalf("ensemble proposer = %q %v, want the confirming second", key, err)
	}
	// The model sees redacted text only.
	llm := LLMDecider{ModelName: "m", Call: func(_ context.Context, _, _, user string) (string, error) {
		if strings.Contains(user, "hunter2") {
			t.Errorf("model input carried the raw secret")
		}
		if !strings.Contains(user, "[redacted:password]") {
			t.Errorf("model input missed the redaction marker: %q", user)
		}
		return `{"subject_key":"Model.Proposed.Key"}`, nil
	}}
	if key, err := llm.ProposeSubjectKey(ctx, DeciderState{Statement: "the password is hunter2-2026, always use tabs"}); err != nil || key != "model.proposed.key" {
		t.Fatalf("llm proposer = %q %v, want normalized model.proposed.key", key, err)
	}
	bad := LLMDecider{ModelName: "m", Call: func(context.Context, string, string, string) (string, error) {
		return `{"subject_key":""}`, nil
	}}
	if _, err := bad.ProposeSubjectKey(ctx, state); !errors.Is(err, ErrDeciderUnavailable) {
		t.Fatalf("empty model key err = %v, want ErrDeciderUnavailable", err)
	}
}

func TestRuleTextOver300RefusedNotClipped(t *testing.T) {
	for _, fac := range factories() {
		t.Run(fac.name, func(t *testing.T) {
			clock := newTestClock()
			m := fac.open(t, clock)
			ctx := context.Background()
			long := strings.Repeat("y", MaxRuleChars+1)
			if _, err := m.AddRule(ctx, Rule{Text: long, Kind: KindFact, ApprovedBy: "u1"}); err == nil {
				t.Fatalf("AddRule over 300 chars succeeded, want refusal")
			} else if !isInvalid(err) {
				t.Fatalf("AddRule over-long err = %v, want ErrInvalid", err)
			}
			// Nothing was clipped into storage: no Rule exists.
			if got, _ := m.Recall(ctx, "y", 10); len(got) != 0 {
				t.Fatalf("over-long text was clipped into storage: %d rules", len(got))
			}
			ok := strings.Repeat("x", MaxRuleChars)
			r, err := m.AddRule(ctx, Rule{Text: ok, Kind: KindFact, ApprovedBy: "u1"})
			if err != nil {
				t.Fatalf("300-char rule refused: %v", err)
			}
			if _, err := m.ReviseRule(ctx, Rule{RuleID: r.RuleID, Text: long, Kind: KindFact, ApprovedBy: "u1"}); !isInvalid(err) {
				t.Fatalf("ReviseRule over-long err = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestSubjectKeyNormalizedOnWrite(t *testing.T) {
	for _, fac := range factories() {
		t.Run(fac.name, func(t *testing.T) {
			clock := newTestClock()
			m := fac.open(t, clock)
			ctx := context.Background()
			id, err := m.AddObservation(ctx, Observation{SessionID: "s", EventID: "e", Author: "u1", Quote: "q", Kind: KindFact, SubjectKey: "Wrap.Errors"})
			if err != nil {
				t.Fatal(err)
			}
			got, err := m.ObservationsBySubjectKey(ctx, "wrap.errors")
			if err != nil || len(got) != 1 || got[0].ID != id || got[0].SubjectKey != "wrap.errors" {
				t.Fatalf("observation key not normalized: %+v %v", got, err)
			}
			r, err := m.AddRule(ctx, Rule{Text: "note", Kind: KindFact, SubjectKey: "Wrap.Errors", ApprovedBy: "u1"})
			if err != nil {
				t.Fatal(err)
			}
			if r.SubjectKey != "wrap.errors" {
				t.Fatalf("rule key = %q, want normalized", r.SubjectKey)
			}
			rules, err := m.RulesBySubjectKey(ctx, "WRAP.ERRORS")
			if err != nil || len(rules) != 1 {
				t.Fatalf("rules by key = %+v %v, want 1", rules, err)
			}
		})
	}
}

func TestObservationsAndRulesBySubject(t *testing.T) {
	for _, fac := range factories() {
		t.Run(fac.name, func(t *testing.T) {
			clock := newTestClock()
			m := fac.open(t, clock)
			ctx := context.Background()
			_, _ = m.AddObservation(ctx, Observation{SessionID: "s1", EventID: "e1", Author: "u1", Quote: "first", Kind: KindFact, SubjectKey: "a.b"})
			_, _ = m.AddObservation(ctx, Observation{SessionID: "s2", EventID: "e2", Author: "u1", Quote: "second", Kind: KindFact, SubjectKey: "a.b"})
			_, _ = m.AddObservation(ctx, Observation{SessionID: "s3", EventID: "e3", Author: "u1", Quote: "other", Kind: KindFact, SubjectKey: "c.d"})
			got, err := m.ObservationsBySubjectKey(ctx, "a.b")
			if err != nil || len(got) != 2 || got[0].SessionID != "s1" || got[1].SessionID != "s2" {
				t.Fatalf("observations by key = %+v %v, want s1 s2 oldest first", got, err)
			}
			if got, _ := m.ObservationsBySubjectKey(ctx, ""); len(got) != 0 {
				t.Fatalf("empty key returned %d, want none", len(got))
			}
			r1, _ := m.AddRule(ctx, Rule{Text: "first rule", Kind: KindFact, SubjectKey: "a.b", ApprovedBy: "u1"})
			r2, err := m.ReviseRule(ctx, Rule{RuleID: r1.RuleID, Text: "first rule v2", Kind: KindFact, SubjectKey: "a.b", ApprovedBy: "u1"})
			if err != nil {
				t.Fatal(err)
			}
			_ = r2
			_, _ = m.AddRule(ctx, Rule{Text: "other rule", Kind: KindFact, SubjectKey: "c.d", ApprovedBy: "u1"})
			rules, err := m.RulesBySubjectKey(ctx, "a.b")
			if err != nil || len(rules) != 2 {
				t.Fatalf("rules by key = %+v %v, want both versions", rules, err)
			}
		})
	}
}

func TestDescribeQueueBudgetAndLatency(t *testing.T) {
	clock := newTestClock()
	m := NewFake(Options{Now: clock.Now})
	ctx := context.Background()
	first, err := m.Enqueue(ctx, QueueItem{Kind: QueueRule, Payload: `{"text":"a"}`})
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(2 * time.Hour)
	second, err := m.Enqueue(ctx, QueueItem{Kind: QueueRule, Payload: `{"text":"b"}`})
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Hour)
	if err := m.Decide(ctx, first, QueueApproved, "u1"); err != nil {
		t.Fatal(err)
	}
	items, _ := m.Queue(ctx)
	now := clock.Now()
	st := DescribeQueue(items, now)
	if st.Pending != 1 || st.Approved != 0+1 || st.Total != 2 {
		t.Fatalf("stats = %+v, want 1 pending 1 approved 2 total", st)
	}
	if st.Budget != QueueBudget {
		t.Fatalf("budget = %d, want %d", st.Budget, QueueBudget)
	}
	if st.OverBudget {
		t.Fatalf("stats over budget with 1 pending of %d", QueueBudget)
	}
	// Approval latency is decided_at minus created_at: 3h for the first item.
	if st.AvgApprovalLatency != 3*time.Hour {
		t.Fatalf("avg latency = %v, want 3h", st.AvgApprovalLatency)
	}
	// Oldest pending age is now minus the second item's creation: 1h.
	if st.OldestPendingAge != time.Hour {
		t.Fatalf("oldest pending = %v, want 1h", st.OldestPendingAge)
	}
	_ = second
	// Rejected items stay as labels and count toward latency.
	clock.Advance(time.Hour)
	if err := m.Decide(ctx, second, QueueRejected, "u1"); err != nil {
		t.Fatal(err)
	}
	items, _ = m.Queue(ctx)
	st = DescribeQueue(items, clock.Now())
	if st.Rejected != 1 || st.Pending != 0 {
		t.Fatalf("after reject stats = %+v, want 1 rejected 0 pending", st)
	}
	if st.AvgApprovalLatency != (3*time.Hour+2*time.Hour)/2 {
		t.Fatalf("avg latency after reject = %v", st.AvgApprovalLatency)
	}
}

func TestQueueItemsForSessionAndPendingForSubject(t *testing.T) {
	items := []QueueItem{
		{ID: 1, Kind: QueueRule, Status: QueuePending, Payload: `{"sessions":["s1","s2"],"text":"a"}`},
		{ID: 2, Kind: QueueRule, Status: QueuePending, Payload: `{"sessionId":"s1","text":"b"}`},
		{ID: 3, Kind: QueueProfilePatch, Status: QueuePending, Payload: `{"sessionId":"s1"}`},
		{ID: 4, Kind: QueueRule, Status: QueueApproved, Payload: `{"sessions":["s1"],"text":"c"}`},
	}
	got := QueueItemsForSession(items, "s1")
	if len(got) != 4 {
		t.Fatalf("session items = %d, want 4 (both shapes, any kind or status)", len(got))
	}
	if got := QueueItemsForSession(items, "missing"); len(got) != 0 {
		t.Fatalf("missing session items = %d, want 0", len(got))
	}
	subj := []QueueItem{
		{ID: 1, Kind: QueueRule, Status: QueuePending, Payload: `{"subject_key":"Wrap.Errors","text":"a"}`},
		{ID: 2, Kind: QueueRule, Status: QueuePending, Payload: `{"subject_key":"other","text":"b"}`},
		{ID: 3, Kind: QueueRule, Status: QueueApproved, Payload: `{"subject_key":"wrap.errors","text":"c"}`},
	}
	if got := PendingForSubject(subj, "wrap.errors"); len(got) != 1 || got[0].ID != 1 {
		t.Fatalf("pending for subject = %+v, want only ID 1", got)
	}
}

func TestObservationByID(t *testing.T) {
	for _, fac := range factories() {
		t.Run(fac.name, func(t *testing.T) {
			clock := newTestClock()
			m := fac.open(t, clock)
			ctx := context.Background()
			id, err := m.AddObservation(ctx, Observation{SessionID: "s", EventID: "e", Author: "u1", Quote: "find me", Kind: KindFact})
			if err != nil {
				t.Fatal(err)
			}
			got, err := m.ObservationByID(ctx, id)
			if err != nil || got.Quote != "find me" || got.ID != id {
				t.Fatalf("by ID = %+v %v, want the observation", got, err)
			}
			if _, err := m.ObservationByID(ctx, id+9999); !errors.Is(err, ErrNotFound) {
				t.Fatalf("missing ID err = %v, want ErrNotFound", err)
			}
		})
	}
}

func TestApproveRuleIsAtomic(t *testing.T) {
	for _, fac := range factories() {
		t.Run(fac.name, func(t *testing.T) {
			clock := newTestClock()
			m := fac.open(t, clock)
			ctx := context.Background()
			old, err := m.AddRule(ctx, Rule{Text: "old wording", Kind: KindPreference, SubjectKey: "wrap.errors", ApprovedBy: "u1"})
			if err != nil {
				t.Fatal(err)
			}
			item, err := m.Enqueue(ctx, QueueItem{Kind: QueueRule, Payload: `{"text":"new wording"}`})
			if err != nil {
				t.Fatal(err)
			}
			// A refused approval changes nothing: no Rule, no supersession,
			// the item stays pending.
			long := "quuxdistinct " + strings.Repeat("y", MaxRuleChars+1)
			if _, err := m.ApproveRule(ctx, item, "u1", Rule{Text: long, Kind: KindFact, ApprovedBy: "u1"}, []string{old.RuleID}); !isInvalid(err) {
				t.Fatalf("over-long approve err = %v, want ErrInvalid", err)
			}
			if queue, _ := m.Queue(ctx); len(queue) != 1 || queue[0].Status != QueuePending {
				t.Fatalf("queue after refused approve = %+v, want still pending", queue)
			}
			if versions, _ := m.RuleVersions(ctx, old.RuleID); len(versions) != 1 || versions[0].Status != StatusActive {
				t.Fatalf("old rule after refused approve = %+v, want still active", versions)
			}
			if got, _ := m.Recall(ctx, "quuxdistinct", 10); len(got) != 0 {
				t.Fatalf("refused approval wrote %d Rules, want none", len(got))
			}
			// The happy path creates the Rule, supersedes the old one and
			// approves the item together.
			rule, err := m.ApproveRule(ctx, item, "u1", Rule{Text: "new wording", Kind: KindFact, SubjectKey: "wrap.errors", ApprovedBy: "u1"}, []string{old.RuleID, "missing", old.RuleID})
			if err != nil {
				t.Fatal(err)
			}
			if rule.Version != 1 || rule.Status != StatusActive {
				t.Fatalf("approved rule = %+v, want active v1", rule)
			}
			if queue, _ := m.Queue(ctx); len(queue) != 1 || queue[0].Status != QueueApproved || queue[0].DecidedBy != "u1" {
				t.Fatalf("queue after approve = %+v, want the approved label", queue)
			}
			if versions, _ := m.RuleVersions(ctx, old.RuleID); len(versions) != 1 || versions[0].Status != StatusSuperseded {
				t.Fatalf("old rule after approve = %+v, want superseded", versions)
			}
			// Decided items, non-rule items and missing items are refused.
			if _, err := m.ApproveRule(ctx, item, "u1", Rule{Text: "again", Kind: KindFact, ApprovedBy: "u1"}, nil); !isInvalid(err) {
				t.Fatalf("second approve err = %v, want ErrInvalid", err)
			}
			profile, err := m.Enqueue(ctx, QueueItem{Kind: QueueProfilePatch, Payload: `{"op":"add"}`})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := m.ApproveRule(ctx, profile, "u1", Rule{Text: "x", Kind: KindFact, ApprovedBy: "u1"}, nil); !isInvalid(err) {
				t.Fatalf("non-rule approve err = %v, want ErrInvalid", err)
			}
			if _, err := m.ApproveRule(ctx, 9999, "u1", Rule{Text: "x", Kind: KindFact, ApprovedBy: "u1"}, nil); !errors.Is(err, ErrNotFound) {
				t.Fatalf("missing approve err = %v, want ErrNotFound", err)
			}
		})
	}
}

func isInvalid(err error) bool {
	if err == nil {
		return false
	}
	for err != nil {
		if err == ErrInvalid {
			return true
		}
		u, ok := err.(interface{ Unwrap() []error })
		if ok {
			found := false
			for _, e := range u.Unwrap() {
				if isInvalid(e) {
					found = true
				}
			}
			if found {
				return true
			}
		}
		e, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = e.Unwrap()
	}
	return false
}
