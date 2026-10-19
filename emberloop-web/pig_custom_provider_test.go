package web

import (
	"os"
	"path/filepath"
	"testing"
)

// A models.json provider the SDK does not list is offered once it holds a
// stored credential or its apiKey environment variable is set.
func TestPigAvailableModelsOffersCustomProviderWithKey(t *testing.T) {
	dir := tempDir(t)
	models := `{"providers":{"acme":{"apiKey":"ACME_TEST_KEY","models":[{"id":"fast"}]}}}`
	if err := os.WriteFile(filepath.Join(dir, "models.json"), []byte(models), 0o600); err != nil {
		t.Fatal(err)
	}
	has := func() bool {
		for _, m := range PigAvailableModels(dir) {
			if m == "acme/fast" {
				return true
			}
		}
		return false
	}
	if has() {
		t.Fatal("custom provider offered without a key")
	}
	t.Setenv("ACME_TEST_KEY", "k")
	if !has() {
		t.Fatal("custom provider with its env key not offered")
	}
	t.Setenv("ACME_TEST_KEY", "")
	auth := `{"acme":{"type":"api_key","key":"k"}}`
	if err := os.WriteFile(filepath.Join(dir, "auth.json"), []byte(auth), 0o600); err != nil {
		t.Fatal(err)
	}
	if !has() {
		t.Fatal("custom provider with a stored credential not offered")
	}
}
