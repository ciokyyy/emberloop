package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"example.com/wick/tools"
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
