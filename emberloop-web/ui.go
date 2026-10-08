package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
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
		serveFile(sub, w, "index.html")
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
	serveFile(sub, w, name)
}

func serveFile(sub fs.FS, w http.ResponseWriter, name string) {
	raw, err := fs.ReadFile(sub, name)
	if err != nil {
		http.Error(w, "ui unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", contentTypeFor(name))
	_, _ = w.Write(raw)
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
	case ".woff2":
		return "font/woff2"
	default:
		return "application/octet-stream"
	}
}
