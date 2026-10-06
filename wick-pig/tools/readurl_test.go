package tools

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Filler keeps every test body above the 100-non-whitespace thin-page line,
// so Tier 1 success tests do not escalate by accident.
const testFiller = " Lorem ipsum dolor sit amet consectetur adipiscing elit sed do eiusmod tempor incididunt ut labore et dolore magna aliqua enim ad minim veniam quis nostrud exercitation."

func longBody(s string) string { return s + testFiller }

func jinaLong(title string) string {
	return "# " + title + "\n\n" + strings.Repeat("Fallback content from the Jina reader tier with plenty of descriptive words to satisfy every check. ", 6) + "\n"
}

type jinaFake struct {
	server *httptest.Server
	hits   *int32
	auth   *string
	path   *string
	status int
	body   string
}

func startJinaFake(t *testing.T, status int, body string) *jinaFake {
	t.Helper()
	var hits int32
	var auth, path string
	authP, pathP := &auth, &path
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		*authP = r.Header.Get("Authorization")
		*pathP = r.URL.String()
		w.Header().Set("Content-Type", "text/markdown")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("WICK_JINA_ENDPOINT", srv.URL+"/")
	return &jinaFake{server: srv, hits: &hits, auth: authP, path: pathP, status: status, body: body}
}

func (f *jinaFake) count() int { return int(atomic.LoadInt32(f.hits)) }

func allowLocal(t *testing.T) {
	t.Helper()
	t.Setenv("WICK_URL_ALLOW_LOOPBACK", "1")
	t.Setenv("WICK_URL_TIMEOUT", "")
	t.Setenv("WICK_URL_MAX_BODY", "")
}

func blockLocal(t *testing.T) {
	t.Helper()
	t.Setenv("WICK_URL_ALLOW_LOOPBACK", "")
}

func readURLPath(t *testing.T, path string) (string, error) {
	t.Helper()
	return run(t, readTool{}, t.TempDir(), map[string]any{"path": path})
}

func mustContain(t *testing.T, out string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("output missing %q:\n%s", w, out)
		}
	}
}

func mustNotContain(t *testing.T, out string, banned ...string) {
	t.Helper()
	for _, b := range banned {
		if strings.Contains(out, b) {
			t.Errorf("output must not contain %q:\n%s", b, out)
		}
	}
}

func TestReadURLHTMLToMarkdown(t *testing.T) {
	allowLocal(t)
	jf := startJinaFake(t, 200, "should not be used")
	t.Setenv("JINA_API_KEY", "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><head><title>T</title></head><body><nav>nav junk here to remove</nav><h1>Hello World Page</h1><p>This is a long enough paragraph with plenty of words to pass the thin-page check easily and more text to be safe.</p><a href="https://example.com/more">read more here</a></body></html>`))
	}))
	t.Cleanup(srv.Close)

	out, err := readURLPath(t, srv.URL+"/page")
	if err != nil {
		t.Fatalf("read url: %v", err)
	}
	mustContain(t, out, "tier1", "text/html", "untrusted content", "# Hello World Page", "[read more here](https://example.com/more)")
	mustNotContain(t, out, "nav junk")
	if jf.count() != 0 {
		t.Errorf("jina hits = %d, want 0", jf.count())
	}
}

func TestReadURLJSONPretty(t *testing.T) {
	allowLocal(t)
	_ = startJinaFake(t, 200, "unused")
	t.Setenv("JINA_API_KEY", "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"b":2,"a":1,"long":"enough content to pass the thin page check with plenty of characters for safety yes indeed and more words to be certain"}`))
	}))
	t.Cleanup(srv.Close)

	out, err := readURLPath(t, srv.URL+"/api")
	if err != nil {
		t.Fatalf("read url: %v", err)
	}
	mustContain(t, out, "tier1", "application/json", `"a": 1`)
	if !strings.Contains(out, "\n  ") {
		t.Errorf("expected indented JSON:\n%s", out)
	}
}

func TestReadURLTextPassthrough(t *testing.T) {
	allowLocal(t)
	_ = startJinaFake(t, 200, "unused")
	t.Setenv("JINA_API_KEY", "")
	body := longBody("line one here and more content to be long enough for the thin check\nline two with plenty of extra words to be safe\nline three yet more words to cross one hundred characters easily\n")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	out, err := readURLPath(t, srv.URL+"/f.txt")
	if err != nil {
		t.Fatalf("read url: %v", err)
	}
	mustContain(t, out, "tier1", "text/plain", "line one here")
}

