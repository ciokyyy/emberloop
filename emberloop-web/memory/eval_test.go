package memory

import (
	"encoding/json"
	"testing"
)

func TestCoversRule(t *testing.T) {
	cases := []struct {
		rule, correction string
		want             bool
	}{
		{"always return wrapped errors", "always return wrapped errors", true},
		{"always return wrapped errors", "never return wrapped errors", true},
		{"always return wrapped errors in handlers", "always return wrapped errors", true},
		{"always return wrapped errors", "use tabs for indentation", false},
		{"", "always return wrapped errors", false},
	}
	for _, c := range cases {
		if got := CoversRule(c.rule, c.correction); got != c.want {
			t.Errorf("CoversRule(%q, %q) = %v, want %v", c.rule, c.correction, got, c.want)
		}
	}
}

func TestRecallAt5Empty(t *testing.T) {
	if got := RecallAt5(0, 0); got != 0 {
		t.Fatalf("RecallAt5(0,0) = %v, want 0", got)
	}
	if got := RecallAt5(1, 3); got < 0.33 || got > 0.34 {
		t.Fatalf("RecallAt5(1,3) = %v, want ~0.333", got)
	}
}

func TestTopCovered(t *testing.T) {
	query := "always return wrapped errors"
	hits := []string{"use tabs for indentation", "always return wrapped errors in handlers"}
	if !TopCovered(hits, query, 5) {
		t.Fatalf("TopCovered missed a refining hit")
	}
	if TopCovered([]string{"use tabs for indentation"}, query, 5) {
		t.Fatalf("TopCovered covered an unrelated hit")
	}
	if TopCovered(nil, query, 5) {
		t.Fatalf("TopCovered covered no hits")
	}
}

