package tui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	emberloop "example.com/emberloop"
	"example.com/emberloop/tools"
	tea "github.com/charmbracelet/bubbletea"
)

// Seams under test (tdd): Loader (the runs tree, reused from model_test.go),
// SettingsBackend (Load/Save are the shared settings functions pointed at a
// temp file, so an edit the tab saves is one the CLI reads back; Models,
// Harnesses and ArchiveSize are stubbed), and tea messages through
// Model.Update with assertions on the stripped View and the saved file.

// settingsStub answers canned model lists per harness and counts refreshes,
// the way the harness cache behaves behind CachedModels.
type settingsStub struct {
	lists     map[string][]emberloop.Model
	refreshes map[string]int
	listErr   map[string]error
}

func (s *settingsStub) models(harness string, refresh bool) ([]emberloop.Model, error) {
	if refresh {
		s.refreshes[harness]++
	}
	if err, ok := s.listErr[harness]; ok {
		return nil, err
	}
	return s.lists[harness], nil
}

// testSettingsBackend points the shared settings store at a temp file and
// clears the env overrides, so the tab reads and writes only the test's file
// through the same functions the CLI uses.
func testSettingsBackend(t *testing.T, stub *settingsStub, archiveBytes *int64) SettingsBackend {
	t.Helper()
	t.Setenv("EMBERLOOP_SETTINGS", filepath.Join(t.TempDir(), "settings.json"))
	t.Setenv("EMBERLOOP_DECISION_MODEL", "")
	t.Setenv("EMBERLOOP_FIND_THRESHOLD", "")
	t.Setenv("EMBERLOOP_MAX_WORKERS", "")
	return SettingsBackend{
		Load:        func() (tools.Settings, error) { return tools.LoadSettings(), nil },
		Save:        tools.SaveSettings,
		Models:      stub.models,
		Harnesses:   func() []string { return []string{"claude", "pig"} },
		ArchiveSize: func() int64 { return *archiveBytes },
	}
}