func TestReadURLSelectors(t *testing.T) {
	allowLocal(t)
	jf := startJinaFake(t, 200, "unused")
	t.Setenv("JINA_API_KEY", "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		for i := 1; i <= 20; i++ {
			_, _ = w.Write([]byte("alpha-content-line-number-" + strconv.Itoa(i) + "-with-extra-words-to-be-long\n"))
		}
	}))
	t.Cleanup(srv.Close)

	out, err := readURLPath(t, srv.URL+"/f.txt:2-3")
	if err != nil {
		t.Fatalf("range selector: %v", err)
	}
	mustContain(t, out, "alpha-content-line-number-2", "alpha-content-line-number-3")
	if strings.Contains(out, "alpha-content-line-number-20") || strings.Contains(out, "alpha-content-line-number-4\n") {
		t.Errorf("range leaked outside 2-3:\n%s", out)
	}

	out, err = readURLPath(t, srv.URL+"/f.txt:2+2")
	if err != nil {
		t.Fatalf("+ selector: %v", err)
	}
	mustContain(t, out, "alpha-content-line-number-2", "alpha-content-line-number-4")
	if strings.Contains(out, "alpha-content-line-number-5\n") {
		t.Errorf("+ selector leaked:\n%s", out)
	}

	out, err = readURLPath(t, srv.URL+"/f.txt:1,3")
	if err != nil {
		t.Fatalf("comma list: %v", err)
	}
	mustContain(t, out, "alpha-content-line-number-1", "alpha-content-line-number-3")

	if jf.count() != 0 {
		t.Errorf("jina hits = %d, want 0", jf.count())
	}
}

func TestReadURLRawReturnsBodyUnshaped(t *testing.T) {
	allowLocal(t)
	jf := startJinaFake(t, 200, jinaLong("unused"))
	t.Setenv("JINA_API_KEY", "")
	rawHTML := `<html><body><h1>Hi</h1><p>x</p><p>captcha challenge just a moment cloudflare</p></body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(rawHTML))
	}))
	t.Cleanup(srv.Close)

	out, err := readURLPath(t, srv.URL+"/page:raw")
	if err != nil {
		t.Fatalf("raw: %v", err)
	}
	mustContain(t, out, "tier1", "<h1>Hi</h1>")
	if jf.count() != 0 {
		t.Errorf("raw must never escalate: jina hits = %d", jf.count())
	}
}

func TestReadURLCollapsedScheme(t *testing.T) {
	allowLocal(t)
	_ = startJinaFake(t, 200, "unused")
	t.Setenv("JINA_API_KEY", "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(longBody("collapsed scheme target with plenty of extra words to pass the thin page check easily\n")))
	}))
	t.Cleanup(srv.Close)

	collapsed := strings.Replace(srv.URL+"/ok", "://", ":/", 1)
	out, err := readURLPath(t, collapsed)
	if err != nil {
		t.Fatalf("collapsed scheme %q: %v", collapsed, err)
	}
	mustContain(t, out, "tier1", "collapsed scheme target")
}

func TestReadURLPortIsNotASelector(t *testing.T) {
	allowLocal(t)
	_ = startJinaFake(t, 200, "unused")
	t.Setenv("JINA_API_KEY", "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(longBody("port target with plenty of extra words to pass the thin page check easily\n")))
	}))
	t.Cleanup(srv.Close)

	out, err := readURLPath(t, srv.URL)
	if err != nil {
		t.Fatalf("bare host:port %q: %v", srv.URL, err)
	}
	mustContain(t, out, "tier1", "port target")
}

func TestReadURLEscalationStatuses(t *testing.T) {
	for _, status := range []int{403, 429, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			allowLocal(t)
			jf := startJinaFake(t, 200, jinaLong("Jina markdown"))
			t.Setenv("JINA_API_KEY", "")
			var originHits int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&originHits, 1)
				w.WriteHeader(status)
				_, _ = w.Write([]byte("denied"))
			}))
			t.Cleanup(srv.Close)

			out, err := readURLPath(t, srv.URL+"/blocked")
			if err != nil {
				t.Fatalf("status %d: %v", status, err)
			}
			mustContain(t, out, "tier2", "Jina markdown")
			if jf.count() != 1 {
				t.Errorf("jina hits = %d, want 1", jf.count())
			}
			if status == 429 && atomic.LoadInt32(&originHits) != 2 {
				t.Errorf("429 must retry once: origin hits = %d, want 2", originHits)
			}
		})
	}
}

func TestReadURLChallengeAndThinEscalate(t *testing.T) {
	t.Run("challenge", func(t *testing.T) {
		allowLocal(t)
		jf := startJinaFake(t, 200, jinaLong("Jina fallback"))
		t.Setenv("JINA_API_KEY", "")
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><body><h1>Checking your browser</h1><p>Just a moment, please verify you are human with this captcha cloudflare challenge page and more text to be long.</p></body></html>`))
		}))
		t.Cleanup(srv.Close)
		out, err := readURLPath(t, srv.URL+"/c")
		if err != nil {
			t.Fatalf("challenge: %v", err)
		}
		mustContain(t, out, "tier2")
		if jf.count() != 1 {
			t.Errorf("jina hits = %d, want 1", jf.count())
		}
	})
	t.Run("thin", func(t *testing.T) {
		allowLocal(t)
		jf := startJinaFake(t, 200, jinaLong("Jina fallback"))
		t.Setenv("JINA_API_KEY", "")
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("hi\n"))
		}))
		t.Cleanup(srv.Close)
		out, err := readURLPath(t, srv.URL+"/thin")
		if err != nil {
			t.Fatalf("thin: %v", err)
		}
		mustContain(t, out, "tier2")
		if jf.count() != 1 {
			t.Errorf("jina hits = %d, want 1", jf.count())
		}
	})
}

