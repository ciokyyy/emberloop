package memory

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestOpenRouterEmbedderBatchIndexesAndLimits(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/embeddings" || r.Header.Get("Authorization") != "Bearer test" {
			t.Errorf("request path/auth: %s %s", r.URL.Path, r.Header.Get("Authorization"))
		}
		var body struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if len(body.Input) != 2 || body.Input[0] != "password=[redacted:password]" {
			t.Errorf("input = %v", body.Input)
		}
		_, _ = w.Write([]byte(`{"data":[{"index":1,"embedding":[0,1]},{"index":0,"embedding":[1,0]}],"usage":{"cost":0.00125}}`))
	}))
	defer srv.Close()
	e := &OpenRouterEmbedder{APIKey: "test", Endpoint: srv.URL + "/api/v1/embeddings"}
	texts := []string{"password=abc1234567890123456", "second"}
	got, err := e.Embed(context.Background(), "model", texts)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, [][]float32{{1, 0}, {0, 1}}) {
		t.Fatalf("vectors = %#v", got)
	}
	if cost, known := e.CostUSD(); !known || cost != .00125 {
		t.Fatalf("cost = %v, %v", cost, known)
	}
	if _, err := e.Embed(context.Background(), "model", make([]string, 33)); err == nil {
		t.Fatal("expected batch limit")
	}
	if texts[0] != "password=abc1234567890123456" {
		t.Fatalf("Embed mutated input: %q", texts[0])
	}
}

func TestVectorEncodingSearchAndModelIsolation(t *testing.T) {
	v := []float32{1, .5, -2}
	encoded := EncodeVector(v)
	decoded, err := DecodeVector(encoded)
	if err != nil || !reflect.DeepEqual(v, decoded) {
		t.Fatalf("decode = %v, %v", decoded, err)
	}
	rows := []VectorCandidate{{1, EncodeVector([]float32{1, 0}), "m1"}, {2, EncodeVector([]float32{0, 1}), "m2"}}
	hits, err := VectorSearch(context.Background(), rows, []float32{0, 1}, "m2", 10, time.Second, nil)
	if err != nil || len(hits) != 1 || hits[0].ID != 2 {
		t.Fatalf("hits = %#v, %v", hits, err)
	}
}

func TestVectorSearchTimeoutFallsBack(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fallbackCalled := false
	hits, err := VectorSearch(ctx, nil, []float32{1}, "m", 5, time.Second, func(context.Context) ([]VectorHit, error) { fallbackCalled = true; return []VectorHit{{ID: 9}}, nil })
	if err != nil || !fallbackCalled || len(hits) != 1 || hits[0].ID != 9 {
		t.Fatalf("fallback = %#v, called %v, err %v", hits, fallbackCalled, err)
	}
}

func TestQueryEmbeddingCacheLRU(t *testing.T) {
	c := NewQueryEmbeddingCache(2)
	c.Put("m", "a", []float32{1})
	c.Put("m", "b", []float32{2})
	_, _ = c.Get("m", "a")
	c.Put("m", "c", []float32{3})
	if _, ok := c.Get("m", "b"); ok {
		t.Fatal("least-recently-used entry retained")
	}
	if _, ok := c.Get("m", "a"); !ok {
		t.Fatal("recent entry evicted")
	}
}

