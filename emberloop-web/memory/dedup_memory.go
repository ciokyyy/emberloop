package memory

import (
	"context"
	"database/sql"
	"math"
)

const (
	memoryDuplicateCosine = 0.95
	memoryRelationCosine  = 0.88
)

type MemoryStoreResult struct {
	ID         int64
	Relation   string
	Duplicate  bool
	Superseded bool
}

func memoryRelation(incoming, existing MemoryItem) string {
	if normalized := NormalizeQuote(incoming.Text); normalized != "" && normalized == NormalizeQuote(existing.Text) {
		return RelDuplicate
	}
	relation := Relation(incoming.Text, existing.Text)
	if incoming.Embedding == nil || existing.Embedding == nil || incoming.EmbeddingModel == "" || incoming.EmbeddingModel != existing.EmbeddingModel {
		return relation
	}
	a, errA := DecodeVector(incoming.Embedding)
	b, errB := DecodeVector(existing.Embedding)
	if errA != nil || errB != nil {
		return relation
	}
	cos := Cosine(a, b)
	shared := sharedSignificantTokens(incoming.Text, existing.Text)
	if relation == RelContradicts {
		if cos >= memoryRelationCosine && shared >= 2 {
			return RelContradicts
		}
		return ""
	}
	if cos >= memoryDuplicateCosine && shared > 0 {
		return RelDuplicate
	}
	if cos >= memoryRelationCosine {
		return RelRefines
	}
	return ""
}

func sharedSignificantTokens(a, b string) int {
	as, bs := tokenSet(NormalizeQuote(a)), tokenSet(NormalizeQuote(b))
	n := 0
	for token := range as {
		if bs[token] && !negationTokens[token] {
			n++
		}
	}
	return n
}

func DedupAndStoreMemory(ctx context.Context, mem Memory, item MemoryItem) (MemoryStoreResult, error) {
	return mem.DedupAndStoreMemory(ctx, item)
}

