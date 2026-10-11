package web

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"example.com/emberloop-web/memory"
)

var errFakeDecider = errDeciderTest("fake decider failure")

type errDeciderTest string

func (e errDeciderTest) Error() string { return string(e) }

// jsonNumber formats a probability for a scripted one-off reply.
func jsonNumber(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// Seam: the Daemon HTTP API with the fake Backend for settings, and
// MineSession with canned events.jsonl files for miner verdicts. The fake
// Decider is memory.ScriptedDecider (d.decider) and the scripted one-off
// (d.deciderOneShot).

func deciderView(t *testing.T, base string) map[string]any {
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
	dec, ok := view["decider"].(map[string]any)
	if !ok {
		t.Fatalf("settings view lacks decider: %v", view)
	}
	return dec
}

// The settings read reports the decider defaults: heuristic, no model, and
// the spec thresholds.
func TestDeciderSettingsDefaults(t *testing.T) {
	srv, _ := settingsServer(t, `{"roles":{}}`)
	dec := deciderView(t, srv.URL)
	if dec["decider_mode"] != deciderHeuristic || dec["memory_model"] != "" {
		t.Fatalf("decider defaults = %v, want heuristic with no model", dec)
	}
	if dec["decider_confirm_low"] != 0.35 || dec["decider_confirm_high"] != 0.7 {
		t.Fatalf("decider thresholds = %v, want 0.35 and 0.7", dec)
	}
}

// The admin write saves the mode, model and thresholds; bad values are
// refused.
func TestDeciderSettingsWriteAndValidate(t *testing.T) {
	t.Setenv("PIG_TEST_FAUX", "1")
	srv, file := settingsServer(t, `{"roles":{}}`)
	put := func(body string) (int, map[string]any) {
		out, code := putSettingsBody(t, srv.URL, body)
		var view map[string]any
		raw, _ := json.Marshal(out)
		_ = json.Unmarshal(raw, &view)
		return code, view
	}
	code, _ := put(`{"decider":{"decider_mode":"ensemble","memory_model":"test-faux/decider","decider_confirm_low":0.4,"decider_confirm_high":0.8}}`)
	if code != http.StatusOK {
		t.Fatalf("decider write status = %d, want 200", code)
	}
	dec := deciderView(t, srv.URL)
	if dec["decider_mode"] != "ensemble" || dec["memory_model"] != "test-faux/decider" {
		t.Fatalf("decider after write = %v", dec)
	}
	raw, _ := os.ReadFile(file)
	for _, want := range []string{`"decider_mode"`, `"memory_model"`, `"decider_confirm_low"`, `"decider_confirm_high"`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("settings file lacks %s: %s", want, raw)
		}
	}
	for _, bad := range []string{
		`{"decider":{"decider_mode":"jev"}}`,
		`{"decider":{"decider_mode":""}}`,
		`{"decider":{"decider_confirm_low":0.9,"decider_confirm_high":0.1}}`,
		`{"decider":{"decider_confirm_low":-0.5}}`,
		`{"decider":{"decider_confirm_high":2}}`,
		`{"decider":{"memory_model":"nope/not-a-model"}}`,
	} {
		if code, _ := put(bad); code != http.StatusBadRequest {
			t.Fatalf("write %s status = %d, want 400", bad, code)
		}
	}
	// A partial write keeps the other knobs.
	if code, _ := put(`{"decider":{"decider_confirm_low":0.2}}`); code != http.StatusOK {
		t.Fatalf("partial write status = %d, want 200", code)
	}
	if dec := deciderView(t, srv.URL); dec["decider_confirm_low"] != 0.2 || dec["decider_confirm_high"] != 0.8 {
		t.Fatalf("decider after partial write = %v, want low 0.2 high kept 0.8", dec)
	}
}

// Crossed thresholds in the file fall back to the defaults and say so in
// the note instead of failing silently.
func TestDeciderSettingsCrossedThresholdsNote(t *testing.T) {
	srv, _ := settingsServer(t, `{"decider_mode":"heuristic","decider_confirm_low":0.9,"decider_confirm_high":0.1}`)
	dec := deciderView(t, srv.URL)
	if dec["decider_confirm_low"] != 0.35 || dec["decider_confirm_high"] != 0.7 {
		t.Fatalf("decider thresholds = %v, want the defaults", dec)
	}
	resp, err := http.Get(srv.URL + "/api/settings")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var view map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	note, _ := view["decider_note"].(string)
	if !strings.Contains(note, "exceeds") || !strings.Contains(note, "defaults") {
		t.Fatalf("decider_note = %q, want the crossed thresholds named", note)
	}
}

