package memory

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// testUser is a valid bank name for engine tests.
const testUser = "0123456789abcdef0123456789abcdef"

// testClock is a settable clock shared by a test's engine or fake.
type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func newTestClock() *testClock {
	return &testClock{t: time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)}
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// factory opens one Memory implementation for the contract suite.
type factory struct {
	name string
	open func(t *testing.T, clock *testClock) Memory
}

// factories are the implementations the contract runs against: the real
// SQLite bank and the in-memory Fake.
func factories() []factory {
	return []factory{
		{"bank", func(t *testing.T, clock *testClock) Memory {
			t.Helper()
			eng, err := Open(tempDir(t), Options{Now: clock.Now})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = eng.Close() })
			b, err := eng.Bank(testUser)
			if err != nil {
				t.Fatal(err)
			}
			return b
		}},
		{"fake", func(t *testing.T, clock *testClock) Memory {
			return NewFake(Options{Now: clock.Now})
		}},
	}
}

// hostileQueries are inputs that would break an unquoted FTS5 MATCH.
var hostileQueries = []string{
	`"`, `""`, `a"b`, `AND`, `OR`, `NOT x`, `x AND (y OR z)`, `((`, `)`,
	`col:term`, `title:"x y"`, `-dash -- x`, `a-b:c`, `NEAR(a b, 2)`, `*`,
	`^start`, `x*`, `\`, `'; DROP TABLE rules; --`, `日本語 テスト`, `🙂`,
	strings.Repeat("x", 5000), "", "   ", "upload handler \"AND\" OR (",
}

