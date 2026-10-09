package web

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"
)

// TestMain keeps session titles off for the whole package: title calls start
// extra adapter connections that would land in shared test stubs. The title
// tests switch them back on per test.
func TestMain(m *testing.M) {
	os.Setenv("EL_SESSION_TITLES", "0")
	os.Exit(m.Run())
}

// getSessions lists every Session on the test Daemon.
func getSessions(t *testing.T, base string) []SessionMeta {
	t.Helper()
	resp, err := http.Get(base + "/api/sessions")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out []SessionMeta
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}
