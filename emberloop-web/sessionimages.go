package web

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Session image attachments. Bytes live under the Session directory
// (<state>/enderbloom/sessions/<id>/images/), so ending the Session deletes
// its images with it. The transcript stores only stable image ids on prompt
// Events; the UI renders chips and thumbnails from them and the agent
// receives the resolved file paths at send time. Inline base64 attachments
// (images.go) keep their own wire flow; the two ride one message together.

// maxStoredImageBytes caps one attached image at 25 MB.
const maxStoredImageBytes = 25 << 20

// maxStoredUploadBytes caps one images POST body: base64 inflates 4/3, so a
// 25 MB image needs room plus JSON framing.
const maxStoredUploadBytes = 40 << 20

// ImageMeta describes one attached image or voice recording. URL is the
// Daemon route that serves its bytes. DurationMs and Peaks (0-100 bars)
// only exist on recordings.
type ImageMeta struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	MediaType  string    `json:"mediaType"`
	Size       int64     `json:"size"`
	URL        string    `json:"url"`
	CreatedAt  time.Time `json:"createdAt"`
	DurationMs int64     `json:"durationMs,omitempty"`
	Peaks      []int     `json:"peaks,omitempty"`
}

// audioIDPrefix marks a stored voice recording id.
const audioIDPrefix = "aud_"

func isAudioID(id string) bool { return strings.HasPrefix(id, audioIDPrefix) }

func (d *Daemon) sessionImagesDir(id string) string {
	return filepath.Join(d.sessionDir(id), "images")
}

func (d *Daemon) imagesIndexFile(id string) string {
	return filepath.Join(d.sessionImagesDir(id), "index.json")
}

func (d *Daemon) imageURL(sessionID, imageID string) string {
	return "/api/sessions/" + sessionID + "/images/" + imageID
}

// readImagesIndex lists the Session's attached images, oldest first.
// Unknown Sessions and missing indexes list empty, never an error.
func (d *Daemon) readImagesIndex(id string) []ImageMeta {
	raw, err := os.ReadFile(d.imagesIndexFile(id))
	if err != nil {
		return nil
	}
	var out []ImageMeta
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	for i := range out {
		out[i].URL = d.imageURL(id, out[i].ID)
	}
	return out
}

func (d *Daemon) writeImagesIndex(id string, metas []ImageMeta) {
	dir := d.sessionImagesDir(id)
	_ = os.MkdirAll(dir, 0o755)
	b, _ := json.Marshal(metas)
	tmp, err := os.CreateTemp(dir, "index-*.json")
	if err != nil {
		return
	}
	name := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return
	}
	_ = os.Rename(name, d.imagesIndexFile(id))
}

// imageFileName stores one upload under a stable id prefix, so the
// on-disk name never collides and never escapes the images directory.
func imageFileName(id, name, mediaType string) string {
	base := filepath.Base(strings.TrimSpace(name))
	base = strings.ReplaceAll(base, string(filepath.Separator), "_")
	if base == "" || base == "." {
		base = "image"
	}
	if len(base) > 80 {
		base = base[:80]
	}
	if filepath.Ext(base) == "" {
		base += storedImageExt(mediaType)
	}
	return id + "-" + base
}

func storedImageExt(mediaType string) string {
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	}
	t := strings.ToLower(strings.TrimSpace(mediaType))
	switch {
	case strings.HasPrefix(t, "audio/webm"):
		return ".webm"
	case strings.HasPrefix(t, "audio/mp4"):
		return ".m4a"
	case strings.HasPrefix(t, "audio/ogg"):
		return ".ogg"
	case strings.HasPrefix(t, "audio/wav"), strings.HasPrefix(t, "audio/x-wav"):
		return ".wav"
	}
	return ".bin"
}

// saveImage stores one decoded image upload. It returns the HTTP status to
// answer with when non-zero.
func (d *Daemon) saveImage(sessionID, name, mediaType string, data []byte) (ImageMeta, int, string) {
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(mediaType)), "image/") {
		return ImageMeta{}, http.StatusBadRequest, "mediaType must be an image/* type"
	}
	return d.saveUpload(sessionID, name, mediaType, data, 0, nil)
}

