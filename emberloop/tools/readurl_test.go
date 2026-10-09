package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
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

func parseIPForTest(t *testing.T, s string) net.IP {
	t.Helper()
	ip := net.ParseIP(s)
	if ip == nil {
		t.Fatalf("bad test ip %q", s)
	}
	return ip
}

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
	t.Setenv("EL_JINA_ENDPOINT", srv.URL+"/")
	return &jinaFake{server: srv, hits: &hits, auth: authP, path: pathP, status: status, body: body}
}

func (f *jinaFake) count() int { return int(atomic.LoadInt32(f.hits)) }

func allowLocal(t *testing.T) {
	t.Helper()
	t.Setenv("EL_URL_ALLOW_LOOPBACK", "1")
	t.Setenv("EL_URL_TIMEOUT", "")
	t.Setenv("EL_URL_MAX_BODY", "")
}

func blockLocal(t *testing.T) {
	t.Helper()
	t.Setenv("EL_URL_ALLOW_LOOPBACK", "")
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
	t.Setenv("EL_URL_TIMEOUT", "200ms")
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
	t.Setenv("EL_CHROME_BIN", filepath.Join(t.TempDir(), "no-such-browser"))
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
	t.Setenv("EL_URL_MAX_BODY", "200")
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

// A page larger than the overflow threshold is stored, not truncated to a
// file: the output carries a ctx: handle and the full body pages through it.
func TestReadURLLargePageUsesOverflowStore(t *testing.T) {
	allowLocal(t)
	_ = startJinaFake(t, 200, "unused")
	t.Setenv("JINA_API_KEY", "")
	t.Setenv("EL_STATE_DIR", t.TempDir())
	t.Setenv("EL_OVERFLOW_THRESHOLD_BYTES", "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		for i := 1; i <= 350; i++ {
			fmt.Fprintf(w, "overflow-line-%03d-with-extra-words-to-be-long-enough\n", i)
		}
	}))
	t.Cleanup(srv.Close)

	root := t.TempDir()
	raw, _ := json.Marshal(map[string]any{"path": srv.URL + "/long"})
	out, err := readTool{}.Execute(context.Background(), root, raw)
	if err != nil {
		t.Fatalf("long: %v", err)
	}
	mustContain(t, out, "tier1", "ctx:", "overflow-line-001")
	mustNotContain(t, out, "saved to")
	var handle string
	for _, f := range strings.Fields(out) {
		if strings.HasPrefix(f, "ctx:") {
			cand := strings.Trim(f, "(),;")
			if _, _, ok := ParseCtxHandle(cand); ok {
				handle = cand
				break
			}
		}
	}
	if handle == "" {
		t.Fatalf("no ctx handle in output:\n%s", out)
	}
	id, _, ok := ParseCtxHandle(handle)
	if !ok {
		t.Fatalf("unparsable handle %q", handle)
	}
	praw, _ := json.Marshal(map[string]any{"path": fmt.Sprintf("ctx:%d:349-350", id)})
	paged, err := readTool{}.Execute(context.Background(), root, praw)
	if err != nil {
		t.Fatalf("page stored: %v", err)
	}
	mustContain(t, paged, "overflow-line-349", "overflow-line-350")
	mustNotContain(t, paged, "overflow-line-001")
}

func TestReadURLTier3NotInstalled(t *testing.T) {
	allowLocal(t)
	t.Setenv("EL_CHROME_BIN", filepath.Join(t.TempDir(), "no-such-browser"))
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
func TestJinaDefaultEndpoint(t *testing.T) {
	t.Setenv("EL_JINA_ENDPOINT", "")
	if got := jinaBase(); got != "https://r.jina.ai/" {
		t.Fatalf("jinaBase() = %q, want the public reader", got)
	}
	if got, want := jinaRequestURL("https://example.com/page"), "https://r.jina.ai/https://example.com/page"; got != want {
		t.Fatalf("jinaRequestURL = %q, want %q", got, want)
	}
	t.Setenv("EL_JINA_ENDPOINT", "http://127.0.0.1:9999/custom/")
	if got, want := jinaRequestURL("https://example.com/page"), "http://127.0.0.1:9999/custom/https://example.com/page"; got != want {
		t.Fatalf("jinaRequestURL with override = %q, want %q", got, want)
	}
}

func TestReadURLRawPDFSuffixReturnsRaw(t *testing.T) {
	allowLocal(t)
	jf := startJinaFake(t, 200, jinaLong("must not be used"))
	t.Setenv("JINA_API_KEY", "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte(longBody("%PDF-1.4 raw bytes with plenty of extra words to be long enough for raw\n")))
	}))
	t.Cleanup(srv.Close)

	out, err := readURLPath(t, srv.URL+"/paper.pdf:raw")
	if err != nil {
		t.Fatalf("raw pdf: %v", err)
	}
	mustContain(t, out, "tier1", "%PDF-1.4")
	if jf.count() != 0 {
		t.Errorf(":raw must never escalate, even for pdf suffixes: jina hits = %d", jf.count())
	}
}

