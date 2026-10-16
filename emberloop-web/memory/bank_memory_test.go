package memory

import (
	"context"
	"database/sql"
	"strconv"
	"testing"
)

func TestBankMemoryGuardAndDelete(t *testing.T) {
	clock := newTestClock()
	m := factories()[0].open(t, clock)
	b := m.(*Bank)
	ctx := context.Background()
	id, err := b.AddMemory(ctx, MemoryItem{SessionID: "s", EventID: "e", Author: "u1", Kind: KindFact, Text: "remember this", Quote: "remember this"})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := b.Ledger(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Stage != "memory_stored" || rows[0].CountsJSON != `{"count":1,"memory_id":`+strconv.FormatInt(id, 10)+`}` {
		t.Fatalf("stored-memory cohort ledger = %+v", rows)
	}
	item, err := b.GetMemory(ctx, id)
	if err != nil || !rows[0].CreatedAt.Equal(item.CreatedAt) {
		t.Fatalf("cohort timestamp = %v, memory timestamp = %v, err %v", rows[0].CreatedAt, item.CreatedAt, err)
	}
	// guard refusal on text
	if err := b.write(ctx, func(db *sql.DB) error {
		_, e := b.wdb.ExecContext(ctx, "UPDATE memories SET text = ? WHERE id = ?", "changed", id)
		return e
	}); err == nil {
		t.Fatal("text update should be refused")
	}
	// guard refusal on author
	if err := b.write(ctx, func(db *sql.DB) error {
		_, e := b.wdb.ExecContext(ctx, "UPDATE memories SET author = ? WHERE id = ?", "u2", id)
		return e
	}); err == nil {
		t.Fatal("author update should be refused")
	}
	// DELETE refused
	if err := b.write(ctx, func(db *sql.DB) error {
		_, e := b.wdb.ExecContext(ctx, "DELETE FROM memories WHERE id = ?", id)
		return e
	}); err == nil {
		t.Fatal("DELETE should be refused outside purge")
	}
}
