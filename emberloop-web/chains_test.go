package web

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
)

// Seam: the Daemon HTTP API with the fake Backend, as in settings_test.go.
// Chain behaviour is observed through GET/PUT responses and the settings
// file, never through internals.

func getChainsView(t *testing.T, base string) (map[string][]chainEntry, int) {
	t.Helper()
	resp, err := http.Get(base + "/api/settings")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Chains map[string][]chainEntry `json:"chains"`
	}
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
	}
	return out.Chains, resp.StatusCode
}

func putChainsBody(t *testing.T, base, body string) (map[string][]chainEntry, int, string) {
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
	var out struct {
		Chains map[string][]chainEntry `json:"chains"`
	}
	var errText string
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
	} else {
		raw := make([]byte, 0, 256)
		buf := make([]byte, 256)
		for {
			n, rerr := resp.Body.Read(buf)
			raw = append(raw, buf[:n]...)
			if rerr != nil {
				break
			}
		}
		errText = string(raw)
	}
	return out.Chains, resp.StatusCode, errText
}

// A chains write round-trips through GET and the file, merges per role, and
// an empty list clears one role while the others stay.
func TestChainSettingsRoundTrip(t *testing.T) {
	srv, file := settingsServer(t, `{"roles": {"worker": {"harness": "pig", "model": "auto"}}}`)

	chains, code, _ := putChainsBody(t, srv.URL, `{"chains": {"worker": [{"backend": "pig", "model": "a"}, {"backend": "claude", "model": "opus"}]}}`)
	if code != http.StatusOK {
		t.Fatalf("write status = %d, want 200", code)
	}
	if len(chains["worker"]) != 2 || chains["worker"][0].Backend != "pig" || chains["worker"][1].Model != "opus" {
		t.Fatalf("chains after write = %+v, want the two worker entries", chains)
	}

	got, code := getChainsView(t, srv.URL)
	if code != http.StatusOK || len(got["worker"]) != 2 {
		t.Fatalf("chains after read = %+v (%d), want the two worker entries", got, code)
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"chains"`) || !strings.Contains(string(raw), `"opus"`) {
		t.Fatalf("settings file lacks the chains: %s", raw)
	}
	// The roles-only write kept its role beside the new chains.
	var kept struct {
		Roles map[string]roleSetting `json:"roles"`
	}
	if err := json.Unmarshal(raw, &kept); err != nil {
		t.Fatal(err)
	}
	if kept.Roles["worker"].Model != "auto" {
		t.Fatalf("roles after chains write = %+v, want the worker role kept", kept.Roles)
	}

	// A second write merges: the new role lands, the worker chain stays.
	chains, code, _ = putChainsBody(t, srv.URL, `{"chains": {"reviewer-spec": [{"backend": "grok", "model": "grok-4.7"}]}}`)
	if code != http.StatusOK || len(chains["worker"]) != 2 || len(chains["reviewer-spec"]) != 1 {
		t.Fatalf("chains after merge = %+v (%d), want worker kept and reviewer-spec added", chains, code)
	}

	// An empty list clears one role and keeps the other.
	chains, code, _ = putChainsBody(t, srv.URL, `{"chains": {"worker": []}}`)
	if code != http.StatusOK {
		t.Fatalf("clear status = %d, want 200", code)
	}
	if _, ok := chains["worker"]; ok || len(chains["reviewer-spec"]) != 1 {
		t.Fatalf("chains after clear = %+v, want worker gone and reviewer-spec kept", chains)
	}
}

// Chain writes validate the entries: at most three per role, a known
// harness, a model, and an authed claude model. Rejected writes keep the
// stored chains.
func TestChainSettingsValidation(t *testing.T) {
	srv, file := settingsServer(t, `{"chains": {}}`)

	for _, bad := range []string{
		`{"chains": {"worker": [{}, {}, {}, {}]}}`,
		`{"chains": {"worker": [{"backend": "gemini", "model": "x"}]}}`,
		`{"chains": {"worker": [{"backend": "pig", "model": ""}]}}`,
		`{"chains": {"worker": [{"backend": "claude", "model": "nope"}]}}`,
		`{"chains": {"pig-title": [{"backend": "claude", "model": "haiku"}]}}`,
		`{"chains": {"worker": [{"backend": "", "model": ""}]}}`,
	} {
		if _, code, _ := putChainsBody(t, srv.URL, bad); code != http.StatusBadRequest {
			t.Errorf("write %s status = %d, want 400", bad, code)
		}
	}
	got, _ := getChainsView(t, srv.URL)
	if len(got) != 0 {
		t.Fatalf("chains after rejected writes = %+v, want none stored", got)
	}
	raw, _ := os.ReadFile(file)
	var saved struct {
		Chains map[string][]chainEntry `json:"chains"`
	}
	_ = json.Unmarshal(raw, &saved)
	if len(saved.Chains) != 0 {
		t.Fatalf("saved chains after rejected writes = %+v, want none", saved.Chains)
	}

	// Three entries with effort are the most a role holds.
	chains, code, _ := putChainsBody(t, srv.URL, `{"chains": {"worker": [{"backend": "pig", "model": "a"}, {"backend": "claude", "model": "opus", "effort": "high"}, {"backend": "grok", "model": "grok-4.7"}]}}`)
	if code != http.StatusOK || len(chains["worker"]) != 3 || chains["worker"][1].Effort != "high" {
		t.Fatalf("three-entry write = %+v (%d), want all three with effort kept", chains, code)
	}
}

// The "harness" spelling reads as "backend": the settings page wrote harness
// before chains existed, and the spec names the field backend.
func TestChainSettingsHarnessAlias(t *testing.T) {
	srv, _ := settingsServer(t, `{}`)

	chains, code, _ := putChainsBody(t, srv.URL, `{"chains": {"worker": [{"harness": "pig", "model": "a"}]}}`)
	if code != http.StatusOK {
		t.Fatalf("harness-spelled write status = %d, want 200", code)
	}
	if len(chains["worker"]) != 1 || chains["worker"][0].Backend != "pig" {
		t.Fatalf("chains after harness write = %+v, want one pig entry", chains)
	}
	got, _ := getChainsView(t, srv.URL)
	if len(got["worker"]) != 1 || got["worker"][0].Backend != "pig" || got["worker"][0].Model != "a" {
		t.Fatalf("chains after read = %+v, want the pig entry under backend", got)
	}
}

// chainFor prefers the stored chain under the role or its alias, and falls
// back to the single "roles" entry as a chain of one.
func TestChainForPrefersChainsThenRoles(t *testing.T) {
	t.Setenv("EL_SETTINGS", useSettings(t, `{"roles": {"worker": {"harness": "pig", "model": "auto", "effort": "high"}}}`))

	got := chainFor("worker")
	if len(got) != 1 || got[0].Backend != "pig" || got[0].Model != "auto" || got[0].Effort != "high" {
		t.Fatalf("chainFor without chains = %+v, want the single roles entry", got)
	}

	t.Setenv("EL_SETTINGS", useSettings(t, `{"roles": {"worker": {"harness": "pig", "model": "auto"}}, "chains": {"implementer": [{"backend": "grok", "model": "grok-4.7"}]}}`))
	got = chainFor("worker")
	if len(got) != 1 || got[0].Backend != "grok" {
		t.Fatalf("chainFor with alias chains = %+v, want the implementer entry", got)
	}

	t.Setenv("EL_SETTINGS", useSettings(t, `{"roles": {"worker": {"harness": "pig", "model": "auto"}}, "chains": {"worker": [{"backend": "claude", "model": "opus"}, {"backend": "pig", "model": "auto"}]}}`))
	got = chainFor("worker")
	if len(got) != 2 || got[0].Backend != "claude" || got[1].Backend != "pig" {
		t.Fatalf("chainFor with chains = %+v, want the stored worker chain", got)
	}

	t.Setenv("EL_SETTINGS", useSettings(t, `{}`))
	if got := chainFor("worker"); len(got) != 0 {
		t.Fatalf("chainFor unconfigured = %+v, want none", got)
	}
	if got := chainFor("reviewer-spec"); len(got) != 0 {
		t.Fatalf("chainFor unconfigured reviewer = %+v, want none", got)
	}
}

// chainAdvances is the ticket-14 predicate on the shared classifier: each
// advancing class moves the chain, anything else does not.
func TestChainAdvancesOnFourClasses(t *testing.T) {
	advance := []string{
		"429 quota exceeded for this key",
		"insufficient_quota",
		"402 Payment Required",
		"no credits left on this key",
		"HTTP 401: authentication_error",
		"session limit reached for this account",
		"too many sessions for this key",
	}
	for _, text := range advance {
		if !chainAdvances(errString(text)) {
			t.Errorf("%q: chainAdvances = false, want true", text)
		}
	}
	stay := []string{
		"Mistral API error (429): Too Many Requests",
		"503 Service Unavailable",
		"Connection error.",
		"OpenAI API error (403): forbidden",
		"pig one-off stopped at the 5m0s limit",
		"bad request: malformed input",
	}
	for _, text := range stay {
		if chainAdvances(errString(text)) {
			t.Errorf("%q: chainAdvances = true, want false", text)
		}
	}
	if chainAdvances(nil) {
		t.Error("nil: chainAdvances = true, want false")
	}
}