// TestContract runs every behavior the Memory interface promises against each
// implementation. The bank-only checks (triggers, FTS internals, file bytes)
// live in bank_test.go and purge_test.go.
func TestContract(t *testing.T) {
	for _, fac := range factories() {
		t.Run(fac.name, func(t *testing.T) {
			t.Run("hostile queries never error", func(t *testing.T) {
				clock := newTestClock()
				m := fac.open(t, clock)
				ctx := context.Background()
				rule, err := m.AddRule(ctx, Rule{Text: "use the upload handler for uploads", Kind: KindPreference, ApprovedBy: "u1"})
				if err != nil {
					t.Fatal(err)
				}
				for _, q := range hostileQueries {
					if _, err := m.Recall(ctx, q, 5); err != nil {
						t.Fatalf("Recall(%q): %v", q, err)
					}
					if _, err := m.SearchObservations(ctx, q, 5); err != nil {
						t.Fatalf("SearchObservations(%q): %v", q, err)
					}
				}
				got, err := m.Recall(ctx, `upload handler "AND" OR (`, 5)
				if err != nil {
					t.Fatal(err)
				}
				if !hasRule(got, rule.RuleID) {
					t.Fatalf("hostile query lost the matching rule: %v", ruleIDs(got))
				}
			})

			t.Run("identifier matches split words both ways", func(t *testing.T) {
				clock := newTestClock()
				m := fac.open(t, clock)
				ctx := context.Background()
				plain, err := m.AddRule(ctx, Rule{Text: "upload handler must close the body", Kind: KindFact, ApprovedBy: "u1"})
				if err != nil {
					t.Fatal(err)
				}
				ident, err := m.AddRule(ctx, Rule{Text: "TestUploadHandler owns retries", Kind: KindFact, ApprovedBy: "u1"})
				if err != nil {
					t.Fatal(err)
				}
				byIdent, err := m.Recall(ctx, "TestUploadHandler", 10)
				if err != nil {
					t.Fatal(err)
				}
				if !hasRule(byIdent, plain.RuleID) {
					t.Errorf("TestUploadHandler did not match %q: %v", "upload handler must close the body", ruleIDs(byIdent))
				}
				byWords, err := m.Recall(ctx, "upload handler", 10)
				if err != nil {
					t.Fatal(err)
				}
				if !hasRule(byWords, ident.RuleID) {
					t.Errorf("upload handler did not match %q: %v", "TestUploadHandler owns retries", ruleIDs(byWords))
				}
			})

			t.Run("recall returns only live rules", func(t *testing.T) {
				clock := newTestClock()
				m := fac.open(t, clock)
				ctx := context.Background()
				active, _ := m.AddRule(ctx, Rule{Text: "retry with backoff on 429", Kind: KindPreference, ApprovedBy: "u1"})
				dormant, _ := m.AddRule(ctx, Rule{Text: "retry with jitter on 503", Kind: KindPreference, ApprovedBy: "u1"})
				if err := m.SetRuleStatus(ctx, dormant.RuleID, StatusDormant); err != nil {
					t.Fatal(err)
				}
				parked, _ := m.AddRule(ctx, Rule{Text: "retry parked note", Kind: KindFact, ApprovedBy: "u1"})
				if err := m.SetRuleStatus(ctx, parked.RuleID, StatusParked); err != nil {
					t.Fatal(err)
				}
				v1, _ := m.AddRule(ctx, Rule{Text: "prefer table tests for retry logic", Kind: KindPreference, ApprovedBy: "u1"})
				v2, err := m.ReviseRule(ctx, Rule{RuleID: v1.RuleID, Text: "prefer table-driven tests for retry logic", Kind: KindPreference, ApprovedBy: "u1"})
				if err != nil {
					t.Fatal(err)
				}
				got, err := m.Recall(ctx, "retry", 20)
				if err != nil {
					t.Fatal(err)
				}
				for _, r := range got {
					if !live(r.Status) {
						t.Errorf("recall returned %s rule %q", r.Status, r.Text)
					}
				}
				for _, want := range []string{active.RuleID, parked.RuleID} {
					if !hasRule(got, want) {
						t.Errorf("recall missed live rule %s: %v", want, ruleIDs(got))
					}
				}
				if hasRule(got, dormant.RuleID) {
					t.Errorf("recall returned dormant rule %s", dormant.RuleID)
				}
				count := 0
				for _, r := range got {
					if r.RuleID == v1.RuleID {
						count++
						if r.Version != v2.Version {
							t.Errorf("recall returned superseded version %d of revised rule", r.Version)
						}
					}
				}
				if count != 1 {
					t.Errorf("revised rule returned %d times, want 1", count)
				}
			})

			t.Run("lists return newest live rows first", func(t *testing.T) {
				clock := newTestClock()
				m := fac.open(t, clock)
				ctx := context.Background()
				if got, _ := m.ListRules(ctx, 10); len(got) != 0 {
					t.Fatalf("empty list = %d, want none", len(got))
				}
				if got, _ := m.ListObservations(ctx, 10); len(got) != 0 {
					t.Fatalf("empty observations = %d, want none", len(got))
				}
				first, _ := m.AddRule(ctx, Rule{Text: "first list rule note", Kind: KindFact, ApprovedBy: "u1"})
				clock.Advance(time.Second)
				second, _ := m.AddRule(ctx, Rule{Text: "second list rule note", Kind: KindFact, ApprovedBy: "u1"})
				if err := m.SetRuleStatus(ctx, first.RuleID, StatusDormant); err != nil {
					t.Fatal(err)
				}
				got, err := m.ListRules(ctx, 10)
				if err != nil {
					t.Fatal(err)
				}
				if len(got) != 1 || got[0].RuleID != second.RuleID {
					t.Fatalf("list = %v, want only the live second rule", ruleIDs(got))
				}
				if _, err := m.AddObservation(ctx, Observation{SessionID: "s1", EventID: "e1", Author: "u1", Quote: "first list observation", Kind: KindFact}); err != nil {
					t.Fatal(err)
				}
				if _, err := m.AddObservation(ctx, Observation{SessionID: "s1", EventID: "e2", Author: "u1", Quote: "second list observation", Kind: KindFact}); err != nil {
					t.Fatal(err)
				}
				obs, err := m.ListObservations(ctx, 10)
				if err != nil {
					t.Fatal(err)
				}
				if len(obs) != 2 || obs[0].EventID != "e2" || obs[1].EventID != "e1" {
					t.Fatalf("observations = %+v, want newest first", obs)
				}
			})

			t.Run("rule versions append and keep the old text", func(t *testing.T) {
				clock := newTestClock()
				m := fac.open(t, clock)
				ctx := context.Background()
				r, _ := m.AddRule(ctx, Rule{Text: "first wording", Kind: KindPreference, ApprovedBy: "u1"})
				if _, err := m.ReviseRule(ctx, Rule{RuleID: r.RuleID, Text: "second wording", Kind: KindPreference, ApprovedBy: "u1"}); err != nil {
					t.Fatal(err)
				}
				versions, err := m.RuleVersions(ctx, r.RuleID)
				if err != nil {
					t.Fatal(err)
				}
				if len(versions) != 2 {
					t.Fatalf("versions = %d, want 2", len(versions))
				}
				if versions[0].Status != StatusSuperseded || versions[0].Text != "first wording" {
					t.Errorf("v1 = %s %q, want superseded %q", versions[0].Status, versions[0].Text, "first wording")
				}
				if versions[1].Status != StatusActive || versions[1].Version != 2 {
					t.Errorf("v2 = %s v%d, want active v2", versions[1].Status, versions[1].Version)
				}
			})

			t.Run("input validation refuses bad records", func(t *testing.T) {
				clock := newTestClock()
				m := fac.open(t, clock)
				ctx := context.Background()
				ok := strings.Repeat("x", MaxRuleChars)
				if _, err := m.AddRule(ctx, Rule{Text: ok, Kind: KindFact, ApprovedBy: "u1"}); err != nil {
					t.Fatalf("%d-char rule refused: %v", MaxRuleChars, err)
				}
				bad := []Rule{
					{Text: "", Kind: KindFact, ApprovedBy: "u1"},
					{Text: strings.Repeat("y", MaxRuleChars+1), Kind: KindFact, ApprovedBy: "u1"},
					{Text: "ok", Kind: "essay", ApprovedBy: "u1"},
					{Text: "ok", Kind: KindFact},
				}
				for _, r := range bad {
					if _, err := m.AddRule(ctx, r); !errors.Is(err, ErrInvalid) {
						t.Errorf("AddRule(%.20q, kind %q, approver %q) err = %v, want ErrInvalid", r.Text, r.Kind, r.ApprovedBy, err)
					}
				}
				if err := m.SetRuleStatus(ctx, "missing", StatusDormant); !errors.Is(err, ErrNotFound) {
					t.Errorf("SetRuleStatus(missing) err = %v, want ErrNotFound", err)
				}
				if _, err := m.AddObservation(ctx, Observation{SessionID: "s", EventID: "e", Author: "u1", Quote: "", Kind: KindFact}); !errors.Is(err, ErrInvalid) {
					t.Errorf("empty quote err = %v, want ErrInvalid", err)
				}
			})

			t.Run("purge removes records and scrubs their text", func(t *testing.T) {
				clock := newTestClock()
				m := fac.open(t, clock)
				ctx := context.Background()
				obs, err := m.AddObservation(ctx, Observation{
					SessionID: "s1", EventID: "e1", Author: "u1", Quote: "never sleep long on 429",
					Kind: KindPreference,
				})
				if err != nil {
					t.Fatal(err)
				}
				doomed, _ := m.AddRule(ctx, Rule{Text: "retry with backoff on 429", Kind: KindPreference, ApprovedBy: "u1"})
				kept, _ := m.AddRule(ctx, Rule{Text: "keep unrelated note about tables", Kind: KindFact, ApprovedBy: "u1"})
				leaking, err := m.Enqueue(ctx, QueueItem{Kind: QueueRule, Payload: `{"text":"retry with backoff on 429"}`})
				if err != nil {
					t.Fatal(err)
				}
				drop, err := m.Enqueue(ctx, QueueItem{Kind: QueuePromotion, Payload: `{"note":"unrelated"}`})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := m.RecordLedger(ctx, LedgerRow{RunID: "r1", Stage: "mine", Status: LedgerFailed, Detail: "failed on retry with backoff on 429"}); err != nil {
					t.Fatal(err)
				}
				res, err := m.Purge(ctx, Selection{
					Observations: []int64{obs},
					Rules:        []string{doomed.RuleID},
					Queue:        []int64{drop},
				})
				if err != nil {
					t.Fatal(err)
				}
				if res.Observations != 1 || res.Rules != 1 || res.Queue != 1 {
					t.Errorf("purge counts = %+v, want 1 observation, 1 rule, 1 queue", res)
				}
				if got, _ := m.SearchObservations(ctx, "429", 10); len(got) != 0 {
					t.Errorf("purged observation still found: %d", len(got))
				}
				if got, _ := m.Recall(ctx, "backoff", 10); len(got) != 0 {
					t.Errorf("purged rule still recalled: %v", ruleIDs(got))
				}
				if versions, _ := m.RuleVersions(ctx, doomed.RuleID); len(versions) != 0 {
					t.Errorf("purged rule has %d versions left", len(versions))
				}
				if got, _ := m.Recall(ctx, "tables", 10); !hasRule(got, kept.RuleID) {
					t.Errorf("purge removed unrelated rule")
				}
				queue, _ := m.Queue(ctx)
				for _, q := range queue {
					if q.ID == drop {
						t.Errorf("purged queue item %d still listed", drop)
					}
					if q.ID == leaking && strings.Contains(q.Payload, "retry with backoff on 429") {
						t.Errorf("queue payload kept purged text: %s", q.Payload)
					}
					if !json.Valid([]byte(q.Payload)) {
						t.Errorf("purge broke queue payload JSON: %s", q.Payload)
					}
				}
				ledger, _ := m.Ledger(ctx)
				for _, l := range ledger {
					if strings.Contains(l.Detail, "retry with backoff on 429") {
						t.Errorf("ledger detail kept purged text: %s", l.Detail)
					}
				}
			})

			t.Run("snapshots follow the six hour rule and keep ten", func(t *testing.T) {
				clock := newTestClock()
				m := fac.open(t, clock)
				ctx := context.Background()
				first, err := m.Snapshot(ctx)
				if err != nil || !first.Taken || first.Kept != 1 {
					t.Fatalf("first snapshot = %+v, %v; want taken, kept 1", first, err)
				}
				clock.Advance(time.Hour)
				if res, _ := m.Snapshot(ctx); res.Taken || res.Kept != 1 {
					t.Fatalf("snapshot 1h later = %+v; want skipped, kept 1", res)
				}
				clock.Advance(5 * time.Hour)
				if res, _ := m.Snapshot(ctx); !res.Taken || res.Kept != 2 {
					t.Fatalf("snapshot 6h after first = %+v; want taken, kept 2", res)
				}
				var last SnapshotResult
				for i := 0; i < SnapshotKeep+2; i++ {
					clock.Advance(SnapshotInterval)
					res, err := m.Snapshot(ctx)
					if err != nil || !res.Taken {
						t.Fatalf("snapshot %d = %+v, %v; want taken", i, res, err)
					}
					last = res
				}
				if last.Kept != SnapshotKeep {
					t.Errorf("kept = %d after many snapshots, want %d", last.Kept, SnapshotKeep)
				}
				if last.Pruned < 1 {
					t.Errorf("pruned = %d, want at least 1 once past the keep limit", last.Pruned)
				}
			})

			t.Run("concurrent writers serialize", func(t *testing.T) {
				clock := newTestClock()
				m := fac.open(t, clock)
				ctx := context.Background()
				const writers, per = 8, 25
				errs := make(chan error, writers+2)
				var wg sync.WaitGroup
				for w := range writers {
					wg.Add(1)
					go func() {
						defer wg.Done()
						for range per {
							if _, err := m.AddObservation(ctx, Observation{
								SessionID: "s", EventID: "e", Author: "u1",
								Quote: "concurrent observation text", Kind: KindFact,
							}); err != nil {
								errs <- err
								return
							}
							if _, err := m.Enqueue(ctx, QueueItem{Kind: QueueRule, Payload: `{"w":` + strconv.Itoa(w) + `}`}); err != nil {
								errs <- err
								return
							}
						}
					}()
				}
				wg.Add(2)
				go func() {
					defer wg.Done()
					for range 20 {
						if _, err := m.Recall(ctx, "concurrent", 5); err != nil {
							errs <- err
							return
						}
					}
				}()
				go func() {
					defer wg.Done()
					if _, err := m.Snapshot(ctx); err != nil {
						errs <- err
					}
				}()
				wg.Wait()
				close(errs)
				for err := range errs {
					t.Errorf("concurrent write: %v", err)
				}
				queue, err := m.Queue(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if len(queue) != writers*per {
					t.Errorf("queue has %d items, want %d", len(queue), writers*per)
				}
			})

			t.Run("decide once and record ledger", func(t *testing.T) {
				clock := newTestClock()
				m := fac.open(t, clock)
				ctx := context.Background()
				id, err := m.Enqueue(ctx, QueueItem{Kind: QueueProfilePatch, Payload: `{"op":"add"}`})
				if err != nil {
					t.Fatal(err)
				}
				if err := m.Decide(ctx, id, QueueRejected, "u1"); err != nil {
					t.Fatal(err)
				}
				if err := m.Decide(ctx, id, QueueApproved, "u1"); !errors.Is(err, ErrInvalid) {
					t.Errorf("second decision err = %v, want ErrInvalid", err)
				}
				if err := m.Decide(ctx, 999, QueueApproved, "u1"); !errors.Is(err, ErrNotFound) {
					t.Errorf("decision on missing item err = %v, want ErrNotFound", err)
				}
				queue, _ := m.Queue(ctx)
				if len(queue) != 1 || queue[0].Status != QueueRejected || queue[0].DecidedBy != "u1" {
					t.Errorf("queue after reject = %+v; want the rejected label kept", queue)
				}
				if _, err := m.RecordLedger(ctx, LedgerRow{Stage: "mine", Status: "OK"}); !errors.Is(err, ErrInvalid) {
					t.Errorf("bad ledger status err = %v, want ErrInvalid", err)
				}
				if _, err := m.RecordLedger(ctx, LedgerRow{Stage: "mine", Status: LedgerNoData}); err != nil {
					t.Fatal(err)
				}
				ledger, _ := m.Ledger(ctx)
				if len(ledger) != 1 || ledger[0].CountsJSON != "{}" {
					t.Errorf("ledger = %+v; want one row with counts {}", ledger)
				}
			})
			t.Run("bump observation evidence", func(t *testing.T) {
				clock := newTestClock()
				m := fac.open(t, clock)
				ctx := context.Background()
				id, err := m.AddObservation(ctx, Observation{SessionID: "s", EventID: "e", Author: "u1", Quote: "bumpable quote", Kind: KindFact})
				if err != nil {
					t.Fatal(err)
				}
				updated, err := m.BumpObservationEvidence(ctx, id)
				if err != nil || updated.ID != id || updated.EvidenceCount != 2 || updated.Quote != "bumpable quote" {
					t.Fatalf("bump = %+v %v, want id %d with evidence 2", updated, err, id)
				}
				if _, err := m.BumpObservationEvidence(ctx, 999999); !errors.Is(err, ErrNotFound) {
					t.Fatalf("bump missing err = %v, want ErrNotFound", err)
				}
			})

			t.Run("observation by event is exact", func(t *testing.T) {
				clock := newTestClock()
				m := fac.open(t, clock)
				ctx := context.Background()
				id, err := m.AddObservation(ctx, Observation{SessionID: "s", EventID: "e", Author: "u1", Quote: "exact event quote", Kind: KindFact})
				if err != nil {
					t.Fatal(err)
				}
				got, err := m.ObservationByEvent(ctx, "s", "e")
				if err != nil || got.ID != id || got.Quote != "exact event quote" {
					t.Fatalf("by event = %+v, %v; want id %d", got, err, id)
				}
				for _, miss := range [][2]string{{"s", "other"}, {"other", "e"}, {"", ""}} {
					if _, err := m.ObservationByEvent(ctx, miss[0], miss[1]); !errors.Is(err, ErrNotFound) {
						t.Errorf("by event %q err = %v, want ErrNotFound", miss, err)
					}
				}
			})

			t.Run("subject lookups and atomic approval", func(t *testing.T) {
				clock := newTestClock()
				m := fac.open(t, clock)
				ctx := context.Background()
				first, err := m.AddObservation(ctx, Observation{SessionID: "s1", EventID: "e1", Author: "u1", Quote: "first", Kind: KindFact, SubjectKey: "Wrap.Errors"})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := m.AddObservation(ctx, Observation{SessionID: "s2", EventID: "e2", Author: "u1", Quote: "second", Kind: KindFact, SubjectKey: "wrap.errors"}); err != nil {
					t.Fatal(err)
				}
				// ObservationByID finds the exact row; subject keys
				// normalize on write so both spellings group together.
				got, err := m.ObservationByID(ctx, first)
				if err != nil || got.Quote != "first" {
					t.Fatalf("by ID = %+v %v, want the first observation", got, err)
				}
				if _, err := m.ObservationByID(ctx, first+9999); !errors.Is(err, ErrNotFound) {
					t.Fatalf("missing ID err = %v, want ErrNotFound", err)
				}
				group, err := m.ObservationsBySubjectKey(ctx, "WRAP.ERRORS")
				if err != nil || len(group) != 2 {
					t.Fatalf("observations by key = %d %v, want 2 normalized together", len(group), err)
				}
				old, err := m.AddRule(ctx, Rule{Text: "old wording", Kind: KindFact, SubjectKey: "Wrap.Errors", ApprovedBy: "u1"})
				if err != nil {
					t.Fatal(err)
				}
				keyed, err := m.RulesBySubjectKey(ctx, "wrap.errors")
				if err != nil || len(keyed) != 1 || keyed[0].RuleID != old.RuleID {
					t.Fatalf("rules by key = %+v %v, want the old rule", keyed, err)
				}
				item, err := m.Enqueue(ctx, QueueItem{Kind: QueueRule, Payload: `{"text":"new wording"}`})
				if err != nil {
					t.Fatal(err)
				}
				// The approval creates the Rule, supersedes the old one
				// and approves the item in one transaction.
				rule, err := m.ApproveRule(ctx, item, "u1", Rule{Text: "new wording", Kind: KindFact, SubjectKey: "wrap.errors", ApprovedBy: "u1"}, []string{old.RuleID})
				if err != nil || rule.Version != 1 {
					t.Fatalf("approve = %+v %v, want active v1", rule, err)
				}
				if queue, _ := m.Queue(ctx); len(queue) != 1 || queue[0].Status != QueueApproved {
					t.Fatalf("queue after approve = %+v, want approved", queue)
				}
				if versions, _ := m.RuleVersions(ctx, old.RuleID); len(versions) != 1 || versions[0].Status != StatusSuperseded {
					t.Fatalf("old rule after approve = %+v, want superseded", versions)
				}
				// A refused approval changes nothing.
				second, err := m.Enqueue(ctx, QueueItem{Kind: QueueRule, Payload: `{"text":"x"}`})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := m.ApproveRule(ctx, second, "u1", Rule{Text: strings.Repeat("y", MaxRuleChars+1), Kind: KindFact, ApprovedBy: "u1"}, nil); !errors.Is(err, ErrInvalid) {
					t.Fatalf("over-long approve err = %v, want ErrInvalid", err)
				}
				if queue, _ := m.Queue(ctx); queue[1].Status != QueuePending {
					t.Fatalf("queue after refused approve = %+v, want still pending", queue[1])
				}
			})
		})
	}
}

func hasRule(rules []Rule, id string) bool {
	for _, r := range rules {
		if r.RuleID == id {
			return true
		}
	}
	return false
}

func ruleIDs(rules []Rule) []string {
	out := make([]string, len(rules))
	for i, r := range rules {
		out[i] = r.RuleID
	}
	return out
}

func TestMemoriesAndEpisodes(t *testing.T) {
	for _, fac := range factories() {
		t.Run(fac.name, func(t *testing.T) {
			clock := newTestClock()
			m := fac.open(t, clock)
			ctx := context.Background()
			id, err := m.AddMemory(ctx, MemoryItem{SessionID: "s", EventID: "e", Author: "u1", Kind: KindFact, Text: "remember this", Quote: "remember this"})
			if err != nil {
				t.Fatal(err)
			}
			got, err := m.GetMemory(ctx, id)
			if err != nil || got.ID != id {
				t.Fatalf("get: %+v %v", got, err)
			}
			all, err := m.ListMemories(ctx, "", "", 10)
			if err != nil || len(all) != 1 {
				t.Fatalf("list all: %d %v", len(all), err)
			}
			if err := m.SetMemoryStatus(ctx, id, StatusForgotten); err != nil {
				t.Fatal(err)
			}
			live, err := m.ListMemories(ctx, "", "", 10)
			if err != nil || len(live) != 0 {
				t.Fatalf("forgotten not excluded: %d %v", len(live), err)
			}
			epID, err := m.AddEpisode(ctx, Episode{SessionID: "s", EventRange: "e1-e2", Author: "u1", Text: "hello user"})
			if err != nil {
				t.Fatal(err)
			}
			epGot, err := m.GetEpisode(ctx, epID)
			if err != nil || epGot.ID != epID {
				t.Fatalf("get episode: %+v %v", epGot, err)
			}
			newID, err := m.AddMemory(ctx, MemoryItem{SessionID: "s2", EventID: "e2", Author: "u1", Kind: KindFact, Text: "new memory", Quote: "new memory"})
			if err != nil {
				t.Fatal(err)
			}
			if err := m.SetMemorySuperseded(ctx, id, newID); err != nil {
				t.Fatal(err)
			}
			source, err := m.MemoryByID(ctx, id)
			if err != nil || source.Status != StatusSuperseded || source.SupersededBy != newID {
				t.Fatalf("superseded source=%+v err=%v", source, err)
			}
			if err := m.SetMemoryStatus(ctx, id, StatusActive); err != nil {
				t.Fatal(err)
			}
			if _, err := m.BumpMemory(ctx, id); err != nil {
				t.Fatal(err)
			}
			if err := m.SetMemoryEmbedding(ctx, id, []byte{1, 2}, "m"); err != nil {
				t.Fatal(err)
			}
			if err := m.SetMemoryPinned(ctx, id, true); err != nil {
				t.Fatal(err)
			}
			_, err = m.Purge(ctx, Selection{Memories: []int64{id, newID}, Episodes: []int64{epID}})
			if err != nil {
				t.Fatal(err)
			}
			if got, _ := m.ListEpisodes(ctx, 10); len(got) != 0 {
				t.Fatalf("episode not purged: %d", len(got))
			}
		})
	}
}

func TestMemoryRevisionContract(t *testing.T) {
	for _, fac := range factories() {
		t.Run(fac.name, func(t *testing.T) {
			ctx := context.Background()
			bank := fac.open(t, newTestClock())
			oldID, err := bank.AddMemory(ctx, MemoryItem{SessionID: "s1", EventID: "e1", Author: "u1", Kind: KindPreference, Text: "Use concise answers", Quote: "Please use concise answers"})
			if err != nil {
				t.Fatal(err)
			}
			old, err := bank.GetMemory(ctx, oldID)
			if err != nil {
				t.Fatal(err)
			}
			nextID, err := bank.ReviseMemory(ctx, oldID, MemoryItem{SessionID: old.SessionID, EventID: old.EventID, Author: old.Author, Kind: old.Kind, Text: "Prefer concise answers", Quote: old.Quote, Importance: old.Importance, Veracity: old.Veracity, ExtractorJSON: old.ExtractorJSON})
			if err != nil {
				t.Fatal(err)
			}
			old, _ = bank.GetMemory(ctx, oldID)
			next, _ := bank.GetMemory(ctx, nextID)
			if old.Status != StatusSuperseded || old.SupersededBy != nextID || old.Text != "Use concise answers" || next.Status != StatusActive || next.Text != "Prefer concise answers" {
				t.Fatalf("revision old=%+v next=%+v", old, next)
			}
			if _, err = bank.ReviseMemory(ctx, 999, MemoryItem{SessionID: "s1", EventID: "e2", Author: "u1", Kind: KindFact, Text: "orphan", Quote: "source"}); !errors.Is(err, ErrNotFound) {
				t.Fatalf("missing source revise error=%v", err)
			}
		})
	}
}

func TestMemorySearchFiltersBeforeLimit(t *testing.T) {
	for _, fac := range factories() {
		t.Run(fac.name, func(t *testing.T) {
			ctx := context.Background()
			bank := fac.open(t, newTestClock())
			for i := 0; i < 25; i++ {
				if _, err := bank.AddMemory(ctx, MemoryItem{SessionID: "s", EventID: strconv.Itoa(i), Author: "u1", Kind: KindFact, Text: "shared searchable memory", Quote: "shared searchable memory"}); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := bank.AddMemory(ctx, MemoryItem{SessionID: "s", EventID: "preferred", Author: "u1", Kind: KindPreference, Text: "shared searchable memory", Quote: "shared searchable memory"}); err != nil {
				t.Fatal(err)
			}
			count, err := bank.CountMemoriesSince(ctx, time.Time{})
			if err != nil || count != 26 {
				t.Fatalf("count since epoch = %d err=%v", count, err)
			}
			got, err := bank.SearchMemories(ctx, "shared searchable", "preference", "", 1)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 1 || got[0].Kind != KindPreference {
				t.Fatalf("filtered top-one search = %+v", got)
			}
		})
	}
}
