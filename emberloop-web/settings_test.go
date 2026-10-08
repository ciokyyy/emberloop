package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Seam: the Daemon HTTP API with the fake Backend. Settings behaviour is
// observed through GET/PUT responses and the live Session cap, never
// through internals.

func settingsServer(t *testing.T, settingsBody string) (*httptest.Server, string) {
	t.Helper()
	dir := t.TempDir()
	settings := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(settings, []byte(settingsBody), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EMBERLOOP_SETTINGS", settings)
	t.Setenv("EMBERLOOP_MAX_WORKERS", "")
	d := NewDaemon(t.TempDir(), NewFakeBackend())
	srv := httptest.NewServer(d.Handler())
	t.Cleanup(srv.Close)
	return srv, settings
}

type settingsResp struct {
	Roles             map[string]roleSetting `json:"roles"`
	MaxWorkers        int                    `json:"max_workers"`
	MaxWorkersFromEnv bool                   `json:"max_workers_from_env"`
}

func getSettingsView(t *testing.T, base string) (settingsResp, int) {
	t.Helper()
	resp, err := http.Get(base + "/api/settings")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out settingsResp
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
	}
	return out, resp.StatusCode
}

func putSettingsBody(t *testing.T, base, body string) (settingsResp, int) {
	t.Helper()
	req, err := http.NewRequest("PUT", base+"/api/settings", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out settingsResp
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
	}
	return out, resp.StatusCode
}

// The settings read returns max_workers and whether the environment
// overrides it.
func TestSettingsReadReturnsMaxWorkersAndEnvFlag(t *testing.T) {
	srv, _ := settingsServer(t, `{"max_workers": 5, "roles": {}}`)

	out, code := getSettingsView(t, srv.URL)
	if code != http.StatusOK {
		t.Fatalf("settings status = %d, want 200", code)
	}
	if out.MaxWorkers != 5 || out.MaxWorkersFromEnv {
		t.Fatalf("settings = %+v, want max_workers 5 with no env override", out)
	}

	t.Setenv("EMBERLOOP_MAX_WORKERS", "2")
	out, _ = getSettingsView(t, srv.URL)
	if out.MaxWorkers != 2 || !out.MaxWorkersFromEnv {
		t.Fatalf("settings under env = %+v, want max_workers 2 from env", out)
	}
}

// The settings write validates and saves max_workers without losing roles.
func TestSettingsWriteValidatesAndSavesMaxWorkers(t *testing.T) {
	srv, settings := settingsServer(t, `{"max_workers": 2, "roles": {"worker": {"harness": "pig", "model": "auto"}}, "tools": {"bash": {}}}`)

	out, code := putSettingsBody(t, srv.URL, `{"max_workers": 4}`)
	if code != http.StatusOK {
		t.Fatalf("write status = %d, want 200", code)
	}
	if out.MaxWorkers != 4 {
		t.Fatalf("write max_workers = %d, want 4", out.MaxWorkers)
	}
	raw, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	// The file keeps roles and unknown fields next to the new cap.
	var kept struct {
		Roles map[string]roleSetting `json:"roles"`
		Tools map[string]any         `json:"tools"`
	}
	if err := json.Unmarshal(raw, &kept); err != nil {
		t.Fatal(err)
	}
	if kept.Roles["worker"].Model != "auto" {
		t.Fatalf("roles after max_workers-only write = %+v, want the worker role kept", kept.Roles)
	}
	if _, ok := kept.Tools["bash"]; !ok {
		t.Fatalf("tools after write missing: %s", raw)
	}

	// Roles-only writes keep the cap.
	out, code = putSettingsBody(t, srv.URL, `{"roles": {"scout": {"harness": "pig", "model": "auto"}}}`)
	if code != http.StatusOK {
		t.Fatalf("roles-only write status = %d, want 200", code)
	}
	if out.Roles["worker"].Model != "auto" || out.Roles["scout"].Model != "auto" {
		t.Fatalf("roles after merge = %+v, want worker kept and scout added", out.Roles)
	}
	if out.MaxWorkers != 4 {
		t.Fatalf("max_workers after roles-only write = %d, want 4", out.MaxWorkers)
	}

	for _, bad := range []string{`{"max_workers": 0}`, `{"max_workers": -2}`, `{"max_workers": "soon"}`} {
		_, code := putSettingsBody(t, srv.URL, bad)
		if code != http.StatusBadRequest {
			t.Fatalf("write %s status = %d, want 400", bad, code)
		}
	}
	raw, _ = os.ReadFile(settings)
	var still struct {
		MaxWorkers int `json:"max_workers"`
	}
	_ = json.Unmarshal(raw, &still)
	if still.MaxWorkers != 4 {
		t.Fatalf("max_workers after rejected writes = %d, want 4 kept", still.MaxWorkers)
	}
}

// A write changes the live cap; over-cap behaviour is unchanged.
func TestSettingsWriteChangesLiveCap(t *testing.T) {
	srv, _ := settingsServer(t, `{"max_workers": 1, "roles": {}}`)
	workDir := t.TempDir()

	_ = createSession(t, srv.URL, "fake", "fake-model", workDir)
	body, _ := json.Marshal(map[string]string{"harness": "fake", "model": "fake-model", "workDir": workDir})
	resp, err := http.Post(srv.URL+"/api/sessions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("over-cap create status = %d, want 429", resp.StatusCode)
	}

	if _, code := putSettingsBody(t, srv.URL, `{"max_workers": 2}`); code != http.StatusOK {
		t.Fatalf("write status = %d, want 200", code)
	}
	resp, err = http.Post(srv.URL+"/api/sessions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create after raising the cap status = %d, want 201", resp.StatusCode)
	}

	// Lowering the cap refuses new Sessions again with the same message.
	if _, code := putSettingsBody(t, srv.URL, `{"max_workers": 1}`); code != http.StatusOK {
		t.Fatalf("write status = %d, want 200", code)
	}
	resp, err = http.Post(srv.URL+"/api/sessions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("over-cap create after lowering status = %d, want 429", resp.StatusCode)
	}
	var out struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Error, "max_workers") {
		t.Fatalf("over-cap error = %q, want it to name max_workers", out.Error)
	}
}

// The environment variable still wins: the read says so, and a write saves
// the file without moving the live cap.
func TestSettingsEnvOverrideWinsOverWrite(t *testing.T) {
	srv, settings := settingsServer(t, `{"max_workers": 5, "roles": {}}`)
	t.Setenv("EMBERLOOP_MAX_WORKERS", "1")

	out, code := putSettingsBody(t, srv.URL, `{"max_workers": 3}`)
	if code != http.StatusOK {
		t.Fatalf("write status = %d, want 200", code)
	}
	if out.MaxWorkers != 1 || !out.MaxWorkersFromEnv {
		t.Fatalf("write under env = %+v, want the live cap 1 from env", out)
	}
	raw, _ := os.ReadFile(settings)
	var saved struct {
		MaxWorkers int `json:"max_workers"`
	}
	_ = json.Unmarshal(raw, &saved)
	if saved.MaxWorkers != 3 {
		t.Fatalf("saved max_workers = %d, want 3 kept for when the env is unset", saved.MaxWorkers)
	}

	d := NewDaemon(t.TempDir(), NewFakeBackend())
	if got := d.MaxRunning(); got != 1 {
		t.Fatalf("live cap under env = %d, want 1", got)
	}
}
