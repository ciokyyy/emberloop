package antigravity

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
)

func TestResolveWireModel(t *testing.T) {
	cases := []struct {
		id, reasoning, wire string
		budget              int
	}{
		{"antigravity-gemini-3.8-flash", "", "gemini-3.8-flash-medium", 4000},
		{"antigravity-gemini-3.8-flash", "high", "gemini-3.8-flash-high", -1},
		{"antigravity-gemini-3.6-flash", "xhigh", "gemini-3.6-flash-high", 10000},
		{"antigravity-gemini-3.5-flash", "minimal", "gemini-3.5-flash-extra-low", 1000},
		{"antigravity-gemini-3.1-pro", "high", "gemini-pro-agent", 10001},
		{"antigravity-gemini-3.1-pro", "medium", "gemini-3.1-pro-low", 1001},
		{"antigravity-claude-sonnet-4-6-thinking", "high", "claude-sonnet-4-6", 1024},
		{"antigravity-claude-opus-4-6-thinking", "", "claude-opus-4-6-thinking", 1024},
		{"antigravity-gpt-oss-120b-medium", "high", "gpt-oss-120b-medium", 8192},
	}
	for _, c := range cases {
		got, _ := resolveWireModel(c.id, c.reasoning)
		if got.Name != c.wire || got.Budget != c.budget {
			t.Errorf("%s/%s = %+v, want %s/%d", c.id, c.reasoning, got, c.wire, c.budget)
		}
	}
}

func TestToGeminiSchema(t *testing.T) {
	in := map[string]any{
		"type": "object", "additionalProperties": false, "$schema": "x",
		"properties": map[string]any{"a": map[string]any{"type": "array"}, "b": map[string]any{"type": "string", "const": "k"}},
		"required":   []any{"a", "ghost"},
	}
	out := toGeminiSchema(in).(map[string]any)
	if out["type"] != "OBJECT" || out["additionalProperties"] != nil || out["$schema"] != nil {
		t.Fatalf("object not cleaned: %v", out)
	}
	a := out["properties"].(map[string]any)["a"].(map[string]any)
	if a["type"] != "ARRAY" || a["items"].(map[string]any)["type"] != "STRING" {
		t.Errorf("array needs items: %v", a)
	}
	if req := out["required"].([]any); len(req) != 1 || req[0] != "a" {
		t.Errorf("required must drop undeclared names: %v", req)
	}
}

func TestConvertMessagesToolRoundTrip(t *testing.T) {
	msgs := []any{
		map[string]any{"role": "user", "content": "hi"},
		map[string]any{"role": "assistant", "provider": "p", "model": "m", "content": []any{
			map[string]any{"type": "thinking", "thinking": "hmm", "thinkingSignature": "s1"},
			map[string]any{"type": "toolCall", "id": "c1", "name": "read", "arguments": map[string]any{"path": "x"}, "thoughtSignature": "s2"},
			map[string]any{"type": "toolCall", "id": "c2", "name": "read", "arguments": map[string]any{"path": "y"}},
		}},
		map[string]any{"role": "toolResult", "toolCallId": "c1", "toolName": "read", "content": []any{map[string]any{"type": "text", "text": "A"}}},
		map[string]any{"role": "toolResult", "toolCallId": "c2", "toolName": "read", "isError": true, "content": []any{map[string]any{"type": "text", "text": "boom"}}},
	}
	got := convertMessages(msgs, "p", "m")
	if len(got) != 3 {
		t.Fatalf("want user, model, merged model results; got %d: %v", len(got), got)
	}
	last := got[2].(map[string]any)
	if last["role"] != "model" || len(last["parts"].([]any)) != 2 {
		t.Errorf("results of the target model's calls merge under role model: %v", last)
	}
	resp := last["parts"].([]any)[1].(map[string]any)["functionResponse"].(map[string]any)["response"].(map[string]any)
	if resp["error"] != "boom" {
		t.Errorf("error result: %v", resp)
	}
	// A different target model gets no signed history and results under user.
	other := convertMessages(msgs, "p", "other")
	for _, p := range other[1].(map[string]any)["parts"].([]any) {
		if _, thought := p.(map[string]any)["thought"]; thought {
			t.Errorf("thinking must not replay to another model")
		}
	}
	if other[2].(map[string]any)["role"] != "user" {
		t.Errorf("foreign tool results go under user")
	}
}

