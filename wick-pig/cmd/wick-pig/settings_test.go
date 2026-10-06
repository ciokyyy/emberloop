package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example.com/wick"
	"example.com/wick/tools"
)

// settingsFile points the settings store at a temp file and clears the env
// overrides, so a test reads and writes only what the command wrote. The
// project layer points at a missing file, so the display shows only the test
// file's own values.
func settingsFile(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "settings.json")
	t.Setenv("WICK_SETTINGS", p)
	t.Setenv("WICK_PROJECT_SETTINGS", filepath.Join(t.TempDir(), "missing.json"))
	t.Setenv("WICK_DECISION_MODEL", "")
	t.Setenv("WICK_FIND_THRESHOLD", "")
	t.Setenv("WICK_MAX_WORKERS", "")
	t.Setenv("WICK_SEARCH_PROVIDER", "")
	t.Setenv("WICK_SEARCH_MODE", "")
	return p
}

func noModels(string, bool) ([]wick.Model, error) { return nil, nil }

// modelList answers a fixed list per harness; a harness with no entry cannot
// be listed, which is what a machine without that CLI looks like.
func modelList(byHarness map[string][]wick.Model) modelSource {
	return func(harness string, refresh bool) ([]wick.Model, error) {
		list, ok := byHarness[harness]
		if !ok {
			return nil, fmt.Errorf("cannot list %s models", harness)
		}
		return list, nil
	}
}

func TestSettingsPrintsCurrentSettings(t *testing.T) {
	p := settingsFile(t)
	write(t, p, `{"decision_model":"typesafe/jev-1.13","threshold":0.3}`)
	var out bytes.Buffer
	if err := runSettings(nil, &out, noModels); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"typesafe/jev-1.13", "0.3"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("settings output %q does not mention %q", out.String(), want)
		}
	}
}

func TestSettingsSavesDecisionModel(t *testing.T) {
	settingsFile(t)
	var out bytes.Buffer
	if err := runSettings([]string{"decision-model", "openai/gpt-5"}, &out, noModels); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "openai/gpt-5") {
		t.Fatalf("output %q does not confirm the new model", out.String())
	}
	if got := tools.LoadSettings().DecisionModel; got != "openai/gpt-5" {
		t.Fatalf("saved decision model is %q, want openai/gpt-5", got)
	}
}

func TestSettingsSavesThreshold(t *testing.T) {
	settingsFile(t)
	var out bytes.Buffer
	if err := runSettings([]string{"threshold", "0.42"}, &out, noModels); err != nil {
		t.Fatal(err)
	}
	if got := tools.LoadSettings().Threshold; got != 0.42 {
		t.Fatalf("saved threshold is %v, want 0.42", got)
	}
}

func TestSettingsRejectsBadThreshold(t *testing.T) {
	p := settingsFile(t)
	var out bytes.Buffer
	if err := runSettings([]string{"threshold", "soon"}, &out, noModels); err == nil {
		t.Fatal("a non-numeric threshold should be an error")
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("a rejected threshold must not be saved")
	}
}

// `settings models` prints a harness's models and the efforts each takes.
func TestSettingsModelsListsHarnessModels(t *testing.T) {
	settingsFile(t)
	var out bytes.Buffer
	src := modelList(map[string][]wick.Model{
		"pig":    {{ID: "deepseek-flash", Efforts: []string{"off", "high"}}},
		"claude": {{ID: "opus", Label: "Opus 5.5", Efforts: []string{"low", "high"}}},
	})
	if err := runSettings([]string{"models"}, &out, src); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"pig:", "deepseek-flash", "off high", "claude:", "opus", "Opus 5.5", "low high"} {
		if !strings.Contains(got, want) {
			t.Errorf("models output %q does not mention %q", got, want)
		}
	}
}

