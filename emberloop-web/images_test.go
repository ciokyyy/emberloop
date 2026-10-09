package web

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

// imageFake records the images its turns receive.
type imageFake struct {
	FakeBackend
	mu   sync.Mutex
	seen [][]Image
}

func (f *imageFake) OnPromptImages(id, text string, images []Image, appendEvent func(kind, text, status string)) {
	f.mu.Lock()
	f.seen = append(f.seen, images)
	f.mu.Unlock()
	f.FakeBackend.OnPrompt(id, text, appendEvent)
}

var tinyPNG = base64.StdEncoding.EncodeToString([]byte("\x89PNG-tiny"))

func postImgJSON(t *testing.T, url string, body any) *http.Response {
	t.Helper()
	raw, _ := json.Marshal(body)
	res, err := http.Post(url, "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestPromptWithImagesReachesBackendAndCountsInEvent(t *testing.T) {
	be := &imageFake{}
	d := NewDaemon(t.TempDir(), be)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()
	id := createSession(t, srv.URL, "fake", "fake-model", t.TempDir())

	res := postImgJSON(t, srv.URL+"/api/sessions/"+id+"/prompt", map[string]any{
		"text":   "what is this",
		"images": []Image{{Mime: "image/png", Data: tinyPNG}},
	})
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("status %d", res.StatusCode)
	}
	events := readSSE(t, srv.URL, id, 0, 3, 5*time.Second)
	found := false
	for _, e := range events {
		if e.Kind == "prompt" && e.Images == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("no prompt event with 1 image in %+v", events)
	}
	be.mu.Lock()
	defer be.mu.Unlock()
	if len(be.seen) != 1 || len(be.seen[0]) != 1 || be.seen[0][0].Data != tinyPNG {
		t.Fatalf("backend saw %+v", be.seen)
	}
}

func TestPromptImagesOnlyIsAccepted(t *testing.T) {
	d := NewDaemon(t.TempDir(), &imageFake{})
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()
	id := createSession(t, srv.URL, "fake", "fake-model", t.TempDir())
	res := postImgJSON(t, srv.URL+"/api/sessions/"+id+"/prompt", map[string]any{
		"images": []Image{{Mime: "image/png", Data: tinyPNG}},
	})
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("status %d", res.StatusCode)
	}
}

func TestPromptRejectsBadImages(t *testing.T) {
	d := NewDaemon(t.TempDir(), &imageFake{})
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()
	id := createSession(t, srv.URL, "fake", "fake-model", t.TempDir())
	for name, img := range map[string]Image{
		"mime":   {Mime: "text/html", Data: tinyPNG},
		"base64": {Mime: "image/png", Data: "!!!"},
		"empty":  {Mime: "image/png", Data: ""},
	} {
		res := postImgJSON(t, srv.URL+"/api/sessions/"+id+"/prompt", map[string]any{"text": "x", "images": []Image{img}})
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s: status %d, want 400", name, res.StatusCode)
		}
	}
}

func TestBackendWithoutImageSupportFailsTheTurnLoudly(t *testing.T) {
	var kinds []string
	promptBackend(NewFakeBackend(), "s", "hi", []Image{{Mime: "image/png", Data: tinyPNG}}, func(kind, text, status string) {
		kinds = append(kinds, kind)
	})
	if len(kinds) != 2 || kinds[0] != "error" {
		t.Fatalf("events = %v", kinds)
	}
}

func TestMergePendingKeepsImages(t *testing.T) {
	a := Image{Mime: "image/png", Data: "a"}
	b := Image{Mime: "image/png", Data: "b"}
	got := mergePending([]queuedMsg{{text: "one", images: []Image{a}}, {text: "", images: []Image{b}}})
	if got.text != "one" || len(got.images) != 2 {
		t.Fatalf("merged = %+v", got)
	}
}

func TestUserBlocksOrdersTextThenImages(t *testing.T) {
	blocks := userBlocks("see", []Image{{Mime: "IMAGE/PNG", Data: "zz"}})
	if len(blocks) != 2 {
		t.Fatalf("blocks = %d", len(blocks))
	}
	if _, ok := blocks[0].(ai.TextContent); !ok {
		t.Fatalf("first block %T", blocks[0])
	}
	if img, ok := blocks[1].(ai.ImageContent); !ok || img.MimeType != "image/png" {
		t.Fatalf("second block %#v", blocks[1])
	}
	if got := userBlocks("", []Image{{Mime: "image/png", Data: "z"}}); len(got) != 1 {
		t.Fatalf("image-only blocks = %d", len(got))
	}
}
