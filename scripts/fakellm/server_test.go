package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	s, err := NewServer(defaultModels(), "", 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func doPost(t *testing.T, s *Server, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("User-Agent", "fakellm-test/1.0")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func TestChatEchoNonStream(t *testing.T) {
	s := testServer(t)
	rec := doPost(t, s, "/v1/chat/completions", `{"model":"fake-small","messages":[{"role":"user","content":"ECHO:hello world"}]}`)
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Choices[0].Message.Content != "hello world" {
		t.Fatalf("got %q", resp.Choices[0].Message.Content)
	}
	if resp.Choices[0].FinishReason != "stop" {
		t.Fatalf("finish %q", resp.Choices[0].FinishReason)
	}
	if resp.Usage.TotalTokens == 0 {
		t.Fatal("missing usage")
	}
}

func TestChatDefaultOK(t *testing.T) {
	s := testServer(t)
	rec := doPost(t, s, "/v1/chat/completions", `{"model":"m","messages":[{"role":"user","content":"hi"}]}`)
	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Choices[0].Message.Content != "OK" {
		t.Fatalf("got %q", resp.Choices[0].Message.Content)
	}
}

func TestChatStreamEcho(t *testing.T) {
	s := testServer(t)
	rec := doPost(t, s, "/v1/chat/completions", `{"model":"m","stream":true,"messages":[{"role":"user","content":"ECHO:stream me"}]}`)
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("ct %q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "data: [DONE]") {
		t.Fatalf("missing DONE:\n%s", body)
	}
	// Reassemble content deltas.
	var got strings.Builder
	sc := bufio.NewScanner(strings.NewReader(body))
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data: ") || line == "data: [DONE]" {
			continue
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &chunk); err == nil && len(chunk.Choices) > 0 {
			got.WriteString(chunk.Choices[0].Delta.Content)
		}
	}
	if got.String() != "stream me" {
		t.Fatalf("reassembled %q", got.String())
	}
}

