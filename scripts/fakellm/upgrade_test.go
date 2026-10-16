package main

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------- (1) Anthropic streaming shapes ----------

func anthEvents(t *testing.T, body string) []map[string]any {
	t.Helper()
	var out []map[string]any
	sc := bufio.NewScanner(strings.NewReader(body))
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	var event string
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "event: ") {
			event = strings.TrimPrefix(line, "event: ")
			continue
		}
		if strings.HasPrefix(line, "data: ") {
			var v map[string]any
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &v); err != nil {
				continue
			}
			v["_event"] = event
			out = append(out, v)
			event = ""
		}
	}
	return out
}

func TestAnthStreamUsageAndInput(t *testing.T) {
	s := testServer(t)
	rec := doPost(t, s, "/v1/messages", `{"model":"m","max_tokens":64,"stream":true,"messages":[{"role":"user","content":"ECHO:hi there"}]}`)
	evs := anthEvents(t, rec.Body.String())
	if len(evs) == 0 {
		t.Fatalf("no events:\n%s", rec.Body.String())
	}
	// message_start.message.usage = {input_tokens, output_tokens:0}.
	start := evs[0]
	msg, _ := start["message"].(map[string]any)
	if msg == nil {
		t.Fatalf("no message in start: %v", start)
	}
	usage, _ := msg["usage"].(map[string]any)
	if usage == nil {
		t.Fatalf("message_start.message missing usage: %v", msg)
	}
	if _, ok := usage["input_tokens"]; !ok {
		t.Fatalf("missing input_tokens: %v", usage)
	}
	if usage["output_tokens"] != float64(0) {
		t.Fatalf("output_tokens != 0: %v", usage)
	}
	// Final message_delta carries usage {output_tokens}.
	var delta map[string]any
	for _, e := range evs {
		if e["_event"] == "message_delta" {
			delta = e
		}
	}
	if delta == nil {
		t.Fatalf("no message_delta")
	}
	du, _ := delta["usage"].(map[string]any)
	if du == nil || du["output_tokens"] == nil {
		t.Fatalf("message_delta missing usage.output_tokens: %v", delta)
	}
}

func TestAnthStreamToolInputEmpty(t *testing.T) {
	s := testServer(t)
	rec := doPost(t, s, "/v1/messages", `{"model":"m","max_tokens":64,"stream":true,
		"messages":[{"role":"user","content":"TOOL:bash:{\"cmd\":\"pwd\"}"}],
		"tools":[{"name":"bash"}]}`)
	evs := anthEvents(t, rec.Body.String())
	found := false
	for _, e := range evs {
		if e["_event"] != "content_block_start" {
			continue
		}
		cb, _ := e["content_block"].(map[string]any)
		if cb == nil || cb["type"] != "tool_use" {
			continue
		}
		found = true
		inp, ok := cb["input"]
		if !ok {
			t.Fatalf("tool_use start missing input: %v", cb)
		}
		m, _ := inp.(map[string]any)
		if m == nil || len(m) != 0 {
			t.Fatalf("input not {}: %v", inp)
		}
	}
	if !found {
		t.Fatalf("no tool_use start:\n%s", rec.Body.String())
	}
	// Tool stream final delta also carries usage.
	var delta map[string]any
	for _, e := range evs {
		if e["_event"] == "message_delta" {
			delta = e
		}
	}
	if du, _ := delta["usage"].(map[string]any); du == nil || du["output_tokens"] == nil {
		t.Fatalf("tool message_delta missing usage: %v", delta)
	}
}

// ---------- (2) OpenAI chat tool-call chunks ----------

func chatStreamChunks(t *testing.T, body string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "data: ") || line == "data: [DONE]" {
			continue
		}
		var v map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &v); err == nil {
			out = append(out, v)
		}
	}
	return out
}