func TestReadURLTimeoutEscalates(t *testing.T) {
	allowLocal(t)
	t.Setenv("WICK_URL_TIMEOUT", "200ms")
	jf := startJinaFake(t, 200, jinaLong("Jina after timeout"))
	t.Setenv("JINA_API_KEY", "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		_, _ = w.Write([]byte("too slow"))
	}))
	t.Cleanup(srv.Close)
	out, err := readURLPath(t, srv.URL+"/slow")
	if err != nil {
		t.Fatalf("slow: %v", err)
	}
	mustContain(t, out, "tier2", "Jina after timeout")
	if jf.count() != 1 {
		t.Errorf("jina hits = %d, want 1", jf.count())
	}
}

func TestReadURLPDFStraightToJina(t *testing.T) {
	allowLocal(t)
	jf := startJinaFake(t, 200, jinaLong("PDF via Jina"))
	t.Setenv("JINA_API_KEY", "test-key-123")
	var originHits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&originHits, 1)
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-1.4 fake"))
	}))
	t.Cleanup(srv.Close)

	out, err := readURLPath(t, srv.URL+"/paper.pdf")
	if err != nil {
		t.Fatalf("pdf: %v", err)
	}
	mustContain(t, out, "tier2", "PDF via Jina", "jina via API key")
	if originHits != 0 {
		t.Errorf("pdf must skip tier 1: origin hits = %d", originHits)
	}
	if jf.count() != 1 {
		t.Errorf("jina hits = %d, want 1", jf.count())
	}
	if got := *jf.auth; got != "Bearer test-key-123" {
		t.Errorf("jina auth = %q, want bearer key", got)
	}
}

func TestReadURLOfficeDocStraightToJina(t *testing.T) {
	allowLocal(t)
	jf := startJinaFake(t, 200, jinaLong("Doc via Jina"))
	t.Setenv("JINA_API_KEY", "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte("fake"))
	}))
	t.Cleanup(srv.Close)

	out, err := readURLPath(t, srv.URL+"/report.docx")
	if err != nil {
		t.Fatalf("docx: %v", err)
	}
	mustContain(t, out, "tier2", "Doc via Jina")
	if jf.count() != 1 {
		t.Errorf("jina hits = %d, want 1", jf.count())
	}
}

func TestReadURLPDFContentTypeEscalates(t *testing.T) {
	allowLocal(t)
	jf := startJinaFake(t, 200, jinaLong("PDF content"))
	t.Setenv("JINA_API_KEY", "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-1.4 fake"))
	}))
	t.Cleanup(srv.Close)

	out, err := readURLPath(t, srv.URL+"/file")
	if err != nil {
		t.Fatalf("pdf content-type: %v", err)
	}
	mustContain(t, out, "tier2")
	if jf.count() != 1 {
		t.Errorf("jina hits = %d, want 1", jf.count())
	}
}

