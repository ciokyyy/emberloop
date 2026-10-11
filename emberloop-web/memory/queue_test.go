package memory

import (
	"context"
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

func TestSupersessionSameAuthorDisagreementOther(t *testing.T) {
	rules := []Rule{
		{RuleID: "r1", Version: 1, Text: "always wrap errors", Kind: KindPreference, SubjectKey: "wrap.errors", Status: StatusActive, ApprovedBy: "u1"},
		{RuleID: "r2", Version: 1, Text: "always wrap errors, bob's take", Kind: KindPreference, SubjectKey: "wrap.errors", Status: StatusActive, ApprovedBy: "bob"},
		{RuleID: "r3", Version: 1, Text: "unrelated note", Kind: KindFact, SubjectKey: "other.key", Status: StatusActive, ApprovedBy: "u1"},
	}
	superseded := FindSuperseded(rules, "Wrap.Errors", "u1")
	if len(superseded) != 1 || superseded[0].RuleID != "r1" {
		t.Fatalf("superseded = %+v, want only r1 (same key, same author)", superseded)
	}
	disagreements := FindDisagreements(rules, "wrap.errors", "u1")
	if len(disagreements) != 1 || disagreements[0].RuleID != "r2" {
		t.Fatalf("disagreements = %+v, want r2 (same key, different author)", disagreements)
	}
	if n := len(FindSuperseded(rules, "", "u1")); n != 0 {
		t.Fatalf("empty key superseded %d, want none", n)
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
