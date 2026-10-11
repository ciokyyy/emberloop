package web

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func uploadImage(t *testing.T, base, session string, body any) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	resp, err := http.Post(base+"/api/sessions/"+session+"/images", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func listImages(t *testing.T, base, session string) (int, []ImageMeta) {
	t.Helper()
	resp, err := http.Get(base + "/api/sessions/" + session + "/images")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Images []ImageMeta `json:"images"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out.Images
}

// Upload, list and serve round-trip one image with its metadata.
func TestImageUploadListServe(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	data := base64.StdEncoding.EncodeToString([]byte("png-bytes"))
	code, out := uploadImage(t, srv.URL, id, map[string]any{
		"name": "a.png", "mediaType": "image/png", "data": data,
	})
	if code != http.StatusCreated {
		t.Fatalf("upload status = %d (%v)", code, out)
	}
	raw, _ := json.Marshal(out["image"])
	var meta ImageMeta
	if err := json.Unmarshal(raw, &meta); err != nil || meta.ID == "" {
		t.Fatalf("upload image = %v", out)
	}
	if meta.MediaType != "image/png" || meta.Size != int64(len("png-bytes")) {
		t.Fatalf("upload meta = %+v", meta)
	}

	if code, metas := listImages(t, srv.URL, id); code != http.StatusOK || len(metas) != 1 || metas[0].ID != meta.ID {
		t.Fatalf("list = %d %+v", code, metas)
	}

	resp, err := http.Get(srv.URL + meta.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	if resp.StatusCode != http.StatusOK || buf.String() != "png-bytes" {
		t.Fatalf("serve = %d %q", resp.StatusCode, buf.String())
	}
	if ct := resp.Header.Get("Content-Type"); ct != "image/png" {
		t.Fatalf("serve content-type = %q", ct)
	}
}

// Uploads refuse non-images, bad data and oversized payloads.
func TestImageUploadRefusals(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	data := base64.StdEncoding.EncodeToString([]byte("x"))
	if code, _ := uploadImage(t, srv.URL, id, map[string]any{
		"name": "a.txt", "mediaType": "text/plain", "data": data,
	}); code != http.StatusBadRequest {
		t.Fatalf("text upload status = %d, want 400", code)
	}
	if code, _ := uploadImage(t, srv.URL, id, map[string]any{
		"name": "a.png", "mediaType": "image/png", "data": "!!!not-base64!!!",
	}); code != http.StatusBadRequest {
		t.Fatalf("bad data status = %d, want 400", code)
	}
	big := base64.StdEncoding.EncodeToString(make([]byte, maxStoredImageBytes+1))
	if code, _ := uploadImage(t, srv.URL, id, map[string]any{
		"name": "big.png", "mediaType": "image/png", "data": big,
	}); code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize status = %d, want 413", code)
	}
	if code, _ := uploadImage(t, srv.URL, "missing", map[string]any{
		"name": "a.png", "mediaType": "image/png", "data": data,
	}); code != http.StatusNotFound {
		t.Fatalf("missing session status = %d, want 404", code)
	}
}

// A prompt carrying stored image ids stores them on the Event and the
// agent receives their file paths (the fake Backend echoes its prompt, so
// the reply carries the resolved path).
func TestPromptWithImageIDsReachesAgentAsPaths(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	data := base64.StdEncoding.EncodeToString([]byte("png-bytes"))
	_, out := uploadImage(t, srv.URL, id, map[string]any{
		"name": "a.png", "mediaType": "image/png", "data": data,
	})
	raw, _ := json.Marshal(out["image"])
	var meta ImageMeta
	_ = json.Unmarshal(raw, &meta)

	body, _ := json.Marshal(map[string]any{"text": "look at this", "imageIds": []string{meta.ID}})
	resp, err := http.Post(srv.URL+"/api/sessions/"+id+"/prompt", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("prompt status = %d", resp.StatusCode)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		_, events := sessionMeta(t, srv.URL, id)
		var prompt *Event
		var reply string
		for i, e := range events {
			if e.Kind == "prompt" && strings.Contains(e.Text, "look at this") {
				prompt = &events[i]
			}
			if e.Kind == "assistant_chunk" {
				reply += e.Text
			}
		}
		if prompt != nil && len(prompt.ImageIDs) == 1 && prompt.ImageIDs[0] == meta.ID && strings.Contains(reply, ".png") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("prompt imageIds = %+v reply = %q", prompt, reply)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A prompt naming an unattached image is refused.
func TestPromptUnknownImageIDRefused(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	body, _ := json.Marshal(map[string]any{"text": "hi", "imageIds": []string{"nope"}})
	resp, err := http.Post(srv.URL+"/api/sessions/"+id+"/prompt", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("prompt status = %d, want 400", resp.StatusCode)
	}
}

// Deleting the Session deletes its attached images with it.
func TestDeleteSessionDeletesImages(t *testing.T) {
	stateDir := tempDir(t)
	d := NewDaemon(stateDir, NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	data := base64.StdEncoding.EncodeToString([]byte("png-bytes"))
	if code, _ := uploadImage(t, srv.URL, id, map[string]any{
		"name": "a.png", "mediaType": "image/png", "data": data,
	}); code != http.StatusCreated {
		t.Fatalf("upload status = %d", code)
	}
	dir := filepath.Join(stateDir, "enderbloom", "sessions", id, "images")
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("images dir missing: %v", err)
	}
	req, _ := http.NewRequest("DELETE", srv.URL+"/api/sessions/"+id, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("images dir survives session delete: %v", err)
	}
}

// Queued stored references merge with the text and stay on the Event.
func TestMergePendingKeepsImageIDs(t *testing.T) {
	got := mergePending([]queuedMsg{{text: "one", imageIDs: []string{"a"}}, {text: "", imageIDs: []string{"b"}}})
	if got.text != "one" || len(got.imageIDs) != 2 {
		t.Fatalf("merged = %+v", got)
	}
}

// A prompt carrying stored image ids reaches the Backend as inline vision
// bytes too, while the prompt Event keeps the clean text plus the ids.
func TestPromptWithImageIDsReachesBackendAsInlineBytes(t *testing.T) {
	be := &imageFake{}
	d := NewDaemon(tempDir(t), be)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	data := base64.StdEncoding.EncodeToString([]byte("png-bytes"))
	_, out := uploadImage(t, srv.URL, id, map[string]any{
		"name": "a.png", "mediaType": "image/png", "data": data,
	})
	raw, _ := json.Marshal(out["image"])
	var meta ImageMeta
	_ = json.Unmarshal(raw, &meta)

	body, _ := json.Marshal(map[string]any{"text": "look at this", "imageIds": []string{meta.ID}})
	resp, err := http.Post(srv.URL+"/api/sessions/"+id+"/prompt", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("prompt status = %d", resp.StatusCode)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		_, events := sessionMeta(t, srv.URL, id)
		var prompt *Event
		var reply string
		for i, e := range events {
			if e.Kind == "prompt" && strings.Contains(e.Text, "look at this") {
				prompt = &events[i]
			}
			if e.Kind == "assistant_chunk" {
				reply += e.Text
			}
		}
		be.mu.Lock()
		seen := append([][]Image(nil), be.seen...)
		be.mu.Unlock()
		if prompt != nil && len(prompt.ImageIDs) == 1 && prompt.ImageIDs[0] == meta.ID &&
			prompt.Text == "look at this" && strings.Contains(reply, ".png") &&
			len(seen) == 1 && len(seen[0]) == 1 {
			if seen[0][0].Mime != "image/png" || seen[0][0].Data != data {
				t.Fatalf("backend image = %+v", seen[0][0])
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("prompt = %+v reply = %q backend = %+v", prompt, reply, seen)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Stored references queued behind a working turn still reach the Backend as
// inline bytes when the Daemon delivers them after the turn.
func TestQueuedImageIDsDeliverInlineBytesAfterTurn(t *testing.T) {
	be := &imageFake{}
	be.Delay = 500 * time.Millisecond
	d := NewDaemon(tempDir(t), be)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	data := base64.StdEncoding.EncodeToString([]byte("png-bytes"))
	_, out := uploadImage(t, srv.URL, id, map[string]any{
		"name": "a.png", "mediaType": "image/png", "data": data,
	})
	raw, _ := json.Marshal(out["image"])
	var meta ImageMeta
	_ = json.Unmarshal(raw, &meta)

	sendPrompt(t, srv.URL, id, "first")
	body, _ := json.Marshal(map[string]any{"text": "second", "imageIds": []string{meta.ID}})
	resp, err := http.Post(srv.URL+"/api/sessions/"+id+"/prompt", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("queued prompt status = %d", resp.StatusCode)
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		_, events := sessionMeta(t, srv.URL, id)
		prompts := 0
		for _, e := range events {
			if e.Kind == "prompt" {
				prompts++
			}
		}
		be.mu.Lock()
		seen := append([][]Image(nil), be.seen...)
		be.mu.Unlock()
		if prompts == 2 && len(seen) == 1 {
			// The imageless first turn runs through OnPrompt, so only the
			// delivered image turn is recorded here.
			if len(seen[0]) != 1 || seen[0][0].Data != data {
				t.Fatalf("second turn images = %+v", seen)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("prompts = %d backend = %+v", prompts, seen)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Unknown stored ids resolve to nothing: the text-only path stays intact.
func TestStoredImagesAsInlineSkipsUnknown(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	data := base64.StdEncoding.EncodeToString([]byte("png-bytes"))
	_, out := uploadImage(t, srv.URL, id, map[string]any{
		"name": "a.png", "mediaType": "image/png", "data": data,
	})
	raw, _ := json.Marshal(out["image"])
	var meta ImageMeta
	_ = json.Unmarshal(raw, &meta)

	got := d.storedImagesAsInline(id, []string{"nope", meta.ID})
	if len(got) != 1 || got[0].Mime != "image/png" || got[0].Data != data {
		t.Fatalf("resolved = %+v", got)
	}
	if len(d.withStoredImages(id, nil, nil)) != 0 {
		t.Fatalf("empty ids should resolve to no images")
	}
}
