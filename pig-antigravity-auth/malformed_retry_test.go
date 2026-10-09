package antigravity

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
)

// serveSequence returns a server that replies with bodies[i] on request i
// (last body repeats), counting requests and capturing envelopes.
func serveSequence(t *testing.T, bodies []string, seen *[]envelope, count *int32) {
	t.Helper()
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := atomic.AddInt32(&n, 1)
		if count != nil {
			*count = i
		}
		if seen != nil {
			var e envelope
			_ = json.NewDecoder(r.Body).Decode(&e)
			*seen = append(*seen, e)
		}
		body := bodies[min(int(i)-1, len(bodies)-1)]
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	old := cloudCodeBase
	cloudCodeBase = srv.URL
	t.Cleanup(func() { cloudCodeBase = old })
}

func TestMalformedThenGoodRetries(t *testing.T) {
	// The discarded attempt reports usage; the winner reports none, so the
	// done message must not carry the discarded attempt's counters.
	malformed := sse(`{"candidates":[{"content":{"parts":[]},"finishReason":"MALFORMED_FUNCTION_CALL","finishMessage":"broken args"}],"usageMetadata":{"promptTokenCount":100,"cachedContentTokenCount":40,"candidatesTokenCount":5}}`)
	good := sse(`{"candidates":[{"content":{"parts":[{"text":"Hello"}]},"finishReason":"STOP"}]}`)
	var count int32
	var seen []envelope
	serveSequence(t, []string{malformed, good}, &seen, &count)
	opts := map[string]any{"apiKey": `{"token":"tok","projectId":"proj-1"}`, "reasoning": "high", "sessionId": "retry-success-1"}
	stream := sdk.CreateAssistantMessageEventStream()
	run(context.Background(), stream, testModel, map[string]any{"messages": []any{}}, opts)
	evs := collect(t, stream)

	if count != 2 {
		t.Fatalf("want 2 requests, got %d", count)
	}
	last := evs[len(evs)-1]
	if last["type"] != "done" {
		t.Fatalf("want done, got %v", last["type"])
	}
	starts := 0
	for _, e := range evs {
		if e["type"] == "start" {
			starts++
		}
		if e["type"] == "error" {
			t.Fatalf("no error event expected, got %v", e)
		}
	}
	if starts != 1 {
		t.Errorf("want one start event, got %d", starts)
	}
	if !strings.Contains(fmt.Sprint(last["message"]), "Hello") {
		t.Errorf("good content missing: %v", last["message"])
	}
	usage := last["message"].(map[string]any)["usage"].(map[string]any)
	if usage["input"] != 0 || usage["output"] != 0 || usage["cacheRead"] != 0 {
		t.Errorf("discarded attempt usage must not survive: %v", usage)
	}
}
func TestThreeMalformedGivesSpecificError(t *testing.T) {
	malformed := sse(`{"candidates":[{"content":{"parts":[]},"finishReason":"MALFORMED_FUNCTION_CALL","finishMessage":"bad arguments: missing field"}]}`)
	var count int32
	serveSequence(t, []string{malformed}, nil, &count)
	opts := map[string]any{"apiKey": `{"token":"tok","projectId":"proj-1"}`, "reasoning": "high", "sessionId": "retry-exhaust-1"}
	stream := sdk.CreateAssistantMessageEventStream()
	run(context.Background(), stream, testModel, map[string]any{"messages": []any{}}, opts)
	evs := collect(t, stream)
	if count != 3 {
		t.Fatalf("want 3 requests, got %d", count)
	}
	last := evs[len(evs)-1]
	if last["type"] != "error" {
		t.Fatalf("want error, got %v", last["type"])
	}
	msg := last["error"].(map[string]any)["errorMessage"].(string)
	if !strings.Contains(msg, "MALFORMED_FUNCTION_CALL") {
		t.Errorf("error must name the finish reason: %q", msg)
	}
	if !strings.Contains(msg, "bad arguments") {
		t.Errorf("error must show the provider finish message: %q", msg)
	}
	if strings.Contains(msg, "empty response") {
		t.Errorf("must not say empty response: %q", msg)
	}
}

