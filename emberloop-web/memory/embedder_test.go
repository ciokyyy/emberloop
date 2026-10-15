package memory

import (
	"context"
	"encoding/json"
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
		_, _ = w.Write([]byte(`{"data":[{"index":1,"embedding":[0,1]},{"index":0,"embedding":[1,0]}]}`))
	}))
	defer srv.Close()
	e := &OpenRouterEmbedder{APIKey: "test", Endpoint: srv.URL + "/api/v1/embeddings"}
	got, err := e.Embed(context.Background(), "model", []string{"password=abc1234567890123456", "second"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, [][]float32{{1, 0}, {0, 1}}) {
		t.Fatalf("vectors = %#v", got)
	}
	if _, err := e.Embed(context.Background(), "model", make([]string, 33)); err == nil {
		t.Fatal("expected batch limit")
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
