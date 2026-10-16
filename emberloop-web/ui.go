package web

// Route entries declare their page data requirements alongside the URL table.
import (
	"bytes"
	"compress/gzip"
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
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
	name := strings.TrimPrefix(p, "/")
	if name == "" || strings.Contains(name, "..") {
		http.NotFound(w, r)
		return
	}
	if fi, err := fs.Stat(sub, name); err == nil && fi.Mode().IsRegular() {
		serveFile(sub, w, r, name)
		return
	}
	if path.Ext(p) != "" {
		http.NotFound(w, r)
		return
	}
	if matchUIRoute(p) {
		serveBootShell(d, sub, w, r, p)
		return
	}
	serveNotFoundPage(w, r)
}

// Prefetch runs the route's read-only API requests in-process with the same
// cookie as the page request. Results are keyed by API path for the browser.
func prefetchPaths(route, sessionID string) []string {
	switch {
	case route == "/":
		return []string{"/api/dashboard", "/api/projects"}
	case strings.HasPrefix(route, "/s/"):
		return []string{"/api/sessions/" + sessionID + "?tail=40", "/api/users", "/api/projects"}
	case strings.HasSuffix(route, "/runs") || strings.Contains(route, "/runs/"):
		return []string{"/api/runs", "/api/projects"}
	case strings.HasSuffix(route, "/settings") || strings.HasPrefix(route, "/settings"):
		return []string{"/api/settings", "/api/projects"}
	case strings.HasPrefix(route, "/p/"):
		return []string{"/api/projects", "/api/sessions"}
	default:
		return nil
	}
}

func serveBootShell(d *Daemon, sub fs.FS, w http.ResponseWriter, r *http.Request, route string) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Vary", "Cookie")
	boot := map[string]json.RawMessage{}
	if _, ok := d.requesterOf(r); ok {
		parts := strings.Split(strings.Trim(route, "/"), "/")
		sessionID := ""
		if len(parts) == 2 && parts[0] == "s" {
			sessionID = parts[1]
		}
		for _, apiPath := range prefetchPaths(route, sessionID) {
			req := httptest.NewRequest(http.MethodGet, apiPath, nil)
			if c, err := r.Cookie(sessionCookie); err == nil {
				req.AddCookie(c)
			}
			if tok := bearerToken(r); tok != "" {
				req.Header.Set("Authorization", "Bearer "+tok)
			}
			rw := httptest.NewRecorder()
			apiHandler(d, apiPath).ServeHTTP(rw, req)
			if rw.Code >= 200 && rw.Code < 300 {
				boot[apiPath] = json.RawMessage(rw.Body.Bytes())
			}
		}
	} else {
		boot["auth"] = json.RawMessage(`{"signedIn":false}`)
	}
	raw, _ := json.Marshal(boot)
	index, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		http.Error(w, "ui unavailable", http.StatusInternalServerError)
		return
	}
	marker := []byte("</head>")
	idx := bytes.Index(index, marker)
	if idx < 0 {
		_, _ = w.Write(index)
		return
	}
	loader := []byte("<script>window.__EL_BOOT__=" + string(raw) + ";</script>")
	index = append(index[:idx], append(loader, index[idx:]...)...)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(index)
}

func apiHandler(d *Daemon, p string) http.Handler {
	switch {
	case p == "/api/dashboard":
		return http.HandlerFunc(d.handleDashboard)
	case p == "/api/projects":
		return http.HandlerFunc(d.handleProjects)
	case p == "/api/sessions":
		return http.HandlerFunc(d.handleSessions)
	case strings.HasPrefix(p, "/api/sessions/"):
		return http.HandlerFunc(d.handleSessionSub)
	case p == "/api/runs":
		return http.HandlerFunc(d.handleRuns)
	case p == "/api/settings":
		return http.HandlerFunc(d.handleSettings)
	case p == "/api/users":
		return http.HandlerFunc(d.handleUsers)
	default:
		return http.NotFoundHandler()
	}
}

// uiRoutePatterns mirrors every route shape parsed by web/src/lib/router.js.
var uiRoutePatterns = []string{
	"/", "/p/<id>", "/p/<id>/runs", "/p/<id>/runs/<runId>",
	"/p/<id>/runs/<runId>/<ticketId>", "/p/<id>/settings", "/s/<id>",
	"/runs", "/runs/<runId>", "/runs/<runId>/<ticketId>",
	"/settings", "/settings/agents", "/settings/runs", "/settings/context",
	"/settings/memory", "/settings/access", "/settings/profile",
	"/settings/notifications", "/settings/sound-voice", "/quota", "/providers", "/memory",
}

func matchUIRoute(p string) bool {
	for _, pattern := range uiRoutePatterns {
		if routePatternMatches(pattern, p) {
			return true
		}
	}
	return false
}

func routePatternMatches(pattern, p string) bool {
	if pattern == "/" {
		return p == "/"
	}
	got := strings.Split(strings.Trim(p, "/"), "/")
	want := strings.Split(strings.Trim(pattern, "/"), "/")
	if len(got) != len(want) {
		return false
	}
	for i, segment := range want {
		if strings.HasPrefix(segment, "<") && strings.HasSuffix(segment, ">") {
			if got[i] == "" {
				return false
			}
		} else if segment != got[i] {
			return false
		}
	}
	return true
}

func serveNotFoundPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNotFound)
	_, _ = io.WriteString(w, `<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Not found · Emberloop</title><body><main><h1>Page not found</h1><p>The page you requested doesn't exist.</p><a href="/">Return to Emberloop</a></main></body></html>`)
}

func serveFile(sub fs.FS, w http.ResponseWriter, r *http.Request, name string) {
	raw, err := fs.ReadFile(sub, name)
	if err != nil {
		http.Error(w, "ui unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", contentTypeFor(name))
	if name == "sw.js" {
		// The worker registers with scope / from the root, but send the
		// header anyway so a nested route registration keeps working, and
		// never let it go stale: a cached worker pins old push code.
		w.Header().Set("Service-Worker-Allowed", "/")
		w.Header().Set("Cache-Control", "no-store")
	} else if strings.HasPrefix(name, "assets/") {
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
	case ".html", ".js", ".css", ".svg", ".json", ".webmanifest", ".map", ".wasm":
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
	case ".webmanifest":
		return "application/manifest+json"
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
