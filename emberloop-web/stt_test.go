package web

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSTTProxiesAudioToOpenRouter(t *testing.T) {
	t.Parallel()
	var gotAuth string
	var got struct {
		Model      string `json:"model"`
		InputAudio struct {
			Data   string `json:"data"`
			Format string `json:"format"`
		} `json:"input_audio"`
	}
	or := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"text":" hello world "}`))
	}))
	defer or.Close()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.sttKeyFn = func(context.Context) string { return "sk-test" }
	d.sttURL = or.URL
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	res, err := http.Post(srv.URL+"/api/stt", "audio/webm;codecs=opus", bytes.NewReader([]byte("AUDIO")))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out struct{ Text string }
	_ = json.NewDecoder(res.Body).Decode(&out)
	if res.StatusCode != 200 || out.Text != "hello world" {
		t.Fatalf("status %d text %q", res.StatusCode, out.Text)
	}
	if gotAuth != "Bearer sk-test" || got.Model != "openai/gpt-transcribe" || got.InputAudio.Format != "webm" {
		t.Fatalf("upstream saw auth=%q %+v", gotAuth, got)
	}
	if raw, _ := base64.StdEncoding.DecodeString(got.InputAudio.Data); string(raw) != "AUDIO" {
		t.Fatalf("audio not forwarded: %q", raw)
	}
}

func TestSTTWithoutKeyIs503AndGetReportsDisabled(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.sttKeyFn = func(context.Context) string { return "" }
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	res, _ := http.Post(srv.URL+"/api/stt", "audio/webm", strings.NewReader("x"))
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status %d, want 503", res.StatusCode)
	}
	res, _ = http.Get(srv.URL + "/api/stt")
	var out struct{ Enabled bool }
	_ = json.NewDecoder(res.Body).Decode(&out)
	if out.Enabled {
		t.Fatal("enabled without a key")
	}
}

func TestSTTRejectsUnknownAudioType(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.sttKeyFn = func(context.Context) string { return "sk-test" }
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	res, _ := http.Post(srv.URL+"/api/stt", "text/plain", strings.NewReader("x"))
	if res.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("status %d, want 415", res.StatusCode)
	}
}

func TestSTTUpstreamErrorIs502WithMessage(t *testing.T) {
	t.Parallel()
	or := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(402)
		_, _ = w.Write([]byte(`{"error":{"message":"no credits"}}`))
	}))
	defer or.Close()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.sttKeyFn = func(context.Context) string { return "sk-test" }
	d.sttURL = or.URL
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	res, _ := http.Post(srv.URL+"/api/stt", "audio/wav", strings.NewReader("x"))
	var out struct{ Error string }
	_ = json.NewDecoder(res.Body).Decode(&out)
	if res.StatusCode != http.StatusBadGateway || out.Error != "no credits" {
		t.Fatalf("status %d error %q", res.StatusCode, out.Error)
	}
}

func TestSTTModelEnvOverride(t *testing.T) {
	t.Setenv("EL_STT_MODEL", "openai/whisper-large-v3")
	t.Setenv("EL_STT_FORMAT_MODEL", "google/gemma-3-4b-it")
	if got := sttModelName(); got != "openai/whisper-large-v3" {
		t.Fatalf("model = %q", got)
	}
	if got := sttFormatModelName(); got != "google/gemma-3-4b-it" {
		t.Fatalf("format model = %q", got)
	}
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.sttKeyFn = func(context.Context) string { return "sk-test" }
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	res, _ := http.Get(srv.URL + "/api/stt")
	var out struct {
		Model       string `json:"model"`
		FormatModel string `json:"formatModel"`
	}
	_ = json.NewDecoder(res.Body).Decode(&out)
	res.Body.Close()
	if out.Model != "openai/whisper-large-v3" || out.FormatModel != "google/gemma-3-4b-it" {
		t.Fatalf("GET reports %+v", out)
	}
}

func TestSTTFormatPassCleansPerSegment(t *testing.T) {
	t.Parallel()
	or := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"text":" um hello world "}`))
	}))
	defer or.Close()
	var chatModel string
	var chatMaxTokens int
	var chatMsgs []map[string]string
	chat := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model     string              `json:"model"`
			MaxTokens int                 `json:"max_tokens"`
			Messages  []map[string]string `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		chatModel = body.Model
		chatMaxTokens = body.MaxTokens
		chatMsgs = body.Messages
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Hello, world."}}]}`))
	}))
	defer chat.Close()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.sttKeyFn = func(context.Context) string { return "sk-test" }
	d.sttURL = or.URL
	d.sttChatURL = chat.URL
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	res, err := http.Post(srv.URL+"/api/stt?format=1", "audio/webm", strings.NewReader("AUDIO"))
	if err != nil {
		t.Fatal(err)
	}
	var out struct{ Text string }
	_ = json.NewDecoder(res.Body).Decode(&out)
	res.Body.Close()
	if res.StatusCode != 200 || out.Text != "Hello, world." {
		t.Fatalf("status %d text %q", res.StatusCode, out.Text)
	}
	if chatModel != "mistralai/mistral-small-24b-instruct-2501" {
		t.Fatalf("format model = %q", chatModel)
	}
	// Short live segments need only a small cap, keeping the opt-in pass fast.
	if chatMaxTokens != 500 {
		t.Fatalf("format max_tokens = %d, want 500", chatMaxTokens)
	}
	if len(chatMsgs) != 2 || !strings.Contains(chatMsgs[1]["content"], "um hello world") {
		t.Fatalf("transcript not sent as data: %+v", chatMsgs)
	}
	if !strings.Contains(strings.ToLower(chatMsgs[0]["content"]), "untrusted") {
		t.Fatalf("system prompt does not pin the transcript as data: %q", chatMsgs[0]["content"])
	}
}

func TestSTTWithoutFormatSkipsChat(t *testing.T) {
	t.Parallel()
	or := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"text":" um hello "}`))
	}))
	defer or.Close()
	chatCalls := 0
	chat := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chatCalls++
	}))
	defer chat.Close()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.sttKeyFn = func(context.Context) string { return "sk-test" }
	d.sttURL = or.URL
	d.sttChatURL = chat.URL
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	for _, path := range []string{"/api/stt", "/api/stt?format=0"} {
		res, _ := http.Post(srv.URL+path, "audio/webm", strings.NewReader("AUDIO"))
		var out struct{ Text string }
		_ = json.NewDecoder(res.Body).Decode(&out)
		res.Body.Close()
		if res.StatusCode != 200 || out.Text != "um hello" {
			t.Fatalf("%s: status %d text %q", path, res.StatusCode, out.Text)
		}
	}
	if chatCalls != 0 {
		t.Fatalf("chat called %d times without ?format=1", chatCalls)
	}
}

func TestSTTFormatFailsOpenOnChatError(t *testing.T) {
	t.Parallel()
	or := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"text":" raw text "}`))
	}))
	defer or.Close()
	chat := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`{"error":{"message":"overloaded"}}`))
	}))
	defer chat.Close()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.sttKeyFn = func(context.Context) string { return "sk-test" }
	d.sttURL = or.URL
	d.sttChatURL = chat.URL
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	res, _ := http.Post(srv.URL+"/api/stt?format=1", "audio/webm", strings.NewReader("AUDIO"))
	var out struct{ Text string }
	_ = json.NewDecoder(res.Body).Decode(&out)
	res.Body.Close()
	if res.StatusCode != 200 || out.Text != "raw text" {
		t.Fatalf("status %d text %q, want fail-open raw text", res.StatusCode, out.Text)
	}
}
