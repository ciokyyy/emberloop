package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed web/dist
var dist embed.FS

func (d *Daemon) serveUI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/index.html" {
		http.NotFound(w, r)
		return
	}
	sub, err := fs.Sub(dist, "web/dist")
	if err != nil {
		http.Error(w, "ui unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	raw, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		http.Error(w, "ui unavailable", http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(raw)
}