// saveUpload stores one decoded upload (an image or a voice recording) and
// records it in the index. Recordings get an "aud_" id and keep their
// duration and waveform peaks; they share the images route and directory
// but never reach the model as files or pixels.
func (d *Daemon) saveUpload(sessionID, name, mediaType string, data []byte, durationMs int64, peaks []int) (ImageMeta, int, string) {
	d.mu.Lock()
	if _, ok := d.sessions[sessionID]; !ok {
		d.mu.Unlock()
		return ImageMeta{}, http.StatusNotFound, "not found"
	}
	d.mu.Unlock()
	kind := strings.ToLower(strings.TrimSpace(mediaType))
	audio := strings.HasPrefix(kind, "audio/")
	if !audio && !strings.HasPrefix(kind, "image/") {
		return ImageMeta{}, http.StatusBadRequest, "mediaType must be an image/* or audio/* type"
	}
	if len(data) == 0 {
		return ImageMeta{}, http.StatusBadRequest, "image data is required"
	}
	if len(data) > maxStoredImageBytes {
		return ImageMeta{}, http.StatusRequestEntityTooLarge, "image exceeds the 25 MB cap"
	}
	id := newID()
	if audio {
		id = audioIDPrefix + id
	}
	file := imageFileName(id, name, mediaType)
	dir := d.sessionImagesDir(sessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ImageMeta{}, http.StatusInternalServerError, "cannot store image"
	}
	if err := os.WriteFile(filepath.Join(dir, file), data, 0o644); err != nil {
		return ImageMeta{}, http.StatusInternalServerError, "cannot store image"
	}
	meta := ImageMeta{
		ID:        id,
		Name:      filepath.Base(strings.TrimSpace(name)),
		MediaType: strings.TrimSpace(mediaType),
		Size:      int64(len(data)),
		URL:       d.imageURL(sessionID, id),
		CreatedAt: time.Now().UTC(),
	}
	if audio {
		if durationMs > 0 {
			meta.DurationMs = durationMs
		}
		if len(peaks) > 200 {
			peaks = peaks[:200]
		}
		meta.Peaks = peaks
	}
	if meta.Name == "" {
		meta.Name = file
	}
	metas := append(d.readImagesIndex(sessionID), meta)
	d.writeImagesIndex(sessionID, metas)
	return meta, 0, ""
}

// imagePath resolves one attached image id to its on-disk file. Empty
// means no such file is stored for the Session.
func (d *Daemon) imagePath(sessionID, imageID string) string {
	entries, err := os.ReadDir(d.sessionImagesDir(sessionID))
	if err != nil {
		return ""
	}
	for _, en := range entries {
		if !en.IsDir() && strings.HasPrefix(en.Name(), imageID+"-") {
			return filepath.Join(d.sessionImagesDir(sessionID), en.Name())
		}
	}
	return ""
}

// imageIDsValid reports whether every id is attached to the Session.
func (d *Daemon) imageIDsValid(sessionID string, ids []string) string {
	have := map[string]bool{}
	for _, m := range d.readImagesIndex(sessionID) {
		have[m.ID] = true
	}
	for _, id := range ids {
		if !have[id] {
			return id
		}
	}
	return ""
}

