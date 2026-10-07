package wick

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// Seam: Nerd Font codepoint table (ticket 10) — every nerd glyph pinned to
// its Nerd Fonts v3 codepoint. Expected values come from the Nerd Fonts
// cheat sheet and the v3 Glyph Sets wiki page, never the implementation.
// No Material Design icon is used: v3 moved mdi out of the legacy
// U+F500-U+FD46 range to U+F0001+, and the old range renders wrong.

// Every nerd icon, as an independent escape from the cheat sheet. A stale
// codepoint fails here before it can render as a wrong glyph.
func TestStripNerdCodepointsPinned(t *testing.T) {
	want := map[string]string{
		"agent":    "", // U+F007 fa-user
		"ctx":      "", // U+F200 fa-chart-pie
		"tokens":   "", // U+F1C0 fa-database
		"cost":     "", // U+F155 fa-dollar-sign
		"model":    "", // U+F2DB fa-microchip
		"tools":    "", // U+F0AD fa-wrench
		"compress": "", // U+F066 fa-compress
		"working":  "", // U+F111 fa-circle
		"idle":     "", // U+F10C fa-circle regular
		"queued":   "", // U+F017 fa-clock
		"sync":     "", // U+F021 fa-sync
		"warning":  "", // U+F071 fa-warning
		"raw":      "", // U+F11C fa-keyboard
	}
	table := StripNerdIconTable()
	if len(table) != len(want) {
		t.Fatalf("nerd icon table has %d entries, want %d", len(table), len(want))
	}
	for name, glyph := range want {
		if table[name] != glyph {
			t.Errorf("nerd icon %q = %q (%U), want %q (%U)", name, table[name], []rune(table[name]), glyph, []rune(glyph))
		}
	}
}

// Every nerd glyph sits in a Nerd Fonts v3 range from the Glyph Sets wiki:
// Font Awesome U+ED00-U+F2FF, Powerline U+E0A0-U+E0A2/U+E0B0-U+E0B3,
// Powerline Extra U+E0A3/U+E0B4-U+E0C8/U+E0CA/U+E0CC-U+E0D7.
func TestStripNerdCodepointsInV3Ranges(t *testing.T) {
	inV3 := func(r rune) bool {
		switch {
		case r >= 0xED00 && r <= 0xF2FF:
			return true
		case r >= 0xE0A0 && r <= 0xE0A2, r >= 0xE0B0 && r <= 0xE0B3:
			return true
		case r == 0xE0A3, r >= 0xE0B4 && r <= 0xE0C8, r == 0xE0CA, r >= 0xE0CC && r <= 0xE0D7:
			return true
		}
		return false
	}
	glyphs := StripNerdIconTable()
	glyphs["sep-hard"] = StripSeparatorGlyph("nerd", "powerline")
	glyphs["sep-thin"] = StripSeparatorGlyph("nerd", "powerline-thin")
	glyphs["sep-round"] = StripSeparatorGlyph("nerd", "rounded")
	left, right := StripCaps("nerd")
	glyphs["cap-left"], glyphs["cap-right"] = left, right
	for name, glyph := range glyphs {
		runes := []rune(glyph)
		if len(runes) != 1 {
			t.Errorf("nerd glyph %q is %d runes, want exactly 1", name, len(runes))
			continue
		}
		if !inV3(runes[0]) {
			t.Errorf("nerd glyph %q is U+%04X, outside the v3 ranges", name, runes[0])
		}
		if w := lipgloss.Width(glyph); w != 1 {
			t.Errorf("nerd glyph %q measures %d cells, want 1", name, w)
		}
	}
}

// Separator glyphs per preset and separator setting, pinned: nerd uses
// Powerline codepoints, unicode plain symbols, ascii plain ASCII.
func TestStripSeparatorGlyphs(t *testing.T) {
	for preset, seps := range map[string]map[string]string{
		"nerd":    {"powerline": "", "powerline-thin": "", "rounded": "", "slash": "/", "pipe": "│", "none": ""},
		"unicode": {"powerline": "▶", "powerline-thin": "›", "rounded": "»", "slash": "/", "pipe": "│", "none": ""},
		"ascii":   {"powerline": ">", "powerline-thin": ">", "rounded": ")", "slash": "/", "pipe": "|", "none": ""},
	} {
		for sep, want := range seps {
			if got := StripSeparatorGlyph(preset, sep); got != want {
				t.Errorf("separator %s/%s = %q, want %q", preset, sep, got, want)
			}
		}
	}
	if got := StripSeparatorGlyph("bogus", "bogus"); got != "" {
		t.Errorf("unknown preset/separator reads as %q, want none", got)
	}
}

// Rounded caps wrap the chip and the state field in nerd only; the other
// presets keep a plain edge.
func TestStripCaps(t *testing.T) {
	left, right := StripCaps("nerd")
	if left != "" || right != "" {
		t.Errorf("nerd caps = %q %q, want rounded caps", left, right)
	}
	for _, preset := range []string{"unicode", "ascii", "bogus"} {
		if left, right := StripCaps(preset); left != "" || right != "" {
			t.Errorf("caps %s = %q %q, want a plain edge", preset, left, right)
		}
	}
}
