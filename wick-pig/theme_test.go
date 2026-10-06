package wick

// Seam: the theme generators through their JSON and byte output. Expected
// values come from the approved probe assets in
// .scratch/input-queue/theme (the borderless wickb pair, run-themed.sh),
// not from the code: the curated tokens below are copied from those files.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustThemeJSON(t *testing.T, gen func(bool) (string, error), dark bool) string {
	t.Helper()
	s, err := gen(dark)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// The generated Mocha pair matches the approved probe look: core tokens and
// the surface-tone borders of the borderless variants.
func TestGeneratedMochaMatchesProbe(t *testing.T) {
	cj := mustThemeJSON(t, ClaudeThemeJSON, true)
	var c struct {
		Overrides map[string]string `json:"overrides"`
	}
	if err := json.Unmarshal([]byte(cj), &c); err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{
		"claude": "#cba6f7", "text": "#cdd6f4", "inverseText": "#181825",
		"promptBorder": "#313244", "bashBorder": "#313244",
		"suggestion": "#f5c2e7", "remember": "#94e2d5",
	} {
		if c.Overrides[k] != want {
			t.Errorf("claude mocha %s = %q, want %q", k, c.Overrides[k], want)
		}
	}
	pj := mustThemeJSON(t, PigThemeJSON, true)
	var p struct {
		Vars   map[string]string `json:"vars"`
		Colors map[string]string `json:"colors"`
	}
	if err := json.Unmarshal([]byte(pj), &p); err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{
		"text": "#cdd6f4", "mauve": "#cba6f7", "mantle": "#181825",
	} {
		if p.Vars[k] != want {
			t.Errorf("pig mocha var %s = %q, want %q", k, p.Vars[k], want)
		}
	}
	for k, want := range map[string]string{
		"accent": "#f5c2e7",
		"border": "surface0", "borderAccent": "surface0", "borderMuted": "surface0",
		"mdCodeBlockBorder": "surface0", "mdQuoteBorder": "surface0", "mdHr": "surface0",
	} {
		if p.Colors[k] != want {
			t.Errorf("pig mocha color %s = %q, want %q", k, p.Colors[k], want)
		}
	}
}

// One palette source feeds the strip and both generators: the strip roles
// equal the generator tokens.
func TestPaletteFeedsStripAndGenerators(t *testing.T) {
	mocha, latte := WickMocha(), WickLatte()
	if mocha.StripBg != "#181825" || latte.StripBg != "#e6e9ef" {
		t.Errorf("strip bg drifts: %q %q", mocha.StripBg, latte.StripBg)
	}
	if mocha.Hues[0].Dark != "#cba6f7" || latte.Hues[0].Light != "#8839ef" {
		t.Error("orchestrator hue drifts from mauve")
	}
	var p struct {
		Vars map[string]string `json:"vars"`
	}
	if err := json.Unmarshal([]byte(mustThemeJSON(t, PigThemeJSON, true)), &p); err != nil {
		t.Fatal(err)
	}
	if p.Vars["mauve"] != mocha.Hues[0].Dark {
		t.Error("pig mauve drifts from the palette")
	}
}

// A Latte variant exists for both harnesses, generated from the same
// source: light base, Latte text, surface-tone borders.
func TestLatteVariantExists(t *testing.T) {
	cj := mustThemeJSON(t, ClaudeThemeJSON, false)
	var c struct {
		Base      string            `json:"base"`
		Overrides map[string]string `json:"overrides"`
	}
	if err := json.Unmarshal([]byte(cj), &c); err != nil {
		t.Fatal(err)
	}
	if c.Base != "light" {
		t.Errorf("claude latte base = %q, want light", c.Base)
	}
	for k, want := range map[string]string{
		"text": "#4c4f69", "claude": "#8839ef", "promptBorder": "#ccd0da",
	} {
		if c.Overrides[k] != want {
			t.Errorf("claude latte %s = %q, want %q", k, c.Overrides[k], want)
		}
	}
	pj := mustThemeJSON(t, PigThemeJSON, false)
	var p struct {
		Appearance string            `json:"appearance"`
		Vars       map[string]string `json:"vars"`
		Colors     map[string]string `json:"colors"`
	}
	if err := json.Unmarshal([]byte(pj), &p); err != nil {
		t.Fatal(err)
	}
	if p.Appearance != "light" {
		t.Errorf("pig latte appearance = %q, want light", p.Appearance)
	}
	if p.Vars["text"] != "#4c4f69" || p.Vars["mantle"] != "#e6e9ef" {
		t.Errorf("pig latte vars drift: %v", p.Vars)
	}
	if p.Colors["border"] != "surface0" {
		t.Errorf("pig latte border = %q, want surface0", p.Colors["border"])
	}
}