// --harness lists one harness, and --refresh rebuilds instead of reading the
// cache.
func TestSettingsModelsFiltersAndRefreshes(t *testing.T) {
	settingsFile(t)
	refreshes := 0
	src := func(harness string, refresh bool) ([]wick.Model, error) {
		if refresh {
			refreshes++
		}
		if harness != "claude" {
			return nil, fmt.Errorf("cannot list %s models", harness)
		}
		return []wick.Model{{ID: "opus"}}, nil
	}
	var out bytes.Buffer
	if err := runSettings([]string{"models", "--harness", "claude", "--refresh"}, &out, src); err != nil {
		t.Fatal(err)
	}
	if refreshes != 1 {
		t.Errorf("--refresh did not rebuild the list (refresh=%d)", refreshes)
	}
	if got := out.String(); !strings.Contains(got, "opus") || strings.Contains(got, "pig:") {
		t.Errorf("--harness claude printed %q", got)
	}
}

func TestSettingsModelsRejectsAnUnknownHarness(t *testing.T) {
	settingsFile(t)
	var out bytes.Buffer
	err := runSettings([]string{"models", "--harness", "gemini"}, &out, noModels)
	if err == nil || !strings.Contains(err.Error(), "harness") {
		t.Fatalf("an unknown harness should fail naming the harness, got %v", err)
	}
}

// A harness whose CLI is missing is a line in the listing, not a failed
// command: the other harnesses still print.
func TestSettingsModelsReportsAHarnessItCannotList(t *testing.T) {
	settingsFile(t)
	src := func(harness string, refresh bool) ([]wick.Model, error) {
		if harness == "claude" {
			return nil, fmt.Errorf("claude not installed")
		}
		return []wick.Model{{ID: "deepseek-flash"}}, nil
	}
	var out bytes.Buffer
	if err := runSettings([]string{"models"}, &out, src); err != nil {
		t.Fatalf("an unlistable harness must not fail the command: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "claude not installed") || !strings.Contains(got, "deepseek-flash") {
		t.Errorf("models output %q should report claude and still list pig", got)
	}
}

func TestSettingsUnknownSubcommandIsUsageError(t *testing.T) {
	settingsFile(t)
	var out bytes.Buffer
	err := runSettings([]string{"colour"}, &out, noModels)
	if err == nil {
		t.Fatal("an unknown settings subcommand should fail")
	}
	if !strings.Contains(err.Error(), "usage") {
		t.Fatalf("error %q should carry the usage line", err)
	}
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The printed slots name their value's source, so an unset slot reads as the
// default in force and a stored one as the file's.
func TestSettingsPrintsSlotsWithTheirSource(t *testing.T) {
	p := settingsFile(t)
	write(t, p, `{"decision_model":"typesafe/jev-1.13","threshold":0.3,"roles":{"orchestrator":{"harness":"claude","model":"opus","effort":"high"}}}`)
	var out bytes.Buffer
	if err := runSettings(nil, &out, noModels); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		"typesafe/jev-1.13 (file)",
		"0.3 (file)",
		"orchestrator: harness claude (file), model opus (file), effort high (file)",
		"implementer: harness pig (default), model - (default), effort - (default)",
		"scout: harness pig (default), model - (default), effort - (default)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("settings output does not mention %q:\n%s", want, got)
		}
	}
	for _, slot := range []string{"orchestrator", "implementer", "code-review", "spec-review", "scout"} {
		if !strings.Contains(got, slot+":") {
			t.Errorf("settings output omits the %s slot:\n%s", slot, got)
		}
	}
}

func TestSettingsSetsRoleFields(t *testing.T) {
	p := settingsFile(t)
	cases := []struct {
		argv []string
		want tools.Role
	}{
		{[]string{"role", "scout", "harness", "claude"}, tools.Role{Harness: "claude"}},
		{[]string{"role", "scout", "model", "haiku"}, tools.Role{Harness: "claude", Model: "haiku"}},
		{[]string{"role", "scout", "effort", "low"}, tools.Role{Harness: "claude", Model: "haiku", Effort: "low"}},
	}
	var out bytes.Buffer
	for _, c := range cases {
		if err := runSettings(c.argv, &out, noModels); err != nil {
			t.Fatalf("%v: %v", c.argv, err)
		}
		if got := tools.LoadSettings().Roles["scout"]; got != c.want {
			t.Fatalf("after %v scout is %+v, want %+v", c.argv, got, c.want)
		}
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("role settings were not written to %s: %v", p, err)
	}
}

