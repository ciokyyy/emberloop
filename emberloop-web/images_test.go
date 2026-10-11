package web

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

// imageFake records the images and text its turns receive.
type imageFake struct {
	FakeBackend
	mu    sync.Mutex
	seen  [][]Image
	texts []string
}

func (f *imageFake) OnPromptImages(id, text string, images []Image, appendEvent func(kind, text, status string)) {
	f.mu.Lock()
	f.seen = append(f.seen, images)
	f.texts = append(f.texts, text)
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
	d := NewDaemon(tempDir(t), be)
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))

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
		if e.Kind == "prompt" && len(e.ImageIDs) == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("no prompt event with 1 stored image in %+v", events)
	}
	be.mu.Lock()
	defer be.mu.Unlock()
	// Inline attachments are stored under stable ids, and the agent gets
	// both their file paths in the text and their bytes inline.
	if len(be.seen) != 1 || len(be.seen[0]) != 1 {
		t.Fatalf("backend saw %+v", be.seen)
	}
	if be.seen[0][0].Mime != "image/png" || be.seen[0][0].Data != tinyPNG {
		t.Fatalf("backend image = %+v", be.seen[0][0])
	}
	if len(be.texts) != 1 || !strings.Contains(be.texts[0], "what is this") || !strings.Contains(be.texts[0], ".png") {
		t.Fatalf("backend text = %q", be.texts)
	}
}

func TestPromptImagesOnlyIsAccepted(t *testing.T) {
	d := NewDaemon(tempDir(t), &imageFake{})
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	res := postImgJSON(t, srv.URL+"/api/sessions/"+id+"/prompt", map[string]any{
		"images": []Image{{Mime: "image/png", Data: tinyPNG}},
	})
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("status %d", res.StatusCode)
	}
}

func TestPromptRejectsBadImages(t *testing.T) {
	d := NewDaemon(tempDir(t), &imageFake{})
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
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

// imageLessBackend is a Backend that cannot take images: a named field, not
// an embedding, so the fake's OnPromptImages is not promoted.
type imageLessBackend struct{ fake *FakeBackend }

func (b *imageLessBackend) OnPrompt(sessionID, text string, appendEvent func(kind, text, status string)) {
	b.fake.OnPrompt(sessionID, text, appendEvent)
}
func (b *imageLessBackend) CreateSession(meta SessionMeta, emit func(kind, text, status string)) error {
	return b.fake.CreateSession(meta, emit)
}
func (b *imageLessBackend) CloseSession(sessionID string) { b.fake.CloseSession(sessionID) }
func (b *imageLessBackend) Models() []string              { return b.fake.Models() }
func (b *imageLessBackend) ReleaseSession(sessionID string) string {
	return b.fake.ReleaseSession(sessionID)
}
func (b *imageLessBackend) ResumeSession(meta SessionMeta, emit func(kind, text, status string)) error {
	return b.fake.ResumeSession(meta, emit)
}
func (b *imageLessBackend) ResumeToken(sessionID string) string {
	return b.fake.ResumeToken(sessionID)
}
func (b *imageLessBackend) ResumeOK(meta SessionMeta) (bool, string) {
	return b.fake.ResumeOK(meta)
}

func TestBackendWithoutImageSupportFailsTheTurnLoudly(t *testing.T) {
	var kinds []string
	promptBackend(&imageLessBackend{fake: NewFakeBackend()}, "s", "hi", []Image{{Mime: "image/png", Data: tinyPNG}}, func(kind, text, status string) {
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