func TestRemoteIPBlocked(t *testing.T) {
	t.Setenv("EL_URL_ALLOW_LOOPBACK", "")
	for _, ip := range []string{"127.0.0.1", "::1", "10.0.0.1", "192.168.1.1", "169.254.169.254", "0.0.0.0"} {
		if reason := remoteIPBlocked(parseIPForTest(t, ip)); reason == "" {
			t.Errorf("remoteIPBlocked(%s) = allowed, want refused", ip)
		}
	}
	if reason := remoteIPBlocked(parseIPForTest(t, "8.8.8.8")); reason != "" {
		t.Errorf("remoteIPBlocked(8.8.8.8) = %q, want allowed", reason)
	}
	t.Setenv("EL_URL_ALLOW_LOOPBACK", "1")
	if reason := remoteIPBlocked(parseIPForTest(t, "127.0.0.1")); reason != "" {
		t.Errorf("loopback with allow flag = %q, want allowed", reason)
	}
	if reason := remoteIPBlocked(parseIPForTest(t, "10.0.0.1")); reason == "" {
		t.Errorf("private with allow-loopback flag must still be refused")
	}
}

// A large page read with an explicit range answers the range inline, with
// the full output's handle alongside: selectors are honoured above the
// threshold, not replaced by the summary.
func TestReadURLLargePageWithSelectorAnswersInline(t *testing.T) {
	allowLocal(t)
	_ = startJinaFake(t, 200, "unused")
	t.Setenv("JINA_API_KEY", "")
	t.Setenv("EL_STATE_DIR", t.TempDir())
	t.Setenv("EL_OVERFLOW_THRESHOLD_BYTES", "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		for i := 1; i <= 350; i++ {
			fmt.Fprintf(w, "overflow-line-%03d-with-extra-words-to-be-long-enough\n", i)
		}
	}))
	t.Cleanup(srv.Close)

	root := t.TempDir()
	raw, _ := json.Marshal(map[string]any{"path": srv.URL + "/long:340-342"})
	out, err := readTool{}.Execute(context.Background(), root, raw)
	if err != nil {
		t.Fatalf("ranged long: %v", err)
	}
	mustContain(t, out, "tier1", "ctx:", "stored as", "overflow-line-340", "overflow-line-342")
	mustNotContain(t, out, "overflow-line-100", "head (")
}

// startBrowserFake installs a fake chrome-headless-shell: a script that
// records its arguments and prints the canned body, so tier 3 is exercised
// through the public read contract without a real browser.
func startBrowserFake(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	bodyFile := filepath.Join(dir, "body.html")
	if err := os.WriteFile(bodyFile, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "chrome-headless-shell")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + argsFile + "\ncat " + bodyFile + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EL_CHROME_BIN", bin)
	return argsFile
}

