package memory

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

func (b *Bank) insertMemory(ctx context.Context, m MemoryItem) (int64, error) {
	if m.Status == "" {
		m.Status = StatusActive
	}
	if m.Veracity == "" {
		m.Veracity = VeracityInferred
	}
	if m.MentionCount == 0 {
		m.MentionCount = 1
	}
	if m.Importance == 0 {
		m.Importance = 0.7
	}
	if err := validMemory(m); err != nil {
		return 0, err
	}
	split := SplitText(m.Text + " " + m.Quote)
	var id int64
	err := b.write(ctx, func(db *sql.DB) error {
		res, err := db.ExecContext(ctx, `INSERT INTO memories (session_id, event_id, author, kind, text, quote, split_text, importance, veracity, mention_count, status, superseded_by, pinned, embedding, embedding_model, extractor_json, observed_commit, created_at, last_seen_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, m.SessionID, m.EventID, m.Author, m.Kind, m.Text, m.Quote, split, m.Importance, m.Veracity, m.MentionCount, m.Status, nullableInt(m.SupersededBy), m.Pinned, m.Embedding, m.EmbeddingModel, m.ExtractorJSON, m.ObservedCommit, timeText(b.now()), timeText(orNow(m.LastSeenAt, b.now())))
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		return err
	})
	if err == nil {
		b.enqueueEmbedding("memory", m.Text, id, func(ctx context.Context, vector []byte, model string) error {
			return b.SetMemoryEmbedding(ctx, id, vector, model)
		})
	}
	return id, err
}
func (b *Bank) AddMemory(ctx context.Context, m MemoryItem) (int64, error) {
	return b.insertMemory(ctx, m)
}
func (b *Bank) InsertMemory(ctx context.Context, m MemoryItem) (int64, error) {
	return b.insertMemory(ctx, m)
}
func (b *Bank) getMemory(ctx context.Context, id int64) (MemoryItem, error) {
	var m MemoryItem
	var created, lastSeen string
	var superseded sql.NullInt64
	var embedding []byte
	err := b.rdb.QueryRowContext(ctx, `SELECT id, session_id, event_id, author, kind, text, quote, importance, veracity, mention_count, status, superseded_by, pinned, embedding, embedding_model, extractor_json, observed_commit, created_at, last_seen_at FROM memories WHERE id = ?`, id).Scan(&m.ID, &m.SessionID, &m.EventID, &m.Author, &m.Kind, &m.Text, &m.Quote, &m.Importance, &m.Veracity, &m.MentionCount, &m.Status, &superseded, &m.Pinned, &embedding, &m.EmbeddingModel, &m.ExtractorJSON, &m.ObservedCommit, &created, &lastSeen)
	if err != nil {
		if err == sql.ErrNoRows {
			return MemoryItem{}, fmt.Errorf("%w: memory %d", ErrNotFound, id)
		}
		return MemoryItem{}, err
	}
	if superseded.Valid {
		m.SupersededBy = superseded.Int64
	}
	m.Embedding = embedding
	m.CreatedAt = parseTimeText(created)
	m.LastSeenAt = parseTimeText(lastSeen)
	return m, nil
}
func (b *Bank) GetMemory(ctx context.Context, id int64) (MemoryItem, error) {
	return b.getMemory(ctx, id)
}
func (b *Bank) MemoryByID(ctx context.Context, id int64) (MemoryItem, error) {
	return b.getMemory(ctx, id)
}
func (b *Bank) ListMemories(ctx context.Context, kind, status string, limit int) ([]MemoryItem, error) {
	q := `SELECT id, session_id, event_id, author, kind, text, quote, importance, veracity, mention_count, status, superseded_by, pinned, embedding, embedding_model, extractor_json, observed_commit, created_at, last_seen_at FROM memories WHERE 1=1`
	var args []any
	if kind != "" {
		q += " AND kind = ?"
		args = append(args, kind)
	}
	if status != "" {
		q += " AND status = ?"
		args = append(args, status)
	} else {
		q += " AND status != 'forgotten'"
	}
	q += " ORDER BY id DESC LIMIT ?"
	args = append(args, clampList(limit))
	rows, err := b.rdb.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MemoryItem
	for rows.Next() {
		var m MemoryItem
		var created, lastSeen string
		var superseded sql.NullInt64
		var embedding []byte
		if err := rows.Scan(&m.ID, &m.SessionID, &m.EventID, &m.Author, &m.Kind, &m.Text, &m.Quote, &m.Importance, &m.Veracity, &m.MentionCount, &m.Status, &superseded, &m.Pinned, &embedding, &m.EmbeddingModel, &m.ExtractorJSON, &m.ObservedCommit, &created, &lastSeen); err != nil {
			return nil, err
		}
		if superseded.Valid {
			m.SupersededBy = superseded.Int64
		}
		m.Embedding = embedding
		m.CreatedAt = parseTimeText(created)
		m.LastSeenAt = parseTimeText(lastSeen)
		out = append(out, m)
	}
	return out, rows.Err()
}
func (b *Bank) SearchMemories(ctx context.Context, text, kind, status string, limit int) ([]MemoryItem, error) {
	q := Query(text)
	if q == "" {
		return b.ListMemories(ctx, kind, status, limit)
	}
	query := `SELECT m.id FROM memories_fts JOIN memories m ON m.id=memories_fts.rowid WHERE memories_fts MATCH ?`
	args := []any{q}
	if kind != "" {
		query += ` AND m.kind = ?`
		args = append(args, kind)
	}
	if status != "" {
		query += ` AND m.status = ?`
		args = append(args, status)
	} else {
		query += ` AND m.status != ?`
		args = append(args, StatusForgotten)
	}
	query += ` ORDER BY bm25(memories_fts),m.id DESC LIMIT ?`
	args = append(args, clampLimit(limit))
	rows, err := b.rdb.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]MemoryItem, 0, len(ids))
	for _, id := range ids {
		m, err := b.GetMemory(ctx, id)
		if err != nil {
			return nil, err
		}
		if kind != "" && m.Kind != kind || status != "" && m.Status != status || status == "" && m.Status == StatusForgotten {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

func (b *Bank) CountMemoriesSince(ctx context.Context, since time.Time) (int, error) {
	var count int
	err := b.rdb.QueryRowContext(ctx, `SELECT COUNT(*) FROM memories WHERE created_at > ?`, timeText(since)).Scan(&count)
	return count, err
}

func (b *Bank) ReviseMemory(ctx context.Context, oldID int64, m MemoryItem) (int64, error) {
	m.ID, m.Status, m.SupersededBy, m.Pinned = 0, StatusActive, 0, 0
	m.Embedding, m.EmbeddingModel = nil, ""
	if m.Veracity == "" {
		m.Veracity = VeracityInferred
	}
	if m.MentionCount == 0 {
		m.MentionCount = 1
	}
	if m.Importance == 0 {
		m.Importance = 0.7
	}
	if err := validMemory(m); err != nil {
		return 0, err
	}
	var id int64
	split := SplitText(m.Text + " " + m.Quote)
	err := b.write(ctx, func(db *sql.DB) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		res, err := tx.ExecContext(ctx, `INSERT INTO memories (session_id,event_id,author,kind,text,quote,split_text,importance,veracity,mention_count,status,superseded_by,pinned,embedding,embedding_model,extractor_json,observed_commit,created_at,last_seen_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, m.SessionID, m.EventID, m.Author, m.Kind, m.Text, m.Quote, split, m.Importance, m.Veracity, m.MentionCount, StatusActive, nil, 0, nil, "", m.ExtractorJSON, m.ObservedCommit, timeText(b.now()), timeText(b.now()))
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		if err != nil {
			return err
		}
		updated, err := tx.ExecContext(ctx, `UPDATE memories SET status=?,superseded_by=? WHERE id=? AND status NOT IN (?,?)`, StatusSuperseded, id, oldID, StatusForgotten, StatusSuperseded)
		if err != nil {
			return err
		}
		if n, _ := updated.RowsAffected(); n == 0 {
			return fmt.Errorf("%w: memory %d", ErrNotFound, oldID)
		}
		return tx.Commit()
	})
	if err == nil {
		b.enqueueEmbedding("memory", m.Text, id, func(ctx context.Context, vector []byte, model string) error {
			return b.SetMemoryEmbedding(ctx, id, vector, model)
		})
	}
	return id, err
}

func (b *Bank) SetMemoryStatus(ctx context.Context, id int64, status string) error {
	if !validMemoryStatus(status) {
		return fmt.Errorf("%w: status %q", ErrInvalid, status)
	}
	return b.write(ctx, func(db *sql.DB) error {
		res, err := db.ExecContext(ctx, `UPDATE memories SET status = ? WHERE id = ?`, status, id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("%w: memory %d", ErrNotFound, id)
		}
		return nil
	})
}

func (b *Bank) SetMemorySuperseded(ctx context.Context, id, supersededBy int64) error {
	if id <= 0 || supersededBy <= 0 || id == supersededBy {
		return fmt.Errorf("%w: invalid Memory supersession", ErrInvalid)
	}
	return b.write(ctx, func(db *sql.DB) error {
		res, err := db.ExecContext(ctx, `UPDATE memories SET status = ?, superseded_by = ? WHERE id = ?`, StatusSuperseded, supersededBy, id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("%w: memory %d", ErrNotFound, id)
		}
		return nil
	})
}
func (b *Bank) PinMemory(ctx context.Context, id int64, pinned int) error {
	return b.write(ctx, func(db *sql.DB) error {
		res, err := db.ExecContext(ctx, `UPDATE memories SET pinned = ? WHERE id = ?`, pinned, id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("%w: memory %d", ErrNotFound, id)
		}
		return nil
	})
}
func (b *Bank) SetMemoryPinned(ctx context.Context, id int64, pinned bool) error {
	v := 0
	if pinned {
		v = 1
	}
	return b.PinMemory(ctx, id, v)
}
func (b *Bank) BumpMemory(ctx context.Context, id int64) (MemoryItem, error) {
	return b.bumpMemory(ctx, id)
}
func (b *Bank) BumpMemoryMention(ctx context.Context, id int64) (MemoryItem, error) {
	return b.bumpMemory(ctx, id)
}
func (b *Bank) bumpMemory(ctx context.Context, id int64) (MemoryItem, error) {
	err := b.write(ctx, func(db *sql.DB) error {
		res, err := db.ExecContext(ctx, `UPDATE memories SET mention_count = mention_count + 1, last_seen_at = ? WHERE id = ?`, timeText(b.now()), id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("%w: memory %d", ErrNotFound, id)
		}
		return nil
	})
	if err != nil {
		return MemoryItem{}, err
	}
	return b.GetMemory(ctx, id)
}
func (b *Bank) SetMemoryEmbedding(ctx context.Context, id int64, embedding []byte, model string) error {
	return b.write(ctx, func(db *sql.DB) error {
		res, err := db.ExecContext(ctx, `UPDATE memories SET embedding = ?, embedding_model = ? WHERE id = ?`, embedding, model, id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("%w: memory %d", ErrNotFound, id)
		}
		return nil
	})
}

// episodes
func (b *Bank) insertEpisode(ctx context.Context, e Episode) (int64, error) {
	if e.Importance == 0 {
		e.Importance = 0.65
	}
	if err := validEpisode(e); err != nil {
		return 0, err
	}
	split := SplitText(e.Text)
	var id int64
	err := b.write(ctx, func(db *sql.DB) error {
		res, err := db.ExecContext(ctx, `INSERT INTO episodes (session_id, event_range, author, text, split_text, importance, embedding, embedding_model, consolidated_at, created_at) VALUES (?,?,?,?,?,?,?,?,?,?)`, e.SessionID, e.EventRange, e.Author, e.Text, split, e.Importance, e.Embedding, e.EmbeddingModel, timeText(e.ConsolidatedAt), timeText(orNow(e.CreatedAt, b.now())))
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		return err
	})
	if err == nil {
		b.enqueueEmbedding("episode", e.Text, id, func(ctx context.Context, vector []byte, model string) error {
			return b.SetEpisodeEmbedding(ctx, id, vector, model)
		})
	}
	return id, err
}
func (b *Bank) AddEpisode(ctx context.Context, e Episode) (int64, error) {
	return b.insertEpisode(ctx, e)
}
func (b *Bank) InsertEpisode(ctx context.Context, e Episode) (int64, error) {
	return b.insertEpisode(ctx, e)
}
func (b *Bank) getEpisode(ctx context.Context, id int64) (Episode, error) {
	var e Episode
	var created, consolidated string
	var embedding []byte
	err := b.rdb.QueryRowContext(ctx, `SELECT id, session_id, event_range, author, text, importance, embedding, embedding_model, consolidated_at, created_at FROM episodes WHERE id = ?`, id).Scan(&e.ID, &e.SessionID, &e.EventRange, &e.Author, &e.Text, &e.Importance, &embedding, &e.EmbeddingModel, &consolidated, &created)
	if err != nil {
		if err == sql.ErrNoRows {
			return Episode{}, fmt.Errorf("%w: episode %d", ErrNotFound, id)
		}
		return Episode{}, err
	}
	e.Embedding = embedding
	e.ConsolidatedAt = parseTimeText(consolidated)
	e.CreatedAt = parseTimeText(created)
	return e, nil
}
func (b *Bank) GetEpisode(ctx context.Context, id int64) (Episode, error) {
	return b.getEpisode(ctx, id)
}
func (b *Bank) EpisodeByID(ctx context.Context, id int64) (Episode, error) {
	return b.getEpisode(ctx, id)
}
func (b *Bank) ListEpisodes(ctx context.Context, limit int) ([]Episode, error) {
	rows, err := b.rdb.QueryContext(ctx, `SELECT id, session_id, event_range, author, text, importance, embedding, embedding_model, consolidated_at, created_at FROM episodes ORDER BY id DESC LIMIT ?`, clampList(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Episode
	for rows.Next() {
		var e Episode
		var created, consolidated string
		var embedding []byte
		if err := rows.Scan(&e.ID, &e.SessionID, &e.EventRange, &e.Author, &e.Text, &e.Importance, &embedding, &e.EmbeddingModel, &consolidated, &created); err != nil {
			return nil, err
		}
		e.Embedding = embedding
		e.ConsolidatedAt = parseTimeText(consolidated)
		e.CreatedAt = parseTimeText(created)
		out = append(out, e)
	}
	return out, rows.Err()
}
func (b *Bank) SetEpisodeEmbedding(ctx context.Context, id int64, embedding []byte, model string) error {
	return b.write(ctx, func(db *sql.DB) error {
		res, err := db.ExecContext(ctx, `UPDATE episodes SET embedding = ?, embedding_model = ? WHERE id = ?`, embedding, model, id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("%w: episode %d", ErrNotFound, id)
		}
		return nil
	})
}
func (b *Bank) SetEpisodeConsolidated(ctx context.Context, id int64, at time.Time) error {
	return b.write(ctx, func(db *sql.DB) error {
		res, err := db.ExecContext(ctx, `UPDATE episodes SET consolidated_at = ? WHERE id = ?`, timeText(at), id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("%w: episode %d", ErrNotFound, id)
		}
		return nil
	})
}

func nullableInt(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}
func orNow(t, fallback time.Time) time.Time {
	if t.IsZero() {
		return fallback
	}
	return t
}
func (b *Bank) enqueueEmbedding(kind, text string, id int64, store func(context.Context, []byte, string) error) {
	worker, _, _ := b.embeddingConfig()
	if worker == nil {
		return
	}
	job := EmbeddingJob{Text: text, Store: store, OnError: func(error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = b.RecordLedger(ctx, LedgerRow{RunID: fmt.Sprintf("embedding-%s-%d", kind, id), Stage: "embedding", Status: LedgerFailed, CountsJSON: `{"failed":1}`, Detail: "embedding computation or persistence failed"})
	}}
	if !worker.Enqueue(job) {
		job.OnError(fmt.Errorf("embedding queue unavailable"))
	}
}