// A model and effort chosen for one harness are kept when the new harness
// still offers them, and cleared with a notice when it does not.
func TestSettingsChangingRoleHarnessClearsWhatTheNewHarnessLacks(t *testing.T) {
	p := settingsFile(t)
	write(t, p, `{"roles":{"orchestrator":{"harness":"pig","model":"m","effort":"high"}}}`)
	src := modelList(map[string][]wick.Model{"claude": {{ID: "opus", Efforts: []string{"low", "high"}}}})
	var out bytes.Buffer
	if err := runSettings([]string{"role", "orchestrator", "harness", "claude"}, &out, src); err != nil {
		t.Fatal(err)
	}
	if got := tools.LoadSettings().Roles["orchestrator"]; got != (tools.Role{Harness: "claude"}) {
		t.Fatalf("harness change left %+v, want only the harness", got)
	}
	if !strings.Contains(out.String(), "cleared") {
		t.Errorf("the cleared model and effort need a notice, got %q", out.String())
	}
}

func TestSettingsChangingRoleHarnessKeepsWhatTheNewHarnessOffers(t *testing.T) {
	p := settingsFile(t)
	write(t, p, `{"roles":{"orchestrator":{"harness":"pig","model":"opus","effort":"high"}}}`)
	src := modelList(map[string][]wick.Model{"claude": {{ID: "opus", Efforts: []string{"low", "high", "max"}}}})
	var out bytes.Buffer
	if err := runSettings([]string{"role", "orchestrator", "harness", "claude"}, &out, src); err != nil {
		t.Fatal(err)
	}
	want := tools.Role{Harness: "claude", Model: "opus", Effort: "high"}
	if got := tools.LoadSettings().Roles["orchestrator"]; got != want {
		t.Fatalf("harness change left %+v, want %+v", got, want)
	}
	if strings.Contains(out.String(), "cleared") {
		t.Errorf("nothing should have been cleared, got %q", out.String())
	}
}

// Saving a slot rejects a model the harness does not offer.
func TestSettingsRejectsAModelTheHarnessDoesNotOffer(t *testing.T) {
	p := settingsFile(t)
	write(t, p, `{"roles":{"orchestrator":{"harness":"claude"}}}`)
	src := modelList(map[string][]wick.Model{"claude": {{ID: "haiku"}, {ID: "opus"}}})
	var out bytes.Buffer
	err := runSettings([]string{"role", "orchestrator", "model", "gpt-9"}, &out, src)
	if err == nil || !strings.Contains(err.Error(), "gpt-9") {
		t.Fatalf("an unknown model must fail naming it, got %v", err)
	}
	if got := tools.LoadSettings().Roles["orchestrator"].Model; got != "" {
		t.Fatalf("a rejected model was saved as %q", got)
	}
}

// The cache is rebuilt once before a value it lacks is rejected, so a model
// added since the last listing is accepted.
func TestSettingsRefreshesOnceBeforeRejectingAModel(t *testing.T) {
	p := settingsFile(t)
	write(t, p, `{"roles":{"orchestrator":{"harness":"claude"}}}`)
	calls := 0
	src := func(harness string, refresh bool) ([]wick.Model, error) {
		calls++
		if refresh {
			return []wick.Model{{ID: "opus"}}, nil
		}
		return []wick.Model{{ID: "haiku"}}, nil
	}
	var out bytes.Buffer
	if err := runSettings([]string{"role", "orchestrator", "model", "opus"}, &out, src); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("the list was fetched %d times, want a cache read and one refresh", calls)
	}
	if got := tools.LoadSettings().Roles["orchestrator"].Model; got != "opus" {
		t.Fatalf("saved model is %q, want opus", got)
	}
}

