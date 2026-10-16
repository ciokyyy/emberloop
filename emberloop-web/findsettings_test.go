package web

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"example.com/emberloop-web/el/tools/systemone"
)

func findView(t *testing.T, base string) map[string]any {
	t.Helper()
	resp, err := http.Get(base + "/api/settings")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var view map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	find, ok := view["find"].(map[string]any)
	if !ok {
		t.Fatalf("settings view lacks find: %v", view)
	}
	return find
}

// Unset, the find model is empty in the file and the view names the pinned
// default plus the quick picks.
func TestFindSettingsDefaults(t *testing.T) {
	t.Setenv("EL_DECISION_MODEL", "")
	srv, _ := settingsServer(t, `{"roles":{}}`)
	find := findView(t, srv.URL)
	if find["decision_model"] != "" || find["default"] != systemone.JevModelPinned || find["from_env"] != false {
		t.Fatalf("find defaults = %v", find)
	}
	if s, _ := find["suggestions"].([]any); len(s) == 0 {
		t.Fatalf("find view lacks suggestions: %v", find)
	}
}

// The admin write saves the shared decision_model key, canonicalizes the
// rolling alias onto the pin, clears on empty and refuses bad ids.
func TestFindSettingsWriteAndValidate(t *testing.T) {
	t.Setenv("EL_DECISION_MODEL", "")
	srv, file := settingsServer(t, `{"roles":{}}`)
	code := func(body string) int {
		_, c := putSettingsBody(t, srv.URL, body)
		return c
	}
	if c := code(`{"find":{"decision_model":"liquid/d1"}}`); c != http.StatusOK {
		t.Fatalf("find write status = %d, want 200", c)
	}
	if got := findView(t, srv.URL)["decision_model"]; got != "liquid/d1" {
		t.Fatalf("find model after write = %v", got)
	}
	raw, _ := os.ReadFile(file)
	if !strings.Contains(string(raw), `"decision_model": "liquid/d1"`) {
		t.Fatalf("settings file lacks decision_model: %s", raw)
	}
	if c := code(`{"find":{"decision_model":"~typesafe/jev-latest"}}`); c != http.StatusOK {
		t.Fatalf("alias write status = %d", c)
	}
	if got := findView(t, srv.URL)["decision_model"]; got != systemone.JevModelPinned {
		t.Fatalf("alias saved as %v, want pin %s", got, systemone.JevModelPinned)
	}
	if c := code(`{"find":{"decision_model":""}}`); c != http.StatusOK {
		t.Fatalf("clear status = %d", c)
	}
	raw, _ = os.ReadFile(file)
	if strings.Contains(string(raw), "decision_model") {
		t.Fatalf("clear left the key: %s", raw)
	}
	for _, bad := range []string{
		`{"find":{"decision_model":"two ids"}}`,
		`{"find":{"decision_model":"bad;id"}}`,
		`{"find":{"decision_model":"` + strings.Repeat("a", 129) + `"}}`,
	} {
		if c := code(bad); c != http.StatusBadRequest {
			t.Fatalf("%s status = %d, want 400", bad, c)
		}
	}
}

// EL_DECISION_MODEL wins over the file, and the view says so for the UI lock.
func TestFindSettingsEnvLock(t *testing.T) {
	t.Setenv("EL_DECISION_MODEL", "liquid/d1")
	srv, _ := settingsServer(t, `{"roles":{}}`)
	if find := findView(t, srv.URL); find["from_env"] != true {
		t.Fatalf("find view = %v, want from_env", find)
	}
}
