package memory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestValidateExtractedItemsDropsInvalidWithoutAnotherCall(t *testing.T) {
	events := []ExtractEvent{{Number: 1, ID: "e1", Text: "I prefer concise answers and always use Go."}}
	items := []ExtractedItem{
		{Kind: KindPreference, Text: "I prefer concise answers.", Quote: "I prefer concise", Event: 1, Stated: true},
		{Kind: KindPreference, Text: "invented", Quote: "not said", Event: 1, Stated: true},
		{Kind: KindPreference, Text: strings.Repeat("x", 301), Quote: "I prefer", Event: 1, Stated: true},
		{Kind: "mystery", Text: "unknown", Quote: "I prefer", Event: 1, Stated: true},
		{Kind: KindFact, Text: "token is ghp_123456789012345678901234", Quote: "I prefer", Event: 1, Stated: true},
	}
	got, dropped := ValidateExtractedItems(items, events)
	if len(got) != 1 || len(dropped) != 4 {
		t.Fatalf("valid=%d dropped=%d", len(got), len(dropped))
	}
	s := &ScriptedExtractor{Results: [][]ExtractedItem{items}}
	_, err := ExtractWithFallback(context.Background(), s, "model", ExtractRequest{Events: events})
	if err != nil {
		t.Fatal(err)
	}
	if s.Calls != 1 {
		t.Fatalf("calls=%d, want one", s.Calls)
	}
}

func TestValidateExtractedItemsRejectsAmbiguousEventReferences(t *testing.T) {
	events := []ExtractEvent{
		{Number: 1, ID: "first", Text: "A safe preference was stated."},
		{Number: 1, ID: "second", Text: "The quote from another event."},
		{Number: 2, ID: "second", Text: "A duplicate event ID."},
	}
	items := []ExtractedItem{
		{Kind: KindPreference, Text: "A safe preference.", Quote: "The quote from another event", Event: 1, Stated: true},
		{Kind: KindFact, Text: "Something happened.", Quote: "A duplicate event ID", Event: 2, Stated: true},
	}
	valid, dropped := ValidateExtractedItems(items, events)
	if len(valid) != 0 || len(dropped) != len(items) {
		t.Fatalf("valid=%+v dropped=%+v", valid, dropped)
	}
	input := ExtractRequest{Events: events}
	s := &ScriptedExtractor{Results: [][]ExtractedItem{items}}
	got, err := ExtractWithFallback(context.Background(), s, "model", input)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 || s.Calls != 1 {
		t.Fatalf("memories=%+v calls=%d", got, s.Calls)
	}
}

func TestMemoryImportance(t *testing.T) {
	for _, tc := range []struct {
		text   string
		stated bool
		want   float64
	}{{"plain statement", true, .7}, {"Always use this", true, .85}, {"we decided to wait", true, .85}, {"plain", false, .5}} {
		if got := MemoryImportance(tc.text, tc.stated); got != tc.want {
			t.Errorf("MemoryImportance(%q,%v)=%v want %v", tc.text, tc.stated, got, tc.want)
		}
	}
}

func TestExtractorEmptyModelUsesHeuristic(t *testing.T) {
	s := &ScriptedExtractor{Results: [][]ExtractedItem{{}}}
	input := ExtractRequest{Events: []ExtractEvent{{Number: 1, ID: "event-1", Text: "Please remember: do not add unrelated dependencies"}}}
	got, err := ExtractWithFallback(context.Background(), s, "", input)
	if err != nil {
		t.Fatal(err)
	}
	if s.Calls != 0 {
		t.Fatalf("extractor calls=%d; empty model must skip LLM", s.Calls)
	}
	if len(got) != 1 || got[0].Importance != .4 || got[0].Veracity != VeracityInferred {
		t.Fatalf("fallback=%+v", got)
	}
}

func TestSuccessfulEmptyOrRejectedModelAnswerDoesNotUseHeuristic(t *testing.T) {
	input := ExtractRequest{Events: []ExtractEvent{{Number: 1, ID: "e", Text: "Please remember: do not change unrelated files"}}}
	results := [][]ExtractedItem{nil, {{Kind: KindPreference, Text: "ungrounded", Quote: "not present", Event: 1, Stated: true}}}
	for _, result := range results {
		s := &ScriptedExtractor{Results: [][]ExtractedItem{result}}
		got, err := ExtractWithFallback(context.Background(), s, "model", input)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 || s.Calls != 1 {
			t.Fatalf("result=%+v calls=%d; successful model answer should win", got, s.Calls)
		}
	}
}

func TestOpenRouterExtractor429RetriesOnceThenFallback(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		http.Error(w, "slow down", http.StatusTooManyRequests)
	}))
	defer srv.Close()
	x := NewOpenRouterExtractor("key", "model", "low")
	x.Endpoint = srv.URL
	x.Sleep = func(context.Context, time.Duration) error { return nil }
	got, err := ExtractWithFallback(context.Background(), x, "model", ExtractRequest{Events: []ExtractEvent{{Number: 1, ID: "e", Text: "Please remember: do not change unrelated files"}}})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("HTTP calls=%d want 2", calls)
	}
	if len(got) != 1 || got[0].Importance != .4 {
		t.Fatalf("fallback=%+v", got)
	}
}

