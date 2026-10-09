package systemone

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// With no env key and no pig on PATH, APIKey reads pig's auth file.
func TestAPIKeyFallsBackToPigAuthFile(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("PATH", t.TempDir())
	path := filepath.Join(t.TempDir(), "auth.json")
	old := authFile
	authFile = func() string { return path }
	t.Cleanup(func() { authFile = old })

	if got := APIKey(context.Background()); got != "" {
		t.Fatalf("missing file: got %q, want empty", got)
	}
	body := `{"openrouter":{"type":"api_key","key":" sk-test \n"},"deepseek":{"type":"api_key","key":"x"}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := APIKey(context.Background()); got != "sk-test" {
		t.Fatalf("got %q, want sk-test", got)
	}
	if err := os.WriteFile(path, []byte(`{"openrouter":{"type":"oauth"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := APIKey(context.Background()); got != "" {
		t.Fatalf("oauth entry: got %q, want empty", got)
	}
}