// Only the admin writes the global decider settings.
func TestDeciderSettingsAreAdminOnly(t *testing.T) {
	settings := filepath.Join(tempDir(t), "settings.json")
	if err := os.WriteFile(settings, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EL_SETTINGS", settings)
	d := NewDaemon(tempDir(t), NewFakeBackend())
	h := d.Handler()
	admin := signedIn(t, h, "alice")
	bob := signedIn(t, h, "bob")
	put := func(cookies ...*http.Cookie) int {
		return authDo(t, h, http.MethodPut, "/api/settings", `{"decider":{"decider_mode":"llm"}}`, cookies...).Code
	}
	if code := put(bob); code != http.StatusForbidden {
		t.Fatalf("plain User writes decider = %d, want 403", code)
	}
	if code := put(); code != http.StatusUnauthorized {
		t.Fatalf("anonymous writes decider = %d, want 401", code)
	}
	if code := put(admin); code != http.StatusOK {
		t.Fatalf("admin writes decider = %d, want 200", code)
	}
}

// A scripted durable below the cut drops the hit: nothing is stored.
func TestMineSessionDropsWithScriptedDecider(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.decider = &memory.ScriptedDecider{Durable: 0.1}
	seedEndedSession(t, d, "s1", "alice", []Event{userPrompt(1, "alice", "No, use tabs here")})
	res, err := d.MineSession("s1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Stored != 0 || res.Dropped != 1 {
		t.Fatalf("result = %+v, want 0 stored and 1 dropped", res)
	}
	if obs := bankQuotes(t, d, "alice", "tabs"); len(obs) != 0 {
		t.Fatalf("observations = %+v, want none", obs)
	}
}

// A scripted verdict stores the kind with the model and version in
// decider_json, and marks queue eligibility at the high threshold.
func TestMineSessionStoresScriptedVerdict(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.decider = &memory.ScriptedDecider{Durable: 0.9, Kind: memory.KindFact, Scope: "situational"}
	seedEndedSession(t, d, "s1", "alice", []Event{userPrompt(1, "alice", "No, use tabs here")})
	res, err := d.MineSession("s1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Stored != 1 || res.Dropped != 0 {
		t.Fatalf("result = %+v, want 1 stored", res)
	}
	obs := bankQuotes(t, d, "alice", "tabs")
	if len(obs) != 1 || obs[0].Kind != memory.KindFact {
		t.Fatalf("observations = %+v, want one fact", obs)
	}
	var doc struct {
		Decider   string  `json:"decider"`
		Model     string  `json:"model"`
		Version   string  `json:"version"`
		Durable   float64 `json:"durable"`
		Kind      string  `json:"kind"`
		Scope     string  `json:"scope_hint"`
		Eligible  bool    `json:"queue_eligible"`
		Confirmed bool    `json:"confirmed"`
	}
	if err := json.Unmarshal([]byte(obs[0].DeciderJSON), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Model != "scripted" || doc.Version != "test" {
		t.Fatalf("decider doc = %+v, want model scripted version test", doc)
	}
	if doc.Durable != 0.9 || doc.Kind != memory.KindFact || doc.Scope != "situational" || !doc.Eligible {
		t.Fatalf("decider doc = %+v, want the scripted verdict eligible", doc)
	}
	if !doc.Confirmed {
		t.Fatalf("decider doc = %+v, want a judged verdict confirmed", doc)
	}
}

// An unavailable confirmer leaves the heuristic verdict standing: the hit is
// stored, never dropped on a missing answer.
func TestMineSessionFallsBackWhenDeciderUnavailable(t *testing.T) {
	for _, scripted := range []*memory.ScriptedDecider{{Unavailable: true}, {Durable: 0.9}} {
		d := NewDaemon(tempDir(t), NewFakeBackend())
		d.decider = scripted
		seedEndedSession(t, d, "s1", "alice", []Event{userPrompt(1, "alice", "No, use tabs here")})
		res, err := d.MineSession("s1")
		if err != nil {
			t.Fatal(err)
		}
		if res.Stored != 1 {
			t.Fatalf("unavailable decider result = %+v, want the heuristic hit stored", res)
		}
	}
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.decider = &memory.ScriptedDecider{Err: errFakeDecider}
	seedEndedSession(t, d, "s1", "alice", []Event{userPrompt(1, "alice", "No, use tabs here")})
	if res, err := d.MineSession("s1"); err != nil || res.Stored != 1 {
		t.Fatalf("erroring decider result = %+v, %v; want the heuristic hit stored", res, err)
	}
}

// The default heuristic mine stores the model and version with every output.
func TestMineSessionStoresHeuristicModelVersion(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	seedEndedSession(t, d, "s1", "alice", []Event{userPrompt(1, "alice", "No, use tabs here")})
	if _, err := d.MineSession("s1"); err != nil {
		t.Fatal(err)
	}
	obs := bankQuotes(t, d, "alice", "tabs")
	if len(obs) != 1 {
		t.Fatalf("observations = %+v, want 1", obs)
	}
	var doc struct {
		Decider   string `json:"decider"`
		Model     string `json:"model"`
		Version   string `json:"version"`
		Confirmed bool   `json:"confirmed"`
	}
	if err := json.Unmarshal([]byte(obs[0].DeciderJSON), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Decider != "heuristic" || doc.Model != "heuristic" || doc.Version != "v1" {
		t.Fatalf("decider doc = %+v, want heuristic/v1", doc)
	}
	if doc.Confirmed {
		t.Fatalf("decider doc = %+v, want the heuristic fallback unconfirmed", doc)
	}
}

// Ensemble mode end to end: below confirmLow drops, between the thresholds
// stores an observation, at confirmHigh marks it queue-eligible.
func TestMineSessionEnsembleThresholds(t *testing.T) {
	useSettings(t, `{"decider_mode":"ensemble","memory_model":"test-faux/decider","decider_confirm_low":0.35,"decider_confirm_high":0.7}`)
	cases := []struct {
		name     string
		durable  float64
		stored   int
		dropped  int
		eligible bool
	}{
		{"drop", 0.1, 0, 1, false},
		{"observe", 0.5, 1, 0, false},
		{"eligible", 0.9, 1, 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := NewDaemon(tempDir(t), NewFakeBackend())
			d.deciderOneShot = func(model, system, user string) (string, error) {
				return `{"answers":[{"question":"durable","noul":` + jsonNumber(c.durable) + `}]}`, nil
			}
			seedEndedSession(t, d, "s1", "alice", []Event{userPrompt(1, "alice", "No, use tabs here")})
			res, err := d.MineSession("s1")
			if err != nil {
				t.Fatal(err)
			}
			if res.Stored != c.stored || res.Dropped != c.dropped {
				t.Fatalf("result = %+v, want %d stored %d dropped", res, c.stored, c.dropped)
			}
			if c.stored == 0 {
				return
			}
			var doc struct {
				Decider  string `json:"decider"`
				Model    string `json:"model"`
				Version  string `json:"version"`
				Eligible bool   `json:"queue_eligible"`
			}
			obs := bankQuotes(t, d, "alice", "tabs")
			if len(obs) != 1 {
				t.Fatalf("observations = %+v, want 1", obs)
			}
			if err := json.Unmarshal([]byte(obs[0].DeciderJSON), &doc); err != nil {
				t.Fatal(err)
			}
			if doc.Eligible != c.eligible {
				t.Fatalf("queue_eligible = %v, want %v", doc.Eligible, c.eligible)
			}
			if doc.Decider != "ensemble" || doc.Model != "test-faux/decider" || doc.Version != "v1" {
				t.Fatalf("decider doc = %+v, want ensemble/test-faux/decider/v1", doc)
			}
		})
	}
}

// Ensemble without a memory model still stores the observation: an
// unconfirmable candidate is never dropped on a missing model.
func TestMineSessionEnsembleWithoutModelStores(t *testing.T) {
	useSettings(t, `{"decider_mode":"ensemble"}`)
	d := NewDaemon(tempDir(t), NewFakeBackend())
	seedEndedSession(t, d, "s1", "alice", []Event{userPrompt(1, "alice", "No, use tabs here")})
	res, err := d.MineSession("s1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Stored != 1 {
		t.Fatalf("result = %+v, want the observation stored", res)
	}
	var doc struct {
		Eligible bool `json:"queue_eligible"`
	}
	obs := bankQuotes(t, d, "alice", "tabs")
	if err := json.Unmarshal([]byte(obs[0].DeciderJSON), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Eligible {
		t.Fatal("unconfirmed observation is queue-eligible, want it held back")
	}
}

// A replayed Event never reaches the Decider: after a cursor reset the same
// Session Events count as duplicates with no confirming call.
func TestMineSessionReplaySkipsDecider(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	keep := &memory.ScriptedDecider{Durable: 0.9}
	d.decider = keep
	seedEndedSession(t, d, "s1", "alice", []Event{
		userPrompt(1, "alice", "No, use tabs here"),
		userPrompt(2, "alice", "don't use mocks for this package"),
	})
	if res, err := d.MineSession("s1"); err != nil || res.Stored != 2 {
		t.Fatalf("first mine = %+v, %v; want 2 stored", res, err)
	}
	// A dropping Decider takes over, then the log head is rewritten so the
	// cursor resets and the same Events replay.
	drop := &memory.ScriptedDecider{Durable: 0.1}
	d.decider = drop
	logPath := filepath.Join(d.sessionDir("s1"), "events.jsonl")
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}
	if err := os.WriteFile(logPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := d.MineSession("s1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Stored != 0 || res.Dropped != 0 || res.Duplicates != 2 || !res.Reset {
		t.Fatalf("replay mine = %+v, want 2 duplicates after a reset, nothing stored or dropped", res)
	}
	if drop.Calls != 0 {
		t.Fatalf("dropping decider called %d times on replay, want 0", drop.Calls)
	}
}

// Redaction runs before every llm call: the one-off sees the marker, never
// the secret.
func TestMineSessionLLMRedactsBeforeCall(t *testing.T) {
	useSettings(t, `{"decider_mode":"llm","memory_model":"test-faux/decider"}`)
	d := NewDaemon(tempDir(t), NewFakeBackend())
	var saw string
	d.deciderOneShot = func(model, system, user string) (string, error) {
		saw = user
		return `{"answers":[` +
			`{"question":"durable","noul":0.9},` +
			`{"question":"kind","choice":"preference","probabilities":{"preference":1,"decision":0,"antipattern":0,"fact":0,"procedure":0},"confidence":1},` +
			`{"question":"scope_hint","choice":"user_rule","probabilities":{"user_rule":1,"project_rule":0,"situational":0,"none":0},"confidence":1}` +
			`]}`, nil
	}
	text := "remember: the password is hunter2-2026, always use tabs"
	seedEndedSession(t, d, "s1", "alice", []Event{userPrompt(1, "alice", text)})
	res, err := d.MineSession("s1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Stored != 1 {
		t.Fatalf("result = %+v, want 1 stored", res)
	}
	if strings.Contains(saw, "hunter2") {
		t.Errorf("llm input holds the secret raw: %q", saw)
	}
	if !strings.Contains(saw, "[redacted:password]") {
		t.Errorf("llm input lacks the redaction marker: %q", saw)
	}
	obs := bankQuotes(t, d, "alice", "tabs")
	if len(obs) != 1 || strings.Contains(obs[0].Quote, "hunter2") {
		t.Fatalf("observations = %+v, want one redacted quote", obs)
	}
}

// A failed llm confirm stores the heuristic hit but never marks it eligible:
// no confirmer answering means no eligibility, even when the heuristic score
// clears the high threshold.
func TestMineSessionLLMErrorNeverEligible(t *testing.T) {
	useSettings(t, `{"decider_mode":"llm","memory_model":"test-faux/decider"}`)
	d := NewDaemon(tempDir(t), NewFakeBackend())
	d.deciderOneShot = func(model, system, user string) (string, error) {
		return "", errFakeDecider
	}
	seedEndedSession(t, d, "s1", "alice", []Event{userPrompt(1, "alice", "No, use tabs here")})
	res, err := d.MineSession("s1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Stored != 1 {
		t.Fatalf("result = %+v, want the heuristic hit stored", res)
	}
	var doc struct {
		Eligible  bool `json:"queue_eligible"`
		Confirmed bool `json:"confirmed"`
	}
	obs := bankQuotes(t, d, "alice", "tabs")
	if len(obs) != 1 {
		t.Fatalf("observations = %+v, want 1", obs)
	}
	if err := json.Unmarshal([]byte(obs[0].DeciderJSON), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Eligible || doc.Confirmed {
		t.Fatalf("decider doc = %+v, want stored unconfirmed and never eligible", doc)
	}
}

// llm without a memory model has no confirmer: the heuristic hit is stored
// (nothing is lost) but never queue-eligible on the heuristic score alone.
func TestMineSessionLLMWithoutModelNeverEligible(t *testing.T) {
	useSettings(t, `{"decider_mode":"llm"}`)
	d := NewDaemon(tempDir(t), NewFakeBackend())
	seedEndedSession(t, d, "s1", "alice", []Event{userPrompt(1, "alice", "No, use tabs here")})
	res, err := d.MineSession("s1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Stored != 1 {
		t.Fatalf("result = %+v, want the heuristic hit stored", res)
	}
	var doc struct {
		Decider   string `json:"decider"`
		Eligible  bool   `json:"queue_eligible"`
		Confirmed bool   `json:"confirmed"`
	}
	obs := bankQuotes(t, d, "alice", "tabs")
	if len(obs) != 1 {
		t.Fatalf("observations = %+v, want 1", obs)
	}
	if err := json.Unmarshal([]byte(obs[0].DeciderJSON), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Decider != "llm" || doc.Eligible || doc.Confirmed {
		t.Fatalf("decider doc = %+v, want llm mode stored unconfirmed and never eligible", doc)
	}
}
