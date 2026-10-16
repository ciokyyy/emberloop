package memory

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"sync"
	"time"
)

const (
	EmbeddingModelSetting = "memory_embedding_model"
	DefaultEmbeddingModel = "qwen/qwen3-embedding-4b"
	MaxEmbeddingBatch     = 32
	MaxVectorsPerBank     = 50000
	EmbeddingQueryBudget  = 800 * time.Millisecond
	// EmbeddingRequestTimeout bounds asynchronous row embedding, not query recall.
	EmbeddingRequestTimeout = 30 * time.Second
)

// Embedder turns redacted text into vectors. Returned vectors preserve input order.
type Embedder interface {
	Embed(context.Context, string, []string) ([][]float32, error)
}

type OpenRouterEmbedder struct {
	APIKey   string
	Endpoint string
	Client   *http.Client
}

func (e *OpenRouterEmbedder) Embed(ctx context.Context, model string, texts []string) ([][]float32, error) {
	if model == "" || len(texts) == 0 || len(texts) > MaxEmbeddingBatch {
		return nil, fmt.Errorf("memory: invalid embedding model or batch size")
	}
	if e == nil || e.APIKey == "" {
		return nil, errors.New("memory: OpenRouter API key is missing")
	}
	inputs := append([]string(nil), texts...)
	for i, text := range inputs {
		inputs[i] = ForEmbedder(text)
	}
	body, err := json.Marshal(map[string]any{"model": model, "input": inputs})
	if err != nil {
		return nil, err
	}
	endpoint := e.Endpoint
	if endpoint == "" {
		endpoint = "https://openrouter.ai/api/v1/embeddings"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+e.APIKey)
	req.Header.Set("Content-Type", "application/json")
	client := e.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("memory: embeddings HTTP %d: %s", resp.StatusCode, string(b))
	}
	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&result); err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, fmt.Errorf("memory: embeddings: %s", result.Error.Message)
	}
	if len(result.Data) != len(texts) {
		return nil, fmt.Errorf("memory: got %d embeddings for %d inputs", len(result.Data), len(texts))
	}
	out := make([][]float32, len(texts))
	for _, item := range result.Data {
		if item.Index < 0 || item.Index >= len(texts) || len(item.Embedding) == 0 || out[item.Index] != nil {
			return nil, errors.New("memory: invalid embedding response index or vector")
		}
		out[item.Index] = item.Embedding
	}
	for _, v := range out {
		if v == nil {
			return nil, errors.New("memory: missing embedding response index")
		}
		for _, x := range v {
			if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
				return nil, errors.New("memory: non-finite embedding")
			}
		}
	}
	return out, nil
}

// FakeEmbedder returns predictable vectors and records every text it receives.
type FakeEmbedder struct {
	mu     sync.Mutex
	Inputs []string
	Vector []float32
	Err    error
}

func (f *FakeEmbedder) Embed(_ context.Context, _ string, texts []string) ([][]float32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Inputs = append(f.Inputs, texts...)
	if f.Err != nil {
		return nil, f.Err
	}
	out := make([][]float32, len(texts))
	for i := range out {
		if len(f.Vector) > 0 {
			out[i] = append([]float32(nil), f.Vector...)
		} else {
			out[i] = []float32{1, 0}
		}
	}
	return out, nil
}

func EncodeVector(v []float32) []byte {
	out := make([]byte, 4*len(v))
	for i, x := range v {
		binary.LittleEndian.PutUint32(out[i*4:], math.Float32bits(x))
	}
	return out
}
func DecodeVector(b []byte) ([]float32, error) {
	if len(b)%4 != 0 {
		return nil, fmt.Errorf("memory: malformed vector length %d", len(b))
	}
	out := make([]float32, len(b)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return out, nil
}
func Cosine(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, aa, bb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		aa += float64(a[i]) * float64(a[i])
		bb += float64(b[i]) * float64(b[i])
	}
	if aa == 0 || bb == 0 {
		return 0
	}
	return dot / math.Sqrt(aa*bb)
}

// EmbeddingWorker is a bounded, non-blocking producer queue. Failed jobs are
// skipped; callers keep the underlying Memory usable without a vector.
type EmbeddingWorker struct {
	embedder Embedder
	model    string
	jobs     chan EmbeddingJob
	done     chan struct{}
	stopped  chan struct{}
	mu       sync.RWMutex
	embedMu  sync.Mutex
	once     sync.Once
}
type EmbeddingJob struct {
	Context context.Context
	Text    string
	Store   func(context.Context, []byte, string) error
	OnError func(error)
}

func NewEmbeddingWorker(embedder Embedder, model string, capacity int) *EmbeddingWorker {
	if capacity < 1 {
		capacity = 64
	}
	w := &EmbeddingWorker{embedder: embedder, model: model, jobs: make(chan EmbeddingJob, capacity), done: make(chan struct{}), stopped: make(chan struct{})}
	go w.run()
	return w
}

