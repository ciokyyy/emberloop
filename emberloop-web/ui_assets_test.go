package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The built UI is a static bundle: index plus its hashed assets must all
// serve from the embedded dist with correct content types.
func TestServesBuiltStaticAssets(t *testing.T) {
	d := NewDaemon(t.TempDir(), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
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