func TestChatToolLoop(t *testing.T) {
	s := testServer(t)
	toolDef := `{"type":"function","function":{"name":"read","description":"r"}}`
	// Turn 1: directive emits a tool call.
	rec := doPost(t, s, "/v1/chat/completions", `{"model":"m","messages":[{"role":"user","content":"TOOL:read:{\"path\":\"a\"}"}],"tools":[`+toolDef+`]}`)
	var r1 struct {
		Choices []struct {
			Message struct {
				ToolCalls []struct {
					Function struct {
						Name, Arguments string
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &r1); err != nil {
		t.Fatal(err)
	}
	if r1.Choices[0].FinishReason != "tool_calls" {
		t.Fatalf("finish %q body %s", r1.Choices[0].FinishReason, rec.Body.String())
	}
	tc := r1.Choices[0].Message.ToolCalls
	if len(tc) != 1 || tc[0].Function.Name != "read" {
		t.Fatalf("calls %+v", tc)
	}
	if !strings.Contains(tc[0].Function.Arguments, `"path"`) {
		t.Fatalf("args %q", tc[0].Function.Arguments)
	}
	// Turn 2: tool result arrives, reply is DONE:<result>.
	rec2 := doPost(t, s, "/v1/chat/completions", `{"model":"m","messages":[
		{"role":"user","content":"TOOL:read:{\"path\":\"a\"}"},
		{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"read","arguments":"{}"}}]},
		{"role":"tool","tool_call_id":"call_1","content":"file-bytes"}],
		"tools":[`+toolDef+`]}`)
	var r2 struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	json.Unmarshal(rec2.Body.Bytes(), &r2)
	if r2.Choices[0].Message.Content != "DONE:file-bytes" {
		t.Fatalf("got %q", r2.Choices[0].Message.Content)
	}
}

func TestChatParallelTools(t *testing.T) {
	s := testServer(t)
	tools := `[{"type":"function","function":{"name":"a"}},{"type":"function","function":{"name":"b"}}]`
	rec := doPost(t, s, "/v1/chat/completions",
		`{"model":"m","messages":[{"role":"user","content":"TOOL:a:{\"x\":1}; TOOL:b:{\"y\":2}"}],"tools":`+tools+`}`)
	var resp struct {
		Choices []struct {
			Message struct {
				ToolCalls []struct {
					Function struct {
						Name string `json:"name"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Choices[0].Message.ToolCalls) != 2 {
		t.Fatalf("body %s", rec.Body.String())
	}
}

func TestChatMissingTool(t *testing.T) {
	s := testServer(t)
	rec := doPost(t, s, "/v1/chat/completions", `{"model":"m","messages":[{"role":"user","content":"TOOL:ghost:{}"}],"tools":[]}`)
	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Choices[0].Message.Content != "MISSING_TOOL:ghost" {
		t.Fatalf("got %q", resp.Choices[0].Message.Content)
	}
}

func TestChatReasoning(t *testing.T) {
	s := testServer(t)
	rec := doPost(t, s, "/v1/chat/completions", `{"model":"m","reasoning_effort":"high","messages":[{"role":"user","content":"hi"}]}`)
	if !strings.Contains(rec.Body.String(), "reasoning_content") {
		t.Fatalf("no reasoning block: %s", rec.Body.String())
	}
}

func TestChatBig(t *testing.T) {
	s := testServer(t)
	rec := doPost(t, s, "/v1/chat/completions", `{"model":"m","messages":[{"role":"user","content":"BIG:4"}]}`)
	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Choices[0].Message.Content) < 4*1024 {
		t.Fatalf("short %d", len(resp.Choices[0].Message.Content))
	}
}

func TestChatSlow(t *testing.T) {
	s, _ := NewServer(defaultModels(), "", 0)
	defer s.Close()
	start := time.Now()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"m","stream":true,"messages":[{"role":"user","content":"ECHO:ab SLOW:50"}]}`))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if elapsed := time.Since(start); elapsed < 40*time.Millisecond {
		t.Fatalf("too fast %v", elapsed)
	}
}

func TestChatHang(t *testing.T) {
	s := testServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"m","stream":true,"messages":[{"role":"user","content":"HANG"}]}`)).WithContext(ctx)
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { s.Handler().ServeHTTP(rec, req); close(done) }()
	time.Sleep(100 * time.Millisecond)
	select {
	case <-done:
		t.Fatal("hang returned without cancel")
	default:
	}
	if !strings.Contains(rec.Body.String(), "data:") {
		t.Fatal("hang sent no first chunk")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("hang did not unblock on cancel")
	}
}

func TestChatCrash(t *testing.T) {
	s := testServer(t)
	// httptest has no hijack support: assert the truncated partial write.
	rec := doPost(t, s, "/v1/chat/completions", `{"model":"m","stream":true,"messages":[{"role":"user","content":"CRASHCONN"}]}`)
	body := rec.Body.String()
	if !strings.Contains(body, "data:") {
		t.Fatalf("no partial body %q", body)
	}
	if strings.Contains(body, "[DONE]") {
		t.Fatalf("crash should not complete: %q", body)
	}
}

func TestResponsesToolLoop(t *testing.T) {
	s := testServer(t)
	rec := doPost(t, s, "/v1/responses", `{"model":"m","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"TOOL:run:{\"cmd\":\"ls\"}"}]}],"tools":[{"type":"function","name":"run"}]}`)
	var r1 struct {
		Output []struct {
			Type      string `json:"type"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"output"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &r1); err != nil {
		t.Fatal(err)
	}
	if len(r1.Output) != 1 || r1.Output[0].Type != "function_call" || r1.Output[0].Name != "run" {
		t.Fatalf("body %s", rec.Body.String())
	}
	rec2 := doPost(t, s, "/v1/responses", `{"model":"m","input":[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"do it"}]},
		{"type":"function_call_output","call_id":"call_1","output":"listed"}]}`)
	if !strings.Contains(rec2.Body.String(), "DONE:listed") {
		t.Fatalf("body %s", rec2.Body.String())
	}
}

func TestResponsesStream(t *testing.T) {
	s := testServer(t)
	rec := doPost(t, s, "/v1/responses", `{"model":"m","stream":true,"input":"ECHO:resp stream"}`)
	body := rec.Body.String()
	if !strings.Contains(body, "event: response.completed") {
		t.Fatalf("body %s", body)
	}
	var got strings.Builder
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var ev struct {
			Delta string `json:"delta"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev); err == nil {
			got.WriteString(ev.Delta)
		}
	}
	_ = got
}

func TestResponsesReasoning(t *testing.T) {
	s := testServer(t)
	rec := doPost(t, s, "/v1/responses", `{"model":"m","reasoning":{"effort":"high"},"input":"hi"}`)
	if !strings.Contains(rec.Body.String(), "reasoning") {
		t.Fatalf("body %s", rec.Body.String())
	}
}

func TestAnthropicToolLoop(t *testing.T) {
	s := testServer(t)
	rec := doPost(t, s, "/v1/messages", `{"model":"m","max_tokens":64,"messages":[{"role":"user","content":"TOOL:bash:{\"cmd\":\"pwd\"}"}],"tools":[{"name":"bash"}]}`)
	var r1 struct {
		Content []struct {
			Type  string         `json:"type"`
			Name  string         `json:"name"`
			Input map[string]any `json:"input"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &r1); err != nil {
		t.Fatal(err)
	}
	if r1.StopReason != "tool_use" {
		t.Fatalf("stop %q body %s", r1.StopReason, rec.Body.String())
	}
	found := false
	for _, b := range r1.Content {
		if b.Type == "tool_use" && b.Name == "bash" {
			found = true
		}
	}
	if !found {
		t.Fatalf("body %s", rec.Body.String())
	}
	rec2 := doPost(t, s, "/v1/messages", `{"model":"m","max_tokens":64,"messages":[
		{"role":"user","content":"TOOL:bash:{\"cmd\":\"pwd\"}"},
		{"role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"bash","input":{}}]},
		{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"/tmp"}]}],
		"tools":[{"name":"bash"}]}`)
	if !strings.Contains(rec2.Body.String(), "DONE:/tmp") {
		t.Fatalf("body %s", rec2.Body.String())
	}
}

func TestAnthropicStreamThinking(t *testing.T) {
	s := testServer(t)
	rec := doPost(t, s, "/v1/messages", `{"model":"m","max_tokens":64,"stream":true,"thinking":{"type":"enabled"},"messages":[{"role":"user","content":"ECHO:think out"}]}`)
	body := rec.Body.String()
	for _, want := range []string{"event: message_start", "event: message_stop", "thinking_delta", "text_delta", "think out"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in:\n%s", want, body)
		}
	}
}

func TestGeminiGenerateAndStream(t *testing.T) {
	s := testServer(t)
	rec := doPost(t, s, "/v1beta/models/gemini-fake:generateContent",
		`{"contents":[{"role":"user","parts":[{"text":"ECHO:gem ok"}]}]}`)
	if !strings.Contains(rec.Body.String(), "gem ok") {
		t.Fatalf("body %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "usageMetadata") {
		t.Fatalf("no usage: %s", rec.Body.String())
	}
	rec2 := doPost(t, s, "/v1beta/models/gemini-fake:streamGenerateContent",
		`{"contents":[{"role":"user","parts":[{"text":"ECHO:gem stream"}]}]}`)
	if ct := rec2.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("ct %q", ct)
	}
	var got strings.Builder
	for _, line := range strings.Split(rec2.Body.String(), "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var chunk struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &chunk); err == nil {
			for _, c := range chunk.Candidates {
				for _, p := range c.Content.Parts {
					got.WriteString(p.Text)
				}
			}
		}
	}
	if !strings.Contains(got.String(), "gem stream") {
		t.Fatalf("reassembled %q full:\n%s", got.String(), rec2.Body.String())
	}
}