// handleSessionImages serves the images collection: POST uploads one
// image as JSON {name, mediaType, data (base64)}, GET lists them.
func (d *Daemon) handleSessionImages(w http.ResponseWriter, r *http.Request, id string) {
	switch r.Method {
	case "GET":
		d.mu.Lock()
		_, ok := d.sessions[id]
		d.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		metas := d.readImagesIndex(id)
		if metas == nil {
			metas = []ImageMeta{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"images": metas})
	case "POST":
		r.Body = http.MaxBytesReader(w, r.Body, maxStoredUploadBytes)
		var req struct {
			Name       string `json:"name"`
			MediaType  string `json:"mediaType"`
			Data       string `json:"data"`
			DurationMs int64  `json:"durationMs"`
			Peaks      []int  `json:"peaks"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Data == "" {
			http.Error(w, "image data is required", http.StatusBadRequest)
			return
		}
		raw, err := base64.StdEncoding.DecodeString(req.Data)
		if err != nil {
			if raw, err = base64.URLEncoding.DecodeString(req.Data); err != nil {
				http.Error(w, "image data is not valid base64", http.StatusBadRequest)
				return
			}
		}
		meta, code, msg := d.saveUpload(id, req.Name, req.MediaType, raw, req.DurationMs, req.Peaks)
		if code != 0 {
			if code == http.StatusNotFound {
				http.NotFound(w, r)
				return
			}
			http.Error(w, msg, code)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"image": meta})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSessionImageFile serves one attached image's bytes.
func (d *Daemon) handleSessionImageFile(w http.ResponseWriter, r *http.Request, id, imageID string) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	d.mu.Lock()
	_, ok := d.sessions[id]
	d.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	var found *ImageMeta
	for _, m := range d.readImagesIndex(id) {
		if m.ID == imageID {
			m := m
			found = &m
			break
		}
	}
	if found == nil {
		http.NotFound(w, r)
		return
	}
	path := d.imagePath(id, imageID)
	if path == "" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", found.MediaType)
	http.ServeFile(w, r, path)
}

// imageRefsText renders the text a backend turn runs on when the message
// carries stored image ids: the user text plus their resolved file paths,
// so the agent can read them. The transcript keeps the clean text with the
// image ids. Voice recordings never appear: the model reads the transcript.
func (d *Daemon) imageRefsText(id, text string, images []string) string {
	paths := make([]string, 0, len(images))
	for _, img := range images {
		if isAudioID(img) {
			continue
		}
		if p := d.imagePath(id, img); p != "" {
			if abs, err := filepath.Abs(p); err == nil {
				p = abs
			}
			paths = append(paths, img+": "+p)
		}
	}
	if len(paths) == 0 {
		return text
	}
	var b strings.Builder
	b.WriteString(text)
	b.WriteString("\n\n[attached images: open these files to see them]")
	for _, p := range paths {
		b.WriteString("\n- " + p)
	}
	return b.String()
}

// storedImagesAsInline resolves Session-stored image ids to inline
// attachments for the Backend turn. The transcript keeps the stable ids
// (and the prompt text keeps their file paths, which the agent can also
// read with its file tools), while the model gets native vision bytes too.
func (d *Daemon) storedImagesAsInline(id string, ids []string) []Image {
	metas := map[string]ImageMeta{}
	for _, m := range d.readImagesIndex(id) {
		metas[m.ID] = m
	}
	var out []Image
	for _, imgID := range ids {
		m, ok := metas[imgID]
		if !ok || isAudioID(imgID) {
			continue
		}
		path := d.imagePath(id, imgID)
		if path == "" {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil || len(raw) == 0 {
			continue
		}
		out = append(out, Image{Mime: m.MediaType, Data: base64.StdEncoding.EncodeToString(raw)})
	}
	return out
}

// withStoredImages appends the Session-stored attachments to the inline
// ones, so the Backend turn runs on both.
func (d *Daemon) withStoredImages(id string, inline []Image, ids []string) []Image {
	if len(ids) == 0 {
		return inline
	}
	return append(inline, d.storedImagesAsInline(id, ids)...)
}

// storeInlineImages persists a prompt's inline attachments under the
// Session's image store and returns their stable ids, so the transcript and
// the agent can reference them later. A block that will not decode is
// skipped; validateImages already vetted the bytes.
func (d *Daemon) storeInlineImages(sessionID string, images []Image) []string {
	ids := make([]string, 0, len(images))
	for i, img := range images {
		raw, err := base64.StdEncoding.DecodeString(img.Data)
		if err != nil {
			continue
		}
		meta, code, _ := d.saveImage(sessionID, "inline-"+strconv.Itoa(i+1), img.Mime, raw)
		if code != 0 {
			continue
		}
		ids = append(ids, meta.ID)
	}
	return ids
}
