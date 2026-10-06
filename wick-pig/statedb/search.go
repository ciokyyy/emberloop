package statedb

import (
	"database/sql"
	"fmt"
)

// Search cache and quota helpers the websearch tool fills. The tables arrive
// with the v2 migration; this file owns their reads and writes, so the tool
// never hand-writes SQL against them.
//
// SearchCacheTTLSeconds is how long an answered search stays fresh: an
// identical search within 15 minutes answers from the row without calling
// the provider again.

// SearchCacheTTLSeconds is the search cache lifetime in seconds.
const SearchCacheTTLSeconds = 15 * 60

// SearchCacheRow is one answered search: key is the cache key (normalised
// query, resolved provider and arguments hashed); query, provider and args
// name what was asked (provider is the answering provider, args the request
// as JSON); results holds the rendered numbered list; created_at is unix
// seconds, and a row older than the TTL reads as a miss.
type SearchCacheRow struct {
	Key       string
	Query     string
	Provider  string
	Args      string
	Results   string
	CreatedAt int64
}

// SearchCacheGet reads the row for key. ok is false for a missing key and
// for a row older than the TTL; a row from the future (clock skew) still
// reads as a hit. nowUnix is unix seconds, so tests own the clock.
func SearchCacheGet(db *sql.DB, key string, nowUnix int64) (row SearchCacheRow, ok bool, err error) {
	row = SearchCacheRow{}
	if err := db.QueryRow(`SELECT key, query, provider, args, results, created_at
		FROM search_cache WHERE key = ?`, key).Scan(
		&row.Key, &row.Query, &row.Provider, &row.Args, &row.Results, &row.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return SearchCacheRow{}, false, nil
		}
		return SearchCacheRow{}, false, fmt.Errorf("statedb: search cache get: %w", err)
	}
	if nowUnix-row.CreatedAt > SearchCacheTTLSeconds {
		return SearchCacheRow{}, false, nil
	}
	return row, true, nil
}

// SearchCachePut stores one answered search, replacing the row for its key,
// so a fresh search refreshes the entry.
func SearchCachePut(db *sql.DB, row SearchCacheRow) error {
	if _, err := db.Exec(`INSERT OR REPLACE INTO search_cache(key, query, provider, args, results, created_at)
		VALUES(?,?,?,?,?,?)`,
		row.Key, row.Query, row.Provider, row.Args, row.Results, row.CreatedAt); err != nil {
		return fmt.Errorf("statedb: search cache put: %w", err)
	}
	return nil
}

// SearchQuotaRow is one live provider call: provider names the engine; cost
// holds Exa's reported cost when it reports one; remaining holds Brave's
// rate-limit remaining when its headers carry one; reset_at holds Brave's
// reset time; created_at is when the call happened. Absent stays NULL:
// nothing is enforced and nothing is estimated when a provider reports
// nothing. The TUI reads the latest row per provider.
type SearchQuotaRow struct {
	Provider  string
	Cost      *float64
	Remaining *int64
	ResetAt   string
	CreatedAt string
}

// InsertSearchQuota records one live provider call.
func InsertSearchQuota(db *sql.DB, q SearchQuotaRow) error {
	if _, err := db.Exec(`INSERT INTO search_quota(provider, cost, remaining, reset_at, created_at)
		VALUES(?,?,?,?,?)`,
		q.Provider, q.Cost, q.Remaining, q.ResetAt, q.CreatedAt); err != nil {
		return fmt.Errorf("statedb: search quota insert: %w", err)
	}
	return nil
}

// LatestSearchQuota returns the latest quota row for provider. ok is false
// when the provider never called.
func LatestSearchQuota(db *sql.DB, provider string) (q SearchQuotaRow, ok bool, err error) {
	q = SearchQuotaRow{}
	if err := db.QueryRow(`SELECT provider, cost, remaining, reset_at, created_at
		FROM search_quota WHERE provider = ? ORDER BY id DESC LIMIT 1`, provider).Scan(
		&q.Provider, &q.Cost, &q.Remaining, &q.ResetAt, &q.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return SearchQuotaRow{}, false, nil
		}
		return SearchQuotaRow{}, false, fmt.Errorf("statedb: latest search quota: %w", err)
	}
	return q, true, nil
}

// LatestReportedSearchQuota returns the latest quota row for provider that
// reports something: a cost, a remaining count or a reset time. A call that
// reached no headers (a failed call with none) still writes its row, but
// that empty row never shadows the last known quota in the footer.
// ok is false when no row reports anything.
func LatestReportedSearchQuota(db *sql.DB, provider string) (q SearchQuotaRow, ok bool, err error) {
	q = SearchQuotaRow{}
	if err := db.QueryRow(`SELECT provider, cost, remaining, reset_at, created_at
		FROM search_quota WHERE provider = ? AND (cost IS NOT NULL OR remaining IS NOT NULL OR reset_at != '')
		ORDER BY id DESC LIMIT 1`, provider).Scan(
		&q.Provider, &q.Cost, &q.Remaining, &q.ResetAt, &q.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return SearchQuotaRow{}, false, nil
		}
		return SearchQuotaRow{}, false, fmt.Errorf("statedb: latest reported search quota: %w", err)
	}
	return q, true, nil
}
