package wick

// Ticket 12 red slice 1: padding defaults come from settings.
// Empty settings mean two columns left/right and one row at the bottom;
// explicit zero restores the edge-to-edge layout.
import (
	"testing"

	"example.com/wick/tools"
)

func TestEffectivePadDefaults(t *testing.T) {
	s := tools.Settings{}
	if got := s.EffectivePadLeft(); got != 4 {
		t.Errorf("pad left = %d, want 4", got)
	}
	if got := s.EffectivePadRight(); got != 4 {
		t.Errorf("pad right = %d, want 4", got)
	}
	if got := s.EffectivePadBottom(); got != 2 {
		t.Errorf("pad bottom = %d, want 2", got)
	}
}

// noPadSettings is the edge-to-edge layout the older wrapper tests assume.
func noPadSettings() tools.Settings { return withNoPad(tools.ContextSettings{}) }

// withNoPad is c with every padding value set to zero.
func withNoPad(c tools.ContextSettings) tools.Settings {
	zero := 0
	c.StripPadLeft, c.StripPadRight, c.StripPadBottom = &zero, &zero, &zero
	return tools.Settings{Context: &c}
}

func TestChildSizePad(t *testing.T) {
	w, h := ChildSizePad(100, 30, 2, false, Pad{Left: 4, Right: 4, Bottom: 2})
	if w != 92 || h != 25 {
		t.Errorf("no frame = %dx%d, want 92x25", w, h)
	}
	w, h = ChildSizePad(100, 30, 2, true, Pad{Left: 4, Right: 4, Bottom: 2})
	if w != 90 || h != 25 {
		t.Errorf("frame = %dx%d, want 90x25", w, h)
	}
	if w, h = ChildSizePad(100, 30, 2, false, Pad{}); w != 100 || h != 27 {
		t.Errorf("zero pad = %dx%d, want 100x27", w, h)
	}
}

func TestFitPadShrinksNarrowTerminals(t *testing.T) {
	got := FitPad(30, 30, 2, false, Pad{Left: 4, Right: 4, Bottom: 2})
	if w, _ := ChildSizePad(30, 30, 2, false, got); w < minPadChildW {
		t.Errorf("child width %d under %d with pad %+v", w, minPadChildW, got)
	}
	got = FitPad(100, 9, 2, false, Pad{Left: 4, Right: 4, Bottom: 2})
	if got.Bottom != 1 || got.Left != 4 {
		t.Errorf("short terminal pad = %+v, want bottom 1 and sides kept", got)
	}
	if got = FitPad(100, 30, 2, false, Pad{Left: 4, Right: 4, Bottom: 2}); got != (Pad{4, 4, 2}) {
		t.Errorf("roomy terminal pad = %+v, want unchanged", got)
	}
}

func TestWrapMarginSetupPad(t *testing.T) {
	if got := wrapMarginSetupPad(100, false, Pad{Left: 4, Right: 4}); got != "\x1b[?69h\x1b[5;96s" {
		t.Errorf("margins = %q", got)
	}
	if got := wrapMarginSetupPad(100, true, Pad{Left: 4, Right: 4}); got != "\x1b[?69h\x1b[6;95s" {
		t.Errorf("framed margins = %q", got)
	}
	if got := wrapMarginSetupPad(100, false, Pad{}); got != "" {
		t.Errorf("zero pad margins = %q, want none", got)
	}
}