func TestAuthorizeURLAndCallback(t *testing.T) {
	p := newPKCE()
	u, state := authorizeURL(p)
	parsed, _ := url.Parse(u)
	q := parsed.Query()
	if q.Get("client_id") != clientID || q.Get("code_challenge_method") != "S256" || q.Get("access_type") != "offline" {
		t.Errorf("bad authorize url: %s", u)
	}
	v, err := verifierFromState(state)
	if err != nil || v != p.verifier {
		t.Fatalf("state must carry the verifier: %q %v", v, err)
	}
	code, st := parseCallback(redirectURI+"?code=abc&state="+state, "other")
	if code != "abc" || st != state {
		t.Errorf("callback url: %q %q", code, st)
	}
	if code, st := parseCallback(" rawcode ", state); code != "rawcode" || st != state {
		t.Errorf("bare code: %q %q", code, st)
	}
}

func TestRefreshKeepsProjectAndRefreshToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "R" {
			http.Error(w, "bad", 400)
			return
		}
		fmt.Fprint(w, `{"access_token":"A2","expires_in":3600}`)
	}))
	defer srv.Close()
	old := tokenURL
	tokenURL = srv.URL
	defer func() { tokenURL = old }()
	got, err := refreshCredentials(context.Background(), sdk.OAuthCredentials{Refresh: "R", Access: "A1", ProjectID: "proj"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Access != "A2" || got.Refresh != "R" || got.ProjectID != "proj" || got.Expires < time.Now().UnixMilli() {
		t.Errorf("refresh result: %+v", got)
	}
}

func TestAPIKeyRoundTrip(t *testing.T) {
	tok, proj := parseAPIKey(apiKey(sdk.OAuthCredentials{Access: "tok", ProjectID: "p1"}))
	if tok != "tok" || proj != "p1" {
		t.Errorf("round trip: %q %q", tok, proj)
	}
	if tok, proj := parseAPIKey("raw-token"); tok != "raw-token" || proj != "" {
		t.Errorf("raw token passes through: %q %q", tok, proj)
	}
}

func sse(chunks ...string) string {
	var b strings.Builder
	for _, c := range chunks {
		fmt.Fprintf(&b, "data: {\"response\":%s}\n\n", c)
	}
	return b.String()
}

func collect(t *testing.T, stream *sdk.ModelEventStream) []map[string]any {
	t.Helper()
	var evs []map[string]any
	for e := range stream.Events(context.Background()) {
		evs = append(evs, e)
	}
	return evs
}

func serve(t *testing.T, body string, seen *envelope) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" || !strings.Contains(r.Header.Get("User-Agent"), "antigravity/cli/") {
			http.Error(w, "unauthorized", 401)
			return
		}
		if seen != nil {
			_ = json.NewDecoder(r.Body).Decode(seen)
		}
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	old := cloudCodeBase
	cloudCodeBase = srv.URL
	t.Cleanup(func() { cloudCodeBase = old })
}

var (
	testModel   = map[string]any{"id": "antigravity-gemini-3.8-flash", "provider": providerID, "api": "google-generative-ai", "reasoning": true, "maxTokens": float64(65536)}
	testOptions = map[string]any{"apiKey": `{"token":"tok","projectId":"proj-1"}`, "reasoning": "high", "sessionId": "s-1"}
)