func TestReadURLTier3BrowserRendersThinPage(t *testing.T) {
	allowLocal(t)
	jf := startJinaFake(t, 503, "jina down")
	t.Setenv("JINA_API_KEY", "")
	argsFile := startBrowserFake(t, `<html><body><h1>Rendered headline here</h1><p>`+strings.Repeat("Browser rendered words with plenty of descriptive text to be long. ", 6)+`</p></body></html>`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><title>App</title></head><body><div id="app"></div></body></html>`))
	}))
	t.Cleanup(srv.Close)

	out, err := readURLPath(t, srv.URL+"/app")
	if err != nil {
		t.Fatalf("thin page via browser: %v", err)
	}
	mustContain(t, out, "tier3", "Rendered headline here", "untrusted content")
	if jf.count() != 1 {
		t.Errorf("jina hits = %d, want 1", jf.count())
	}
	rawArgs, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("browser fake never ran: %v", err)
	}
	if !strings.Contains(string(rawArgs), srv.URL+"/app") {
		t.Errorf("browser got args %q, want the page URL", string(rawArgs))
	}
}

func TestReadURLTier3BrowserRendersChallengePage(t *testing.T) {
	allowLocal(t)
	jf := startJinaFake(t, 503, "jina down")
	t.Setenv("JINA_API_KEY", "")
	startBrowserFake(t, `<html><body><h1>Real article text</h1><p>`+strings.Repeat("Human readable article words with plenty of descriptive text. ", 6)+`</p></body></html>`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><h1>Checking your browser</h1><p>Just a moment, please verify you are human with this captcha cloudflare challenge page and more text to be long.</p></body></html>`))
	}))
	t.Cleanup(srv.Close)

	out, err := readURLPath(t, srv.URL+"/gated")
	if err != nil {
		t.Fatalf("challenge via browser: %v", err)
	}
	mustContain(t, out, "tier3", "Real article text")
	if jf.count() != 1 {
		t.Errorf("jina hits = %d, want 1", jf.count())
	}
}

func TestReadURLTier3BlockedAddressNeverLaunchesBrowser(t *testing.T) {
	allowLocal(t)
	_ = startJinaFake(t, 200, "must not be used")
	t.Setenv("JINA_API_KEY", "")
	argsFile := startBrowserFake(t, "<html><body>must not run</body></html>")

	_, err := readURLPath(t, "http://10.0.0.1/secret")
	if err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("private target must be refused, got %v", err)
	}
	if _, serr := os.Stat(argsFile); !os.IsNotExist(serr) {
		t.Errorf("browser launched for a blocked address")
	}
}

func TestReadURLTier3CredentialedPageUsesBrowserNotJina(t *testing.T) {
	allowLocal(t)
	jf := startJinaFake(t, 200, "must not be used")
	t.Setenv("JINA_API_KEY", "")
	startBrowserFake(t, `<html><body><h1>Secret dashboard page</h1><p>`+strings.Repeat("Authenticated content words with plenty of descriptive text. ", 6)+`</p></body></html>`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		_, _ = w.Write([]byte("denied"))
	}))
	t.Cleanup(srv.Close)

	withCreds := strings.Replace(srv.URL+"/page?token=secret123", "://", "://user:pass@", 1)
	out, err := readURLPath(t, withCreds)
	if err != nil {
		t.Fatalf("credentialed page via browser: %v", err)
	}
	mustContain(t, out, "tier3", "Secret dashboard page", "jina skipped")
	if jf.count() != 0 {
		t.Errorf("credentialed escalation must never reach jina: hits = %d", jf.count())
	}
}

func TestReadURLTier3Timeout(t *testing.T) {
	allowLocal(t)
	_ = startJinaFake(t, 503, "jina down")
	t.Setenv("JINA_API_KEY", "")
	t.Setenv("EL_URL_TIMEOUT", "200ms")
	dir := t.TempDir()
	bin := filepath.Join(dir, "chrome-headless-shell")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nsleep 5\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EL_CHROME_BIN", bin)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		_, _ = w.Write([]byte("denied"))
	}))
	t.Cleanup(srv.Close)

	_, err := readURLPath(t, srv.URL+"/slow")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "timed out") {
		t.Fatalf("slow browser must time out, got %v", err)
	}
}

func TestReadURLTier3OversizeCapped(t *testing.T) {
	allowLocal(t)
	_ = startJinaFake(t, 503, "jina down")
	t.Setenv("JINA_API_KEY", "")
	t.Setenv("EL_URL_MAX_BODY", "200")
	startBrowserFake(t, "<html><body><p>"+strings.Repeat("browser oversize words ", 100)+"</p></body></html>")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		_, _ = w.Write([]byte("denied"))
	}))
	t.Cleanup(srv.Close)

	out, err := readURLPath(t, srv.URL+"/big")
	if err != nil {
		t.Fatalf("oversize browser page: %v", err)
	}
	mustContain(t, out, "tier3", "capped")
}

func writeExecFake(t *testing.T, dir, name, script string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReadURLTier3LaunchFailureReportsStderr(t *testing.T) {
	allowLocal(t)
	_ = startJinaFake(t, 503, "jina down")
	t.Setenv("JINA_API_KEY", "")
	bin := writeExecFake(t, t.TempDir(), "chrome-headless-shell", "#!/bin/sh\necho boom happened >&2\nexit 3\n")
	t.Setenv("EL_CHROME_BIN", bin)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		_, _ = w.Write([]byte("denied"))
	}))
	t.Cleanup(srv.Close)

	_, err := readURLPath(t, srv.URL+"/dead")
	if err == nil || !strings.Contains(err.Error(), "failed") || !strings.Contains(err.Error(), "boom happened") {
		t.Fatalf("launch failure must report stderr, got %v", err)
	}
}

