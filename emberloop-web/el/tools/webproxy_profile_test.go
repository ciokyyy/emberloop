package tools

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// profileFake is a Daemon stub for the profile proxy: Session-bound read and
// propose with scripted statuses and bodies.
type profileFake struct {
	readCode    int
	readBody    string
	proposeCode int
	proposeBody string
	proposed    []map[string]any
}

func (f *profileFake) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/sessions/", func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
		_, sub, _ := strings.Cut(rest, "/")
		switch {
		case sub == "profile" && r.Method == http.MethodGet:
			code := f.readCode
			if code == 0 {
				code = http.StatusOK
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(code)
			_, _ = w.Write([]byte(f.readBody))
		case sub == "profile/propose" && r.Method == http.MethodPost:
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			if ops, _ := req["ops"].([]any); ops != nil {
				f.proposed = append(f.proposed, req)
			}
			code := f.proposeCode
			if code == 0 {
				code = http.StatusCreated
			}
			if code == http.StatusCreated && f.proposeBody == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(code)
				_, _ = w.Write([]byte(`{"id":9}`))
				return
			}
			w.WriteHeader(code)
			_, _ = w.Write([]byte(f.proposeBody))
		default:
			http.NotFound(w, r)
		}
	})
	return mux
}

func TestWebProxyProfileRead(t *testing.T) {
	f := &profileFake{readBody: `{"version":1,"lines":[],"rendered":"use tabs"}`}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	bindProxy(t, srv, "sess")

	out, err := proxyCall(t, "profile", `{"action":"read"}`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "use tabs" {
		t.Fatalf("read = %q, want the rendered profile", out)
	}

	empty := &profileFake{readBody: `{"version":0,"lines":[],"rendered":""}`}
	srv2 := httptest.NewServer(empty.handler())
	defer srv2.Close()
	bindProxy(t, srv2, "sess")
	out, err = proxyCall(t, "profile", `{"action":"read"}`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "(no profile saved)" {
		t.Fatalf("empty read = %q, want the empty note", out)
	}
}

func TestWebProxyProfileReadErrorKeepsBody(t *testing.T) {
	f := &profileFake{readCode: http.StatusBadRequest, readBody: "profile is 3000 bytes, max 2048: condense it"}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	bindProxy(t, srv, "sess")

	_, err := proxyCall(t, "profile", `{"action":"read"}`)
	if err == nil || !strings.Contains(err.Error(), "condense") {
		t.Fatalf("read error = %v, want the Daemon condense reason", err)
	}
}

func TestWebProxyProfileArgChecks(t *testing.T) {
	f := &profileFake{readBody: `{}`}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	bindProxy(t, srv, "sess")

	for _, args := range []string{
		`{}`,
		`{"action":"delete"}`,
		`{"action":"propose"}`,
		`{"action":"propose","ops":[]}`,
		`{"action":"propose","ops":[{"op":"add","text":"x","tag":"other"}]}`,
		`not json`,
	} {
		if _, err := proxyCall(t, "profile", args); err == nil {
			t.Errorf("profile %s must fail", args)
		}
	}
}

func TestWebProxyProfileProposePostsAndReportsID(t *testing.T) {
	f := &profileFake{}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	bindProxy(t, srv, "sess")

	out, err := proxyCall(t, "profile", `{"action":"propose","ops":[{"op":"add","text":"tabs","tag":"workflow"}],"reason":"user asked"}`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "proposed profile patch 9 for human approval" {
		t.Fatalf("propose = %q", out)
	}
	if len(f.proposed) != 1 {
		t.Fatalf("proposed = %v, want the ops and reason posted", f.proposed)
	}
}

func TestWebProxyProfileProposeErrorKeepsBody(t *testing.T) {
	for _, tc := range []struct {
		name string
		code int
		body string
		want string
	}{
		{"explicit request", http.StatusBadRequest, "profile patches need an explicit User request mentioning the Profile", "explicit User request"},
		{"cap", http.StatusBadRequest, "profile is 3000 bytes, max 2048: condense it to fewer, shorter lines", "condense"},
		{"barred", http.StatusBadRequest, "lines that assert expertise or lower the review bar cannot carry tag", "cannot carry tag"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &profileFake{proposeCode: tc.code, proposeBody: tc.body}
			srv := httptest.NewServer(f.handler())
			defer srv.Close()
			bindProxy(t, srv, "sess")

			_, err := proxyCall(t, "profile", `{"action":"propose","ops":[{"op":"add","text":"x","tag":"workflow"}],"reason":"r"}`)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("propose error = %v, want %q", err, tc.want)
			}
		})
	}
}
