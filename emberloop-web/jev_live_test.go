package web

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"example.com/emberloop-web/memory"
)

// TestLiveJevDecider calls the real decision endpoint through the Jev client:
// one durable question plus kind and scope, asserting every asked question is
// answered with in-range values. Not in the main check: set EL_LIVE_JEV=1
// with JEV_API_KEY for the hosted endpoint (or JEV_BASE_URL for a local
// laya-serve /v1/systemone URL, whose key may stay empty), and optional
// JEV_MODEL (default jev-latest). Each call bills like any other.
func TestLiveJevDecider(t *testing.T) {
	if os.Getenv("EL_LIVE_JEV") == "" {
		t.Skip("set EL_LIVE_JEV=1 with JEV_API_KEY for the real-endpoint probe")
	}
	baseURL := strings.TrimSpace(os.Getenv("JEV_BASE_URL"))
	if baseURL == "" {
		baseURL = "https://jev-api.org/api/v1/decisions"
	}
	key := strings.TrimSpace(os.Getenv("JEV_API_KEY"))
	if key == "" && !strings.Contains(baseURL, "127.0.0.1") && !strings.Contains(baseURL, "localhost") {
		t.Skip("set JEV_API_KEY for a hosted endpoint, or point JEV_BASE_URL at a local server")
	}
	model := strings.TrimSpace(os.Getenv("JEV_MODEL"))
	if model == "" {
		model = memory.DefaultJevModel
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	d := memory.JevDecider{Config: memory.JevConfig{BaseURL: baseURL, APIKey: key, Model: model}}
	res, err := d.Decide(ctx, memory.DeciderState{Statement: "No, always use tabs in this repo"}, []memory.DeciderQuestion{
		memory.Noul(memory.QuestionDurable),
		memory.KindQuestion(),
		memory.ScopeQuestion(),
	})
	if err != nil {
		t.Fatalf("live jev call: %v", err)
	}
	if p, ok := res.Noul(memory.QuestionDurable); !ok || p < 0 || p > 1 {
		t.Fatalf("live durable = %v, want an answered probability", p)
	}
	for _, q := range []string{memory.QuestionKind, memory.QuestionScopeHint} {
		if got, ok := res.Choice(q); !ok || got == "" {
			t.Fatalf("live %s unanswered: %+v", q, res.Answers)
		}
	}
	t.Logf("live jev model %s durable answer: %+v", res.Model, res.Answers[memory.QuestionDurable])
}