// A harness whose list cannot be built lets a free-text model through, with a
// warning rather than a rejection.
func TestSettingsAllowsFreeTextWhenTheListCannotBeBuilt(t *testing.T) {
	p := settingsFile(t)
	write(t, p, `{"roles":{"orchestrator":{"harness":"claude"}}}`)
	src := func(string, bool) ([]wick.Model, error) { return nil, fmt.Errorf("claude not installed") }
	var out bytes.Buffer
	if err := runSettings([]string{"role", "orchestrator", "model", "mystery-model"}, &out, src); err != nil {
		t.Fatal(err)
	}
	if got := tools.LoadSettings().Roles["orchestrator"].Model; got != "mystery-model" {
		t.Fatalf("saved model is %q, want the free text", got)
	}
	if !strings.Contains(out.String(), "unchecked") {
		t.Errorf("a free-text model needs a warning, got %q", out.String())
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("the free-text model was not saved: %v", err)
	}
}

// An effort the chosen model does not take is rejected; one it does take is
// saved.
func TestSettingsChecksTheEffortAgainstTheModel(t *testing.T) {
	p := settingsFile(t)
	write(t, p, `{"roles":{"orchestrator":{"harness":"claude","model":"haiku"}}}`)
	src := modelList(map[string][]wick.Model{"claude": {
		{ID: "haiku"},
		{ID: "opus", Efforts: []string{"low", "high", "max"}},
	}})
	var out bytes.Buffer
	err := runSettings([]string{"role", "orchestrator", "effort", "high"}, &out, src)
	if err == nil || !strings.Contains(err.Error(), "haiku") {
		t.Fatalf("a model that takes no effort must reject it, got %v", err)
	}
	if err := runSettings([]string{"role", "orchestrator", "model", "opus"}, &out, src); err != nil {
		t.Fatal(err)
	}
	if err := runSettings([]string{"role", "orchestrator", "effort", "high"}, &out, src); err != nil {
		t.Fatal(err)
	}
	if got := tools.LoadSettings().Roles["orchestrator"]; got != (tools.Role{Harness: "claude", Model: "opus", Effort: "high"}) {
		t.Fatalf("effort saved as %+v", got)
	}
	if err := runSettings([]string{"role", "orchestrator", "effort", "off"}, &out, src); err == nil {
		t.Fatal("off is not a claude effort and must be rejected")
	}
}

// Setting the harness already in force is not a change, so model and effort
// stay: for a stored harness, and for the built-in default of an unset slot.
func TestSettingsSamenessKeepsModelAndEffort(t *testing.T) {
	p := settingsFile(t)
	write(t, p, `{"roles":{"orchestrator":{"harness":"claude","model":"opus","effort":"high"},"scout":{"model":"haiku","effort":"low"}}}`)
	var out bytes.Buffer
	if err := runSettings([]string{"role", "orchestrator", "harness", "claude"}, &out, noModels); err != nil {
		t.Fatal(err)
	}
	if got := tools.LoadSettings().Roles["orchestrator"]; got != (tools.Role{Harness: "claude", Model: "opus", Effort: "high"}) {
		t.Fatalf("same-harness set changed %+v", got)
	}
	// scout stores no harness, so pig is already the harness in force.
	if err := runSettings([]string{"role", "scout", "harness", "pig"}, &out, noModels); err != nil {
		t.Fatal(err)
	}
	if got := tools.LoadSettings().Roles["scout"]; got != (tools.Role{Harness: "pig", Model: "haiku", Effort: "low"}) {
		t.Fatalf("naming the default harness changed %+v", got)
	}
}

