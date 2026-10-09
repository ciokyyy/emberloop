package web

import (
	"bytes"
	"compress/gzip"
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"sync"
)

//go:embed web/dist
var dist embed.FS

func (d *Daemon) serveUI(w http.ResponseWriter, r *http.Request) {
	sub, err := fs.Sub(dist, "web/dist")
	if err != nil {
		http.Error(w, "ui unavailable", http.StatusInternalServerError)
		return
	}
	p := path.Clean(r.URL.Path)
	if p == "/" || p == "/index.html" {
		serveFile(sub, w, r, "index.html")
		return
	}
	// Static bundle assets (hashed JS/CSS, favicon). Anything else 404s:
	// the app is one page served at /.
	name := strings.TrimPrefix(p, "/")
	if name == "" || strings.Contains(name, "..") {
		http.NotFound(w, r)
		return
	}
	fi, err := fs.Stat(sub, name)
	if err != nil || !fi.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	serveFile(sub, w, r, name)
}

func serveFile(sub fs.FS, w http.ResponseWriter, r *http.Request, name string) {
	raw, err := fs.ReadFile(sub, name)
	if err != nil {
		http.Error(w, "ui unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", contentTypeFor(name))
	if strings.HasPrefix(name, "assets/") {
		// Vite hashes these names, so content never changes under one name.
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	if compressible(name) && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		if z := gzipCached(name, raw); z != nil {
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Set("Vary", "Accept-Encoding")
			_, _ = w.Write(z)
			return
		}
	}
	_, _ = w.Write(raw)
}

var gzCache sync.Map // name -> []byte

func compressible(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".html", ".js", ".css", ".svg", ".json", ".map", ".wasm":
		return true
	}
	return false
}

func gzipCached(name string, raw []byte) []byte {
	if v, ok := gzCache.Load(name); ok {
		return v.([]byte)
	}
	var buf bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if _, err := zw.Write(raw); err != nil || zw.Close() != nil {
		return nil
	}
	gzCache.Store(name, buf.Bytes())
	return buf.Bytes()
}

func contentTypeFor(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".js":
		return "text/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".json", ".map":
		return "application/json"
	case ".png":
		return "image/png"
	case ".ico":
		return "image/x-icon"
	case ".wasm":
		return "application/wasm"
	case ".woff2":
		return "font/woff2"
	default:
		return "application/octet-stream"
	}
}
