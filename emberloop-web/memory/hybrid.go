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
	values := []*float64{&w.Vector, &w.FTS, &w.Importance}
	for _, v := range values {
		if math.IsNaN(*v) || math.IsInf(*v, 0) || *v < 0 {
			*v = 0
		}
	}
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
	vectorScores := make(map[int64]float64)
	if len(queryVec) > 0 {
		for _, hit := range b.vectorSearchTable(ctx, `SELECT id,embedding,embedding_model FROM memories WHERE status='active' AND embedding IS NOT NULL ORDER BY id LIMIT 50000`, queryVec, model) {
			vectorScores[hit.ID] = hit.Similarity
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
		imp := m.Importance
		candidateWeights := normalizedWeights(weights, hasVec)
		score := candidateWeights.FTS*lex + candidateWeights.Importance*imp
		if hasVec {
			score += candidateWeights.Vector * cos
		}
		score *= .7 + .3*kindDecay(m, b.now())
		if m.Veracity == VeracityInferred {
			score *= .8
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
	_, embedder, model := b.embeddingConfig()
	if embedder == nil || model == "" {
		return nil, ""
	}
	if v, ok := b.queryCache.Get(model, text); ok {
		return v, model
	}
	qctx, cancel := context.WithTimeout(ctx, EmbeddingQueryBudget)
	defer cancel()
	vectors, err := embedder.Embed(qctx, model, []string{ForEmbedder(text)})
	if err != nil || len(vectors) != 1 || len(vectors[0]) == 0 {
		return nil, ""
	}
	b.queryCache.Put(model, text, vectors[0])
	return vectors[0], model
}

// RecallEpisodes searches Episodes for the agent recall tool only. Results
// use the same per-table FTS/vector/importance scoring as Memory recall and
// are never passed to the injection builder.
func (b *Bank) RecallEpisodes(ctx context.Context, text string, weights RecallWeights, limit int) ([]EpisodeHit, error) {
	q := Query(text)
	if q == "" {
		return nil, nil
	}
	rows, err := b.rdb.QueryContext(ctx, `SELECT e.id,e.session_id,e.event_range,e.author,e.text,e.importance,e.embedding,e.embedding_model,e.consolidated_at,e.created_at,bm25(episodes_fts) FROM episodes_fts JOIN episodes e ON e.id=episodes_fts.rowid WHERE episodes_fts MATCH ? ORDER BY bm25(episodes_fts),e.id LIMIT 40`, q)
	if err != nil {
		return nil, err
	}
	type rankedEpisode struct {
		e    Episode
		bm25 float64
	}
	fts := map[int64]rankedEpisode{}
	for rows.Next() {
		var e Episode
		var c, created string
		var rank float64
		if err := rows.Scan(&e.ID, &e.SessionID, &e.EventRange, &e.Author, &e.Text, &e.Importance, &e.Embedding, &e.EmbeddingModel, &c, &created, &rank); err != nil {
			rows.Close()
			return nil, err
		}
		e.ConsolidatedAt = parseTimeText(c)
		e.CreatedAt = parseTimeText(created)
		fts[e.ID] = rankedEpisode{e, rank}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	lo, hi := 0.0, 0.0
	first := true
	for _, r := range fts {
		if first || r.bm25 < lo {
			lo = r.bm25
		}
		if first || r.bm25 > hi {
			hi = r.bm25
		}
		first = false
	}
	vec, model := b.queryVector(ctx, text)
	vector := map[int64]float64{}
	if len(vec) > 0 {
		for _, hit := range b.vectorSearchTable(ctx, `SELECT id,embedding,embedding_model FROM episodes WHERE embedding IS NOT NULL ORDER BY id LIMIT 50000`, vec, model) {
			vector[hit.ID] = hit.Similarity
		}
	}
	ids := map[int64]bool{}
	for id := range fts {
		ids[id] = true
	}
	for id := range vector {
		ids[id] = true
	}
	out := make([]EpisodeHit, 0, len(ids))
	for id := range ids {
		r, ok := fts[id]
		if !ok {
			e, err := b.GetEpisode(ctx, id)
			if err != nil {
				continue
			}
			r.e = e
		}
		cos, hasVec := vector[id]
		lex := 0.0
		if _, ok := fts[id]; ok {
			if hi == lo {
				lex = 1
			} else {
				lex = (hi - r.bm25) / (hi - lo)
			}
		}
		w := normalizedWeights(weights, hasVec)
		score := w.FTS*lex + w.Importance*r.e.Importance
		if hasVec {
			score += w.Vector * cos
		}
		out = append(out, EpisodeHit{r.e, score})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Episode.ID < out[j].Episode.ID
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

// vectorSearchTable shares one 800ms context across loading rows and cosine ranking.
// If loading or ranking times out, callers retain the FTS-only result set.
func (b *Bank) vectorSearchTable(ctx context.Context, query string, vector []float32, model string) []VectorHit {
	budgetCtx, cancel := context.WithTimeout(ctx, EmbeddingQueryBudget)
	defer cancel()
	rows, err := b.rdb.QueryContext(budgetCtx, query)
	if err != nil {
		return nil
	}
	candidates := make([]VectorCandidate, 0, MaxVectorsPerBank)
	for rows.Next() {
		if len(candidates)%64 == 0 && budgetCtx.Err() != nil {
			rows.Close()
			return nil
		}
		var c VectorCandidate
		if err := rows.Scan(&c.ID, &c.Vector, &c.Model); err != nil {
			rows.Close()
			return nil
		}
		candidates = append(candidates, c)
	}
	if rows.Err() != nil || budgetCtx.Err() != nil {
		rows.Close()
		return nil
	}
	rows.Close()
	hits, err := VectorSearch(budgetCtx, candidates, vector, model, 40, EmbeddingQueryBudget, nil)
	if err != nil {
		return nil
	}
	return hits
}