func TestQueueItemLabels(t *testing.T) {
	items := []QueueItem{
		{ID: 1, Kind: QueueRule, Payload: `{"text":"always wrap errors","kind":"preference"}`, Status: QueueApproved},
		{ID: 2, Kind: QueueRule, Payload: `{"quote":"never wrap errors"}`, Status: QueueRejected},
		{ID: 3, Kind: QueueRule, Payload: `{"text":"unjudged candidate"}`, Status: QueuePending},
		{ID: 4, Kind: QueueRule, Payload: `{"note":"unrelated"}`, Status: QueueRejected},
	}
	labels := QueueItemLabels(items)
	// The pending row carries text but is unjudged, so it is skipped rather
	// than scored as a negative; the textless row is skipped, never fabricated.
	if len(labels) != 2 {
		t.Fatalf("labels = %+v, want 2 (pending and textless rows skip)", labels)
	}
	if labels[0].ID != "queue-1" || !labels[0].Durable || labels[0].Kind != KindPreference {
		t.Errorf("queue-1 = %+v", labels[0])
	}
	if labels[1].ID != "queue-2" || labels[1].Durable {
		t.Errorf("queue-2 = %+v, want rejected as non-durable", labels[1])
	}
}
func TestParseDeciderLabels(t *testing.T) {
	raw := []byte("{\"id\":\"a\",\"text\":\"remember: always wrap errors\",\"durable\":true}\n" +
		"\n" +
		"{\"id\":\"b\",\"text\":\"What time is lunch?\",\"durable\":false,\"kind\":\"fact\"}\n")
	labels, err := ParseDeciderLabels(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(labels) != 2 || labels[0].ID != "a" || !labels[0].Durable || labels[1].Durable {
		t.Fatalf("labels = %+v", labels)
	}
	for _, bad := range []string{
		"{\"text\":\"x\",\"durable\":true}\n",
		"{\"id\":\"a\",\"text\":\"\",\"durable\":true}\n",
		"{\"id\":\"a\",\"text\":\"x\",\"durable\":true,\"kind\":\"essay\"}\n",
		"{\"id\":\"a\",\"text\":\"x\",\"durable\":true}\n{\"id\":\"a\",\"text\":\"y\",\"durable\":false}\n",
		"not json\n",
	} {
		if _, err := ParseDeciderLabels([]byte(bad)); err == nil {
			t.Errorf("ParseDeciderLabels(%q) succeeded, want error", bad)
		}
	}
}

func TestScoreDeciderPrecision(t *testing.T) {
	labels := []DeciderLabel{
		{ID: "a", Text: "remember: always wrap errors", Durable: true},
		{ID: "b", Text: "remember: never wrap errors", Durable: true},
		{ID: "c", Text: "What time is lunch?", Durable: false},
		{ID: "d", Text: "remember: use tabs", Durable: false},
	}
	scores := DeciderScores{"a": 0.9, "b": 0.2, "c": 0.8}
	ev := ScoreDecider("llm", labels, scores, 0.7)
	// Predicted: a (TP), c (FP). b is FN, d unscored.
	if ev.Predicted != 2 || ev.TP != 1 || ev.FP != 1 || ev.FN != 1 || ev.Scored != 3 {
		t.Fatalf("eval = %+v", ev)
	}
	if ev.Precision != 0.5 {
		t.Fatalf("precision = %v, want 0.5", ev.Precision)
	}
	empty := ScoreDecider("jev", labels, nil, 0.7)
	if empty.Predicted != 0 || empty.Precision != 0 || empty.Scored != 0 || empty.Labels != 4 {
		t.Fatalf("unavailable eval = %+v", empty)
	}
}

func TestScoreEvalDecidersHeuristicLive(t *testing.T) {
	labels := []DeciderLabel{
		{ID: "a", Text: "remember: always wrap errors", Durable: true},
		{ID: "b", Text: "What time is lunch?", Durable: false},
	}
	evals := ScoreEvalDeciders(labels, nil, DefaultConfirmHigh)
	var heuristic *DeciderEval
	for i := range evals {
		if evals[i].Decider == "heuristic" {
			heuristic = &evals[i]
		}
	}
	if heuristic == nil {
		t.Fatalf("evals = %+v, want heuristic", evals)
	}
	if heuristic.Scored != 2 {
		t.Fatalf("heuristic scored %d, want 2 (live, no scores file)", heuristic.Scored)
	}
	if len(evals) != len(EvalDeciders) {
		t.Fatalf("evals = %d, want %d (heuristic, llm, jev, laya)", len(evals), len(EvalDeciders))
	}
}

func TestParseDeciderScores(t *testing.T) {
	raw, _ := json.Marshal(map[string]map[string]float64{
		"llm": {"a": 0.9},
		"jev": {"a": 0.1},
	})
	m, err := ParseDeciderScores(raw)
	if err != nil {
		t.Fatal(err)
	}
	if m["llm"]["a"] != 0.9 {
		t.Fatalf("scores = %+v", m)
	}
	if _, err := ParseDeciderScores([]byte(`{"llm":{"a":2}}`)); err == nil {
		t.Fatalf("out-of-range probability succeeded")
	}
	if m, err := ParseDeciderScores(nil); err != nil || len(m) != 0 {
		t.Fatalf("empty scores = %+v, %v", m, err)
	}
}

func TestScoreEvalCasesGap(t *testing.T) {
	query := "always return wrapped errors"
	covering := "always return wrapped errors in handlers"
	cases := []EvalCase{
		{Query: query, FTSHits: []string{covering}, EmbedderHits: []string{"use tabs for indentation"}},
		{Query: query, FTSHits: []string{"use tabs for indentation"}, EmbedderHits: []string{covering}},
	}
	fts, emb, gap := ScoreEvalCases(cases, 5)
	if fts != 0.5 || emb != 0.5 || gap != 0 {
		t.Fatalf("fts=%v emb=%v gap=%v, want 0.5 0.5 0", fts, emb, gap)
	}
	skewed := []EvalCase{
		{Query: query, FTSHits: []string{covering}, EmbedderHits: []string{"use tabs"}},
	}
	fts, emb, gap = ScoreEvalCases(skewed, 5)
	if fts != 1 || emb != 0 || gap != -1 {
		t.Fatalf("fts=%v emb=%v gap=%v, want 1 0 -1", fts, emb, gap)
	}
}
