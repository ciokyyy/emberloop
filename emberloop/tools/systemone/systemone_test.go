package systemone

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// probeBody is the shape the live endpoint returned on 2026-10-05 for a plain
// request with no usage parameter: usage comes back by default, alongside the
// resolved model, a generation id and the serving provider.
const probeBody = `{"model":"typesafe/jev-1.13-20260917","answers":{"q1":{"type":"noul","noul":0.65}},"usage":{"input_tokens":278,"output_tokens":21,"cost":0.000011676},"id":"gen-dec-1791237092-NwAW4V0hD1GqPMkgL5J0","provider":"TypeSafe"}`

func TestOpenRouterRequestsUsageAndReturnsIt(t *testing.T) {
	var mu sync.Mutex
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		mu.Lock()
		gotBody = body
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(probeBody))
	}))
	defer srv.Close()

	p := NewOpenRouter("test-key")
	p.endpoint = srv.URL
	resp, err := p.Evaluate(context.Background(), Request{
		Model:     JevModelPinned,
		State:     map[string]any{"query": "probe"},
		Questions: map[string]Question{"q1": {Type: TypeNoul, Instructions: "Is this a probe?"}},
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	mu.Lock()
	usage, _ := gotBody["usage"].(map[string]any)
	sent := gotBody["usage"]
	mu.Unlock()
	if usage["include"] != true {
		t.Errorf("request usage = %v, want {\"include\": true}", sent)
	}
	if resp.Answers["q1"].Noul == nil || *resp.Answers["q1"].Noul != 0.65 {
		t.Errorf("answers = %+v, want q1 noul 0.65", resp.Answers)
	}
	if resp.Usage == nil {
		t.Fatalf("usage = nil, want %+v", probeBody)
	}
	if resp.Usage.InputTokens != 278 || resp.Usage.OutputTokens != 21 {
		t.Errorf("usage tokens = %+v, want 278 in / 21 out", resp.Usage)
	}
	if resp.Usage.Cost != 0.000011676 {
		t.Errorf("usage cost = %v, want 0.000011676", resp.Usage.Cost)
	}
	if resp.Generation != "gen-dec-1791237092-NwAW4V0hD1GqPMkgL5J0" {
		t.Errorf("generation = %q", resp.Generation)
	}
	if resp.Provider != "TypeSafe" {
		t.Errorf("provider = %q, want TypeSafe", resp.Provider)
	}
	if resp.Model != "typesafe/jev-1.13-20260917" {
		t.Errorf("model = %q, want the resolved model", resp.Model)
	}
}

func TestOpenRouterToleratesAResponseWithoutUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"answers":{"q1":{"type":"noul","noul":0.5}}}`))
	}))
	defer srv.Close()

	p := NewOpenRouter("test-key")
	p.endpoint = srv.URL
	resp, err := p.Evaluate(context.Background(), Request{
		Model:     JevModelPinned,
		State:     map[string]any{"query": "probe"},
		Questions: map[string]Question{"q1": {Type: TypeNoul, Instructions: "Is this a probe?"}},
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if resp.Usage != nil {
		t.Errorf("usage = %+v, want nil when the endpoint returns none", resp.Usage)
	}
	if resp.Answers["q1"].Noul == nil || *resp.Answers["q1"].Noul != 0.5 {
		t.Errorf("answers = %+v, want q1 noul 0.5", resp.Answers)
	}
}