func TestReadURLTier3EmptyPage(t *testing.T) {
	allowLocal(t)
	_ = startJinaFake(t, 503, "jina down")
	t.Setenv("JINA_API_KEY", "")
	bin := writeExecFake(t, t.TempDir(), "chrome-headless-shell", "#!/bin/sh\nexit 0\n")
	t.Setenv("EL_CHROME_BIN", bin)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		_, _ = w.Write([]byte("denied"))
	}))
	t.Cleanup(srv.Close)

	_, err := readURLPath(t, srv.URL+"/dead")
	if err == nil || !strings.Contains(err.Error(), "empty page") {
		t.Fatalf("empty render must error, got %v", err)
	}
}

func TestReadURLTier3ShellFallbackToFullChrome(t *testing.T) {
	allowLocal(t)
	_ = startJinaFake(t, 503, "jina down")
	t.Setenv("JINA_API_KEY", "")
	dir := t.TempDir()
	writeExecFake(t, dir, "chrome-headless-shell", "#!/bin/sh\nexit 1\n")
	bodyFile := filepath.Join(dir, "body.html")
	if err := os.WriteFile(bodyFile, []byte(`<html><body><h1>Fallback chrome render</h1><p>`+strings.Repeat("Full chrome words with plenty of descriptive text. ", 6)+`</p></body></html>`), 0o644); err != nil {
		t.Fatal(err)
	}
	writeExecFake(t, dir, "chromium", "#!/bin/sh\ncat "+bodyFile+"\n")
	t.Setenv("EL_BIN_DIR", dir)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		_, _ = w.Write([]byte("denied"))
	}))
	t.Cleanup(srv.Close)

	out, err := readURLPath(t, srv.URL+"/dead")
	if err != nil {
		t.Fatalf("shell failure must fall back to full chrome: %v", err)
	}
	mustContain(t, out, "tier3", "Fallback chrome render")
}

func TestReadURLTier3ShellFirstWins(t *testing.T) {
	allowLocal(t)
	_ = startJinaFake(t, 503, "jina down")
	t.Setenv("JINA_API_KEY", "")
	dir := t.TempDir()
	shellBody := filepath.Join(dir, "shell.html")
	if err := os.WriteFile(shellBody, []byte(`<html><body><h1>Shell render wins</h1><p>`+strings.Repeat("Shell words with plenty of descriptive text. ", 6)+`</p></body></html>`), 0o644); err != nil {
		t.Fatal(err)
	}
	writeExecFake(t, dir, "chrome-headless-shell", "#!/bin/sh\ncat "+shellBody+"\n")
	otherBody := filepath.Join(dir, "other.html")
	if err := os.WriteFile(otherBody, []byte(`<html><body><h1>Other render loses</h1><p>`+strings.Repeat("Other words with plenty of descriptive text. ", 6)+`</p></body></html>`), 0o644); err != nil {
		t.Fatal(err)
	}
	writeExecFake(t, dir, "chromium", "#!/bin/sh\ncat "+otherBody+"\n")
	t.Setenv("EL_BIN_DIR", dir)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		_, _ = w.Write([]byte("denied"))
	}))
	t.Cleanup(srv.Close)

	out, err := readURLPath(t, srv.URL+"/dead")
	if err != nil {
		t.Fatalf("shell render: %v", err)
	}
	mustContain(t, out, "tier3", "Shell render wins")
	mustNotContain(t, out, "Other render loses")
}

