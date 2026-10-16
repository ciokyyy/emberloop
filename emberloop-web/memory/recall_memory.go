package memory

import (
	"context"
	"database/sql"
	"strings"
)

// RecallMemoryItems searches Memory text and quotes. Superseded rows are hidden
// unless explicitly requested; forgotten and parked rows are never recalled.
func (b *Bank) RecallMemoryItems(ctx context.Context, text string, limit int, includeSuperseded bool) ([]MemoryItem, error) {
	query := Query(text)
	if query == "" {
		return []MemoryItem{}, nil
	}
	statusFilter := `m.status = 'active'`
	if includeSuperseded {
		statusFilter = `(m.status = 'active' OR m.status = 'superseded')`
	}
	rows, err := b.rdb.QueryContext(ctx, `SELECT m.id, m.session_id, m.event_id, m.author, m.kind, m.text, m.quote, m.importance, m.veracity, m.mention_count, m.status, m.superseded_by, m.pinned, m.embedding, m.embedding_model, m.extractor_json, m.observed_commit, m.created_at, m.last_seen_at FROM memories_fts JOIN memories m ON m.id = memories_fts.rowid WHERE memories_fts MATCH ? AND `+statusFilter+` ORDER BY bm25(memories_fts), m.id DESC LIMIT ?`, query, clampLimit(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMemories(rows)
}

func (f *Fake) RecallMemoryItems(_ context.Context, text string, limit int, includeSuperseded bool) ([]MemoryItem, error) {
	terms := tokenizeQuote(text)
	if len(terms) == 0 {
		return []MemoryItem{}, nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []MemoryItem
	for _, item := range f.mems {
		if item.Status != StatusActive && !(includeSuperseded && item.Status == StatusSuperseded) {
			continue
		}
		content := " " + strings.ToLower(item.Text+" "+item.Quote) + " "
		for _, term := range terms {
			if strings.Contains(content, term) {
				out = append(out, item)
				break
			}
		}
	}
	if len(out) > clampLimit(limit) {
		out = out[:clampLimit(limit)]
	}
	return out, nil
}

func scanMemories(rows *sql.Rows) ([]MemoryItem, error) {
	var out []MemoryItem
	for rows.Next() {
		var m MemoryItem
		var superseded sql.NullInt64
		var created, seen string
		if err := rows.Scan(&m.ID, &m.SessionID, &m.EventID, &m.Author, &m.Kind, &m.Text, &m.Quote, &m.Importance, &m.Veracity, &m.MentionCount, &m.Status, &superseded, &m.Pinned, &m.Embedding, &m.EmbeddingModel, &m.ExtractorJSON, &m.ObservedCommit, &created, &seen); err != nil {
			return nil, err
		}
		if superseded.Valid {
			m.SupersededBy = superseded.Int64
		}
		m.CreatedAt, m.LastSeenAt = parseTimeText(created), parseTimeText(seen)
		out = append(out, m)
	}
	return out, rows.Err()
}