func TestSettingsRejectsAnUnknownSlot(t *testing.T) {
	p := settingsFile(t)
	var out bytes.Buffer
	err := runSettings([]string{"role", "worker", "model", "m"}, &out, noModels)
	if err == nil || !strings.Contains(err.Error(), "role slot") {
		t.Fatalf("unknown slot must fail naming the slot, got %v", err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("a rejected slot must not be saved")
	}
}

func TestSettingsRejectsAnUnknownHarness(t *testing.T) {
	p := settingsFile(t)
	var out bytes.Buffer
	err := runSettings([]string{"role", "orchestrator", "harness", "gemini"}, &out, noModels)
	if err == nil || !strings.Contains(err.Error(), "harness") {
		t.Fatalf("unknown harness must fail naming the harness, got %v", err)
	}
	if err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("a harness typo should carry the usage line, got %v", err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("a rejected harness must not be saved")
	}
}

func TestSettingsRejectsARoleWithExtraArguments(t *testing.T) {
	p := settingsFile(t)
	var out bytes.Buffer
	if err := runSettings([]string{"role", "scout", "model", "haiku", "junk"}, &out, noModels); err == nil {
		t.Fatal("extra role arguments should fail")
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("a rejected role edit must not be saved")
	}
}

func TestSettingsRejectsAnUnknownRoleField(t *testing.T) {
	settingsFile(t)
	var out bytes.Buffer
	if err := runSettings([]string{"role", "orchestrator", "temperature", "0.5"}, &out, noModels); err == nil {
		t.Fatal("an unknown role field should fail")
	}
}

func TestSettingsRolePrintsTheNewValue(t *testing.T) {
	settingsFile(t)
	var out bytes.Buffer
	if err := runSettings([]string{"role", "spec-review", "model", "opus"}, &out, noModels); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "spec-review: harness pig (default), model opus (file)") {
		t.Fatalf("output %q does not confirm the new model with its source", out.String())
	}
}

// With no model chosen the effort is checked against every model the harness
// offers: one some model takes is saved, one none takes is rejected.
func TestSettingsChecksAnEffortWithNoModelChosen(t *testing.T) {
	settingsFile(t)
	src := modelList(map[string][]wick.Model{"claude": {
		{ID: "haiku"},
		{ID: "opus", Efforts: []string{"low", "high"}},
	}})
	var out bytes.Buffer
	if err := runSettings([]string{"role", "scout", "harness", "claude"}, &out, src); err != nil {
		t.Fatal(err)
	}
	if err := runSettings([]string{"role", "scout", "effort", "high"}, &out, src); err != nil {
		t.Fatalf("an effort some model takes should be accepted: %v", err)
	}
	if got := tools.LoadSettings().Roles["scout"].Effort; got != "high" {
		t.Fatalf("saved effort is %q, want high", got)
	}
	if err := runSettings([]string{"role", "scout", "effort", "off"}, &out, src); err == nil {
		t.Fatal("an effort no model of the harness takes should be rejected")
	}
}

// A harness switch with no model chosen keeps an effort some model of the new
// harness takes, and clears one none takes.
func TestSettingsHarnessSwitchAndAnEffortWithNoModel(t *testing.T) {
	p := settingsFile(t)
	write(t, p, `{"roles":{"scout":{"harness":"pig","effort":"high"}}}`)
	src := modelList(map[string][]wick.Model{"claude": {
		{ID: "haiku"},
		{ID: "opus", Efforts: []string{"low", "high"}},
	}})
	var out bytes.Buffer
	if err := runSettings([]string{"role", "scout", "harness", "claude"}, &out, src); err != nil {
		t.Fatal(err)
	}
	if got := tools.LoadSettings().Roles["scout"]; got != (tools.Role{Harness: "claude", Effort: "high"}) {
		t.Fatalf("harness change left %+v, want the effort kept", got)
	}
	// off is a pig level no claude model takes.
	if err := runSettings([]string{"role", "scout", "harness", "pig"}, &out, src); err != nil {
		t.Fatal(err)
	}
	write(t, p, `{"roles":{"scout":{"harness":"pig","effort":"off"}}}`)
	if err := runSettings([]string{"role", "scout", "harness", "claude"}, &out, src); err != nil {
		t.Fatal(err)
	}
	if got := tools.LoadSettings().Roles["scout"]; got != (tools.Role{Harness: "claude"}) {
		t.Fatalf("harness change left %+v, want the effort cleared", got)
	}
	if !strings.Contains(out.String(), "cleared") {
		t.Errorf("a cleared effort needs a notice, got %q", out.String())
	}
}

