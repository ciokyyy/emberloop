package statedb

import (
	"testing"
	"time"
)

// Seam: search cache and quota helpers — a row within 15 minutes reads as a
// hit, an older row as a miss, and a put refreshes the entry; quota keeps one
// row per call and the latest row per provider, with absent values staying
// NULL.

func TestSearchCacheHitMissAndExpiry(t *testing.T) {
	db, _ := openTemp(t)
	now := time.Now().Unix()
	row := SearchCacheRow{Key: "k1", Query: "q", Provider: "brave", Args: `{"count":10}`, Results: "1. hi", CreatedAt: now}
	if err := SearchCachePut(db, row); err != nil {
		t.Fatal(err)
	}
	got, ok, err := SearchCacheGet(db, "k1", now)
	if err != nil || !ok {
		t.Fatalf("get = %+v, %v, %v; want a hit", got, ok, err)
	}
	if got.Results != "1. hi" || got.Provider != "brave" {
		t.Fatalf("get = %+v; want the stored row", got)
	}
	if _, ok, err := SearchCacheGet(db, "k1", now+SearchCacheTTLSeconds+1); err != nil || ok {
		t.Fatalf("expired get ok = %v, %v; want a miss", ok, err)
	}
	if _, ok, err := SearchCacheGet(db, "missing", now); err != nil || ok {
		t.Fatalf("missing get ok = %v, %v; want a miss", ok, err)
	}
}

func TestSearchCachePutRefreshes(t *testing.T) {
	db, _ := openTemp(t)
	now := time.Now().Unix()
	old := SearchCacheRow{Key: "k1", Query: "q", Provider: "brave", Args: `{}`, Results: "old", CreatedAt: now - SearchCacheTTLSeconds - 1}
	if err := SearchCachePut(db, old); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := SearchCacheGet(db, "k1", now); ok {
		t.Fatal("expired row reads as a hit")
	}
	fresh := SearchCacheRow{Key: "k1", Query: "q", Provider: "exa", Args: `{}`, Results: "new", CreatedAt: now}
	if err := SearchCachePut(db, fresh); err != nil {
		t.Fatal(err)
	}
	got, ok, err := SearchCacheGet(db, "k1", now)
	if err != nil || !ok || got.Results != "new" || got.Provider != "exa" {
		t.Fatalf("get after refresh = %+v, %v, %v; want the new row", got, ok, err)
	}
}

func TestSearchQuotaKeepsLatestPerProvider(t *testing.T) {
	db, _ := openTemp(t)
	cost := 0.0025
	remaining := int64(9989)
	if err := InsertSearchQuota(db, SearchQuotaRow{Provider: "exa", Cost: &cost, CreatedAt: "2026-10-06T10:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := InsertSearchQuota(db, SearchQuotaRow{Provider: "brave", Remaining: &remaining, ResetAt: "reset", CreatedAt: "2026-10-06T10:01:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := InsertSearchQuota(db, SearchQuotaRow{Provider: "exa", CreatedAt: "2026-10-06T10:02:00Z"}); err != nil {
		t.Fatal(err)
	}
	exa, ok, err := LatestSearchQuota(db, "exa")
	if err != nil || !ok {
		t.Fatalf("latest exa = %+v, %v, %v; want a row", exa, ok, err)
	}
	if exa.Cost != nil || exa.CreatedAt != "2026-10-06T10:02:00Z" {
		t.Fatalf("latest exa = %+v; want the second row with NULL cost", exa)
	}
	brave, ok, err := LatestSearchQuota(db, "brave")
	if err != nil || !ok {
		t.Fatalf("latest brave = %+v, %v, %v; want a row", brave, ok, err)
	}
	if brave.Remaining == nil || *brave.Remaining != 9989 || brave.ResetAt != "reset" {
		t.Fatalf("latest brave = %+v; want remaining 9989 with the reset", brave)
	}
	if _, ok, err := LatestSearchQuota(db, "tavily"); err != nil || ok {
		t.Fatalf("unknown provider ok = %v, %v; want none", ok, err)
	}
}

// An empty row never shadows the last reported quota: the reported lookup
// skips rows that carry nothing.
func TestLatestReportedSearchQuotaSkipsEmptyRows(t *testing.T) {
	db, _ := openTemp(t)
	remaining := int64(9989)
	if err := InsertSearchQuota(db, SearchQuotaRow{Provider: "brave", Remaining: &remaining, ResetAt: "reset", CreatedAt: "2026-10-06T10:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := InsertSearchQuota(db, SearchQuotaRow{Provider: "brave", CreatedAt: "2026-10-06T10:01:00Z"}); err != nil {
		t.Fatal(err)
	}
	got, ok, err := LatestReportedSearchQuota(db, "brave")
	if err != nil || !ok {
		t.Fatalf("latest reported = %+v, %v, %v; want the first row", got, ok, err)
	}
	if got.Remaining == nil || *got.Remaining != 9989 {
		t.Fatalf("latest reported = %+v; want remaining 9989", got)
	}
	if _, ok, err := LatestReportedSearchQuota(db, "exa"); err != nil || ok {
		t.Fatalf("unknown provider ok = %v, %v; want none", ok, err)
	}
}
