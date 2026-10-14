package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"example.com/emberloop/tools"
)

// `settings strip-rows` sets the header height; anything but 1 or 2 is an
// error that saves nothing.
func TestSettingsStripRows(t *testing.T) {
	settingsFile(t)
	var out bytes.Buffer
	if err := runSettings([]string{"strip-rows", "1"}, &out, noModels); err != nil {
		t.Fatal(err)
	}
	if got := tools.LoadSettings().EffectiveStripRows(); got != 1 {
		t.Fatalf("saved strip rows = %d, want 1", got)
	}
	if !strings.Contains(out.String(), "strip-rows: 1") {
		t.Fatalf("output %q does not confirm the new height", out.String())
	}
}

func TestSettingsStripRowsRejectsOther(t *testing.T) {
	p := settingsFile(t)
	var out bytes.Buffer
	if err := runSettings([]string{"strip-rows", "3"}, &out, noModels); err == nil {
		t.Fatal("strip-rows 3 should be an error")
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("a rejected strip-rows must not be saved")
	}
}

// `settings strip-frame` toggles the agent-colored frame; anything but
// on or off is an error.
func TestSettingsStripFrame(t *testing.T) {
	settingsFile(t)
	var out bytes.Buffer
	if err := runSettings([]string{"strip-frame", "off"}, &out, noModels); err != nil {
		t.Fatal(err)
	}
	if tools.LoadSettings().StripFrameEnabled() {
		t.Fatal("saved frame is on, want off")
	}
	if !strings.Contains(out.String(), "strip-frame: off") {
		t.Fatalf("output %q does not confirm the frame", out.String())
	}
	if err := runSettings([]string{"strip-frame", "maybe"}, &out, noModels); err == nil {
		t.Fatal("strip-frame maybe should be an error")
	}
}

// `settings strip-rule` restores the drawn rule line above the strip; the
// default stays the soft edge. Anything but on or off is an error.
func TestSettingsStripRule(t *testing.T) {
	settingsFile(t)
	var out bytes.Buffer
	if err := runSettings([]string{"strip-rule", "on"}, &out, noModels); err != nil {
		t.Fatal(err)
	}
	if !tools.LoadSettings().StripRuleEnabled() {
		t.Fatal("saved strip edge is soft, want the rule line")
	}
	if !strings.Contains(out.String(), "strip-rule: on") {
		t.Fatalf("output %q does not confirm the rule line", out.String())
	}
	if err := runSettings([]string{"strip-rule", "maybe"}, &out, noModels); err == nil {
		t.Fatal("strip-rule maybe should be an error")
	}
}

// `settings compress-model` names the summarizer model per harness; an
// unknown harness is an error.
func TestSettingsCompressModel(t *testing.T) {
	settingsFile(t)
	var out bytes.Buffer
	if err := runSettings([]string{"compress-model", "pig", "cheap-long"}, &out, noModels); err != nil {
		t.Fatal(err)
	}
	if got := tools.LoadSettings().EffectiveCompressModel("pig"); got != "cheap-long" {
		t.Fatalf("saved compress model = %q, want cheap-long", got)
	}
	if err := runSettings([]string{"compress-model", "nope", "m"}, &out, noModels); err == nil {
		t.Fatal("an unknown harness should be an error")
	}
}

// `settings auto-compress` flips the automatic trigger; anything but on or
// off is an error. Off stops auto-compress only: the manual key keeps
// working.
func TestSettingsAutoCompress(t *testing.T) {
	settingsFile(t)
	var out bytes.Buffer
	if err := runSettings([]string{"auto-compress", "off"}, &out, noModels); err != nil {
		t.Fatal(err)
	}
	if tools.LoadSettings().EffectiveAutoCompress() {
		t.Fatal("saved auto-compress is on, want off")
	}
	if !strings.Contains(out.String(), "auto-compress: off") {
		t.Fatalf("output %q does not confirm the switch", out.String())
	}
	if err := runSettings([]string{"auto-compress", "maybe"}, &out, noModels); err == nil {
		t.Fatal("auto-compress maybe should be an error")
	}
}

// `settings compress-cooldown` sets the tool calls after a compress that
// block a second auto-compress; anything but a whole number above zero is
// an error that saves nothing.
func TestSettingsCompressCooldown(t *testing.T) {
	settingsFile(t)
	var out bytes.Buffer
	if err := runSettings([]string{"compress-cooldown", "3"}, &out, noModels); err != nil {
		t.Fatal(err)
	}
	if got := tools.LoadSettings().EffectiveCompressCooldown(); got != 3 {
		t.Fatalf("saved cooldown = %d, want 3", got)
	}
	if !strings.Contains(out.String(), "compress-cooldown: 3") {
		t.Fatalf("output %q does not confirm the cooldown", out.String())
	}
	p := settingsFile(t)
	if err := runSettings([]string{"compress-cooldown", "0"}, &out, noModels); err == nil {
		t.Fatal("compress-cooldown 0 should be an error")
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("a rejected compress-cooldown must not be saved")
	}
}

// `settings strip-symbols` picks the strip glyph set; anything but nerd,
// unicode or ascii is an error that saves nothing.
func TestSettingsStripSymbols(t *testing.T) {
	settingsFile(t)
	var out bytes.Buffer
	if err := runSettings([]string{"strip-symbols", "nerd"}, &out, noModels); err != nil {
		t.Fatal(err)
	}
	if got := tools.LoadSettings().EffectiveStripSymbols(); got != tools.StripSymbolsNerd {
		t.Fatalf("saved strip symbols = %q, want nerd", got)
	}
	if !strings.Contains(out.String(), "strip-symbols: nerd") {
		t.Fatalf("output %q does not confirm the glyph set", out.String())
	}
}

func TestSettingsStripSymbolsRejectsOther(t *testing.T) {
	p := settingsFile(t)
	var out bytes.Buffer
	if err := runSettings([]string{"strip-symbols", "emoji"}, &out, noModels); err == nil {
		t.Fatal("strip-symbols emoji should be an error")
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("rejected strip-symbols must not save")
	}
}

// `settings strip-separator` picks the between-segment separator; anything
// outside the six is an error that saves nothing.
func TestSettingsStripSeparator(t *testing.T) {
	settingsFile(t)
	var out bytes.Buffer
	if err := runSettings([]string{"strip-separator", "rounded"}, &out, noModels); err != nil {
		t.Fatal(err)
	}
	if got := tools.LoadSettings().EffectiveStripSeparator(); got != tools.StripSeparatorRounded {
		t.Fatalf("saved strip separator = %q, want rounded", got)
	}
	if !strings.Contains(out.String(), "strip-separator: rounded") {
		t.Fatalf("output %q does not confirm the separator", out.String())
	}
}

func TestSettingsStripSeparatorRejectsOther(t *testing.T) {
	p := settingsFile(t)
	var out bytes.Buffer
	if err := runSettings([]string{"strip-separator", "arrow"}, &out, noModels); err == nil {
		t.Fatal("strip-separator arrow should be an error")
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("rejected strip-separator must not save")
	}
}
