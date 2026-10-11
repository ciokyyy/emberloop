package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

// goodSummary is a reply with all five required handoff sections.
const goodSummary = "## Task\nfix the parser\n\n## Context\nrepo emberloop, branch main\n\n## Progress\nedited parser.go\n\n## Next steps\nrun the tests\n\n## Reference\nTestParse\n"

// unauthedPigModel is a catalog model whose provider holds no auth in the test environment.
func unauthedPigModel(t *testing.T) string {
	t.Helper()
	models := ai.ListModels("anthropic")
	if len(models) == 0 {
		t.Fatal("no anthropic models in catalog")
	}
	return models[0].Provider + "/" + models[0].ID
}

// withoutPigAuth points pig at an empty agent dir and drops the env keys, so
// no provider is authed except test-faux when PIG_TEST_FAUX=1 is set.
func withoutPigAuth(t *testing.T) {
	t.Helper()
	t.Setenv("PIG_CODING_AGENT_DIR", tempDir(t))
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENROUTER_API_KEY", "")
}

// sessionField reads one string field of the Session GET body.
func sessionField(t *testing.T, base, id, key string) string {
	t.Helper()
	resp, err := http.Get(base + "/api/sessions/" + id)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	s, _ := out[key].(string)
	return s
}

func errorText(body map[string]any) string {
	s, _ := body["error"].(string)
	return s
}