func (b *Bank) DedupAndStoreMemory(ctx context.Context, item MemoryItem) (result MemoryStoreResult, err error) {
	if item.Status == "" {
		item.Status = StatusActive
	}
	if item.Veracity == "" {
		item.Veracity = VeracityInferred
	}
	if item.MentionCount == 0 {
		item.MentionCount = 1
	}
	if item.Importance == 0 {
		item.Importance = 0.7
	}
	if err = validMemory(item); err != nil {
		return result, err
	}
	err = b.write(ctx, func(db *sql.DB) error {
		rows, e := db.QueryContext(ctx, `SELECT id, session_id, event_id, author, kind, text, quote, importance, veracity, mention_count, status, superseded_by, pinned, embedding, embedding_model, extractor_json, observed_commit, created_at, last_seen_at FROM memories WHERE status = 'active' ORDER BY id DESC`)
		if e != nil {
			return e
		}
		var active []MemoryItem
		for rows.Next() {
			var old MemoryItem
			var superseded sql.NullInt64
			var created, seen string
			if e = rows.Scan(&old.ID, &old.SessionID, &old.EventID, &old.Author, &old.Kind, &old.Text, &old.Quote, &old.Importance, &old.Veracity, &old.MentionCount, &old.Status, &superseded, &old.Pinned, &old.Embedding, &old.EmbeddingModel, &old.ExtractorJSON, &old.ObservedCommit, &created, &seen); e != nil {
				rows.Close()
				return e
			}
			if superseded.Valid {
				old.SupersededBy = superseded.Int64
			}
			old.CreatedAt, old.LastSeenAt = parseTimeText(created), parseTimeText(seen)
			active = append(active, old)
		}
		if e = rows.Err(); e != nil {
			rows.Close()
			return e
		}
		if e = rows.Close(); e != nil {
			return e
		}
		for _, old := range active {
			relation := memoryRelation(item, old)
			if relation != RelDuplicate {
				continue
			}
			now := b.now()
			_, e = db.ExecContext(ctx, `UPDATE memories SET mention_count = mention_count + 1, last_seen_at = ?, importance = MIN(0.95, importance + 0.05) WHERE id = ? AND status = 'active'`, timeText(now), old.ID)
			if e != nil {
				return e
			}
			result = MemoryStoreResult{ID: old.ID, Relation: relation, Duplicate: true}
			return nil
		}
		var supersedeID int64
		for _, old := range active {
			if old.Author == item.Author && memoryRelation(item, old) == RelContradicts {
				supersedeID = old.ID
				result.Relation, result.Superseded = RelContradicts, true
				break
			}
		}
		if !result.Superseded {
			for _, old := range active {
				relation := memoryRelation(item, old)
				if result.Relation == "" && (relation == RelContradicts || relation == RelRefines) {
					result.Relation = relation
				}
			}
		}
		split := SplitText(item.Text + " " + item.Quote)
		res, e := db.ExecContext(ctx, `INSERT INTO memories (session_id, event_id, author, kind, text, quote, split_text, importance, veracity, mention_count, status, superseded_by, pinned, embedding, embedding_model, extractor_json, observed_commit, created_at, last_seen_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, item.SessionID, item.EventID, item.Author, item.Kind, item.Text, item.Quote, split, item.Importance, item.Veracity, item.MentionCount, item.Status, nullableInt(item.SupersededBy), item.Pinned, item.Embedding, item.EmbeddingModel, item.ExtractorJSON, item.ObservedCommit, timeText(b.now()), timeText(orNow(item.LastSeenAt, b.now())))
		if e != nil {
			return e
		}
		result.ID, e = res.LastInsertId()
		if e != nil {
			return e
		}
		if result.Superseded {
			_, e = db.ExecContext(ctx, `UPDATE memories SET status = 'superseded', superseded_by = ? WHERE id = ? AND status = 'active'`, result.ID, supersedeID)
			if e != nil {
				return e
			}
		}
		return nil
	})
	if err == nil && !result.Duplicate && item.Embedding == nil {
		b.enqueueEmbedding("memory", item.Text, result.ID, func(ctx context.Context, vector []byte, model string) error {
			return b.SetMemoryEmbedding(ctx, result.ID, vector, model)
		})
	}
	return result, err
}

func (f *Fake) DedupAndStoreMemory(_ context.Context, item MemoryItem) (MemoryStoreResult, error) {
	if item.Status == "" {
		item.Status = StatusActive
	}
	if item.Veracity == "" {
		item.Veracity = VeracityInferred
	}
	if item.MentionCount == 0 {
		item.MentionCount = 1
	}
	if item.Importance == 0 {
		item.Importance = 0.7
	}
	if err := validMemory(item); err != nil {
		return MemoryStoreResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.mems {
		old := &f.mems[i]
		if old.Status != StatusActive {
			continue
		}
		r := memoryRelation(item, *old)
		if r == RelDuplicate {
			old.MentionCount++
			old.LastSeenAt = f.now()
			old.Importance = math.Min(0.95, old.Importance+0.05)
			return MemoryStoreResult{ID: old.ID, Relation: r, Duplicate: true}, nil
		}
	}
	supersedeIndex := -1
	for i := range f.mems {
		old := &f.mems[i]
		if old.Status == StatusActive && old.Author == item.Author && memoryRelation(item, *old) == RelContradicts {
			supersedeIndex = i
			break
		}
	}
	relation := ""
	if supersedeIndex >= 0 {
		relation = RelContradicts
	} else {
		for i := range f.mems {
			old := &f.mems[i]
			if old.Status != StatusActive {
				continue
			}
			r := memoryRelation(item, *old)
			if relation == "" && (r == RelContradicts || r == RelRefines) {
				relation = r
			}
		}
	}
	f.nextMem++
	item.ID, item.CreatedAt, item.LastSeenAt = f.nextMem, f.now(), f.now()
	if supersedeIndex >= 0 {
		f.mems[supersedeIndex].Status, f.mems[supersedeIndex].SupersededBy = StatusSuperseded, item.ID
	}
	f.mems = append(f.mems, item)
	return MemoryStoreResult{ID: item.ID, Relation: relation, Superseded: supersedeIndex >= 0}, nil
}