func writeSettingsFile(t *testing.T, body string) {
	t.Helper()
	if err := os.WriteFile(os.Getenv("EMBERLOOP_SETTINGS"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// enterSettings builds a model over the canned runs tree with the settings
// backend and switches to the Settings tab.
func enterSettings(t *testing.T, be SettingsBackend) Model {
	t.Helper()
	snap := cannedSnapshot("/repo-a", "/repo-b")
	var calls int
	m := loadModel(t, &snap, &calls)
	m.settings.backend = be
	m, _ = pressKey(m, "tab")
	if m.tab != tabSettings {
		t.Fatalf("after tab = %v, want settings", m.tab)
	}
	return m
}

var ansiStrip = regexp.MustCompile("\x1b\\[[0-9;]*m")

// plainView strips styling escape codes, so assertions read the words the
// tab shows rather than the colours around the cursor.
func plainView(m Model) string { return ansiStrip.ReplaceAllString(m.View(), "") }

// A stored slot reads with its file source beside every field, an unset slot
// reads as the default in force, and the decision model, threshold and
// archive size are listed too.
func TestSettingsTabListsSlotsWithSources(t *testing.T) {
	stub := &settingsStub{lists: map[string][]emberloop.Model{
		"pig":    {{ID: "deepseek/flash", Efforts: []string{"off", "high"}}},
		"claude": {{ID: "opus", Efforts: []string{"low", "high"}}},
	}, refreshes: map[string]int{}}
	var archive int64 = 12 * 1024 * 1024
	be := testSettingsBackend(t, stub, &archive)
	writeSettingsFile(t, `{"roles":{"orchestrator":{"harness":"claude","model":"opus","effort":"high"}}}`)
	m := enterSettings(t, be)
	got := plainView(m)
	for _, want := range []string{
		"orchestrator: harness claude (file), model opus (file), effort high (file)",
		"implementer: harness pig (default), model - (default), effort - (default)",
		"scout: harness pig (default), model - (default), effort - (default)",
		"decision-model: typesafe/jev-1.13 (default)",
		"threshold: 0.2 (default)",
		"archive: 12.0 MB",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("settings view does not mention %q:\n%s", want, got)
		}
	}
}

// Switching a slot's harness keeps the model and effort the new harness
// still offers, and resets what it does not with a notice. Both persist
// through the shared save, so the CLI reads them back.
func TestSettingsHarnessSwitchKeepsValidAndResetsInvalid(t *testing.T) {
	stub := &settingsStub{lists: map[string][]emberloop.Model{
		"pig":    {{ID: "m", Efforts: []string{"off", "high"}}},
		"claude": {{ID: "m", Efforts: []string{"off", "high"}}, {ID: "opus", Efforts: []string{"low"}}},
	}, refreshes: map[string]int{}}
	var archive int64
	be := testSettingsBackend(t, stub, &archive)
	writeSettingsFile(t, `{"roles":{"scout":{"harness":"pig","model":"m","effort":"high"}}}`)
	m := enterSettings(t, be)
	for i := 0; i < 4; i++ {
		m, _ = pressKey(m, "j")
	}
	m, _ = pressKey(m, "enter")
	if got := tools.LoadSettings().Roles["scout"]; got != (tools.Role{Harness: "claude", Model: "m", Effort: "high"}) {
		t.Fatalf("after the switch scout is %+v, want model and effort kept", got)
	}
	if m.settings.notice != "" {
		t.Fatalf("keeping everything needs no notice, got %q", m.settings.notice)
	}

	writeSettingsFile(t, `{"roles":{"scout":{"harness":"pig","model":"gone","effort":"ultra"}}}`)
	m.reloadSettingsValues()
	m, _ = pressKey(m, "enter")
	if got := tools.LoadSettings().Roles["scout"]; got != (tools.Role{Harness: "claude"}) {
		t.Fatalf("after the switch scout is %+v, want model and effort cleared", got)
	}
	if !strings.Contains(m.settings.notice, "cleared") ||
		!strings.Contains(m.settings.notice, `"gone"`) ||
		!strings.Contains(m.settings.notice, `"ultra"`) {
		t.Fatalf("a reset needs a notice naming both, got %q", m.settings.notice)
	}
}

// The model picker offers the harness's cached list: picking a model saves
// it, typing enters free text, and an effort the new model does not take is
// cleared with a notice.
func TestSettingsModelPickerPicksAndFreesText(t *testing.T) {
	stub := &settingsStub{lists: map[string][]emberloop.Model{
		"pig":    {{ID: "m", Efforts: []string{"off", "high"}}},
		"claude": {{ID: "opus", Efforts: []string{"low", "high"}}, {ID: "haiku"}},
	}, refreshes: map[string]int{}}
	var archive int64
	be := testSettingsBackend(t, stub, &archive)
	writeSettingsFile(t, `{"roles":{"scout":{"harness":"claude","model":"opus","effort":"high"}}}`)
	m := enterSettings(t, be)
	for i := 0; i < 4; i++ {
		m, _ = pressKey(m, "j")
	}
	m, _ = pressKey(m, "l")
	m, _ = pressKey(m, "enter")
	if !m.settings.pickerOpen {
		t.Fatal("enter on the model must open the picker")
	}
	got := plainView(m)
	for _, want := range []string{"opus", "haiku"} {
		if !strings.Contains(got, want) {
			t.Fatalf("picker view does not offer %q:\n%s", want, got)
		}
	}
	m, _ = pressKey(m, "down")
	m, _ = pressKey(m, "enter")
	if got := tools.LoadSettings().Roles["scout"]; got != (tools.Role{Harness: "claude", Model: "haiku"}) {
		t.Fatalf("after picking haiku scout is %+v, want the model saved and the effort cleared", got)
	}
	if !strings.Contains(m.settings.notice, "cleared") {
		t.Fatalf("dropping the effort needs a notice, got %q", m.settings.notice)
	}

	m, _ = pressKey(m, "enter")
	for _, r := range []string{"c", "u", "s", "t", "o", "m", "-", "9"} {
		m, _ = pressKey(m, r)
	}
	m, _ = pressKey(m, "enter")
	if got := tools.LoadSettings().Roles["scout"]; got != (tools.Role{Harness: "claude", Model: "custom-9"}) {
		t.Fatalf("after free text scout is %+v, want the custom model saved", got)
	}
}

// The effort row shows the selected model's levels with the chosen one
// bracketed; moving across levels never saves, enter applies. A model with
// no levels shows no row.
func TestSettingsEffortRowAppliesLevels(t *testing.T) {
	stub := &settingsStub{lists: map[string][]emberloop.Model{
		"pig":    {{ID: "m", Efforts: []string{"off"}}},
		"claude": {{ID: "opus", Efforts: []string{"low", "high", "max"}}},
	}, refreshes: map[string]int{}}
	var archive int64
	be := testSettingsBackend(t, stub, &archive)
	writeSettingsFile(t, `{"roles":{"scout":{"harness":"claude","model":"opus","effort":"high"}}}`)
	m := enterSettings(t, be)
	for i := 0; i < 4; i++ {
		m, _ = pressKey(m, "j")
	}
	if got := plainView(m); !strings.Contains(got, "low  [high]  max") {
		t.Fatalf("effort row must bracket the chosen level:\n%s", got)
	}
	m, _ = pressKey(m, "l")
	m, _ = pressKey(m, "l")
	if got := tools.LoadSettings().Roles["scout"].Effort; got != "high" {
		t.Fatalf("moving across levels saved effort %q, want no save", got)
	}
	m, _ = pressKey(m, "enter")
	if got := tools.LoadSettings().Roles["scout"].Effort; got != "low" {
		t.Fatalf("after enter effort is %q, want low", got)
	}
	if got := plainView(m); !strings.Contains(got, "[low]") {
		t.Fatalf("effort row must bracket the new level:\n%s", got)
	}
}

// A model with no levels shows no effort row at all.
func TestSettingsEffortRowHiddenWithoutLevels(t *testing.T) {
	stub := &settingsStub{lists: map[string][]emberloop.Model{
		"pig":    {{ID: "m"}},
		"claude": {{ID: "solo"}},
	}, refreshes: map[string]int{}}
	var archive int64
	be := testSettingsBackend(t, stub, &archive)
	writeSettingsFile(t, `{"roles":{"scout":{"harness":"claude","model":"solo"}}}`)
	m := enterSettings(t, be)
	got := plainView(m)
	if !strings.Contains(got, "scout: harness claude (file), model solo (file), effort - (default)") {
		t.Fatalf("slot line must still show the effort value:\n%s", got)
	}
	if strings.Contains(got, "[") {
		t.Fatalf("no levels means no bracketed row:\n%s", got)
	}
}

// Clicks change values too: the harness cycles, the model opens the picker
// and an option picks, and a level applies.
func TestSettingsClicksChangeHarnessModelAndEffort(t *testing.T) {
	stub := &settingsStub{lists: map[string][]emberloop.Model{
		"pig":    {{ID: "m", Efforts: []string{"off", "high"}}},
		"claude": {{ID: "opus", Efforts: []string{"low", "high"}}, {ID: "haiku"}},
	}, refreshes: map[string]int{}}
	var archive int64
	be := testSettingsBackend(t, stub, &archive)
	writeSettingsFile(t, `{"roles":{"scout":{"harness":"pig","model":"m","effort":"high"}}}`)
	m := enterSettings(t, be)

	_ = m.View()
	x, y := waitZone(t, m, settingsHarnessZone("scout"))
	updated, _ := m.Update(tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	m = updated.(Model)
	if got := tools.LoadSettings().Roles["scout"]; got != (tools.Role{Harness: "claude"}) {
		t.Fatalf("after clicking the harness scout is %+v, want the switch with model and effort cleared", got)
	}

	_ = m.View()
	x, y = waitZone(t, m, settingsModelZone("scout"))
	updated, _ = m.Update(tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	m = updated.(Model)
	if !m.settings.pickerOpen {
		t.Fatal("clicking the model must open the picker")
	}
	_ = m.View()
	x, y = waitZone(t, m, settingsPickerZone(0))
	updated, _ = m.Update(tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	m = updated.(Model)
	if got := tools.LoadSettings().Roles["scout"]; got != (tools.Role{Harness: "claude", Model: "opus"}) {
		t.Fatalf("after clicking the option scout is %+v, want opus picked", got)
	}

	_ = m.View()
	x, y = waitZone(t, m, settingsLevelZone("scout", 1))
	updated, _ = m.Update(tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	m = updated.(Model)
	if got := tools.LoadSettings().Roles["scout"].Effort; got != "high" {
		t.Fatalf("after clicking the level effort is %q, want high", got)
	}
}

var errTestUnavailable = errTest("unavailable")

type errTest string

func (e errTest) Error() string { return string(e) }

// `r` rebuilds every harness's model list, so new models show up; a
// harness that cannot be listed keeps its cached list with a notice.
func TestSettingsRefreshRebuildsModelLists(t *testing.T) {
	stub := &settingsStub{lists: map[string][]emberloop.Model{
		"pig":    {{ID: "m", Efforts: []string{"off"}}},
		"claude": {{ID: "opus", Efforts: []string{"low"}}},
	}, refreshes: map[string]int{}}
	var archive int64
	be := testSettingsBackend(t, stub, &archive)
	m := enterSettings(t, be)
	m, cmd := pressKey(m, "r")
	if cmd == nil {
		t.Fatal("r must schedule a refresh")
	}
	updated, next := m.Update(cmd())
	m = updated.(Model)
	_ = next
	if stub.refreshes["pig"] != 1 || stub.refreshes["claude"] != 1 {
		t.Fatalf("refreshes = %v, want one rebuild per harness", stub.refreshes)
	}
	if !strings.Contains(m.settings.notice, "refreshed") {
		t.Fatalf("a clean refresh needs a notice, got %q", m.settings.notice)
	}

	stub.lists["claude"] = []emberloop.Model{{ID: "opus", Efforts: []string{"low"}}, {ID: "haiku"}}
	stub.listErr = map[string]error{"pig": errTestUnavailable}
	m, cmd = pressKey(m, "r")
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if len(m.settings.lists["claude"]) != 2 {
		t.Fatalf("claude list = %v, want the rebuilt two models", m.settings.lists["claude"])
	}
	if len(m.settings.lists["pig"]) != 1 {
		t.Fatalf("pig list = %v, want the cached list kept", m.settings.lists["pig"])
	}
	if !strings.Contains(m.settings.notice, "pig") {
		t.Fatalf("a failed harness needs a notice naming it, got %q", m.settings.notice)
	}
}

// The decision model and threshold edit through free text and persist
// through the shared save; a bad threshold is rejected without saving.
func TestSettingsDecisionAndThresholdEdit(t *testing.T) {
	stub := &settingsStub{lists: map[string][]emberloop.Model{
		"pig":    {{ID: "m"}},
		"claude": {{ID: "opus"}},
	}, refreshes: map[string]int{}}
	var archive int64
	be := testSettingsBackend(t, stub, &archive)
	writeSettingsFile(t, `{}`)
	m := enterSettings(t, be)
	for i := 0; i < len(tools.RoleSlots); i++ {
		m, _ = pressKey(m, "j")
	}
	m, _ = pressKey(m, "enter")
	for _, r := range []string{"t", "y", "p", "e", "s", "a", "f", "e", "/", "j", "e", "v", "-", "l", "a", "t", "e", "s", "t"} {
		m, _ = pressKey(m, r)
	}
	m, _ = pressKey(m, "enter")
	if got := tools.LoadSettings().DecisionModel; got != "typesafe/jev-1.13" {
		t.Fatalf("saved decision model is %q, want the canonical pinned id", got)
	}

	m, _ = pressKey(m, "j")
	m, _ = pressKey(m, "enter")
	for _, r := range []string{"0", ".", "4", "2"} {
		m, _ = pressKey(m, r)
	}
	m, _ = pressKey(m, "enter")
	if got := tools.LoadSettings().Threshold; got != 0.42 {
		t.Fatalf("saved threshold is %v, want 0.42", got)
	}

	m, _ = pressKey(m, "enter")
	m, _ = pressKey(m, "s")
	m, _ = pressKey(m, "o")
	m, _ = pressKey(m, "o")
	m, _ = pressKey(m, "n")
	m, _ = pressKey(m, "enter")
	if got := tools.LoadSettings().Threshold; got != 0.42 {
		t.Fatalf("a rejected threshold must not save, file holds %v", got)
	}
	if !strings.Contains(m.settings.notice, "not a number above zero") {
		t.Fatalf("a rejected threshold needs a notice, got %q", m.settings.notice)
	}
}

// A poll tick re-reads the file, so a CLI edit shows up; an open editor or
// picker holds its own state through the tick.
func TestSettingsPollReloadsEditedFile(t *testing.T) {
	stub := &settingsStub{lists: map[string][]emberloop.Model{
		"pig":    {{ID: "m"}},
		"claude": {{ID: "opus"}},
	}, refreshes: map[string]int{}}
	var archive int64
	be := testSettingsBackend(t, stub, &archive)
	writeSettingsFile(t, `{"roles":{"scout":{"harness":"pig","model":"m"}}}`)
	m := enterSettings(t, be)
	if got := plainView(m); !strings.Contains(got, "scout: harness pig (file), model m (file)") {
		t.Fatalf("tab must show the file first:\n%s", got)
	}

	writeSettingsFile(t, `{"roles":{"scout":{"harness":"claude","model":"opus"}}}`)
	updated, cmd := m.Update(tickMsg{})
	m = runCmds(updated.(Model), cmd)
	if got := plainView(m); !strings.Contains(got, "scout: harness claude (file), model opus (file)") {
		t.Fatalf("a tick must pick up the CLI edit:\n%s", got)
	}

	// An open picker is not disturbed by the tick.
	for i := 0; i < 4; i++ {
		m, _ = pressKey(m, "j")
	}
	m, _ = pressKey(m, "l")
	m, _ = pressKey(m, "enter")
	if !m.settings.pickerOpen {
		t.Fatal("enter on the model must open the picker")
	}
	writeSettingsFile(t, `{"roles":{"scout":{"harness":"pig","model":"m"}}}`)
	updated, cmd = m.Update(tickMsg{})
	m = runCmds(updated.(Model), cmd)
	if !m.settings.pickerOpen {
		t.Fatal("a tick must not close the picker")
	}
	if got := m.settings.values.Roles["scout"]; got != (tools.Role{Harness: "claude", Model: "opus"}) {
		t.Fatalf("a tick must not rewrite values under an open picker, got %+v", got)
	}
}

// In the picker every rune reaches free text — even one that is a hotkey
// elsewhere — while arrows move the highlight.
func TestSettingsPickerFreeTextKeepsHotkeyRunes(t *testing.T) {
	stub := &settingsStub{lists: map[string][]emberloop.Model{
		"pig":    {{ID: "m"}},
		"claude": {{ID: "opus"}, {ID: "haiku"}},
	}, refreshes: map[string]int{}}
	var archive int64
	be := testSettingsBackend(t, stub, &archive)
	writeSettingsFile(t, `{"roles":{"scout":{"harness":"claude"}}}`)
	m := enterSettings(t, be)
	for i := 0; i < 4; i++ {
		m, _ = pressKey(m, "j")
	}
	m, _ = pressKey(m, "l")
	m, _ = pressKey(m, "enter")
	m, _ = pressKey(m, "down")
	if m.settings.pickerIndex != 1 {
		t.Fatalf("down moved the highlight to %d, want haiku at 1", m.settings.pickerIndex)
	}
	m, _ = pressKey(m, "r")
	if m.settings.editing != editSlotModel {
		t.Fatal("r in the picker must open free text, not refresh")
	}
	m, _ = pressKey(m, "1")
	m, _ = pressKey(m, "enter")
	if got := tools.LoadSettings().Roles["scout"]; got != (tools.Role{Harness: "claude", Model: "r1"}) {
		t.Fatalf("after free text scout is %+v, want model r1", got)
	}
}

// A saved value reads back with its file source at once, not a tick later.
func TestSettingsSavedValueShowsFileSource(t *testing.T) {
	stub := &settingsStub{lists: map[string][]emberloop.Model{
		"pig":    {{ID: "m"}},
		"claude": {{ID: "opus"}},
	}, refreshes: map[string]int{}}
	var archive int64
	be := testSettingsBackend(t, stub, &archive)
	writeSettingsFile(t, `{}`)
	m := enterSettings(t, be)
	for i := 0; i < len(tools.RoleSlots); i++ {
		m, _ = pressKey(m, "j")
	}
	m, _ = pressKey(m, "enter")
	m, _ = pressKey(m, "x")
	m, _ = pressKey(m, "enter")
	if got := plainView(m); !strings.Contains(got, "decision-model: x (file)") {
		t.Fatalf("a saved decision model must read (file) at once:\n%s", got)
	}
}