func TestReadURLBlockedAddresses(t *testing.T) {
	blockLocal(t)
	_ = startJinaFake(t, 200, "must not be used")
	t.Setenv("JINA_API_KEY", "")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("x"))
	}))
	t.Cleanup(srv.Close)

	if _, err := readURLPath(t, srv.URL+"/x"); err == nil || !strings.Contains(strings.ToLower(err.Error()), "loopback") {
		t.Errorf("loopback without allow must be refused, got %v", err)
	}
	for _, target := range []string{"http://10.0.0.1/", "http://192.168.1.1/", "http://169.254.169.254/"} {
		if _, err := readURLPath(t, target); err == nil || !strings.Contains(err.Error(), "refused") {
			t.Errorf("%s must be refused, got %v", target, err)
		}
	}
}

func TestReadURLRedirectToPrivateRefused(t *testing.T) {
	allowLocal(t)
	jf := startJinaFake(t, 200, "must not be used")
	t.Setenv("JINA_API_KEY", "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://10.0.0.1/secret", http.StatusFound)
	}))
	t.Cleanup(srv.Close)

	_, err := readURLPath(t, srv.URL+"/go")
	if err == nil || (!strings.Contains(err.Error(), "refused") && !strings.Contains(err.Error(), "private")) {
		t.Errorf("redirect to private must be refused, got %v", err)
	}
	if jf.count() != 0 {
		t.Errorf("blocked redirect must never reach jina: hits = %d", jf.count())
	}
}

func TestReadURLCredentialsSkipJina(t *testing.T) {
	allowLocal(t)
	jf := startJinaFake(t, 200, "must not be used")
	t.Setenv("JINA_API_KEY", "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(longBody("credential page with plenty of extra words to pass the thin page check easily\n")))
	}))
	t.Cleanup(srv.Close)

	withCreds := strings.Replace(srv.URL+"/page", "://", "://user:pass@", 1)
	out, err := readURLPath(t, withCreds)
	if err != nil {
		t.Fatalf("credentialed url: %v", err)
	}
	mustContain(t, out, "tier1", "jina skipped", "credentials")
	if jf.count() != 0 {
		t.Errorf("credentialed url must never reach jina: hits = %d", jf.count())
	}
}

func TestReadURLTokenParamsSkipJina(t *testing.T) {
	allowLocal(t)
	jf := startJinaFake(t, 200, "must not be used")
	t.Setenv("JINA_API_KEY", "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(longBody("token page with plenty of extra words to pass the thin page check easily\n")))
	}))
	t.Cleanup(srv.Close)

	out, err := readURLPath(t, srv.URL+"/page?token=secret123")
	if err != nil {
		t.Fatalf("token url: %v", err)
	}
	mustContain(t, out, "tier1", "jina skipped", "token")
	if jf.count() != 0 {
		t.Errorf("token url must never reach jina: hits = %d", jf.count())
	}

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		_, _ = w.Write([]byte("denied"))
	}))
	t.Cleanup(srv2.Close)
	_, err = readURLPath(t, srv2.URL+"/page?api_key=abc")
	if err == nil || !strings.Contains(err.Error(), "not installed") || !strings.Contains(strings.ToLower(err.Error()), "jina skipped") {
		t.Errorf("tokened 403 must end at tier 3 with jina skipped, got %v", err)
	}
	if jf.count() != 0 {
		t.Errorf("tokened escalation must never reach jina: hits = %d", jf.count())
	}
}

func TestReadURLOversizeCapped(t *testing.T) {
	allowLocal(t)
	jf := startJinaFake(t, 200, "unused")
	t.Setenv("JINA_API_KEY", "")
	t.Setenv("WICK_URL_MAX_BODY", "200")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(strings.Repeat("oversize-body-words ", 100) + "\n"))
	}))
	t.Cleanup(srv.Close)

	out, err := readURLPath(t, srv.URL+"/big")
	if err != nil {
		t.Fatalf("oversize: %v", err)
	}
	mustContain(t, out, "tier1", "capped")
	if jf.count() != 0 {
		t.Errorf("oversize must not escalate: jina hits = %d", jf.count())
	}
}