func TestOpenRouterExtractorTimeoutRetriesOnce(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			time.Sleep(40 * time.Millisecond)
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"items\":[]}"}}]}`))
	}))
	defer srv.Close()
	x := NewOpenRouterExtractor("key", "model", "low")
	x.Endpoint = srv.URL
	x.Client = &http.Client{Timeout: 10 * time.Millisecond}
	x.Sleep = func(context.Context, time.Duration) error { return nil }
	if _, err := x.Extract(context.Background(), ExtractRequest{}); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("HTTP calls=%d want 2", got)
	}
}

func TestOpenRouterExtractorBodyTimeoutRetriesOnce(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			time.Sleep(40 * time.Millisecond)
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"items\":[]}"}}]}`))
	}))
	defer srv.Close()
	x := NewOpenRouterExtractor("key", "model", "low")
	x.Endpoint = srv.URL
	x.Client = &http.Client{Timeout: 10 * time.Millisecond}
	x.Sleep = func(context.Context, time.Duration) error { return nil }
	if _, err := x.Extract(context.Background(), ExtractRequest{}); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("HTTP calls=%d want 2", got)
	}
}

func TestOpenRouterExtractorRequestAndJSON(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("auth=%q", r.Header.Get("Authorization"))
		}
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		if req.Temperature != 0 || req.MaxTokens != 4000 || req.Reasoning.Effort != "low" || req.Messages[0].Content != ExtractorPromptX1 || !req.Usage.Include {
			t.Errorf("request=%+v", req)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"items\":[{\"kind\":\"fact\",\"text\":\"The user uses Go.\",\"quote\":\"I use Go\",\"event\":1,\"stated\":true}] }"}}],"usage":{"cost":0.000123}}`))
	}))
	defer srv.Close()
	x := NewOpenRouterExtractor("secret", "model", "low")
	x.Endpoint = srv.URL
	got, err := x.Extract(context.Background(), ExtractRequest{Events: []ExtractEvent{{Number: 1, ID: "e", Text: "I use Go"}}})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || len(got) != 1 || got[0].Kind != KindFact {
		t.Fatalf("calls=%d got=%+v", calls, got)
	}
	if cost, known := x.CostUSD(); !known || cost != 0.000123 {
		t.Fatalf("cost=%v known=%v", cost, known)
	}
}

func TestLiveExtract(t *testing.T) {
	if os.Getenv("EL_LIVE_EXTRACT") != "1" {
		t.Skip("set EL_LIVE_EXTRACT=1 to run the OpenRouter extraction eval")
	}
	key := strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY"))
	if key == "" {
		t.Fatal("EL_LIVE_EXTRACT=1 requires OPENROUTER_API_KEY")
	}
	data, err := os.ReadFile("testdata/extract_eval.json")
	if err != nil {
		t.Fatal(err)
	}
	var eval struct {
		Positives []struct {
			Text    string `json:"text"`
			Concept string `json:"concept"`
		} `json:"positives"`
		Negatives []struct {
			Text string `json:"text"`
		} `json:"negatives"`
	}
	if err := json.Unmarshal(data, &eval); err != nil {
		t.Fatal(err)
	}
	model := os.Getenv("EL_EXTRACT_MODEL")
	if model == "" {
		model = DefaultExtractorModel
	}
	x := NewOpenRouterExtractor(key, model, "low")
	caught, falseAlarms := 0, 0
	for i, p := range eval.Positives {
		input := ExtractRequest{Events: []ExtractEvent{{Number: 1, ID: "positive", Text: p.Text}}}
		items, err := x.Extract(context.Background(), input)
		if err != nil {
			t.Fatalf("positive %d: %v", i+1, err)
		}
		valid, _ := ValidateExtractedItems(items, input.Events)
		for _, item := range valid {
			if item.Event == 1 && conceptMatches(p.Concept, item.Text) {
				caught++
				break
			}
		}
	}
	for i, p := range eval.Negatives {
		input := ExtractRequest{Events: []ExtractEvent{{Number: 1, ID: "negative", Text: p.Text}}}
		items, err := x.Extract(context.Background(), input)
		if err != nil {
			t.Fatalf("negative %d: %v", i+1, err)
		}
		valid, _ := ValidateExtractedItems(items, input.Events)
		if len(valid) > 0 {
			falseAlarms++
		}
	}
	t.Logf("TestLiveExtract model=%s prompt=%s recall=%d/%d false_alarms=%d/%d", model, ExtractorPromptVersion, caught, len(eval.Positives), falseAlarms, len(eval.Negatives))
	if caught < 10 || falseAlarms > 1 {
		t.Fatalf("acceptance failed: recall %d/%d, false alarms %d/%d", caught, len(eval.Positives), falseAlarms, len(eval.Negatives))
	}
}

func conceptMatches(concept, text string) bool {
	words := func(s string) map[string]bool {
		m := map[string]bool{}
		for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return r < 'a' || r > 'z' }) {
			if len(w) > 3 {
				m[w] = true
			}
		}
		return m
	}
	want, got := words(concept), words(text)
	if len(want) == 0 {
		return false
	}
	matched := 0
	for w := range want {
		if got[w] {
			matched++
		}
	}
	return float64(matched)/float64(len(want)) >= .35
}
