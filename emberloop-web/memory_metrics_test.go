package web

import (
	"testing"
	"time"

	"example.com/emberloop-web/memory"
)

func TestSummarizeMemoryMetricsUsesLedgerRows(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	rows := []memory.LedgerRow{
		{Stage: "ingest", CountsJSON: `{"stored":4}`, Detail: "model=extract cost_usd=0.02", CreatedAt: now.Add(-time.Hour)},
		{Stage: "ingest", CountsJSON: `{"stored":3}`, Detail: "cost_usd=0.01", CreatedAt: now.Add(-48 * time.Hour)},
		{Stage: "memory_stored", CountsJSON: `{"count":1,"memory_id":1}`, CreatedAt: now.Add(-10 * 24 * time.Hour)},
		{Stage: "memory_stored", CountsJSON: `{"count":1,"memory_id":2}`, CreatedAt: now.Add(-9 * 24 * time.Hour)},
		{Stage: "memory_stored", CountsJSON: `{"count":1,"memory_id":3}`, CreatedAt: now.Add(-2 * 24 * time.Hour)},
		{Stage: "memory_stored", CountsJSON: `{"count":1,"memory_id":4}`, CreatedAt: now.Add(-time.Hour)},
		{Stage: "memory_stored", CountsJSON: `{"count":1,"memory_id":5}`, CreatedAt: now.Add(-time.Hour)},
		{Stage: "memory_stored", CountsJSON: `{"count":1,"memory_id":6}`, CreatedAt: now.Add(-time.Hour)},
		{Stage: "memory_stored", CountsJSON: `{"count":1,"memory_id":7}`, CreatedAt: now.Add(-time.Hour)},
		{Stage: "memory_forgotten", CountsJSON: `{"count":1,"memory_id":1,"age_hours":48}`, Detail: now.Add(-10 * 24 * time.Hour).Format(time.RFC3339Nano), CreatedAt: now.Add(-8 * 24 * time.Hour)},
		{Stage: "memory_forgotten", CountsJSON: `{"count":1,"memory_id":2,"age_hours":900}`, Detail: now.Add(-9 * 24 * time.Hour).Format(time.RFC3339Nano), CreatedAt: now.Add(-time.Hour)},
		{Stage: "memory_forgotten", CountsJSON: `{"count":1,"memory_id":3,"age_hours":24}`, Detail: now.Add(-2 * 24 * time.Hour).Format(time.RFC3339Nano), CreatedAt: now.Add(-time.Hour)},
		{Stage: "memory_promoted", CountsJSON: `{"count":1,"memory_id":1}`, CreatedAt: now.Add(-time.Hour)},
		{Stage: "memory_promoted", CountsJSON: `{"count":1,"memory_id":3}`, CreatedAt: now.Add(-time.Hour)},
		{Stage: "injection", CountsJSON: `{"memories":6,"creates":4}`, CreatedAt: now.Add(-time.Hour)},
		{Stage: "memory_contradicted", CountsJSON: `{"count":3}`, CreatedAt: now.Add(-time.Hour)},
		{Stage: "embedding", Detail: "cost_usd=0.031", CreatedAt: now.Add(-time.Hour)},
	}
	got := calculateMemoryStats(rows, now)
	if got.MemoriesPerDay != 4 || got.Forgotten7Day != 0.5 || got.PromotedShare != 2.0/7 || got.InjectionHitRate != 1.5 || got.Contradictions != 3 {
		t.Fatalf("ledger-derived proxies = %+v", got)
	}
	if got.ExtractorCostUSD != .02 || got.EmbedderCostUSD != .031 || !got.CostAlert {
		t.Fatalf("ledger-derived costs = %+v, want .02 + .031 and alert", got)
	}
}
