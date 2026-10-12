package web

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A voice recording uploads through the images route, keeps its duration and
// peaks, serves back, and never reaches the model as a file path or pixels.
func TestAudioUploadKeepsPeaksAndStaysOutOfModelInput(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	data := base64.StdEncoding.EncodeToString([]byte("webm-bytes"))
	code, out := uploadImage(t, srv.URL, id, map[string]any{
		"name": "voice.webm", "mediaType": "audio/webm;codecs=opus", "data": data,
		"durationMs": 4200, "peaks": []int{10, 80, 40},
	})
	if code != http.StatusCreated {
		t.Fatalf("upload status = %d (%v)", code, out)
	}
	raw, _ := json.Marshal(out["image"])
	var meta ImageMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatal(err)
	}
	if !isAudioID(meta.ID) || meta.DurationMs != 4200 || len(meta.Peaks) != 3 {
		t.Fatalf("audio meta = %+v", meta)
	}

	resp, err := http.Get(srv.URL + meta.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "audio/webm") {
		t.Fatalf("serve = %d %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}

	if got := d.imageRefsText(id, "hello", []string{meta.ID}); got != "hello" {
		t.Fatalf("model text = %q, want the transcript only", got)
	}
	if imgs := d.withStoredImages(id, nil, []string{meta.ID}); len(imgs) != 0 {
		t.Fatalf("audio leaked into inline images: %d", len(imgs))
	}
}

func TestUploadRejectsOtherMediaTypes(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	code, _ := uploadImage(t, srv.URL, id, map[string]any{
		"name": "a.txt", "mediaType": "text/plain", "data": base64.StdEncoding.EncodeToString([]byte("x")),
	})
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", code)
	}
}