func TestReadURLTruncationOverflowFile(t *testing.T) {
	allowLocal(t)
	_ = startJinaFake(t, 200, "unused")
	t.Setenv("JINA_API_KEY", "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		for i := 1; i <= 350; i++ {
			fmt.Fprintf(w, "overflow-line-%03d-with-extra-words-to-be-long-enough\n", i)
		}
	}))
	t.Cleanup(srv.Close)

	out, err := readURLPath(t, srv.URL+"/long")
	if err != nil {
		t.Fatalf("long: %v", err)
	}
	mustContain(t, out, "tier1", "truncated", "saved to")
	mustContain(t, out, "overflow-line-001")
	mustNotContain(t, out, "overflow-line-350")

	var overflow string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "saved to ") {
			fields := strings.Fields(line)
			for i, f := range fields {
				if f == "to" && i+1 < len(fields) {
					overflow = strings.Trim(fields[i+1], ";,")
				}
			}
		}
	}
	if overflow == "" {
		t.Fatalf("no overflow path in output:\n%s", out)
	}
	data, err := os.ReadFile(filepath.Clean(overflow))
	t.Cleanup(func() { _ = os.Remove(overflow) })
	if err != nil {
		t.Fatalf("overflow file: %v", err)
	}
	if !strings.Contains(string(data), "overflow-line-350") || !strings.Contains(string(data), "overflow-line-001") {
		t.Errorf("overflow file must hold the full body, got %d bytes", len(data))
	}
}

func TestReadURLTier3NotInstalled(t *testing.T) {
	allowLocal(t)
	_ = startJinaFake(t, 503, "jina down")
	t.Setenv("JINA_API_KEY", "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		_, _ = w.Write([]byte("denied"))
	}))
	t.Cleanup(srv.Close)

	_, err := readURLPath(t, srv.URL+"/dead")
	if err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("tier 3 must report not installed, got %v", err)
	}
}

func TestReadURLJinaAnonymousNoted(t *testing.T) {
	allowLocal(t)
	_ = startJinaFake(t, 200, jinaLong("Anonymous fallback"))
	t.Setenv("JINA_API_KEY", "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		_, _ = w.Write([]byte("denied"))
	}))
	t.Cleanup(srv.Close)

	out, err := readURLPath(t, srv.URL+"/x")
	if err != nil {
		t.Fatalf("anonymous jina: %v", err)
	}
	mustContain(t, out, "tier2", "anonymous", "JINA_API_KEY")
}

func TestPeelURLSelectors(t *testing.T) {
	cases := []struct {
		in       string
		url      string
		raw      bool
		spec     string
		isURL    bool
		repaired string
	}{
		{"https://example.com/page:1-10", "https://example.com/page", false, "1-10", true, ""},
		{"https://example.com/page:raw", "https://example.com/page", true, "", true, ""},
		{"https://example.com/page:10-20:raw", "https://example.com/page", true, "10-20", true, ""},
		{"https://example.com/page:raw:10-20", "https://example.com/page", true, "10-20", true, ""},
		{"http://127.0.0.1:8080/x", "http://127.0.0.1:8080/x", false, "", true, ""},
		{"http://127.0.0.1:8080", "http://127.0.0.1:8080", false, "", true, ""},
		{"www.example.com/page", "https://www.example.com/page", false, "", true, "https://www.example.com/page"},
		{"https:/example.com/page", "https://example.com/page", false, "", true, "https://example.com/page"},
		{"notes.md", "notes.md", false, "", false, ""},
	}
	for _, c := range cases {
		if got := isURLTarget(c.in); got != c.isURL {
			t.Errorf("isURLTarget(%q) = %v, want %v", c.in, got, c.isURL)
		}
		u, raw, spec := peelURLSelectors(normalizeURLTarget(c.in))
		if c.isURL && u != c.url {
			t.Errorf("peel(%q) url = %q, want %q", c.in, u, c.url)
		}
		if raw != c.raw || spec != c.spec {
			t.Errorf("peel(%q) = (%q,%v,%q), want raw=%v spec=%q", c.in, u, raw, spec, c.raw, c.spec)
		}
		if c.repaired != "" && normalizeURLTarget(c.in) != c.repaired {
			t.Errorf("normalize(%q) = %q, want %q", c.in, normalizeURLTarget(c.in), c.repaired)
		}
	}
}
