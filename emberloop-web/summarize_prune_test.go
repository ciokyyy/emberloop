package web

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// prunerSummarizer is a scripted summarizer that also prunes.
type prunerSummarizer struct {
	*scriptedSummarizer
	pruned []summaryBlock
	err    error
	calls  int
	target int
}

func (p *prunerSummarizer) Prune(_ []Event, target int) ([]summaryBlock, error) {
	p.calls++
	p.target = target
	return p.pruned, p.err
}

func bigLog() []Event {
	return []Event{
		{Seq: 1, Kind: "prompt", Text: "build the thing " + strings.Repeat("x", pruneMinChars+1000)},
		{Seq: 2, Kind: "assistant_chunk", Text: "on it"},
	}
}

func TestWriteHandoffUsesPrunedBlocksForOversizeLog(t *testing.T) {
	sum := &prunerSummarizer{
		scriptedSummarizer: &scriptedSummarizer{replies: []string{goodSummary}},
		pruned:             []summaryBlock{{text: "USER:\nbuild the thing (pruned)"}},
	}
	var notes []string
	res := writeHandoff(sum, "m", "", bigLog(), func(s string) { notes = append(notes, s) })
	if sum.calls != 1 || sum.target != pruneTargetTokens {
		t.Fatalf("prune calls = %d target = %d", sum.calls, sum.target)
	}
	if len(sum.scriptedSummarizer.calls) != 1 || !strings.Contains(sum.scriptedSummarizer.calls[0].user, "(pruned)") {
		t.Fatalf("summarizer did not read the pruned log: %+v", sum.scriptedSummarizer.calls)
	}
	if len(sum.scriptedSummarizer.calls[0].user) > 1000 {
		t.Fatalf("summarizer input was not the pruned trace: %d chars", len(sum.scriptedSummarizer.calls[0].user))
	}
	if res.fallback != stepSummary || res.blocked {
		t.Fatalf("result = %+v", res)
	}
	if len(notes) == 0 || !strings.Contains(notes[0], "prune: Relace Compact cut the log") {
		t.Fatalf("notes = %v", notes)
	}
}

func TestPruneSkippedForSmallLog(t *testing.T) {
	sum := &prunerSummarizer{scriptedSummarizer: &scriptedSummarizer{replies: []string{goodSummary}}}
	events := []Event{{Seq: 1, Kind: "prompt", Text: "small"}}
	writeHandoff(sum, "m", "", events, func(string) {})
	if sum.calls != 0 {
		t.Fatalf("prune ran on a small log: %d calls", sum.calls)
	}
}

func TestPruneFailureKeepsFullLogAndNeverBlocks(t *testing.T) {
	sum := &prunerSummarizer{
		scriptedSummarizer: &scriptedSummarizer{replies: []string{goodSummary}},
		err:                errors.New("relace compact: HTTP 402: Out of credits"),
	}
	var notes []string
	res := writeHandoff(sum, "m", "", bigLog(), func(s string) { notes = append(notes, s) })
	if res.blocked || res.fallback != stepSummary {
		t.Fatalf("a failed prune changed the outcome: %+v", res)
	}
	if got := len(sum.scriptedSummarizer.calls[0].user); got < pruneMinChars {
		t.Fatalf("summarizer did not read the full log: %d chars", got)
	}
	if len(notes) == 0 || !strings.Contains(notes[0], "using the full log") {
		t.Fatalf("notes = %v", notes)
	}
}

func TestPruneOffIsSilentAndKeepsFullLog(t *testing.T) {
	sum := &prunerSummarizer{scriptedSummarizer: &scriptedSummarizer{replies: []string{goodSummary}}, err: errPruneOff}
	var notes []string
	writeHandoff(sum, "m", "", bigLog(), func(s string) { notes = append(notes, s) })
	if len(notes) != 0 {
		t.Fatalf("prune off made noise: %v", notes)
	}
}

func TestPruneThatDoesNotShrinkKeepsFullLog(t *testing.T) {
	sum := &prunerSummarizer{
		scriptedSummarizer: &scriptedSummarizer{replies: []string{goodSummary}},
		pruned:             []summaryBlock{{text: "USER:\n" + strings.Repeat("y", pruneMinChars*2)}},
	}
	writeHandoff(sum, "m", "", bigLog(), func(string) {})
	if got := len(sum.scriptedSummarizer.calls[0].user); got > pruneMinChars+5000 {
		t.Fatalf("summarizer read the larger pruned log: %d chars", got)
	}
}

