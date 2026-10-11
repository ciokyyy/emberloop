package memory

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// bankFiles lists every file under dir whose contents could hold bank text:
// bank databases, WAL and shared-memory files, and snapshots.
func bankFiles(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	out := make(map[string][]byte)
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[path] = data
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestPurgeLeavesNoTextOnDiskOrInSnapshots(t *testing.T) {
	b, dir, _ := openTestBank(t)
	ctx := context.Background()
	const secret = "zanzibar-quartz-secret"
	quote := "we never log " + secret

	obs, err := b.AddObservation(ctx, Observation{
		SessionID: "s1", EventID: "e1", Author: testUser, Quote: quote, Kind: KindPreference,
	})
	if err != nil {
		t.Fatal(err)
	}
	rule, err := b.AddRule(ctx, Rule{Text: "never log " + secret, Kind: KindPreference, ApprovedBy: testUser})
	if err != nil {
		t.Fatal(err)
	}
	// Queue and ledger hold the text too, but are not selected: purge must
	// scrub them rather than delete them.
	if _, err := b.Enqueue(ctx, QueueItem{Kind: QueueRule, Payload: `{"quote":"` + quote + `"}`}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.RecordLedger(ctx, LedgerRow{Stage: "mine", Status: LedgerFailed, Detail: "saw " + quote}); err != nil {
		t.Fatal(err)
	}
	snap, err := b.Snapshot(ctx)
	if err != nil || !snap.Taken {
		t.Fatalf("snapshot = %+v, %v", snap, err)
	}
	if !anyContains(bankFiles(t, dir), secret) {
		t.Fatal("sanity: secret should be on disk before purge")
	}

	res, err := b.Purge(ctx, Selection{Observations: []int64{obs}, Rules: []string{rule.RuleID}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Observations != 1 || res.Rules != 1 || res.Snapshots != 1 {
		t.Fatalf("purge = %+v, want 1 observation, 1 rule, 1 snapshot", res)
	}
	if res.Scrubbed < 2 {
		t.Fatalf("scrubbed = %d, want queue and ledger rows changed", res.Scrubbed)
	}
	for path, data := range bankFiles(t, dir) {
		if bytes.Contains(data, []byte(secret)) {
			t.Errorf("%s still holds purged text", strings.TrimPrefix(path, dir))
		}
	}
	queue, _ := b.Queue(ctx)
	if len(queue) != 1 || strings.Contains(queue[0].Payload, secret) {
		t.Errorf("queue after purge = %+v; want one item without the secret", queue)
	}
	ledger, _ := b.Ledger(ctx)
	if len(ledger) != 1 || strings.Contains(ledger[0].Detail, secret) {
		t.Errorf("ledger after purge = %+v; want one row without the secret", ledger)
	}
}

func anyContains(files map[string][]byte, s string) bool {
	for _, data := range files {
		if bytes.Contains(data, []byte(s)) {
			return true
		}
	}
	return false
}

func TestPurgeEmptySelectionIsNoOp(t *testing.T) {
	b, _, _ := openTestBank(t)
	ctx := context.Background()
	if _, err := b.AddRule(ctx, Rule{Text: "keep me", Kind: KindFact, ApprovedBy: testUser}); err != nil {
		t.Fatal(err)
	}
	res, err := b.Purge(ctx, Selection{})
	if err != nil {
		t.Fatal(err)
	}
	if res != (PurgeResult{}) {
		t.Fatalf("empty purge = %+v, want zero", res)
	}
	if got, _ := b.Recall(ctx, "keep", 5); len(got) != 1 {
		t.Fatalf("empty purge removed a rule")
	}
}

func TestPurgeDoesNotDeleteUnselectedRows(t *testing.T) {
	b, _, _ := openTestBank(t)
	ctx := context.Background()
	first, err := b.AddObservation(ctx, Observation{SessionID: "s", EventID: "e", Author: testUser, Quote: "one thing", Kind: KindFact})
	if err != nil {
		t.Fatal(err)
	}
	second, err := b.AddObservation(ctx, Observation{SessionID: "s", EventID: "e2", Author: testUser, Quote: "two thing", Kind: KindFact})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Purge(ctx, Selection{Observations: []int64{first}}); err != nil {
		t.Fatal(err)
	}
	got, err := b.SearchObservations(ctx, "thing", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != second {
		t.Fatalf("after purging %d, search = %+v; want only %d", first, got, second)
	}
}