func TestGeminiToolLoopAndThinking(t *testing.T) {
	s := testServer(t)
	rec := doPost(t, s, "/v1beta/models/gemini-fake:generateContent", `{
		"contents":[{"role":"user","parts":[{"text":"TOOL:lookup:{\"k\":\"v\"}"}]}],
		"tools":[{"functionDeclarations":[{"name":"lookup"}]}]}`)
	if !strings.Contains(rec.Body.String(), "functionCall") || !strings.Contains(rec.Body.String(), "lookup") {
		t.Fatalf("body %s", rec.Body.String())
	}
	rec2 := doPost(t, s, "/v1beta/models/gemini-fake:generateContent", `{
		"contents":[
			{"role":"user","parts":[{"text":"go"}]},
			{"role":"model","parts":[{"functionCall":{"name":"lookup","args":{}}}]},
			{"role":"user","parts":[{"functionResponse":{"name":"lookup","response":"found-it"}}]}],
		"tools":[{"functionDeclarations":[{"name":"lookup"}]}]}`)
	if !strings.Contains(rec2.Body.String(), "DONE:") || !strings.Contains(rec2.Body.String(), "found-it") {
		t.Fatalf("body %s", rec2.Body.String())
	}
	rec3 := doPost(t, s, "/v1beta/models/gemini-fake:generateContent", `{
		"contents":[{"role":"user","parts":[{"text":"hi"}]}],
		"generationConfig":{"thinkingConfig":{"thinkingBudget":100}}}`)
	if !strings.Contains(rec3.Body.String(), "thought") {
		t.Fatalf("no thought part: %s", rec3.Body.String())
	}
}