// An effort added to the harness since the cache was written is accepted after
// one refresh, not rejected.
func TestSettingsRefreshesOnceBeforeRejectingAnEffort(t *testing.T) {
	p := settingsFile(t)
	write(t, p, `{"roles":{"orchestrator":{"harness":"claude","model":"opus"}}}`)
	calls := 0
	src := func(harness string, refresh bool) ([]wick.Model, error) {
		calls++
		if refresh {
			return []wick.Model{{ID: "opus", Efforts: []string{"low", "high", "max"}}}, nil
		}
		return []wick.Model{{ID: "opus", Efforts: []string{"low", "high"}}}, nil
	}
	var out bytes.Buffer
	if err := runSettings([]string{"role", "orchestrator", "effort", "max"}, &out, src); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("the list was fetched %d times, want a cache read and one refresh", calls)
	}
	if got := tools.LoadSettings().Roles["orchestrator"].Effort; got != "max" {
		t.Fatalf("saved effort is %q, want max", got)
	}
}

// A model change clears an effort the new model does not take, with a notice.
func TestSettingsChangingTheModelClearsAnEffortItDoesNotTake(t *testing.T) {
	p := settingsFile(t)
	write(t, p, `{"roles":{"orchestrator":{"harness":"claude","model":"opus","effort":"max"}}}`)
	src := modelList(map[string][]wick.Model{"claude": {
		{ID: "opus", Efforts: []string{"low", "high", "max"}},
		{ID: "haiku"},
	}})
	var out bytes.Buffer
	if err := runSettings([]string{"role", "orchestrator", "model", "haiku"}, &out, src); err != nil {
		t.Fatal(err)
	}
	if got := tools.LoadSettings().Roles["orchestrator"]; got != (tools.Role{Harness: "claude", Model: "haiku"}) {
		t.Fatalf("model change left %+v, want the effort cleared", got)
	}
	if !strings.Contains(out.String(), "cleared") {
		t.Errorf("a cleared effort needs a notice, got %q", out.String())
	}
}

// An effort saved for a model the cache does not know costs one rebuild, not
// two: the lookup already refreshed before the effort is judged.
func TestSettingsRefreshesOnceForAnEffortOnAFreeTextModel(t *testing.T) {
	p := settingsFile(t)
	write(t, p, `{"roles":{"orchestrator":{"harness":"claude","model":"custom-1"}}}`)
	calls := 0
	src := func(harness string, refresh bool) ([]wick.Model, error) {
		calls++
		return []wick.Model{{ID: "opus", Efforts: []string{"low", "high"}}}, nil
	}
	var out bytes.Buffer
	if err := runSettings([]string{"role", "orchestrator", "effort", "high"}, &out, src); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("the list was fetched %d times, want one cache read and one refresh", calls)
	}
}

// The worker cap is readable and settable through the settings command and
// file, defaulting to three (spec story 44).
func TestSettingsSavesMaxWorkers(t *testing.T) {
	settingsFile(t)
	var out bytes.Buffer
	if err := runSettings([]string{"max-workers", "5"}, &out, noModels); err != nil {
		t.Fatal(err)
	}
	if got := tools.LoadSettings().MaxWorkers; got != 5 {
		t.Fatalf("saved max-workers is %d, want 5", got)
	}
	if !strings.Contains(out.String(), "max-workers: 5 (file)") {
		t.Fatalf("output %q does not confirm the new cap with its source", out.String())
	}
}

func TestSettingsRejectsBadMaxWorkers(t *testing.T) {
	p := settingsFile(t)
	var out bytes.Buffer
	for _, bad := range []string{"soon", "0", "-2"} {
		if err := runSettings([]string{"max-workers", bad}, &out, noModels); err == nil {
			t.Fatalf("max-workers %q should be an error", bad)
		}
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("a rejected max-workers must not be saved")
	}
}

func TestSettingsPrintsMaxWorkersDefault(t *testing.T) {
	settingsFile(t)
	var out bytes.Buffer
	if err := runSettings(nil, &out, noModels); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "max-workers: 3 (default)") {
		t.Fatalf("output %q does not show the default cap of three", out.String())
	}
}

