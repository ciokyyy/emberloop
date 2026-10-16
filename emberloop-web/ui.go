package web

import (
	"bytes"
	"compress/gzip"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"modernc.org/quickjs"
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
	if p == "/" {
		serveBootShell(d, sub, w, r, p)
		return
	}
	if p == "/index.html" {
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
		return []string{"/api/auth/me", "/api/dashboard", "/api/projects"}
	case strings.HasPrefix(route, "/s/"):
		return []string{"/api/auth/me", "/api/sessions/" + sessionID + "?tail=40", "/api/users", "/api/projects"}
	case strings.HasSuffix(route, "/runs") || strings.Contains(route, "/runs/"):
		return []string{"/api/auth/me", "/api/runs", "/api/projects"}
	case strings.HasSuffix(route, "/settings") || strings.HasPrefix(route, "/settings"):
		return []string{"/api/auth/me", "/api/settings", "/api/projects"}
	case strings.HasPrefix(route, "/p/"):
		return []string{"/api/auth/me", "/api/projects", "/api/sessions"}
	default:
		return nil
	}
}

func serveBootShell(d *Daemon, sub fs.FS, w http.ResponseWriter, r *http.Request, route string) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Vary", "Cookie")
	boot := map[string]json.RawMessage{}
	parts := strings.Split(strings.Trim(route, "/"), "/")
	sessionID := ""
	if len(parts) == 2 && parts[0] == "s" {
		sessionID = parts[1]
	}
	for _, apiPath := range prefetchPaths(route, sessionID) {
		u, err := url.Parse(apiPath)
		if err != nil {
			continue
		}
		req := r.Clone(r.Context())
		req.Method, req.URL = http.MethodGet, u
		req.Body = http.NoBody
		rw := httptest.NewRecorder()
		apiHandler(d, u.Path).ServeHTTP(rw, req)
		if rw.Code >= 200 && rw.Code < 300 {
			boot[apiPath] = json.RawMessage(rw.Body.Bytes())
		}
	}
	if _, ok := boot["/api/auth/me"]; !ok {
		boot["auth"] = json.RawMessage(`{"signedIn":false}`)
		stateReq := r.Clone(r.Context())
		stateReq.Method, stateReq.URL = http.MethodGet, &url.URL{Path: "/api/auth/state"}
		stateResp := httptest.NewRecorder()
		d.handleAuthState(stateResp, stateReq)
		if stateResp.Code == http.StatusOK {
			boot["/api/auth/state"] = json.RawMessage(stateResp.Body.Bytes())
		}
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
	if route == "/" && boot["/api/auth/me"] != nil {
		if html, err := renderDashboardWithin(200 * time.Millisecond); err == nil {
			index = bytes.Replace(index, []byte(`<script type="module" src="/src/main.js"></script>`), []byte(`<div id="el-ssr-dashboard">`+html+`</div><script type="module" src="/src/main.js"></script>`), 1)
		} else {
			log.Printf("SSR render fallback for %s: %v", route, err)
		}
	}
	_, _ = w.Write(index)
}

func apiHandler(d *Daemon, p string) http.Handler {
	switch {
	case p == "/api/auth/me":
		return http.HandlerFunc(d.handleMe)
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

func renderDashboardWithin(limit time.Duration) (html string, err error) {
	type result struct {
		html string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				done <- result{err: fmt.Errorf("panic: %v", recovered)}
			}
		}()
		bundle, e := dist.ReadFile("web/dist/ssr/ssr.js")
		if e != nil {
			done <- result{err: e}
			return
		}
		vm, e := quickjs.NewVM()
		if e != nil {
			done <- result{err: e}
			return
		}
		defer vm.Close()
		if _, e = vm.Eval(string(bundle), quickjs.EvalModule); e != nil {
			done <- result{err: e}
			return
		}
		value, e := vm.Call("__elRenderDashboard")
		if e != nil {
			done <- result{err: e}
			return
		}
		s, ok := value.(string)
		if !ok || s == "" {
			done <- result{err: fmt.Errorf("empty SSR output")}
			return
		}
		done <- result{html: s}
	}()
	timer := time.NewTimer(limit)
	defer timer.Stop()
	select {
	case got := <-done:
		return got.html, got.err
	case <-timer.C:
		return "", fmt.Errorf("render exceeded %s", limit)
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