// Golden fixtures: the generated files compare byte-equal to the checked-in
// pair, so a palette edit shows up as a fixture diff.
func TestGeneratedThemesMatchFixtures(t *testing.T) {
	for _, dark := range []bool{true, false} {
		v := ThemeVariant(dark)
		for file, gen := range map[string]func(bool) (string, error){
			"claude-" + v + ".json": ClaudeThemeJSON,
			"pig-" + v + ".json":    PigThemeJSON,
		} {
			got, err := gen(dark)
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(filepath.Join("testdata", "theme", file))
			if err != nil {
				t.Fatal(err)
			}
			if got != string(want) {
				t.Errorf("%s differs from fixture; re-check against the probe", file)
			}
		}
	}
}

// The OSC set sequence is the probe's palette in the probe's order:
// background, foreground, cursor, then ANSI 0-15.
func TestOSCSetMatchesProbe(t *testing.T) {
	got := ThemeSetSequence(true)
	if !strings.HasPrefix(got, "\x1b]11;#1e1e2e\x07\x1b]10;#cdd6f4\x07\x1b]12;#f5e0dc\x07") {
		t.Errorf("mocha set prefix drifts: %q", got[:60])
	}
	for _, want := range []string{
		"\x1b]4;0;#45475a\x07", "\x1b]4;7;#bac2de\x07",
		"\x1b]4;8;#585b70\x07", "\x1b]4;15;#a6adc8\x07",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("mocha set misses %q", want)
		}
	}
	latte := ThemeSetSequence(false)
	if !strings.HasPrefix(latte, "\x1b]11;#eff1f5\x07\x1b]10;#4c4f69\x07\x1b]12;#dc8a78\x07") {
		t.Errorf("latte set prefix drifts: %q", latte[:60])
	}
}

// Exit resets the pane: OSC 104, 110, 111, 112 in the spec's order.
func TestOSCResetSequence(t *testing.T) {
	if got, want := ThemeResetSequence(), "\x1b]104\x07\x1b]110\x07\x1b]111\x07\x1b]112\x07"; got != want {
		t.Errorf("reset = %q, want %q", got, want)
	}
}

// Launch uses per-run flags only: pig gains --theme/--use-theme pointing at
// the generated file, claude gains the theme ref inside its launch settings.
func TestThemeLaunchArgs(t *testing.T) {
	dir := t.TempDir()
	pargs, err := ThemeLaunchArgs("pig", []string{"--session-id", "s"}, true, dir)
	if err != nil {
		t.Fatal(err)
	}
	theme, name := "", ""
	for i, a := range pargs {
		if a == "--theme" {
			theme = pargs[i+1]
		}
		if a == "--use-theme" {
			name = pargs[i+1]
		}
	}
	if name != "wick-mocha" {
		t.Errorf("pig use-theme = %q, want wick-mocha", name)
	}
	if b, err := os.ReadFile(theme); err != nil || !strings.Contains(string(b), `"wick-mocha"`) {
		t.Errorf("pig --theme points at %q, want the generated file", theme)
	}
	cargs, err := ThemeLaunchArgs("claude", []string{"--settings", `{"hooks":{}}`}, false, dir)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	for i, a := range cargs {
		if a == "--settings" {
			if err := json.Unmarshal([]byte(cargs[i+1]), &m); err != nil {
				t.Fatal(err)
			}
		}
	}
	if m["theme"] != "custom:wick-latte" {
		t.Errorf("claude theme = %v, want custom:wick-latte", m["theme"])
	}
	if _, ok := m["hooks"]; !ok {
		t.Error("claude launch settings lost the stop gate")
	}
}