func TestRetryingSummarizerPruneFollowsTheSetting(t *testing.T) {
	inner := &prunerSummarizer{scriptedSummarizer: &scriptedSummarizer{}, pruned: []summaryBlock{{text: "a"}}}
	off := &retryingSummarizer{next: inner, noPrune: true}
	if _, err := off.Prune(nil, 1); !errors.Is(err, errPruneOff) {
		t.Fatalf("setting off: err = %v", err)
	}
	on := &retryingSummarizer{next: inner}
	if blocks, err := on.Prune(nil, 1); err != nil || len(blocks) != 1 {
		t.Fatalf("setting on: %v %v", blocks, err)
	}
	plain := &retryingSummarizer{next: &scriptedSummarizer{}}
	if _, err := plain.Prune(nil, 1); !errors.Is(err, errPruneOff) {
		t.Fatalf("summarizer without prune: err = %v", err)
	}
}

func TestRelacePrunerWithoutKeyIsOff(t *testing.T) {
	t.Setenv("RELACE_API_KEY", "")
	p := relacePruner{agentDir: tempDir(t)}
	if _, err := p.Prune(bigLog(), 1000); !errors.Is(err, errPruneOff) {
		t.Fatalf("err = %v", err)
	}
}

func TestRelacePrunerSendsTraceAndRendersPrunedBlocks(t *testing.T) {
	t.Setenv("RELACE_API_KEY", "rlc-test")
	var gotAuth, gotPath string
	var gotBody struct {
		Messages     []traceMessage `json:"messages"`
		TargetTokens int            `json:"target_tokens"`
		AgentModel   string         `json:"agent_model"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath = r.Header.Get("Authorization"), r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		_, _ = w.Write([]byte(`{"messages":[
			{"role":"user","content":"fix the bug"},
			{"role":"assistant","content":"","tool_calls":[{"id":"c1","type":"function","function":{"name":"bash","arguments":"{\"command\":\"go test\"}"}}]},
			{"role":"tool","tool_call_id":"c1","content":"FAIL x_test.go"},
			{"role":"assistant","content":"found it"}],
			"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`))
	}))
	defer srv.Close()

	events := []Event{
		{Seq: 1, Kind: "prompt", Text: "fix the bug"},
		{Seq: 2, Kind: "thinking_chunk", Text: "hmm"},
		{Seq: 3, Kind: "tool_call_started", Text: `{"id":"c1","name":"bash","input":"{\"command\":\"go test\"}","status":"running"}`},
		{Seq: 4, Kind: "tool_call_updated", Text: `{"id":"c1","name":"bash","input":"{\"command\":\"go test\"}","output":"FAIL x_test.go","status":"failed"}`},
		{Seq: 5, Kind: "assistant_chunk", Text: "found "},
		{Seq: 6, Kind: "assistant_chunk", Text: "it"},
	}
	blocks, err := relacePruner{baseURL: srv.URL}.Prune(events, 32000)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1/code/compact" || gotAuth != "Bearer rlc-test" || gotBody.TargetTokens != 32000 || gotBody.AgentModel == "" {
		t.Fatalf("request = path %q auth %q body %+v", gotPath, gotAuth, gotBody)
	}
	// prompt, one tool call pair (updated in place), one assistant text.
	if len(gotBody.Messages) != 4 || gotBody.Messages[0].Role != "user" || gotBody.Messages[1].ToolCalls[0].ID != "c1" ||
		gotBody.Messages[2].Role != "tool" || !strings.HasPrefix(gotBody.Messages[2].text(), "[failed] FAIL x_test.go") ||
		gotBody.Messages[3].text() != "found it" {
		t.Fatalf("trace = %+v", gotBody.Messages)
	}
	if len(blocks) != 3 || blocks[0].text != "USER:\nfix the bug" || !strings.HasPrefix(blocks[1].text, "TOOL bash\ninput: ") ||
		blocks[1].result != "FAIL x_test.go" || blocks[2].text != "ASSISTANT:\nfound it" {
		t.Fatalf("blocks = %+v", blocks)
	}
}

func TestRelacePrunerReportsHTTPFailure(t *testing.T) {
	t.Setenv("RELACE_API_KEY", "rlc-test")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{"error":"Out of credits."}`))
	}))
	defer srv.Close()
	_, err := relacePruner{baseURL: srv.URL}.Prune(bigLog(), 1000)
	if err == nil || !strings.Contains(err.Error(), "HTTP 402") {
		t.Fatalf("err = %v", err)
	}
}

func TestCompressPruneSettingDefaultsOnAndPatches(t *testing.T) {
	if !parseCompressSettings(map[string]json.RawMessage{}).Prune {
		t.Fatal("prune should default on")
	}
	all := map[string]json.RawMessage{}
	off := false
	(&compressPatch{Prune: &off}).apply(all)
	if parseCompressSettings(all).Prune {
		t.Fatal("compress_prune=false did not turn the prune off")
	}
}