// `settings tool` tunes one native tool: enabled, placement and rename, plus
// per-role overrides of enabled and placement.
func TestSettingsSetsToolFields(t *testing.T) {
	settingsFile(t)
	var out bytes.Buffer
	for _, argv := range [][]string{
		{"tool", "bash", "enabled", "false"},
		{"tool", "bash", "placement", "execute"},
		{"tool", "bash", "rename", "sh"},
	} {
		if err := runSettings(argv, &out, noModels); err != nil {
			t.Fatalf("%v: %v", argv, err)
		}
	}
	got := tools.LoadSettings().Tools["bash"]
	if got.Enabled == nil || *got.Enabled {
		t.Fatalf("bash enabled not saved: %+v", got)
	}
	if got.Placement != "execute" || got.Rename != "sh" {
		t.Fatalf("bash placement/rename not saved: %+v", got)
	}
	if !strings.Contains(out.String(), "bash") {
		t.Fatalf("output %q does not confirm the tool edit", out.String())
	}
}

func TestSettingsSetsToolRoleOverrides(t *testing.T) {
	settingsFile(t)
	var out bytes.Buffer
	if err := runSettings([]string{"tool", "bash", "role", "scout", "enabled", "false"}, &out, noModels); err != nil {
		t.Fatal(err)
	}
	if err := runSettings([]string{"tool", "bash", "role", "scout", "placement", "execute"}, &out, noModels); err != nil {
		t.Fatal(err)
	}
	ov := tools.LoadSettings().Tools["bash"].Roles["scout"]
	if ov.Enabled == nil || *ov.Enabled || ov.Placement != "execute" {
		t.Fatalf("scout override not saved: %+v", ov)
	}
}

func TestSettingsRejectsBadToolEdits(t *testing.T) {
	p := settingsFile(t)
	var out bytes.Buffer
	for _, argv := range [][]string{
		{"tool", "bash", "enabled", "soon"},
		{"tool", "bash", "placement", "sideways"},
		{"tool", "bash", "colour", "red"},
		{"tool", "bash", "role", "worker", "enabled", "false"},
		{"tool", "bash", "role", "scout", "rename", "x"},
		{"tool", "bash"},
	} {
		if err := runSettings(argv, &out, noModels); err == nil {
			t.Errorf("%v should be an error", argv)
		} else if !strings.Contains(err.Error(), "usage") {
			t.Errorf("%v error %q should carry the usage line", argv, err)
		}
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("a rejected tool edit must not be saved")
	}
}

// `settings mcp-server` registers and tunes an outside server: command, args,
// env, placement, include and exclude globs, rename mappings and per-role
// overrides.
func TestSettingsSetsMCPServerFields(t *testing.T) {
	settingsFile(t)
	var out bytes.Buffer
	for _, argv := range [][]string{
		{"mcp-server", "github", "command", "npx"},
		{"mcp-server", "github", "args", "-y,github-mcp"},
		{"mcp-server", "github", "env", "GITHUB_TOKEN=secret"},
		{"mcp-server", "github", "placement", "execute"},
		{"mcp-server", "github", "include", "search_*,get_*"},
		{"mcp-server", "github", "exclude", "delete_*"},
		{"mcp-server", "github", "rename", "search_repositories=gh_search"},
		{"mcp-server", "github", "role", "review", "placement", "main"},
	} {
		if err := runSettings(argv, &out, noModels); err != nil {
			t.Fatalf("%v: %v", argv, err)
		}
	}
	got := tools.LoadSettings().MCPServers["github"]
	if got.Command != "npx" || got.Placement != "execute" {
		t.Fatalf("server command/placement not saved: %+v", got)
	}
	if len(got.Args) != 2 || got.Args[0] != "-y" || got.Env["GITHUB_TOKEN"] != "secret" {
		t.Fatalf("server args/env not saved: %+v", got)
	}
	if len(got.Include) != 2 || len(got.Exclude) != 1 || got.Rename["search_repositories"] != "gh_search" {
		t.Fatalf("server globs/rename not saved: %+v", got)
	}
	if got.Roles["review"].Placement != "main" {
		t.Fatalf("server role override not saved: %+v", got.Roles)
	}
	if !strings.Contains(out.String(), "github") {
		t.Fatalf("output %q does not confirm the server edit", out.String())
	}
}

