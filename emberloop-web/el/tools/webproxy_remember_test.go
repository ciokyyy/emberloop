package tools

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// rememberFake is a Daemon stub for the remember and recall proxies:
// Session-bound propose with scripted statuses and a scripted recall body.
type rememberFake struct {
	rememberCode int
	rememberBody string
	remembered   []map[string]any
	recallCode   int
	recallBody   string
	recallQuery  string
}

func (f *rememberFake) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/sessions/", func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
		_, sub, _ := strings.Cut(rest, "/")
		switch {
		case sub == "remember" && r.Method == http.MethodPost:
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			f.remembered = append(f.remembered, req)
			code := f.rememberCode
			if code == 0 {
				code = http.StatusCreated
			}
			if code == http.StatusCreated && f.rememberBody == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(code)
				_, _ = w.Write([]byte(`{"id":7}`))
				return
			}
			w.WriteHeader(code)
			_, _ = w.Write([]byte(f.rememberBody))
		case sub == "recall" && r.Method == http.MethodGet:
			f.recallQuery = r.URL.Query().Get("query")
			code := f.recallCode
			if code == 0 {
				code = http.StatusOK
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(code)
			if f.recallBody != "" {
				_, _ = w.Write([]byte(f.recallBody))
				return
			}
			_, _ = w.Write([]byte(`{"results":[],"text":""}`))
		default:
			http.NotFound(w, r)
		}
	})
	return mux
}

func TestWebProxyRememberArgChecks(t *testing.T) {
	f := &rememberFake{}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	bindProxy(t, srv, "sess")

	for _, args := range []string{
		`{}`,
		`{"text":""}`,
		`{"text":"note"}`,
		`{"text":"note","reason":""}`,
		`not json`,
	} {
		if _, err := proxyCall(t, "remember", args); err == nil {
			t.Errorf("remember %s must fail", args)
		}
	}
}

func TestWebProxyRememberPostsAndReportsID(t *testing.T) {
	f := &rememberFake{}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	bindProxy(t, srv, "sess")

	out, err := proxyCall(t, "remember", `{"text":"use tabs","reason":"user prefers tabs","quote":"use tabs here"}`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "proposed rule 7 for human approval" {
		t.Fatalf("remember = %q", out)
	}
	if len(f.remembered) != 1 || f.remembered[0]["text"] != "use tabs" || f.remembered[0]["reason"] != "user prefers tabs" {
		t.Fatalf("remembered = %v, want the text and reason posted", f.remembered)
	}
}

func TestWebProxyRememberErrorKeepsBody(t *testing.T) {
	for _, tc := range []struct {
		name string
		code int
		body string
		want string
	}{
		{"grounding", http.StatusBadRequest, "quote is not a substring of any user message in this session", "substring"},
		{"limit", http.StatusTooManyRequests, "remember limit 5 per session reached", "limit"},
		{"role", http.StatusForbidden, "only top-level Sessions propose Rules", "top-level"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &rememberFake{rememberCode: tc.code, rememberBody: tc.body}
			srv := httptest.NewServer(f.handler())
			defer srv.Close()
			bindProxy(t, srv, "sess")

			_, err := proxyCall(t, "remember", `{"text":"use tabs","reason":"r"}`)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("remember error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestWebProxyRecallArgChecks(t *testing.T) {
	f := &rememberFake{}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	bindProxy(t, srv, "sess")

	for _, args := range []string{
		`{}`,
		`{"query":""}`,
		`not json`,
	} {
		if _, err := proxyCall(t, "recall", args); err == nil {
			t.Errorf("recall %s must fail", args)
		}
	}
}

func TestWebProxyRecallReturnsText(t *testing.T) {
	f := &rememberFake{recallBody: `{"results":[],"text":"rule: use tabs"}`}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	bindProxy(t, srv, "sess")

	out, err := proxyCall(t, "recall", `{"query":"tabs"}`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "rule: use tabs" {
		t.Fatalf("recall = %q, want the Daemon text", out)
	}
	if f.recallQuery != "tabs" {
		t.Fatalf("recall query = %q, want tabs", f.recallQuery)
	}

	empty := &rememberFake{}
	srv2 := httptest.NewServer(empty.handler())
	defer srv2.Close()
	bindProxy(t, srv2, "sess")
	out, err = proxyCall(t, "recall", `{"query":"nothing"}`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "(no matching rules)" {
		t.Fatalf("empty recall = %q, want the empty note", out)
	}
}

func TestWebProxyRecallErrorKeepsBody(t *testing.T) {
	f := &rememberFake{recallCode: http.StatusBadRequest, recallBody: "query is required"}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	bindProxy(t, srv, "sess")

	_, err := proxyCall(t, "recall", `{"query":"x"}`)
	if err == nil || !strings.Contains(err.Error(), "query is required") {
		t.Fatalf("recall error = %v, want the Daemon reason", err)
	}
}
