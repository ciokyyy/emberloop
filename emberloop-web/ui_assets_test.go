package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The built UI is a static bundle: index plus its hashed assets must all
// serve from the embedded dist with correct content types.
func TestServesBuiltStaticAssets(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	get := func(path string) (int, string) {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		return resp.StatusCode, resp.Header.Get("Content-Type")
	}

	if code, ct := get("/favicon.svg"); code != http.StatusOK || !strings.Contains(ct, "svg") {
		t.Fatalf("favicon status = %d type = %q", code, ct)
	}
	if code, _ := get("/does-not-exist-xyz.js"); code != http.StatusNotFound {
		t.Fatalf("missing asset status = %d, want 404", code)
	}
	if code, _ := get("/assets"); code != http.StatusNotFound {
		t.Fatalf("directory status = %d, want 404", code)
	}
}

func TestContentTypeFor(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"index.html":  "text/html",
		"app-abc.js":  "javascript",
		"app-abc.css": "text/css",
		"icon.svg":    "svg",
		"app.js.map":  "application/json",
	}
	for name, want := range cases {
		if got := contentTypeFor(name); !strings.Contains(got, want) {
			t.Fatalf("contentTypeFor(%q) = %q, want %q", name, got, want)
		}
	}
}
// Direct loads of every route the browser router owns (see
// web/src/lib/router.js) serve the app shell, so refresh, bookmarks and PWA
// shortcuts land in the app; unknown paths and /api/* still 404.
func TestSPAFallbackServesKnownRoutes(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	get := func(path string) (int, string, string) {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, resp.Header.Get("Content-Type"), string(raw)
	}

	for _, path := range []string{"/s/abc123", "/runs", "/runs/r1", "/runs/r1/t1", "/settings", "/quota", "/memory"} {
		code, ct, body := get(path)
		if code != http.StatusOK || !strings.Contains(ct, "text/html") || !strings.Contains(body, "<!doctype html>") {
			t.Fatalf("GET %s = %d type %q (want 200 app shell)", path, code, ct)
		}
	}
	for _, path := range []string{"/nope", "/quota/x", "/memory/x", "/s", "/s/"} {
		if code, _, _ := get(path); code != http.StatusNotFound {
			t.Fatalf("GET %s = %d, want 404", path, code)
		}
	}
}