func TestSettingsRemovesAnMCPServer(t *testing.T) {
	p := settingsFile(t)
	// The server carries an unknown field: removing it must drop that too,
	// or the save resurrects the entry as a command-less server.
	write(t, p, `{"mcp_servers":{"github":{"command":"npx","future_field":"kept"}}}`)
	var out bytes.Buffer
	if err := runSettings([]string{"mcp-server", "remove", "github"}, &out, noModels); err != nil {
		t.Fatal(err)
	}
	if _, ok := tools.LoadSettings().MCPServers["github"]; ok {
		t.Fatal("removed server still saved")
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "github") {
		t.Fatalf("removed server resurrected in the file: %s", raw)
	}
	if err := runSettings([]string{"mcp-server", "remove", "github"}, &out, noModels); err == nil {
		t.Fatal("removing an unknown server should fail")
	}
}

func TestSettingsRejectsBadMCPServerEdits(t *testing.T) {
	p := settingsFile(t)
	var out bytes.Buffer
	for _, argv := range [][]string{
		{"mcp-server", "github", "placement", "sideways"},
		{"mcp-server", "github", "env", "NOEQUALS"},
		{"mcp-server", "github", "rename", "NOEQUALS"},
		{"mcp-server", "github", "include", "[unclosed"},
		{"mcp-server", "github", "role", "worker", "placement", "main"},
		{"mcp-server", "github"},
	} {
		if err := runSettings(argv, &out, noModels); err == nil {
			t.Errorf("%v should be an error", argv)
		} else if !strings.Contains(err.Error(), "usage") {
			t.Errorf("%v error %q should carry the usage line", argv, err)
		}
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("a rejected server edit must not be saved")
	}
}

// The bare command shows the new keys with their values and sources.
func TestSettingsPrintsToolsAndServers(t *testing.T) {
	p := settingsFile(t)
	write(t, p, `{"tools":{"bash":{"enabled":false,"rename":"sh","roles":{"scout":{"placement":"execute"}}}},"mcp_servers":{"github":{"command":"npx","placement":"execute","rename":{"a":"b"}}}}`)
	var out bytes.Buffer
	if err := runSettings(nil, &out, noModels); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		"bash: enabled false (file)",
		"rename sh (file)",
		"scout: enabled false (file), placement execute (file)",
		"github",
		`command "npx" (file)`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("settings output does not mention %q:\n%s", want, got)
		}
	}
}

// With nothing stored the new sections read as empty, not missing.
func TestSettingsPrintsEmptyToolsAndServers(t *testing.T) {
	settingsFile(t)
	var out bytes.Buffer
	if err := runSettings(nil, &out, noModels); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "tools:") || !strings.Contains(got, "mcp-servers:") {
		t.Errorf("settings output omits the new sections:\n%s", got)
	}
}

// Project values print with a project source, in the slots and the new
// sections alike.
func TestSettingsPrintsProjectSources(t *testing.T) {
	p := settingsFile(t)
	write(t, p, `{"max_workers":5,"roles":{"scout":{"model":"global-m"}}}`)
	project := filepath.Join(t.TempDir(), "project.json")
	write(t, project, `{"max_workers":7,"roles":{"scout":{"model":"project-m"}},"tools":{"bash":{"roles":{"scout":{"enabled":false}}}}}`)
	t.Setenv("WICK_PROJECT_SETTINGS", project)
	var out bytes.Buffer
	if err := runSettings(nil, &out, noModels); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		"max-workers: 7 (project)",
		"scout: harness pig (default), model project-m (project)",
		"scout: enabled false (project)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("settings output does not mention %q:\n%s", want, got)
		}
	}
}