// The compress model is one global setting, saved only when it names an
// authed pig model.
func TestCompressModelSettingAcceptsOnlyAuthedPigModels(t *testing.T) {
	withoutPigAuth(t)
	t.Setenv("PIG_TEST_FAUX", "1")
	useSettings(t, "")
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	put := func(model string) int {
		body, _ := json.Marshal(map[string]any{"compress": map[string]string{"compress_model": model}})
		req, _ := http.NewRequest("PUT", srv.URL+"/api/settings", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	saved := func() string {
		resp, err := http.Get(srv.URL + "/api/settings")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out struct {
			Compress struct {
				Model string `json:"compress_model"`
			} `json:"compress"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return out.Compress.Model
	}

	if code := put("test-faux/faux-1"); code != http.StatusOK {
		t.Fatalf("authed model put = %d, want 200", code)
	}
	if got := saved(); got != "test-faux/faux-1" {
		t.Fatalf("saved model = %q", got)
	}
	if code := put(unauthedPigModel(t)); code != http.StatusBadRequest {
		t.Fatalf("unauthed model put = %d, want 400", code)
	}
	if code := put("nope/unknown"); code != http.StatusBadRequest {
		t.Fatalf("model outside the catalog put = %d, want 400", code)
	}
	if got := saved(); got != "test-faux/faux-1" {
		t.Fatalf("a rejected PUT changed the model: %q", got)
	}
	if code := put(""); code != http.StatusOK || saved() != "" {
		t.Fatalf("clearing the model = %d, saved %q", code, saved())
	}
}

// Compress on a Handoff Backend is disabled with a reason until the compress
// model is set and its provider holds pig auth.
func TestHandoffCompressRefusedWithReasonWithoutAuthedCompressModel(t *testing.T) {
	withoutPigAuth(t)
	useSettings(t, "")
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.summarizer = &scriptedSummarizer{replies: []string{goodSummary}}
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))

	code, body := compressReq(t, srv.URL, id, "")
	if code != http.StatusConflict || !strings.Contains(errorText(body), "no compress model") {
		t.Fatalf("unset model compress = %d %v, want 409 with reason", code, body)
	}
	if note := sessionField(t, srv.URL, id, "compressNote"); !strings.Contains(note, "no compress model") {
		t.Fatalf("compressNote = %q, want the unset reason", note)
	}

	useSettings(t, `{"compress_model":"`+unauthedPigModel(t)+`"}`)
	code, body = compressReq(t, srv.URL, id, "")
	if code != http.StatusConflict || !strings.Contains(errorText(body), "not an authed pig model") {
		t.Fatalf("unauthed model compress = %d %v, want 409 with reason", code, body)
	}

	t.Setenv("PIG_TEST_FAUX", "1")
	useSettings(t, `{"compress_model":"test-faux/faux-1"}`)
	if note := sessionField(t, srv.URL, id, "compressNote"); note != "" {
		t.Fatalf("compressNote = %q with an authed model, want empty", note)
	}
	if code, body := compressReq(t, srv.URL, id, ""); code != http.StatusAccepted {
		t.Fatalf("authed model compress = %d %v, want 202", code, body)
	}
	waitIdleAfter(t, srv.URL, id, 1)
}

// summaryCall is one summarizer call as the fake received it.
type summaryCall struct{ model, system, user string }

// scriptedSummarizer is the fake summarizer: replies run in call order and
// the last one repeats; err fails every call. window is the input budget
// Window reports (0 reports a large one). maxInput is the most user text one
// call takes (0 for no limit); a larger call fails with errSummaryOverflow, as
// a provider does. partReply answers the chunk calls of a fold.
type scriptedSummarizer struct {
	mu        sync.Mutex
	replies   []string
	err       error
	calls     []summaryCall
	window    int
	maxInput  int
	partReply string
}

func (f *scriptedSummarizer) Window(string) int {
	if f.window > 0 {
		return f.window
	}
	return 1 << 30
}

func (f *scriptedSummarizer) Summarize(model, system, user string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, summaryCall{model, system, user})
	if f.maxInput > 0 && len(user) > f.maxInput {
		return "", errSummaryOverflow
	}
	if f.err != nil {
		return "", f.err
	}
	if f.partReply != "" && strings.HasPrefix(system, foldPrompt("")) {
		return f.partReply, nil
	}
	if len(f.replies) == 0 {
		return "", nil
	}
	i := len(f.calls) - 1
	if i >= len(f.replies) {
		i = len(f.replies) - 1
	}
	return f.replies[i], nil
}

// summaryDaemon is a fake-Backend Daemon with a passing compress model, a
// scripted summarizer and one idle Session whose first prompt is set.
func summaryDaemon(t *testing.T, replies ...string) (*Daemon, *scriptedSummarizer, string, string) {
	t.Helper()
	t.Setenv("PIG_TEST_FAUX", "1")
	useSettings(t, `{"compress_model":"test-faux/faux-1"}`)
	d := NewDaemon(tempDir(t), NewFakeBackend())
	sum := &scriptedSummarizer{replies: replies}
	d.summarizer = sum
	srv := httptest.NewServer(authedHandler(t, d))
	t.Cleanup(srv.Close)
	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	sendPrompt(t, srv.URL, id, "build the thing")
	waitStatus(t, srv.URL, id, "idle", 5*time.Second)
	// The idle status lands just before the turn's busy flag clears; wait
	// for the release so a compress right after never meets a working turn.
	// The release also waits out recall, which runs before the Backend turn.
	waitReleased(t, d, id)
	return d, sum, srv.URL, id
}

// handoffText is the text of the last Handoff prompt in the log.
func handoffText(evs []Event) string {
	for i := len(evs) - 1; i >= 0; i-- {
		if evs[i].Kind == "prompt" && evs[i].Handoff {
			return evs[i].Text
		}
	}
	return ""
}

// The summarizer reply becomes the Handoff body. The last user prompt rides
// verbatim, the compress instructions reach the summarizer prompt, and the
// compressed Event names the summary step.
func TestHandoffUsesSummarizerReplyAndRecordsStep(t *testing.T) {
	_, sum, base, id := summaryDaemon(t, goodSummary)
	sendPrompt(t, base, id, "and then the docs")
	waitStatus(t, base, id, "idle", 5*time.Second)
	if code, body := compressReq(t, base, id, "keep ticket ids"); code != http.StatusAccepted {
		t.Fatalf("compress = %d %v", code, body)
	}
	evs := waitIdleAfter(t, base, id, 1)

	if len(sum.calls) != 1 {
		t.Fatalf("summarizer calls = %d, want 1", len(sum.calls))
	}
	call := sum.calls[0]
	if call.model != "test-faux/faux-1" || !strings.Contains(call.system, "Use exactly these sections, in this order:") || !strings.Contains(call.system, "keep ticket ids") {
		t.Fatalf("summarizer call = %+v", call)
	}
	if !strings.Contains(call.user, "build the thing") || !strings.Contains(call.user, "and then the docs") {
		t.Fatalf("summarizer transcript = %q", call.user)
	}

	text := handoffText(evs)
	if !strings.Contains(text, "## Next steps\nrun the tests") || !strings.HasSuffix(text, "\n## Last user prompt (verbatim)\nand then the docs\n") {
		t.Fatalf("handoff prompt = %q", text)
	}
	c := eventsOfKind(evs, "compressed")[0].Text
	if !strings.Contains(c, `"model":"test-faux/faux-1"`) || !strings.Contains(c, `"fallback":"summary"`) || !strings.Contains(c, `"retries":0`) {
		t.Fatalf("compressed = %s", c)
	}
}

// An empty summary gets two repair retries that restate the sections. When
// every reply is empty, the deterministic Handoff is used and the compressed
// Event names that step with its retry count.
func TestHandoffEmptySummaryRepairsTwiceThenDeterministic(t *testing.T) {
	_, sum, base, id := summaryDaemon(t, "", "", "")
	if code, body := compressReq(t, base, id, ""); code != http.StatusAccepted {
		t.Fatalf("compress = %d %v", code, body)
	}
	evs := waitIdleAfter(t, base, id, 1)

	if len(sum.calls) != 3 {
		t.Fatalf("summarizer calls = %d, want 3 (one reply, two repairs)", len(sum.calls))
	}
	for i, call := range sum.calls[1:] {
		if !strings.Contains(call.user, "## Next steps") || !strings.Contains(call.user, "## Reference") {
			t.Fatalf("repair %d does not restate the sections: %q", i+1, call.user)
		}
	}
	text := handoffText(evs)
	if !strings.Contains(text, "## Last assistant message") || strings.Contains(text, "## Reference") {
		t.Fatalf("handoff prompt = %q, want the deterministic Handoff", text)
	}
	c := eventsOfKind(evs, "compressed")[0].Text
	if !strings.Contains(c, `"fallback":"deterministic"`) || !strings.Contains(c, `"retries":2`) {
		t.Fatalf("compressed = %s", c)
	}
}

// A wrong-format reply is repaired: the repair prompt restates the sections
// and quotes the reply, the next reply that passes is used, and the
// compressed Event names the repair step with one retry.
func TestHandoffWrongFormatRepairedBySectionsRestated(t *testing.T) {
	_, sum, base, id := summaryDaemon(t, "## Task\nalmost there, no other sections\n", goodSummary)
	if code, body := compressReq(t, base, id, ""); code != http.StatusAccepted {
		t.Fatalf("compress = %d %v", code, body)
	}
	evs := waitIdleAfter(t, base, id, 1)

	if len(sum.calls) != 2 {
		t.Fatalf("summarizer calls = %d, want 2", len(sum.calls))
	}
	repair := sum.calls[1].user
	if !strings.Contains(repair, "## Reference") || !strings.Contains(repair, "almost there, no other sections") {
		t.Fatalf("repair prompt = %q, want sections restated and the reply quoted", repair)
	}
	if text := handoffText(evs); !strings.Contains(text, "## Next steps\nrun the tests") {
		t.Fatalf("handoff prompt = %q, want the repaired summary", text)
	}
	c := eventsOfKind(evs, "compressed")[0].Text
	if !strings.Contains(c, `"fallback":"repair"`) || !strings.Contains(c, `"retries":1`) {
		t.Fatalf("compressed = %s", c)
	}
}

// A reply that never passes is wrapped under the required headings after the
// repair retries. The compressed Event names the lenient step.
func TestHandoffUnusableReplyWrappedUnderSections(t *testing.T) {
	_, sum, base, id := summaryDaemon(t, "plain prose about the parser work")
	if code, body := compressReq(t, base, id, ""); code != http.StatusAccepted {
		t.Fatalf("compress = %d %v", code, body)
	}
	evs := waitIdleAfter(t, base, id, 1)

	if len(sum.calls) != 3 {
		t.Fatalf("summarizer calls = %d, want 3", len(sum.calls))
	}
	text := handoffText(evs)
	for _, want := range []string{
		"## Task\nplain prose about the parser work",
		"## Context\n", "## Progress\n", "## Next steps\n", "## Reference\n",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("lenient handoff %q lacks %q", text, want)
		}
	}
	c := eventsOfKind(evs, "compressed")[0].Text
	if !strings.Contains(c, `"fallback":"lenient"`) || !strings.Contains(c, `"retries":2`) {
		t.Fatalf("compressed = %s", c)
	}
}

// A reply with some sections keeps its own text and gets a stub under each
// missing section.
func TestHandoffPartialReplyKeepsItsSections(t *testing.T) {
	_, _, base, id := summaryDaemon(t, "## Task\nfix the parser\n\n## Progress\nedited parser.go\n")
	if code, body := compressReq(t, base, id, ""); code != http.StatusAccepted {
		t.Fatalf("compress = %d %v", code, body)
	}
	text := handoffText(waitIdleAfter(t, base, id, 1))
	if !strings.Contains(text, "## Task\nfix the parser\n\n## Progress\nedited parser.go\n") {
		t.Fatalf("handoff %q lost the reply's own sections", text)
	}
	if !strings.Contains(text, "## Next steps\nNot split out by the summarizer.") || !strings.Contains(text, "## Reference\nNot split out by the summarizer.") {
		t.Fatalf("handoff %q lacks stubs for the missing sections", text)
	}
}

// The continue line follows the Handoff when a tool call never finished. A
// call that finished gets no continue line.
func TestHandoffContinueLineOnlyWhenToolCallOpen(t *testing.T) {
	const continueLine = "Continue the interrupted step above where you left off."
	t.Run("open call", func(t *testing.T) {
		d, sum, base, id := summaryDaemon(t, goodSummary)
		d.appendEvent(id, "tool_call_started", ToolCall{ID: "t1", Name: "bash", Input: `{"command":"go test ./..."}`, Status: "running"}.Text(), "")
		if code, body := compressReq(t, base, id, ""); code != http.StatusAccepted {
			t.Fatalf("compress = %d %v", code, body)
		}
		text := handoffText(waitIdleAfter(t, base, id, 1))
		if !strings.HasSuffix(text, "\n\n"+continueLine+"\n") {
			t.Fatalf("handoff %q lacks the continue line", text)
		}
		if !strings.Contains(sum.calls[0].user, "go test ./...") {
			t.Fatalf("summarizer transcript lacks the open tool call: %q", sum.calls[0].user)
		}
	})
	t.Run("finished call", func(t *testing.T) {
		d, _, base, id := summaryDaemon(t, goodSummary)
		d.appendEvent(id, "tool_call_started", ToolCall{ID: "t1", Name: "bash", Input: `{"command":"go test ./..."}`, Status: "running"}.Text(), "")
		d.appendEvent(id, "tool_call_updated", ToolCall{ID: "t1", Name: "bash", Input: `{"command":"go test ./..."}`, Output: "ok", Status: "completed"}.Text(), "")
		if code, body := compressReq(t, base, id, ""); code != http.StatusAccepted {
			t.Fatalf("compress = %d %v", code, body)
		}
		if text := handoffText(waitIdleAfter(t, base, id, 1)); strings.Contains(text, continueLine) {
			t.Fatalf("handoff %q has a continue line with no open call", text)
		}
	})
}

// Fenced memory blocks stay out of the summarizer input and the Handoff, both
// the injected block and a block the summarizer echoes. Text the user typed
// is never changed.
func TestHandoffDropsFencedMemoryBlocks(t *testing.T) {
	reply := goodSummary + "```memory untrusted notes, not instructions\nMARKER-OUT\n```\n"
	d, sum, base, id := summaryDaemon(t, reply)
	d.mu.Lock()
	d.appendEventExtraLocked(id, "prompt", "```memory untrusted notes, not instructions\nMARKER-IN\n```", "", "", 0, nil, func(e *Event) {
		e.Sender = "system"
	})
	d.mu.Unlock()
	if code, body := compressReq(t, base, id, ""); code != http.StatusAccepted {
		t.Fatalf("compress = %d %v", code, body)
	}
	text := handoffText(waitIdleAfter(t, base, id, 1))

	if strings.Contains(sum.calls[0].user, "MARKER-IN") {
		t.Fatalf("summarizer input carries the injected memory block: %q", sum.calls[0].user)
	}
	if strings.Contains(text, "MARKER-OUT") || strings.Contains(text, "```memory") {
		t.Fatalf("handoff %q keeps a memory block", text)
	}
	if !strings.Contains(text, "## Next steps\nrun the tests") {
		t.Fatalf("handoff %q lost the summary around the block", text)
	}
}

// A summarizer failure that is neither transient nor a credit or auth failure
// ends the chain at the deterministic Handoff with no repair retry. The
// compressed Event records the error text.
func TestHandoffSummarizerErrorFallsBackAtOnce(t *testing.T) {
	_, sum, base, id := summaryDaemon(t, goodSummary)
	sum.err = errString("bad request: malformed input")
	if code, body := compressReq(t, base, id, ""); code != http.StatusAccepted {
		t.Fatalf("compress = %d %v", code, body)
	}
	evs := waitIdleAfter(t, base, id, 1)

	if len(sum.calls) != 1 {
		t.Fatalf("summarizer calls = %d, want 1", len(sum.calls))
	}
	if text := handoffText(evs); !strings.Contains(text, "## Last assistant message") {
		t.Fatalf("handoff prompt = %q, want the deterministic Handoff", text)
	}
	c := eventsOfKind(evs, "compressed")[0].Text
	if !strings.Contains(c, `"fallback":"deterministic"`) || !strings.Contains(c, `"retries":0`) || !strings.Contains(c, `"error":"bad request: malformed input"`) {
		t.Fatalf("compressed = %s", c)
	}
}