// EmbedText computes one redacted vector for pre-insert deduplication. A
// successful vector is stored on the inserted row so the worker need not
// issue a second request.
func (w *EmbeddingWorker) EmbedText(ctx context.Context, text string) ([]byte, string, error) {
	if w == nil || w.embedder == nil || w.model == "" {
		return nil, "", errors.New("memory: embedding unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, EmbeddingRequestTimeout)
	defer cancel()
	w.embedMu.Lock()
	defer w.embedMu.Unlock()
	vectors, err := w.embedder.Embed(ctx, w.model, []string{ForEmbedder(text)})
	if err != nil {
		return nil, "", err
	}
	if len(vectors) != 1 {
		return nil, "", errors.New("memory: embedding provider returned an unexpected vector count")
	}
	return EncodeVector(vectors[0]), w.model, nil
}
func (w *EmbeddingWorker) Enqueue(job EmbeddingJob) bool {
	if w == nil || w.embedder == nil || w.model == "" || job.Store == nil {
		return false
	}
	if job.Context == nil {
		job.Context = context.Background()
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	select {
	case <-w.done:
		return false
	default:
	}
	select {
	case w.jobs <- job:
		return true
	default:
		return false
	}
}

// Close stops accepting work, waits for any active job, and reports queued work
// as skipped before returning. Engine closes banks only after this join.
func (w *EmbeddingWorker) Close() {
	if w == nil {
		return
	}
	w.mu.Lock()
	w.once.Do(func() { close(w.done) })
	w.mu.Unlock()
	<-w.stopped
}
func (w *EmbeddingWorker) run() {
	defer close(w.stopped)
	for {
		select {
		case <-w.done:
			w.failPending()
			return
		default:
		}
		select {
		case <-w.done:
			w.failPending()
			return
		case job := <-w.jobs:
			w.process(job)
		}
	}
}
func (w *EmbeddingWorker) process(job EmbeddingJob) {
	ctx, cancel := context.WithTimeout(job.Context, EmbeddingRequestTimeout)
	w.embedMu.Lock()
	vectors, err := w.embedder.Embed(ctx, w.model, []string{ForEmbedder(job.Text)})
	w.embedMu.Unlock()
	if err == nil && len(vectors) != 1 {
		err = errors.New("embedding provider returned an unexpected vector count")
	}
	if err == nil {
		err = job.Store(ctx, EncodeVector(vectors[0]), w.model)
	}
	cancel()
	if err != nil {
		w.report(job, err)
	}
}
func (w *EmbeddingWorker) failPending() {
	for {
		select {
		case job := <-w.jobs:
			w.report(job, errors.New("embedding skipped during shutdown"))
		default:
			return
		}
	}
}
func (w *EmbeddingWorker) report(job EmbeddingJob, err error) {
	if job.OnError != nil {
		job.OnError(err)
	}
}

type VectorCandidate struct {
	ID     int64
	Vector []byte
	Model  string
}
type VectorHit struct {
	ID         int64
	Similarity float64
}

// VectorSearch shares one caller-derived budget between cosine ranking and fallback.
func VectorSearch(ctx context.Context, candidates []VectorCandidate, query []float32, model string, limit int, budget time.Duration, fallback func(context.Context) ([]VectorHit, error)) ([]VectorHit, error) {
	if budget <= 0 {
		budget = EmbeddingQueryBudget
	}
	if limit < 1 || limit > 40 {
		limit = 40
	}
	queryCtx, queryCancel := context.WithTimeout(ctx, budget)
	defer queryCancel()
	searchBudget := budget / 2
	if searchBudget <= 0 {
		searchBudget = budget
	}
	searchCtx, searchCancel := context.WithTimeout(queryCtx, searchBudget)
	defer searchCancel()
	fallbackSearch := func() ([]VectorHit, error) {
		if fallback == nil {
			return nil, searchCtx.Err()
		}
		return fallback(queryCtx)
	}
	type result struct {
		hits []VectorHit
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		hits := make([]VectorHit, 0)
		for i, row := range candidates {
			if i >= MaxVectorsPerBank {
				break
			}
			if i%128 == 0 {
				if err := searchCtx.Err(); err != nil {
					ch <- result{err: err}
					return
				}
			}
			if row.Model != model || len(row.Vector) == 0 {
				continue
			}
			v, err := DecodeVector(row.Vector)
			if err == nil && len(v) == len(query) {
				hits = append(hits, VectorHit{row.ID, Cosine(query, v)})
			}
		}
		if err := searchCtx.Err(); err != nil {
			ch <- result{err: err}
			return
		}
		sort.Slice(hits, func(i, j int) bool {
			if hits[i].Similarity == hits[j].Similarity {
				return hits[i].ID < hits[j].ID
			}
			return hits[i].Similarity > hits[j].Similarity
		})
		if len(hits) > limit {
			hits = hits[:limit]
		}
		ch <- result{hits: hits}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			return fallbackSearch()
		}
		return r.hits, nil
	case <-searchCtx.Done():
		return fallbackSearch()
	}
}

// QueryEmbeddingCache is a bounded LRU keyed by model and exact query text.
type QueryEmbeddingCache struct {
	mu       sync.Mutex
	capacity int
	items    map[string][]float32
	order    []string
}

func NewQueryEmbeddingCache(capacity int) *QueryEmbeddingCache {
	if capacity < 1 {
		capacity = 256
	}
	return &QueryEmbeddingCache{capacity: capacity, items: make(map[string][]float32)}
}
func (c *QueryEmbeddingCache) Get(model, text string) ([]float32, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := model + "\x00" + text
	v, ok := c.items[key]
	if !ok {
		return nil, false
	}
	for i, k := range c.order {
		if k == key {
			c.order = append(c.order[:i], c.order[i+1:]...)
			break
		}
	}
	c.order = append(c.order, key)
	return append([]float32(nil), v...), true
}
func (c *QueryEmbeddingCache) Put(model, text string, vector []float32) {
	if len(vector) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	key := model + "\x00" + text
	if _, ok := c.items[key]; ok {
		for i, k := range c.order {
			if k == key {
				c.order = append(c.order[:i], c.order[i+1:]...)
				break
			}
		}
	}
	c.items[key] = append([]float32(nil), vector...)
	c.order = append(c.order, key)
	for len(c.order) > c.capacity {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.items, oldest)
	}
}
