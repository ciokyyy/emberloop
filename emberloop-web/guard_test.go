package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The guard accepts loopback, plus hostnames on the allow list (a local
// reverse proxy forwards its public name), and nothing else.
func TestGuardAllowedHosts(t *testing.T) {
	d := NewDaemon(t.TempDir(), NewFakeBackend())
	d.SetAllowedHosts([]string{"Box.tail1234.ts.net."})
	h := d.Handler()
	cases := []struct {
		host, origin string
		want         int
	}{
		{"127.0.0.1:7940", "", http.StatusOK},
		{"box.tail1234.ts.net", "", http.StatusOK},
		{"box.tail1234.ts.net:7940", "https://box.tail1234.ts.net:7940", http.StatusOK},
		{"evil.example.com", "", http.StatusForbidden},
		{"box.tail1234.ts.net", "https://evil.example.com", http.StatusForbidden},
		{"127.0.0.1:7940", "https://evil.example.com", http.StatusForbidden},
	}
	for _, c := range cases {
		req := httptest.NewRequest("GET", "/api/projects", nil)
		req.Host = c.host
		if c.origin != "" {
			req.Header.Set("Origin", c.origin)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("host %q origin %q = %d, want %d", c.host, c.origin, rec.Code, c.want)
		}
	}
	plain := NewDaemon(t.TempDir(), NewFakeBackend()).Handler()
	req := httptest.NewRequest("GET", "/api/projects", nil)
	req.Host = "box.tail1234.ts.net"
	rec := httptest.NewRecorder()
	plain.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("no allow list = %d, want 403", rec.Code)
	}
}

// Tailnet mode accepts Tailscale addresses (100.64.0.0/10) and nothing else new.
func TestGuardTailnetAddresses(t *testing.T) {
	d := NewDaemon(t.TempDir(), NewFakeBackend())
	d.AllowTailnet(true)
	h := d.Handler()
	for host, want := range map[string]int{
		"100.94.50.96:7940":  http.StatusOK,
		"100.127.255.1:7940": http.StatusOK,
		"100.128.0.1:7940":   http.StatusForbidden,
		"192.168.1.20:7940":  http.StatusForbidden,
		"10.0.0.5:7940":      http.StatusForbidden,
	} {
		req := httptest.NewRequest("GET", "/api/projects", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("host %s = %d, want %d", host, rec.Code, want)
		}
	}
	off := NewDaemon(t.TempDir(), NewFakeBackend()).Handler()
	req := httptest.NewRequest("GET", "/api/projects", nil)
	req.Host = "100.94.50.96:7940"
	rec := httptest.NewRecorder()
	off.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("tailnet off = %d, want 403", rec.Code)
	}
}
