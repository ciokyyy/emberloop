package memory

import (
	"context"
	"math"
	"sort"
	"time"
)

// RecallWeights configure the relative contribution of semantic, lexical and
// importance signals. Zero values select the documented defaults.
type RecallWeights struct{ Vector, FTS, Importance float64 }

type MemoryHit struct {
	Memory MemoryItem
	Score  float64
}
type EpisodeHit struct {
	Episode Episode
	Score   float64
}

func normalizedWeights(w RecallWeights, hasVector bool) RecallWeights {
	if w == (RecallWeights{}) {
		w = RecallWeights{Vector: .5, FTS: .3, Importance: .2}
	}
	if !hasVector {
		w.Vector = 0
	}
	total := w.Vector + w.FTS + w.Importance
	if total <= 0 {
		return RecallWeights{FTS: 1}
	}
	return RecallWeights{w.Vector / total, w.FTS / total, w.Importance / total}
}

func kindDecay(m MemoryItem, now time.Time) float64 {
	if m.Pinned != 0 || m.Kind == KindPreference || m.Kind == KindInstruction {
		return 1
	}
	half := 0.0
	switch m.Kind {
	case KindDecision:
		half = 180
	case KindFact:
		half = 90
	}
	if half == 0 || m.CreatedAt.IsZero() {
		return 1
	}
	days := now.Sub(m.CreatedAt).Hours() / 24
	if days < 0 {
		days = 0
	}
	return math.Pow(.5, days/half)
}

// RecallMemories returns up to 40 FTS and 40 vector candidates, then scores
// their union. BM25 normalization is confined to this FTS table's candidate
// set; no score from Rules, notes or Episodes is compared here.
func (b *Bank) RecallMemories(ctx context.Context, text string, weights RecallWeights, limit int) ([]MemoryHit, error) {
	q := Query(text)
	if q == "" {
		return nil, nil
	}
	fts := map[int64]float64{}
	rows, err := b.rdb.QueryContext(ctx, `SELECT m.id,bm25(memories_fts) FROM memories_fts JOIN memories m ON m.id=memories_fts.rowid WHERE memories_fts MATCH ? AND m.status='active' ORDER BY bm25(memories_fts),m.id LIMIT 40`, q)
	if err != nil {
		return nil, err
	}
	var min, max float64
	first := true
	for rows.Next() {
		var id int64
		var score float64
		if err := rows.Scan(&id, &score); err != nil {
			rows.Close()
			return nil, err
		}
		fts[id] = score
		if first || score < min {
			min = score
		}
		if first || score > max {
			max = score
		}
		first = false
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	queryVec, model := b.queryVector(ctx, text)
	vectorScores := map[int64]float64{}
	if len(queryVec) > 0 {
		vs, err := b.rdb.QueryContext(ctx, `SELECT id,embedding,embedding_model FROM memories WHERE status='active' AND embedding IS NOT NULL`)
		if err != nil {
			return nil, err
		}
		candidates := []VectorCandidate{}
		for vs.Next() {
			var c VectorCandidate
			if err := vs.Scan(&c.ID, &c.Vector, &c.Model); err != nil {
				vs.Close()
				return nil, err
			}
			candidates = append(candidates, c)
		}
		if err := vs.Err(); err != nil {
			vs.Close()
			return nil, err
		}
		vs.Close()
		hits, _ := VectorSearch(ctx, candidates, queryVec, model, 40, EmbeddingQueryBudget, nil)
		for _, h := range hits {
			vectorScores[h.ID] = h.Similarity
		}
	}
	ids := map[int64]bool{}
	for id := range fts {
		ids[id] = true
	}
	for id := range vectorScores {
		ids[id] = true
	}
	if len(ids) == 0 {
		return nil, nil
	}
	out := []MemoryHit{}
	for id := range ids {
		m, e := b.GetMemory(ctx, id)
		if e != nil || m.Status != StatusActive {
			continue
		}
		cos, hasVec := vectorScores[id]
		_, hasFTS := fts[id]
		if hasVec && cos < .30 && !hasFTS {
			continue
		}
		lex := 0.0
		if hasFTS {
			if max == min {
				lex = 1
			} else {
				lex = (max - fts[id]) / (max - min)
			}
		}
		imp := m.Importance * kindDecay(m, b.now())
		if m.Veracity == VeracityInferred {
			imp *= .8
		}
		candidateWeights := normalizedWeights(weights, hasVec)
		score := candidateWeights.FTS*lex + candidateWeights.Importance*imp
		if hasVec {
			score += candidateWeights.Vector * cos
		}
		out = append(out, MemoryHit{m, score})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Memory.ID < out[j].Memory.ID
		}
		return out[i].Score > out[j].Score
	})
	if limit <= 0 || limit > MaxRecall {
		limit = MaxRecall
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (b *Bank) queryVector(ctx context.Context, text string) ([]float32, string) {
	if b.embedder == nil || b.embeddingModel == "" {
		return nil, ""
	}
	if v, ok := b.queryCache.Get(b.embeddingModel, text); ok {
		return v, b.embeddingModel
	}
	qctx, cancel := context.WithTimeout(ctx, EmbeddingQueryBudget)
	defer cancel()
	vectors, err := b.embedder.Embed(qctx, b.embeddingModel, []string{ForEmbedder(text)})
	if err != nil || len(vectors) != 1 || len(vectors[0]) == 0 {
		return nil, ""
	}
	b.queryCache.Put(b.embeddingModel, text, vectors[0])
	return vectors[0], b.embeddingModel
}

// RecallEpisodes searches episode text for the agent recall tool only. These
// results are intentionally not used by the injection builder.
func (b *Bank) RecallEpisodes(ctx context.Context, text string, limit int) ([]EpisodeHit, error) {
	q := Query(text)
	if q == "" {
		return nil, nil
	}
	rows, err := b.rdb.QueryContext(ctx, `SELECT e.id,e.session_id,e.event_range,e.author,e.text,e.importance,e.embedding,e.embedding_model,e.consolidated_at,e.created_at,bm25(episodes_fts) FROM episodes_fts JOIN episodes e ON e.id=episodes_fts.rowid WHERE episodes_fts MATCH ? ORDER BY bm25(episodes_fts),e.id LIMIT 40`, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var raw []struct {
		e Episode
		s float64
	}
	for rows.Next() {
		var e Episode
		var consolidated, created string
		var s float64
		if err := rows.Scan(&e.ID, &e.SessionID, &e.EventRange, &e.Author, &e.Text, &e.Importance, &e.Embedding, &e.EmbeddingModel, &consolidated, &created, &s); err != nil {
			return nil, err
		}
		e.ConsolidatedAt = parseTimeText(consolidated)
		e.CreatedAt = parseTimeText(created)
		raw = append(raw, struct {
			e Episode
			s float64
		}{e, s})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var lo, hi float64
	if len(raw) > 0 {
		lo, hi = raw[0].s, raw[0].s
		for _, r := range raw {
			if r.s < lo {
				lo = r.s
			}
			if r.s > hi {
				hi = r.s
			}
		}
	}
	out := make([]EpisodeHit, 0, len(raw))
	for _, r := range raw {
		score := r.e.Importance
		if hi != lo {
			score += .35 * (hi - r.s) / (hi - lo)
		}
		out = append(out, EpisodeHit{r.e, score})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if limit <= 0 || limit > MaxRecall {
		limit = MaxRecall
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