func TestStreamTextToolAndUsage(t *testing.T) {
	var seen envelope
	serve(t, sse(
		`{"candidates":[{"content":{"parts":[{"thought":true,"text":"think","thoughtSignature":"ts"}]}}]}`,
		`{"candidates":[{"content":{"parts":[{"text":"Hel"}]}}]}`,
		`{"candidates":[{"content":{"parts":[{"text":"lo"},{"functionCall":{"name":"read","args":{"path":"a"}}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":100,"cachedContentTokenCount":40,"candidatesTokenCount":5,"thoughtsTokenCount":3}}`,
	), &seen)
	transcript := map[string]any{
		"systemPrompt": "be brief",
		"messages":     []any{map[string]any{"role": "user", "content": "hi"}},
		"tools":        []any{map[string]any{"name": "read", "description": "d", "parameters": map[string]any{"type": "object", "properties": map[string]any{}}}},
	}
	stream := sdk.CreateAssistantMessageEventStream()
	run(context.Background(), stream, testModel, transcript, testOptions)
	evs := collect(t, stream)

	var types []string
	for _, e := range evs {
		types = append(types, e["type"].(string))
	}
	want := "start thinking_start thinking_delta thinking_end text_start text_delta text_delta text_end toolcall_start toolcall_end done"
	if strings.Join(types, " ") != want {
		t.Fatalf("events:\n got %v\nwant %s", types, want)
	}
	done := evs[len(evs)-1]
	if done["reason"] != "toolUse" {
		t.Errorf("reason = %v", done["reason"])
	}
	msg := done["message"].(map[string]any)
	usage := msg["usage"].(map[string]any)
	if usage["input"] != 60 || usage["output"] != 8 || usage["cacheRead"] != 40 || usage["totalTokens"] != 108 {
		t.Errorf("usage: %v", usage)
	}
	content := msg["content"].([]any)
	if len(content) != 3 || content[1].(map[string]any)["text"] != "Hello" {
		t.Errorf("content: %v", content)
	}

	if seen.Project != "proj-1" || seen.Model != "gemini-3.8-flash-high" || seen.RequestType != "agent" {
		t.Errorf("envelope: %+v", seen)
	}
	if !strings.HasPrefix(seen.RequestID, "agent/") || seen.Request.Labels["model_enum"] != "MODEL_PLACEHOLDER_M318" {
		t.Errorf("metadata: %s %v", seen.RequestID, seen.Request.Labels)
	}
	if seen.Request.ToolConfig == nil || seen.Request.SessionID == "" {
		t.Errorf("request: %+v", seen.Request)
	}
}

func TestStreamErrors(t *testing.T) {
	cases := map[string]struct{ body, want string }{
		"embedded error": {sse(`{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","message":"quota"}}`), "RESOURCE_EXHAUSTED"},
		"no terminal":    {sse(`{"candidates":[{"content":{"parts":[{"text":"x"}]}}]}`), "without a terminal"},
		"empty":          {sse(`{"candidates":[{"finishReason":"STOP"}]}`), "empty response"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			serve(t, c.body, nil)
			stream := sdk.CreateAssistantMessageEventStream()
			run(context.Background(), stream, testModel, map[string]any{"messages": []any{}}, testOptions)
			evs := collect(t, stream)
			last := evs[len(evs)-1]
			if last["type"] != "error" || !strings.Contains(last["error"].(map[string]any)["errorMessage"].(string), c.want) {
				t.Errorf("last event: %v", last)
			}
		})
	}
}

func TestStreamHTTPFailureAndMissingToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "nope", 403) }))
	defer srv.Close()
	old := cloudCodeBase
	cloudCodeBase = srv.URL
	defer func() { cloudCodeBase = old }()
	for name, opts := range map[string]map[string]any{"http": testOptions, "token": {}} {
		stream := sdk.CreateAssistantMessageEventStream()
		run(context.Background(), stream, testModel, map[string]any{}, opts)
		evs := collect(t, stream)
		if last := evs[len(evs)-1]; last["type"] != "error" {
			t.Errorf("%s: want error, got %v", name, last)
		}
	}
}

func TestSessionRequestIdentity(t *testing.T) {
	s := newSessionStore()
	a, ts1 := s.begin("k")
	b, ts2 := s.begin("k")
	if a != b || ts2 <= ts1 {
		t.Fatalf("one session per key with increasing timestamps")
	}
	contents := []any{
		map[string]any{"role": "user", "parts": []any{map[string]any{"text": "x"}}},
		map[string]any{"role": "model", "parts": []any{map[string]any{"functionResponse": map[string]any{}}}},
	}
	id, labels := agentMetadata(a, contents, "claude-sonnet-4-6", ts1)
	if labels["last_step_index"] != "3" || labels["used_claude"] != "true" || !strings.HasSuffix(id, "/4") {
		t.Errorf("%s %v", id, labels)
	}
	s.complete("k")
	if _, l := agentMetadata(a, contents, "claude-sonnet-4-6", ts2); l["last_execution_id"] == "" || l["last_step_index"] != "4" {
		t.Errorf("after an execution: %v", l)
	}
	if fnv1a64Signed("") != "-3750763034362895579" {
		t.Errorf("fnv of empty = %s", fnv1a64Signed(""))
	}
}

func TestExtensionRegisters(t *testing.T) {
	if Extension() == nil {
		t.Fatal("no extension")
	}
}