func TestEmptyStopKeepsOldError(t *testing.T) {
	var count int32
	serveSequence(t, []string{sse(`{"candidates":[{"finishReason":"STOP"}]}`)}, nil, &count)
	opts := map[string]any{"apiKey": `{"token":"tok","projectId":"proj-1"}`, "reasoning": "high", "sessionId": "retry-empty-1"}
	stream := sdk.CreateAssistantMessageEventStream()
	run(context.Background(), stream, testModel, map[string]any{"messages": []any{}}, opts)
	evs := collect(t, stream)
	if count != 1 {
		t.Fatalf("empty STOP must not retry, got %d requests", count)
	}
	last := evs[len(evs)-1]
	if last["type"] != "error" {
		t.Fatalf("want error, got %v", last["type"])
	}
	msg := last["error"].(map[string]any)["errorMessage"].(string)
	if !strings.Contains(msg, "empty response") {
		t.Errorf("empty STOP keeps old error: %q", msg)
	}
}

func TestNoRetryAfterPartialContent(t *testing.T) {
	// Text then a malformed finish in the same turn: content was emitted,
	// so the turn succeeds with what exists and no second request happens.
	body := sse(`{"candidates":[{"content":{"parts":[{"text":"hi"}]},"finishReason":"MALFORMED_FUNCTION_CALL","finishMessage":"late break"}]}`)
	var count int32
	serveSequence(t, []string{body}, nil, &count)
	opts := map[string]any{"apiKey": `{"token":"tok","projectId":"proj-1"}`, "reasoning": "high", "sessionId": "retry-partial-1"}
	stream := sdk.CreateAssistantMessageEventStream()
	run(context.Background(), stream, testModel, map[string]any{"messages": []any{}}, opts)
	evs := collect(t, stream)
	if count != 1 {
		t.Fatalf("must not retry after partial content, got %d requests", count)
	}
	last := evs[len(evs)-1]
	if last["type"] != "done" {
		t.Fatalf("want done with partial content, got %v", last["type"])
	}
}

func TestEquivalentReasonRetries(t *testing.T) {
	malformed := sse(`{"candidates":[{"content":{"parts":[]},"finishReason":"INVALID_TOOL_CALL","finishMessage":"nope"}]}`)
	good := sse(`{"candidates":[{"content":{"parts":[{"text":"recovered"}]},"finishReason":"STOP"}]}`)
	var count int32
	serveSequence(t, []string{malformed, good}, nil, &count)
	opts := map[string]any{"apiKey": `{"token":"tok","projectId":"proj-1"}`, "reasoning": "high", "sessionId": "retry-equiv-1"}
	stream := sdk.CreateAssistantMessageEventStream()
	run(context.Background(), stream, testModel, map[string]any{"messages": []any{}}, opts)
	evs := collect(t, stream)
	if count != 2 {
		t.Fatalf("equivalent reason must retry, got %d requests", count)
	}
	if last := evs[len(evs)-1]; last["type"] != "done" {
		t.Fatalf("want done, got %v", last["type"])
	}
}

func TestSessionCompleteOnlyOnSuccess(t *testing.T) {
	malformed := sse(`{"candidates":[{"content":{"parts":[]},"finishReason":"MALFORMED_FUNCTION_CALL","finishMessage":"bad"}]}`)
	good := sse(`{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`)
	var seen []envelope
	var count int32
	serveSequence(t, []string{malformed, good}, &seen, &count)
	opts := map[string]any{"apiKey": `{"token":"tok","projectId":"proj-1"}`, "reasoning": "high", "sessionId": "retry-session-1"}
	stream := sdk.CreateAssistantMessageEventStream()
	run(context.Background(), stream, testModel, map[string]any{"messages": []any{}}, opts)
	_ = collect(t, stream)
	if count != 2 {
		t.Fatalf("want 2 requests, got %d", count)
	}
	if len(seen) != 2 {
		t.Fatalf("want 2 envelopes, got %d", len(seen))
	}
	// The retry must not carry last_execution_id: the failed attempt ends
	// no execution, so the rebuilt request starts a fresh one.
	if _, ok := seen[1].Request.Labels["last_execution_id"]; ok {
		t.Errorf("retry must not carry last_execution_id: %v", seen[1].Request.Labels)
	}
	sessions.mu.Lock()
	lastExec := sessions.entries["retry-session-1"].lastExecutionID
	sessions.mu.Unlock()
	if lastExec == "" {
		t.Errorf("successful attempt must mark the session complete")
	}

	// Exhausted retries never complete the session.
	var count2 int32
	serveSequence(t, []string{malformed}, nil, &count2)
	opts2 := map[string]any{"apiKey": `{"token":"tok","projectId":"proj-1"}`, "reasoning": "high", "sessionId": "retry-session-2"}
	stream2 := sdk.CreateAssistantMessageEventStream()
	run(context.Background(), stream2, testModel, map[string]any{"messages": []any{}}, opts2)
	_ = collect(t, stream2)
	if count2 != 3 {
		t.Fatalf("want 3 requests, got %d", count2)
	}
	sessions.mu.Lock()
	lastExec2 := sessions.entries["retry-session-2"].lastExecutionID
	sessions.mu.Unlock()
	if lastExec2 != "" {
		t.Errorf("failed attempts must not mark the session complete")
	}
}