func TestReadURLTier3LaunchArgs(t *testing.T) {
	allowLocal(t)
	_ = startJinaFake(t, 503, "jina down")
	t.Setenv("JINA_API_KEY", "")
	argsFile := startBrowserFake(t, `<html><body><h1>Args probe page</h1><p>`+strings.Repeat("Args probe words with plenty of descriptive text. ", 6)+`</p></body></html>`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		_, _ = w.Write([]byte("denied"))
	}))
	t.Cleanup(srv.Close)

	if _, err := readURLPath(t, srv.URL+"/dead"); err != nil {
		t.Fatalf("args probe: %v", err)
	}
	raw, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	args := string(raw)
	for _, want := range []string{"--dump-dom", "--user-data-dir=", "--host-resolver-rules=", "--disable-dev-shm-usage"} {
		if !strings.Contains(args, want) {
			t.Errorf("browser args missing %q:\n%s", want, args)
		}
	}
	if os.Geteuid() == 0 || os.Getenv("EL_CHROME_NO_SANDBOX") == "1" {
		if !strings.Contains(args, "--no-sandbox") {
			t.Errorf("root must pass --no-sandbox:\n%s", args)
		}
	} else if strings.Contains(args, "--no-sandbox") {
		t.Errorf("non-root must keep the browser sandbox:\n%s", args)
	}
}

func TestReadURLTier3RedirectResolvedBeforeLaunch(t *testing.T) {
	allowLocal(t)
	jf := startJinaFake(t, 503, "jina down")
	t.Setenv("JINA_API_KEY", "")
	argsFile := startBrowserFake(t, `<html><body><h1>Final page render</h1><p>`+strings.Repeat("Final page words with plenty of descriptive text. ", 6)+`</p></body></html>`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/start":
			http.Redirect(w, r, "/final", http.StatusFound)
		default:
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><head><title>App</title></head><body><div id="app"></div></body></html>`))
		}
	}))
	t.Cleanup(srv.Close)

	out, err := readURLPath(t, srv.URL+"/start")
	if err != nil {
		t.Fatalf("redirected page via browser: %v", err)
	}
	mustContain(t, out, "tier3", "Final page render")
	if jf.count() != 1 {
		t.Errorf("jina hits = %d, want 1", jf.count())
	}
	raw, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "/final") {
		t.Errorf("browser must launch at the resolved final URL, got:\n%s", string(raw))
	}
}

func TestBrowserHostBlockRules(t *testing.T) {
	t.Setenv("EL_URL_ALLOW_LOOPBACK", "")
	blocked := browserHostBlockRules()
	for _, want := range []string{"MAP 10.* ~NOTFOUND", "MAP 192.168.* ~NOTFOUND", "MAP 169.254.* ~NOTFOUND", "MAP 127.* ~NOTFOUND", "MAP 0.0.0.0 ~NOTFOUND", "MAP 172.31.* ~NOTFOUND"} {
		if !strings.Contains(blocked, want) {
			t.Errorf("rules missing %q: %s", want, blocked)
		}
	}
	t.Setenv("EL_URL_ALLOW_LOOPBACK", "1")
	if strings.Contains(browserHostBlockRules(), "MAP 127.* ~NOTFOUND") {
		t.Errorf("allow-loopback must lift only the loopback rule: %s", browserHostBlockRules())
	}
	if !strings.Contains(browserHostBlockRules(), "MAP 10.* ~NOTFOUND") {
		t.Errorf("allow-loopback must keep private ranges blocked: %s", browserHostBlockRules())
	}
}

func TestReadURLTier3ErrorPageRefused(t *testing.T) {
	allowLocal(t)
	_ = startJinaFake(t, 503, "jina down")
	t.Setenv("JINA_API_KEY", "")
	bin := writeExecFake(t, t.TempDir(), "chrome-headless-shell",
		"#!/bin/sh\nprintf '%s\\n' '<html><body class=\"main-frame-error\"><div>ERR_NAME_NOT_RESOLVED</div><p>"+strings.Repeat("Navigation error words with plenty of descriptive text. ", 6)+"</p></body></html>'\n")
	t.Setenv("EL_CHROME_BIN", bin)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		_, _ = w.Write([]byte("denied"))
	}))
	t.Cleanup(srv.Close)

	_, err := readURLPath(t, srv.URL+"/dead")
	if err == nil || !strings.Contains(err.Error(), "did not load") {
		t.Fatalf("navigation-error dump must fail, got %v", err)
	}
}

func TestIsBrowserErrorPage(t *testing.T) {
	if !isBrowserErrorPage(`<body class="main-frame-error">ERR_NAME_NOT_RESOLVED</body>`) {
		t.Errorf("error dump not detected")
	}
	if isBrowserErrorPage(`<html><body><p>A page mentioning chromewebdata in prose ` + strings.Repeat("with words ", 20) + `</p></body></html>`) {
		t.Errorf("prose mentioning chromewebdata must not count as an error dump")
	}
}
