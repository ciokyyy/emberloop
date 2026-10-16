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

// Direct loads of every browser route shape (see web/src/lib/router.js) must
// serve the shell. Keep this table aligned with uiRoutePatterns.
func TestRouteTableServesKnownRoutes(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	routes := []string{
		"/", "/p/project-1", "/p/none", "/p/project-1/runs",
		"/p/project-1/runs/run-1", "/p/project-1/runs/run-1/ticket-1",
		"/p/project-1/settings", "/s/session-1", "/runs",
		"/runs/run-1", "/runs/run-1/ticket-1", "/settings",
		"/settings/agents", "/settings/runs", "/settings/context", "/settings/memory",
		"/settings/access", "/settings/profile", "/settings/notifications",
		"/settings/sound-voice", "/quota", "/providers", "/memory",
	}
	for _, route := range routes {
		t.Run(route, func(t *testing.T) {
			resp, err := http.Get(srv.URL + route)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK || !strings.Contains(resp.Header.Get("Content-Type"), "text/html") || !strings.Contains(string(body), "<!doctype html>") {
				t.Fatalf("GET %s = %d type %q (want 200 app shell)", route, resp.StatusCode, resp.Header.Get("Content-Type"))
			}
		})
	}
}

func TestRouteTable404sUnknownPathsAndAssets(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	for _, route := range []string{"/nope", "/settings/nope", "/p/id/runs/run/ticket/extra", "/quota/x", "/memory/x", "/s", "/s/"} {
		resp, err := http.Get(srv.URL + route)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound || !strings.Contains(resp.Header.Get("Content-Type"), "text/html") || !strings.Contains(string(body), "Page not found") {
			t.Fatalf("GET %s = %d type %q body %q, want 404 page", route, resp.StatusCode, resp.Header.Get("Content-Type"), body)
		}
	}
	for _, route := range []string{"/does-not-exist-xyz.js", "/api/not-a-route"} {
		resp, err := http.Get(srv.URL + route)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("GET %s = %d, want 404", route, resp.StatusCode)
		}
	}
}