func TestEmbeddingWorkerDoesNotBlockAndPersistsLittleEndian(t *testing.T) {
	fake := &FakeEmbedder{Vector: []float32{1, 0}}
	worker := NewEmbeddingWorker(fake, "m", 1)
	defer worker.Close()
	stored := make(chan []byte, 1)
	if !worker.Enqueue(EmbeddingJob{Text: "redacted only", Store: func(_ context.Context, v []byte, model string) error {
		if model != "m" {
			t.Errorf("model = %q", model)
		}
		stored <- v
		return nil
	}}) {
		t.Fatal("enqueue failed")
	}
	select {
	case b := <-stored:
		got, _ := DecodeVector(b)
		if !reflect.DeepEqual(got, []float32{1, 0}) {
			t.Fatalf("stored vector %v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("worker did not persist vector")
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if !reflect.DeepEqual(fake.Inputs, []string{"redacted only"}) {
		t.Fatalf("embedder inputs %v", fake.Inputs)
	}
}
func TestEngineWorkerEmbedsInsertedMemoriesAndEpisodes(t *testing.T) {
	fake := &FakeEmbedder{Vector: []float32{0.25, 0.75}}
	eng, err := Open(tempDir(t), Options{Embedder: fake, EmbeddingModel: "model-x"})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()
	bank, err := eng.Bank("alice")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	memoryID, err := bank.AddMemory(ctx, MemoryItem{SessionID: "s", EventID: "m1", Author: "alice", Kind: KindFact, Text: "password=abc1234567890123456", Quote: "redacted quote"})
	if err != nil {
		t.Fatal(err)
	}
	episodeID, err := bank.AddEpisode(ctx, Episode{SessionID: "s", EventRange: "e1", Author: "alice", Text: "password=abc1234567890123456"})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		m, merr := bank.GetMemory(ctx, memoryID)
		e, eerr := bank.GetEpisode(ctx, episodeID)
		if merr == nil && eerr == nil && m.EmbeddingModel == "model-x" && len(m.Embedding) > 0 && e.EmbeddingModel == "model-x" && len(e.Embedding) > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	m, _ := bank.GetMemory(ctx, memoryID)
	e, _ := bank.GetEpisode(ctx, episodeID)
	if m.EmbeddingModel != "model-x" || len(m.Embedding) == 0 || e.EmbeddingModel != "model-x" || len(e.Embedding) == 0 {
		t.Fatalf("vectors not persisted asynchronously: memory=%+v episode=%+v", m, e)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.Inputs) != 2 || fake.Inputs[0] != "password=[redacted:password]" || fake.Inputs[1] != "password=[redacted:password]" {
		t.Fatalf("unredacted embedder inputs: %q", fake.Inputs)
	}
}

func TestEngineEmbeddingSettingsApplyToOpenBanks(t *testing.T) {
	eng, err := Open(tempDir(t), Options{Embedder: &FakeEmbedder{Vector: []float32{1, 0}}, EmbeddingModel: "old-model"})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()
	bank, err := eng.Bank("alice")
	if err != nil {
		t.Fatal(err)
	}
	next := &FakeEmbedder{Vector: []float32{0, 1}}
	eng.SetEmbedding(next, "new-model")
	id, err := bank.AddMemory(context.Background(), MemoryItem{SessionID: "s", EventID: "m1", Author: "alice", Kind: KindFact, Text: "new setting", Quote: "new setting"})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		item, getErr := bank.GetMemory(context.Background(), id)
		if getErr == nil && item.EmbeddingModel == "new-model" && len(item.Embedding) > 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	item, _ := bank.GetMemory(context.Background(), id)
	t.Fatalf("updated embedding setting did not reach open bank: %+v", item)
}

type blockingEmbedder struct {
	started chan struct{}
	release chan struct{}
}

func (e *blockingEmbedder) Embed(ctx context.Context, _ string, _ []string) ([][]float32, error) {
	close(e.started)
	select {
	case <-e.release:
		return [][]float32{{1, 0}}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestEngineCloseJoinsEmbeddingBeforeClosingBanks(t *testing.T) {
	embedder := &blockingEmbedder{started: make(chan struct{}), release: make(chan struct{})}
	root := tempDir(t)
	eng, err := Open(root, Options{Embedder: embedder, EmbeddingModel: "m"})
	if err != nil {
		t.Fatal(err)
	}
	bank, err := eng.Bank("alice")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bank.AddMemory(context.Background(), MemoryItem{SessionID: "s", EventID: "e", Author: "alice", Kind: KindFact, Text: "fact", Quote: "fact"}); err != nil {
		t.Fatal(err)
	}
	<-embedder.started
	closed := make(chan error, 1)
	go func() { closed <- eng.Close() }()
	select {
	case err := <-closed:
		t.Fatalf("Close returned before active embedding finished: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(embedder.release)
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close failed to join worker")
	}
	reader, err := Open(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	readBank, err := reader.Bank("alice")
	if err != nil {
		t.Fatal(err)
	}
	row, err := readBank.GetMemory(context.Background(), 1)
	if err != nil || row.EmbeddingModel != "m" || len(row.Embedding) == 0 {
		t.Fatalf("in-flight store did not finish before bank close: %+v, %v", row, err)
	}

}
func TestEmbeddingFailuresAreRecorded(t *testing.T) {
	fake := &FakeEmbedder{Err: errors.New("offline")}
	eng, err := Open(tempDir(t), Options{Embedder: fake, EmbeddingModel: "m"})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()
	bank, err := eng.Bank("alice")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bank.AddMemory(context.Background(), MemoryItem{SessionID: "s", EventID: "e", Author: "alice", Kind: KindFact, Text: "fact", Quote: "fact"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		rows, err := bank.Ledger(context.Background())
		if err == nil {
			for _, row := range rows {
				if row.Stage == "embedding" && row.Status == LedgerFailed {
					return
				}
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("embedding failure was not recorded")
}
