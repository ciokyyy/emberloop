package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The installable shell: manifest, worker and icons serve with the headers
// install and push depend on.
func TestServesPWAInstallFiles(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	// Manifest: right content type, Emberloop standalone metadata.
	resp, err := http.Get(srv.URL + "/manifest.webmanifest")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("manifest status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/manifest+json" {
		t.Fatalf("manifest content type = %q", ct)
	}
	var man struct {
		Name       string `json:"name"`
		Display    string `json:"display"`
		StartURL   string `json:"start_url"`
		Scope      string `json:"scope"`
		ThemeColor string `json:"theme_color"`
		BGColor    string `json:"background_color"`
		Icons      []struct {
			Src     string `json:"src"`
			Sizes   string `json:"sizes"`
			Purpose string `json:"purpose"`
		} `json:"icons"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&man); err != nil {
		t.Fatal(err)
	}
	if man.Name != "Emberloop" || man.Display != "standalone" || man.StartURL != "/" || man.Scope != "/" {
		t.Fatalf("manifest identity %+v", man)
	}
	var any192, any512, maskable bool
	for _, ic := range man.Icons {
		if ic.Purpose == "any" && ic.Sizes == "192x192" {
			any192 = true
		}
		if ic.Purpose == "any" && ic.Sizes == "512x512" {
			any512 = true
		}
		if strings.Contains(ic.Purpose, "maskable") {
			maskable = true
		}
	}
	if !any192 || !any512 || !maskable {
		t.Fatalf("manifest icons miss 192/512/maskable: %+v", man.Icons)
	}

	// Worker: allowed at scope root, never stale-cached.
	resp, err = http.Get(srv.URL + "/sw.js")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("sw status = %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Service-Worker-Allowed"); got != "/" {
		t.Fatalf("Service-Worker-Allowed = %q", got)
	}
	if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Fatalf("sw Cache-Control = %q, want no-store", cc)
	}
	buf := new(strings.Builder)
	b := make([]byte, 1<<20)
	n, _ := resp.Body.Read(b)
	buf.Write(b[:n])
	body := buf.String()
	if !strings.Contains(body, "/api/") {
		t.Fatalf("sw missing the /api/* bypass")
	}
	if !strings.Contains(body, "push") {
		t.Fatalf("sw missing the push handler")
	}

	// Icons: every manifest src resolves as an image.
	for _, ic := range man.Icons {
		r, err := http.Get(srv.URL + ic.Src)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		if r.StatusCode != http.StatusOK {
			t.Fatalf("GET %s status = %d", ic.Src, r.StatusCode)
		}
		if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "image/") {
			t.Fatalf("GET %s content type = %q", ic.Src, ct)
		}
	}
}

// The shell entry still advertises the manifest and install meta.
func TestIndexAdvertisesPWA(t *testing.T) {
	t.Parallel()
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b := make([]byte, 1<<20)
	n, _ := resp.Body.Read(b)
	body := string(b[:n])
	for _, want := range []string{"manifest.webmanifest", "theme-color", "apple-touch-icon", "apple-mobile-web-app-capable"} {
		if !strings.Contains(body, want) {
			t.Fatalf("index missing %q", want)
		}
	}
}
