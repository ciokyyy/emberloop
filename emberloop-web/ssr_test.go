package web

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"modernc.org/quickjs"
)

func renderDashboard(t testing.TB) string {
	t.Helper()
	bundle, err := os.ReadFile("web/dist/ssr/ssr.js")
	if err != nil {
		t.Fatal(err)
	}
	vm, err := quickjs.NewVM()
	if err != nil {
		t.Fatal(err)
	}
	defer vm.Close()
	if _, err := vm.Eval(string(bundle), quickjs.EvalModule); err != nil {
		t.Fatal(err)
	}
	result, err := vm.Call("__elRenderDashboard")
	if err != nil {
		t.Fatal(err)
	}
	html, ok := result.(string)
	if !ok || html == "" {
		t.Fatalf("SSR returned %T %v, want HTML", result, result)
	}
	return html
}

func TestDashboardSSRSpike(t *testing.T) {
	html := renderDashboard(t)
	if !strings.Contains(html, "All projects") || !strings.Contains(html, "Nothing needs attention") {
		t.Fatalf("SSR did not render the Dashboard with its initial payload: %s", html)
	}
}

func TestDashboardSSRBrowserHydration(t *testing.T) {
	chromium, err := exec.LookPath("chromium")
	if err != nil {
		chromium, err = exec.LookPath("chromium-browser")
	}
	if err != nil {
		t.Skip("Chromium is not installed; browser hydration probe skipped")
	}
	hydrator, err := filepath.Abs("web/dist/ssr/hydrate.iife.js")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(hydrator); err != nil {
		t.Fatalf("build the browser hydrator with npm run build: %v", err)
	}
	hydratorURL := (&url.URL{Scheme: "file", Path: hydrator}).String()
	page := fmt.Sprintf(`<!doctype html><html><body>
<div id="dashboard-root">%s</div>
<script>
window.__EL_SSR_WARNINGS__ = [];
for (const method of ['warn', 'error']) {
  const original = console[method].bind(console);
  console[method] = (...args) => {
    const message = args.map(String).join(' ');
    if (/hydration|mismatch/i.test(message)) window.__EL_SSR_WARNINGS__.push(message);
    original(...args);
  };
}
window.addEventListener('load', () => setTimeout(() => {
  document.body.dataset.ssrHydrated = String(window.__EL_SSR_HYDRATED__ === true);
  document.body.dataset.ssrWarnings = String(window.__EL_SSR_WARNINGS__.length);
}, 0));
</script>
<script src="%s"></script>
</body></html>`, renderDashboard(t), hydratorURL)
	temp := tempDir(t)
	pagePath := filepath.Join(temp, "dashboard.html")
	if err := os.WriteFile(pagePath, []byte(page), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chromium,
		"--headless", "--no-sandbox", "--disable-gpu", "--allow-file-access-from-files",
		"--enable-logging=stderr", "--dump-dom", (&url.URL{Scheme: "file", Path: pagePath}).String(),
	).CombinedOutput()
	if err != nil {
		t.Fatalf("Chromium hydration probe failed: %v\n%s", err, output)
	}
	dom := string(output)
	if !strings.Contains(dom, `data-ssr-hydrated="true"`) {
		t.Fatalf("browser did not run the hydration entry:\n%s", dom)
	}
	if !strings.Contains(dom, `data-ssr-warnings="0"`) {
		t.Fatalf("browser reported an SSR hydration/mismatch warning:\n%s", dom)
	}
}

func BenchmarkDashboardSSRWarm(b *testing.B) {
	bundle, err := os.ReadFile("web/dist/ssr/ssr.js")
	if err != nil {
		b.Fatal(err)
	}
	vm, err := quickjs.NewVM()
	if err != nil {
		b.Fatal(err)
	}
	defer vm.Close()
	if _, err := vm.Eval(string(bundle), quickjs.EvalModule); err != nil {
		b.Fatal(err)
	}
	if _, err := vm.Call("__elRenderDashboard"); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := vm.Call("__elRenderDashboard"); err != nil {
			b.Fatal(err)
		}
	}
}