func TestModelCatalogEndpoints(t *testing.T) {
	s := testServer(t)
	h := s.Handler()

	for path, check := range map[string]string{
		"/v1/models":                          `"object":"model"`,
		"/api/v1/models":                      `"context_length"`,
		"/api.json":                           `"tool_call"`,
		"/api/models/providers/openai?types=": `"provider":"openai"`,
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("%s status %d", path, rec.Code)
		}
		// Normalize whitespace for the compact-JSON checks.
		compact := strings.ReplaceAll(rec.Body.String(), " ", "")
		if !strings.Contains(compact, check) {
			t.Fatalf("%s missing %s in %s", path, check, rec.Body.String())
		}
	}
	// OpenRouter pricing fields present.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/models", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), `"pricing"`) {
		t.Fatalf("no pricing: %s", rec.Body.String())
	}
}

func TestModelsFileShapes(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"strings.json": `["a-one","b-two"]`,
		"objects.json": `[{"id":"x/y","name":"Why","context_length":999,"pricing":{"prompt":"1","completion":"2"}}]`,
		"wrapped.json": `{"data":[{"id":"w-1"}]}`,
	} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		ms, err := LoadModels(p)
		if err != nil || len(ms) == 0 {
			t.Fatalf("%s: %v %+v", name, err, ms)
		}
	}
	ms, _ := LoadModels(filepath.Join(dir, "objects.json"))
	if ms[0].ContextLength != 999 || ms[0].PromptPrice != "1" {
		t.Fatalf("fields %+v", ms[0])
	}
}

func TestStatsAndLog(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "req.jsonl")
	s, err := NewServer(defaultModels(), logPath, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	h := s.Handler()
	post := func(path, body string) {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer x")
		req.Header.Set("User-Agent", "bench-harness")
		h.ServeHTTP(httptest.NewRecorder(), req)
	}
	post("/v1/chat/completions", `{"model":"fake-small","messages":[{"role":"user","content":"hi"}]}`)
	post("/v1/messages", `{"model":"m","max_tokens":8,"messages":[{"role":"user","content":"hi"}]}`)

	req := httptest.NewRequest(http.MethodGet, "/__stats", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var rawStats struct {
		Total  int            `json:"total"`
		ByPath map[string]int `json:"by_path"`
	}
	_ = rawStats
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if raw["total"].(float64) < 2 {
		t.Fatalf("stats %s", rec.Body.String())
	}

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(logBytes)), "\n")
	if len(lines) != 2 {
		t.Fatalf("log lines %d: %s", len(lines), string(logBytes))
	}
	var entry map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"path", "model", "tools", "auth", "ua"} {
		if _, ok := entry[k]; !ok {
			t.Fatalf("log missing %q: %s", k, lines[0])
		}
	}
	if entry["auth"] != true || entry["ua"] != "bench-harness" {
		t.Fatalf("log fields: %s", lines[0])
	}
}

func TestFailureDirectivesAllProtocols(t *testing.T) {
	s := testServer(t)
	// HANG on anthropic non-stream: headers sent, body held.
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/v1/messages",
		strings.NewReader(`{"model":"m","max_tokens":8,"messages":[{"role":"user","content":"HANG"}]}`)).WithContext(ctx)
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { s.Handler().ServeHTTP(rec, req); close(done) }()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("anthropic hang did not unblock")
	}
	// CRASHCONN on gemini stream: partial, no completion.
	rec2 := doPost(t, s, "/v1beta/models/g:streamGenerateContent",
		`{"contents":[{"role":"user","parts":[{"text":"CRASHCONN"}]}]}`)
	if !strings.Contains(rec2.Body.String(), "data:") {
		t.Fatalf("no partial %q", rec2.Body.String())
	}
	// Consume io to satisfy import.
	_, _ = io.Discard, context.Background
}