func TestChatToolIndexNonStream(t *testing.T) {
	s := testServer(t)
	tools := `[{"type":"function","function":{"name":"a"}},{"type":"function","function":{"name":"b"}}]`
	rec := doPost(t, s, "/v1/chat/completions",
		`{"model":"m","messages":[{"role":"user","content":"TOOL:a:{\"x\":1}; TOOL:b:{\"y\":2}"}],"tools":`+tools+`}`)
	var resp struct {
		Choices []struct {
			Message struct {
				ToolCalls []map[string]any `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	tcs := resp.Choices[0].Message.ToolCalls
	if len(tcs) != 2 {
		t.Fatalf("body %s", rec.Body.String())
	}
	for i, tc := range tcs {
		if tc["index"] != float64(i) {
			t.Fatalf("call %d missing index %d: %v", i, i, tc)
		}
	}
}

func TestChatToolStreamRealistic(t *testing.T) {
	s := testServer(t)
	rec := doPost(t, s, "/v1/chat/completions",
		`{"model":"m","stream":true,"stream_options":{"include_usage":true},
		  "messages":[{"role":"user","content":"TOOL:read:{\"path\":\"a\"}"}],
		  "tools":[{"type":"function","function":{"name":"read"}}]}`)
	chunks := chatStreamChunks(t, rec.Body.String())
	if len(chunks) < 3 {
		t.Fatalf("too few chunks:\n%s", rec.Body.String())
	}
	// Collect tool_calls deltas in order.
	type tc struct {
		index int
		id    string
		typ   string
		name  string
		args  string
	}
	var seq []tc
	for _, ch := range chunks {
		chs, _ := ch["choices"].([]any)
		if len(chs) == 0 {
			continue
		}
		cm, _ := chs[0].(map[string]any)
		dm, _ := cm["delta"].(map[string]any)
		tcs, _ := dm["tool_calls"].([]any)
		for _, t0 := range tcs {
			tm, _ := t0.(map[string]any)
			e := tc{index: -1}
			if ix, ok := tm["index"].(float64); ok {
				e.index = int(ix)
			}
			e.id, _ = tm["id"].(string)
			e.typ, _ = tm["type"].(string)
			if fn, ok := tm["function"].(map[string]any); ok {
				e.name, _ = fn["name"].(string)
				e.args, _ = fn["arguments"].(string)
			}
			seq = append(seq, e)
		}
	}
	if len(seq) < 2 {
		t.Fatalf("no tool deltas: %+v\n%s", seq, rec.Body.String())
	}
	if seq[0].index != 0 || seq[0].id == "" || seq[0].typ != "function" || seq[0].name != "read" {
		t.Fatalf("first chunk must carry index/id/type/name: %+v", seq[0])
	}
	// Arguments reassemble to the full JSON.
	var got strings.Builder
	for _, e := range seq {
		got.WriteString(e.args)
		if e.index != 0 {
			t.Fatalf("delta missing per-call index 0: %+v", e)
		}
	}
	if !strings.Contains(got.String(), `"path"`) {
		t.Fatalf("args did not reassemble: %q", got.String())
	}
	// Final chunk carries usage when include_usage.
	last := chunks[len(chunks)-1]
	if _, ok := last["usage"]; !ok {
		t.Fatalf("final chunk missing usage:\n%s", rec.Body.String())
	}
}

// ---------- (3) unique tool ids ----------

func TestToolIDsUniqueAcrossTurns(t *testing.T) {
	s := testServer(t)
	seen := map[string]bool{}
	for i := 0; i < 3; i++ {
		rec := doPost(t, s, "/v1/chat/completions",
			`{"model":"m","messages":[{"role":"user","content":"TOOL:read:{\"n\":`+string(rune('0'+i))+`}"}],"tools":[{"type":"function","function":{"name":"read"}}]}`)
		var r struct {
			Choices []struct {
				Message struct {
					ToolCalls []struct {
						ID string `json:"id"`
					} `json:"tool_calls"`
				} `json:"message"`
			} `json:"choices"`
		}
		json.Unmarshal(rec.Body.Bytes(), &r)
		id := r.Choices[0].Message.ToolCalls[0].ID
		if seen[id] {
			t.Fatalf("reused id %q", id)
		}
		seen[id] = true
		if !strings.HasPrefix(id, "call_") {
			t.Fatalf("bad prefix %q", id)
		}
	}
	rec := doPost(t, s, "/v1/messages",
		`{"model":"m","max_tokens":8,"messages":[{"role":"user","content":"TOOL:bash:{}"}],"tools":[{"name":"bash"}]}`)
	if seen["toolu_dup"] {
		t.Fatal("unreachable")
	}
	var ar struct {
		Content []struct {
			ID string `json:"id"`
		} `json:"content"`
	}
	json.Unmarshal(rec.Body.Bytes(), &ar)
	for _, b := range ar.Content {
		if b.ID != "" {
			if seen[b.ID] {
				t.Fatalf("anthropic reused id %q", b.ID)
			}
			seen[b.ID] = true
		}
	}
}

// ---------- (4) COOPERATE ----------

func coopChat(t *testing.T, s *Server, prompt, tools, hdr string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"m","messages":[{"role":"user","content":`+mustJSON(prompt)+`}],"tools":`+tools+`}`))
	req.Header.Set("Content-Type", "application/json")
	if hdr != "" {
		req.Header.Set("X-Fake-Cooperate", hdr)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec.Body.String()
}

func TestCooperateFuzzy(t *testing.T) {
	s := testServer(t)
	s.SetCooperate(true)
	variants := []string{"echo", "acpbench-stub_echo", "mcp__acpbench_stub__echo",
		"acpbench_stub__echo", "acpbench-stub__echo", "functions.echo"}
	for _, v := range variants {
		body := coopChat(t, s, "please use the echo tool",
			`[{"type":"function","function":{"name":`+mustJSON(v)+`}}]`, "")
		if !strings.Contains(body, `"name":"`+v+`"`) && !strings.Contains(body, v) {
			t.Fatalf("variant %q: no call:\n%s", v, body)
		}
		if !strings.Contains(body, "hello") {
			t.Fatalf("variant %q: default text arg missing:\n%s", v, body)
		}
	}
	// Case-insensitive mention.
	body := coopChat(t, s, "Please CALL the ECHO tool now",
		`[{"type":"function","function":{"name":"echo"}}]`, "")
	if !strings.Contains(body, "tool_calls") {
		t.Fatalf("case-insensitive failed:\n%s", body)
	}
	// Header enables without the flag.
	s2 := testServer(t)
	body = coopChat(t, s2, "use echo please", `[{"type":"function","function":{"name":"echo"}}]`, "1")
	if !strings.Contains(body, "tool_calls") {
		t.Fatalf("header cooperate failed:\n%s", body)
	}
	// Env enables without the flag.
	t.Setenv("FAKELLM_COOPERATE", "1")
	body = coopChat(t, s2, "use echo please", `[{"type":"function","function":{"name":"echo"}}]`, "")
	if !strings.Contains(body, "tool_calls") {
		t.Fatalf("env cooperate failed:\n%s", body)
	}
}

func TestCooperateFireOnceAndSlow(t *testing.T) {
	s := testServer(t)
	s.SetCooperate(true)
	tools := `[{"type":"function","function":{"name":"echo"}}]`
	// Turn 1: NL prompt -> one call.
	rec := doPost(t, s, "/v1/chat/completions", `{"model":"m","messages":[{"role":"user","content":"can you use echo?"}],"tools":`+tools+`}`)
	var r1 struct {
		Choices []struct {
			Message struct {
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name, Arguments string
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	json.Unmarshal(rec.Body.Bytes(), &r1)
	if len(r1.Choices[0].Message.ToolCalls) != 1 {
		t.Fatalf("expected exactly one call: %s", rec.Body.String())
	}
	callID := r1.Choices[0].Message.ToolCalls[0].ID
	// Turn 2: feed result back, still mentioning the tool -> DONE, no loop.
	rec2 := doPost(t, s, "/v1/chat/completions", `{"model":"m","messages":[
		{"role":"user","content":"can you use echo?"},
		{"role":"assistant","tool_calls":[{"id":"`+callID+`","type":"function","function":{"name":"echo","arguments":"{}"}}]},
		{"role":"tool","tool_call_id":"`+callID+`","content":"hi-out"}],
		"tools":`+tools+`}`)
	if !strings.Contains(rec2.Body.String(), "DONE:hi-out") {
		t.Fatalf("no DONE:\n%s", rec2.Body.String())
	}
	if strings.Contains(rec2.Body.String(), "tool_calls") {
		t.Fatalf("looped:\n%s", rec2.Body.String())
	}
	// Slow: prompt picks the slow tool with ms from the prompt.
	slowTools := `[{"type":"function","function":{"name":"acpbench_stub__slow","description":"s"}}]`
	rec3 := doPost(t, s, "/v1/chat/completions", `{"model":"m","messages":[{"role":"user","content":"please run the slow tool for 200ms"}],"tools":`+slowTools+`}`)
	// Need cooperate on this server: s has it on. (Args are JSON-escaped in the body.)
	if !strings.Contains(rec3.Body.String(), "slow") || !strings.Contains(rec3.Body.String(), "200") {
		t.Fatalf("slow ms missing:\n%s", rec3.Body.String())
	}
	// Slow default 5000 when no number.
	rec4 := doPost(t, s, "/v1/chat/completions", `{"model":"m","messages":[{"role":"user","content":"use the slow tool"}],"tools":`+slowTools+`}`)
	if !strings.Contains(rec4.Body.String(), "5000") {
		t.Fatalf("slow default missing:\n%s", rec4.Body.String())
	}
}

// ---------- (5) failure injection ----------

func TestFaultHTTPStatuses(t *testing.T) {
	for _, tc := range []struct {
		directive string
		status    int
		retry     bool
	}{
		{"FAULT:429", 429, true}, {"FAULT:500", 500, false}, {"FAULT:502", 502, false},
		{"FAULT:503", 503, false}, {"FAULT:401", 401, false},
	} {
		s := testServer(t)
		rec := doPost(t, s, "/v1/chat/completions",
			`{"model":"m","messages":[{"role":"user","content":"`+tc.directive+`"}]}`)
		if rec.Code != tc.status {
			t.Fatalf("%s: status %d body %s", tc.directive, rec.Code, rec.Body.String())
		}
		if tc.retry && rec.Header().Get("Retry-After") == "" {
			t.Fatalf("%s: missing Retry-After", tc.directive)
		}
	}
	// 529 keeps the Anthropic overloaded envelope.
	s := testServer(t)
	rec := doPost(t, s, "/v1/messages",
		`{"model":"m","max_tokens":8,"messages":[{"role":"user","content":"FAULT:529"}]}`)
	if rec.Code != 529 {
		t.Fatalf("529 status %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "overloaded_error") {
		t.Fatalf("529 shape: %s", rec.Body.String())
	}
}

func TestFaultStreamErrorShapes(t *testing.T) {
	s := testServer(t)
	rec := doPost(t, s, "/v1/messages",
		`{"model":"m","max_tokens":8,"stream":true,"messages":[{"role":"user","content":"FAULT:SSE_ERROR"}]}`)
	if !strings.Contains(rec.Body.String(), "event: error") || !strings.Contains(rec.Body.String(), "overloaded_error") {
		t.Fatalf("anthropic stream error:\n%s", rec.Body.String())
	}
	rec2 := doPost(t, s, "/v1/chat/completions",
		`{"model":"m","stream":true,"messages":[{"role":"user","content":"FAULT:SSE_ERROR"}]}`)
	if !strings.Contains(rec2.Body.String(), `"error"`) || strings.Contains(rec2.Body.String(), "[DONE]") {
		t.Fatalf("openai stream error:\n%s", rec2.Body.String())
	}
}

func TestFaultStallTTFB(t *testing.T) {
	s := testServer(t)
	start := time.Now()
	rec := doPost(t, s, "/v1/chat/completions",
		`{"model":"m","stream":true,"messages":[{"role":"user","content":"ECHO:stall test here FAULT:STALL:1:200"}]}`)
	if elapsed := time.Since(start); elapsed < 150*time.Millisecond {
		t.Fatalf("stall too fast %v", elapsed)
	}
	if !strings.Contains(rec.Body.String(), "stall test") {
		t.Fatalf("stall dropped content:\n%s", rec.Body.String())
	}
	start = time.Now()
	doPost(t, s, "/v1/chat/completions",
		`{"model":"m","messages":[{"role":"user","content":"hi FAULT:TTFB:150"}]}`)
	if elapsed := time.Since(start); elapsed < 120*time.Millisecond {
		t.Fatalf("ttfb too fast %v", elapsed)
	}
}

func TestFaultTruncatedInvalidEmptyLengthDup(t *testing.T) {
	s := testServer(t)
	rec := doPost(t, s, "/v1/chat/completions", `{"model":"m","messages":[{"role":"user","content":"FAULT:TRUNCATED"}]}`)
	var v any
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err == nil {
		t.Fatalf("truncated parsed as valid JSON: %s", rec.Body.String())
	}
	rec = doPost(t, s, "/v1/chat/completions",
		`{"model":"m","stream":true,"messages":[{"role":"user","content":"FAULT:INVALID_SSE"}]}`)
	if strings.Contains(rec.Body.String(), "data: [DONE]") || !strings.Contains(rec.Body.String(), "not-an-sse") {
		t.Fatalf("invalid sse:\n%s", rec.Body.String())
	}
	rec = doPost(t, s, "/v1/chat/completions", `{"model":"m","messages":[{"role":"user","content":"FAULT:EMPTY"}]}`)
	if !strings.Contains(rec.Body.String(), `"content":""`) {
		t.Fatalf("empty:\n%s", rec.Body.String())
	}
	rec = doPost(t, s, "/v1/chat/completions", `{"model":"m","messages":[{"role":"user","content":"FAULT:LENGTH"}]}`)
	if !strings.Contains(rec.Body.String(), `"length"`) {
		t.Fatalf("length:\n%s", rec.Body.String())
	}
	rec = doPost(t, s, "/v1/chat/completions",
		`{"model":"m","stream":true,"messages":[{"role":"user","content":"ECHO:duplicate words here yes FAULT:DUPLICATE"}]}`)
	chunks := chatStreamChunks(t, rec.Body.String())
	var texts []string
	for _, ch := range chunks {
		if chs, ok := ch["choices"].([]any); ok && len(chs) > 0 {
			if cm, ok := chs[0].(map[string]any); ok {
				if dm, ok := cm["delta"].(map[string]any); ok {
					if c, ok := dm["content"].(string); ok && c != "" {
						texts = append(texts, c)
					}
				}
			}
		}
	}
	seen := map[string]int{}
	for _, x := range texts {
		seen[x]++
	}
	dup := false
	for _, n := range seen {
		if n >= 2 {
			dup = true
		}
	}
	if !dup {
		t.Fatalf("no duplicated chunks %q:\n%s", texts, rec.Body.String())
	}
}

func TestFaultsByCounterAndRate(t *testing.T) {
	s := testServer(t)
	s.SetFaults(map[int]string{2: faultHTTP500})
	doPost(t, s, "/v1/chat/completions", `{"model":"m","messages":[{"role":"user","content":"hi"}]}`)
	rec := doPost(t, s, "/v1/chat/completions", `{"model":"m","messages":[{"role":"user","content":"hi"}]}`)
	if rec.Code != 500 {
		t.Fatalf("counter fault: %d %s", rec.Code, rec.Body.String())
	}
	// Seeded soak: with p=1 every request fails deterministically.
	dir := t.TempDir()
	logPath := filepath.Join(dir, "f.jsonl")
	s2, err := NewServer(defaultModels(), logPath, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	s2.SetFaultRate(1.0, 7)
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
			strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`))
		rec := httptest.NewRecorder()
		s2.Handler().ServeHTTP(rec, req)
	}
	lb, _ := os.ReadFile(logPath)
	lines := strings.Split(strings.TrimSpace(string(lb)), "\n")
	if len(lines) != 5 {
		t.Fatalf("log lines %d", len(lines))
	}
	for _, ln := range lines {
		var e map[string]any
		json.Unmarshal([]byte(ln), &e)
		if e["fault"] == nil || e["fault"] == "" {
			t.Fatalf("missing fault class in log: %s", ln)
		}
	}
}

// ---------- (6) mutable catalog ----------

func TestCatalogMutable(t *testing.T) {
	s := testServer(t)
	h := s.Handler()
	get := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	before := get("/v1/models").Body.String()
	if !strings.Contains(before, "fake-small") {
		t.Fatalf("before: %s", before)
	}
	put := httptest.NewRequest(http.MethodPut, "/__catalog", strings.NewReader(`["bench-a","bench-b"]`))
	pr := httptest.NewRecorder()
	h.ServeHTTP(pr, put)
	if pr.Code != 200 {
		t.Fatalf("put %d %s", pr.Code, pr.Body.String())
	}
	after := get("/v1/models").Body.String()
	if !strings.Contains(after, "bench-a") || strings.Contains(after, "fake-small") {
		t.Fatalf("after: %s", after)
	}
	cat := get("/__catalog").Body.String()
	if !strings.Contains(cat, "bench-b") {
		t.Fatalf("catalog get: %s", cat)
	}
	stats := get("/__stats").Body.String()
	var st struct {
		CatalogFetches int `json:"catalog_fetches"`
	}
	json.Unmarshal([]byte(stats), &st)
	// before + after + catalog get = 3 fetches (PUT is not a fetch).
	if st.CatalogFetches < 3 {
		t.Fatalf("fetches %d: %s", st.CatalogFetches, stats)
	}
}

// ---------- (7) stats ----------

func TestStatsRich(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewServer(defaultModels(), filepath.Join(dir, "l.jsonl"), 0)
	defer s.Close()
	h := s.Handler()
	post := func(body string) {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
		h.ServeHTTP(httptest.NewRecorder(), req)
	}
	post(`{"model":"fake-small","messages":[{"role":"user","content":"hi"}]}`)
	post(`{"model":"fake-small","messages":[{"role":"user","content":"FAULT:500"}]}`)
	req := httptest.NewRequest(http.MethodGet, "/__stats", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var st struct {
		Total      int            `json:"total"`
		ByModel    map[string]int `json:"by_model"`
		ByFault    map[string]int `json:"by_fault"`
		LastReq    string         `json:"last_request"`
		CatFetches int            `json:"catalog_fetches"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if st.ByModel["fake-small"] < 2 {
		t.Fatalf("by_model: %s", rec.Body.String())
	}
	if st.ByFault["http500"] < 1 {
		t.Fatalf("by_fault: %s", rec.Body.String())
	}
	if st.LastReq == "" {
		t.Fatalf("last_request missing: %s", rec.Body.String())
	}
	if _, err := time.Parse(time.RFC3339Nano, st.LastReq); err != nil {
		t.Fatalf("last_request bad time %q", st.LastReq)
	}
}