func TestThoughtOnlyStopIsRetryableError(t *testing.T) {
	body := sse(`{"candidates":[{"content":{"parts":[{"thought":true,"text":"planning"}]},"finishReason":"STOP"}]}`)
	serveSequence(t, []string{body}, nil, nil)
	opts := map[string]any{"apiKey": `{"token":"tok","projectId":"proj-1"}`, "sessionId": "thought-only-1"}
	stream := sdk.CreateAssistantMessageEventStream()
	run(context.Background(), stream, testModel, map[string]any{"messages": []any{}}, opts)
	evs := collect(t, stream)
	last := evs[len(evs)-1]
	if last["type"] != "error" {
		t.Fatalf("want error, got %v", last["type"])
	}
	if m, _ := json.Marshal(last); !strings.Contains(string(m), "server error") {
		t.Fatalf("error not retryable-worded: %s", m)
	}
}

// Pig carries the system prompt and tools on a system message, not on
// top-level transcript fields.
func TestSystemMessageFeedsPromptAndTools(t *testing.T) {
	tr := map[string]any{"messages": []any{
		map[string]any{"role": "system", "content": "be brief", "toolsAdded": []any{
			map[string]any{"name": "read", "description": "d", "parameters": map[string]any{"type": "object", "properties": map[string]any{}}},
			map[string]any{"name": "gone", "description": "d", "parameters": map[string]any{"type": "object", "properties": map[string]any{}}},
		}},
		map[string]any{"role": "system", "content": []any{map[string]any{"type": "text", "text": "later"}}, "toolsRemoved": []any{map[string]any{"name": "gone"}}},
		map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": "hi"}}},
	}}
	env, _ := buildEnvelope(testModel, tr, map[string]any{"sessionId": "sys-1"}, "p")
	if len(env.Request.Contents) != 1 {
		t.Fatalf("system message leaked into contents: %v", env.Request.Contents)
	}
	b, _ := json.Marshal(env.Request)
	got := string(b)
	if !strings.Contains(got, `"name":"read"`) || strings.Contains(got, `"name":"gone"`) {
		t.Fatalf("tools wrong: %s", got)
	}
	if !strings.Contains(got, `be brief\n\nlater`) {
		t.Fatalf("system prompt missing: %s", got)
	}
}

// A worker that just called spawn (or tell, finish) ends its turn on purpose
// and waits for the notice: a thinking-only stop right after one is a normal
// end of turn, not a retryable error. Any other tool result still errors.
func TestThoughtOnlyStopAfterSpawnResultIsNormalEnd(t *testing.T) {
	body := sse(`{"candidates":[{"content":{"parts":[{"thought":true,"text":"waiting for the notice"}]},"finishReason":"STOP"}]}`)
	for _, c := range []struct {
		tool    string
		wantErr bool
	}{
		{"mcp__el__spawn", false},
		{"mcp__el__tell", false},
		{"mcp__el__finish", false},
		{"mcp__el__execute", true},
	} {
		serveSequence(t, []string{body}, nil, nil)
		tr := map[string]any{"messages": []any{
			map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": "go"}}},
			map[string]any{"role": "toolResult", "toolCallId": "c1", "toolName": c.tool, "content": []any{map[string]any{"type": "text", "text": "started"}}},
		}}
		opts := map[string]any{"apiKey": `{"token":"tok","projectId":"proj-1"}`, "sessionId": "thought-after-" + c.tool}
		stream := sdk.CreateAssistantMessageEventStream()
		run(context.Background(), stream, testModel, tr, opts)
		evs := collect(t, stream)
		last := evs[len(evs)-1]
		if got := last["type"] == "error"; got != c.wantErr {
			t.Errorf("after %s: final event %v, want error=%v", c.tool, last["type"], c.wantErr)
		}
	}
}
