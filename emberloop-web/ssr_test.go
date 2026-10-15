package web

import (
	"os"
	"strings"
	"testing"

	"modernc.org/quickjs"
)

func TestDashboardSSRSpike(t *testing.T) {
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
	if !strings.Contains(html, "All projects") || !strings.Contains(html, "Loading Dashboard") {
		t.Fatalf("SSR did not render the Dashboard shell: %s", html)
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
