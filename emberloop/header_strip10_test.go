package el

import (
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Seam: styled strip render (ticket 10) — RenderHeader is a pure function
// of snapshot, width and opts (preset, separator, rows, theme, profile).
// Goldens pin exact stripped lines; expected values are worked by hand from
// the ticket, never copied from the implementation. Glyph escapes are the
// Nerd Fonts v3 codepoints pinned in strip_symbols_test.go.

func renderStrip(snap HeaderSnapshot, width int, symbols, separator string) string {
	return RenderHeader(snap, width, HeaderOpts{Dark: true, Profile: termenv.TrueColor, Symbols: symbols, Separator: separator})
}

// The default stays the current look: unicode with no separator renders
// byte for byte like before, so a terminal without a Nerd Font is unchanged.
func TestRenderStripDefaultUnchanged(t *testing.T) {
	for _, width := range []int{100, 80, 40} {
		got := renderStrip(wideSnapshot(), width, "", "")
		want := renderTrue(wideSnapshot(), width)
		if got != want {
			t.Errorf("default render at %d differs:\ngot:\n%q\nwant:\n%q", width, got, want)
		}
		if got := renderStrip(wideSnapshot(), width, "unicode", "none"); got != want {
			t.Errorf("unicode/none render at %d differs:\ngot:\n%q\nwant:\n%q", width, got, want)
		}
	}
}

// Nerd wide tier: icons per segment, powerline separators, rounded caps on
// the chip; the bar flexes to the room the numbers leave (11 cells here, 8
// filled at 71%: (11*71+50)/100).
func TestRenderStripNerdWideGolden(t *testing.T) {
	out := renderStrip(wideSnapshot(), 100, "nerd", "powerline")
	rows := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(rows) != 2 {
		t.Fatalf("nerd wide strip is %d rows, want 2:\n%s", len(rows), out)
	}
	bar := strings.Repeat("▰", 8) + strings.Repeat("▱", 3)
	wantFirst := " orchestrator    ctx  " + bar + "  71%   142k / 200k   ~$1.82   opus   compressed ×2 "
	if plain := stripANSI(rows[0]); plain != wantFirst {
		t.Errorf("nerd wide row 1 differs:\ngot:\n%q\nwant:\n%q", plain, wantFirst)
	}
	wantSecond := "▤ reading emberloop/header.go" + strings.Repeat(" ", 39) + "5h ◔ 37% ↻1h40m   wk ◔ 14% ↻4d0h"
	if plain := stripANSI(rows[1]); plain != wantSecond {
		t.Errorf("nerd wide row 2 differs:\ngot:\n%q\nwant:\n%q", plain, wantSecond)
	}
}

// Nerd narrow tier: the chip no longer fits, so percent, cost and the
// capped state pill remain.
func TestRenderStripNerdNarrowGolden(t *testing.T) {
	out := renderStrip(wideSnapshot(), 40, "nerd", "powerline")
	rows := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(rows) != 1 {
		t.Fatalf("nerd narrow strip is %d rows, want 1:\n%s", len(rows), out)
	}
	want := "71%   ~$1.82" + strings.Repeat(" ", 14) + " working "
	if plain := stripANSI(rows[0]); plain != want {
		t.Errorf("nerd narrow differs:\ngot:\n%q\nwant:\n%q", plain, want)
	}
}

// Unicode with a pipe separator: labels only, plain edges, pipe between
// segments; bar 12 cells, 9 filled at 71%.
func TestRenderStripUnicodePipeGolden(t *testing.T) {
	out := renderStrip(wideSnapshot(), 80, "unicode", "pipe")
	rows := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(rows) != 1 {
		t.Fatalf("unicode pipe strip is %d rows, want 1:\n%s", len(rows), out)
	}
	bar := strings.Repeat("▰", 9) + strings.Repeat("▱", 3)
	want := "orchestrator │ ctx │ " + bar + " │ 71% │ 142k / 200k │ ~$1.82 │ opus  ● working"
	if plain := stripANSI(rows[0]); plain != want {
		t.Errorf("unicode pipe differs:\ngot:\n%q\nwant:\n%q", plain, want)
	}
}

// ASCII narrow: labels only, ASCII separators, no symbol prefixes. The chip
// no longer fits beside the slash gaps, so percent, cost and state remain.
func TestRenderStripAsciiNarrowGolden(t *testing.T) {
	out := renderStrip(wideSnapshot(), 40, "ascii", "slash")
	rows := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(rows) != 1 {
		t.Fatalf("ascii narrow strip is %d rows, want 1:\n%s", len(rows), out)
	}
	want := "71% / ~$1.82" + strings.Repeat(" ", 21) + "working"
	if plain := stripANSI(rows[0]); plain != want {
		t.Errorf("ascii narrow differs:\ngot:\n%q\nwant:\n%q", plain, want)
	}
}

// Every separator setting renders its glyph between segments, and none
// renders no glyph. Per preset: nerd uses Powerline codepoints, unicode
// plain symbols, ascii plain ASCII. Slash is pinned by the goldens (it is
// label content too), caps are not separators.
func TestRenderStripSeparators(t *testing.T) {
	seps := map[string]map[string]string{
		"nerd":    {"powerline": "", "powerline-thin": "", "rounded": "", "pipe": "│"},
		"unicode": {"powerline": "▶", "powerline-thin": "›", "rounded": "»", "pipe": "│"},
		"ascii":   {"powerline": ">", "powerline-thin": ">", "rounded": ")", "pipe": "|"},
	}
	noneBanned := map[string][]string{
		"nerd":    {"", "", "│"},
		"unicode": {"▶", "›", "»", "│"},
		"ascii":   {">", ")", "|"},
	}
	for preset, glyphs := range seps {
		for sep, glyph := range glyphs {
			plain := stripANSI(renderStrip(wideSnapshot(), 80, preset, sep))
			if !strings.Contains(plain, glyph) {
				t.Errorf("separator %s/%s misses %q:\n%s", preset, sep, glyph, plain)
			}
		}
		plain := stripANSI(renderStrip(wideSnapshot(), 80, preset, "none"))
		for _, banned := range noneBanned[preset] {
			if strings.Contains(plain, banned) {
				t.Errorf("separator none (%s) leaks %q:\n%s", preset, banned, plain)
			}
		}
	}
}

// sgrOf renders one foreground color through the shared Lip Gloss
// renderer: the SGR bytes below pin the palette token, not Lip Gloss's own
// hex rounding.
func sgrOf(hex, bg string) string {
	r := lipgloss.NewRenderer(io.Discard, termenv.WithProfile(termenv.TrueColor))
	r.SetColorProfile(termenv.TrueColor)
	s := r.NewStyle().Foreground(lipgloss.Color(hex)).Background(lipgloss.Color(bg)).Render("x")
	return s[:strings.Index(s, "x")]
}

// Segment colors come from the shared palette: model reads the accent,
// cost the cost token, tools the tool token, the separator the dim token,
// the 71% working state the warm threshold — dark and light. Expected
// sequences are encoded from the palette tokens through the shared
// renderer, so a segment on the wrong token fails.
func TestRenderStripSegmentColors(t *testing.T) {
	mocha, latte := ElMocha(), ElLatte()
	dark := renderStrip(wideSnapshot(), 100, "nerd", "powerline")
	for _, hex := range []string{mocha.Accent, mocha.Cost, mocha.Tool, mocha.Dim, mocha.Warm} {
		if want := sgrOf(hex, mocha.StripBg); !strings.Contains(dark, want) {
			t.Errorf("dark strip misses palette token %s (%q)", hex, want)
		}
	}
	light := RenderHeader(wideSnapshot(), 100, HeaderOpts{Dark: false, Profile: termenv.TrueColor, Symbols: "nerd", Separator: "powerline"})
	for _, hex := range []string{latte.Accent, latte.Cost, latte.Tool, latte.Dim} {
		if want := sgrOf(hex, latte.StripBg); !strings.Contains(light, want) {
			t.Errorf("light strip misses palette token %s (%q)", hex, want)
		}
	}
}

// Downgraded profiles keep the same text in every preset, without
// truecolor sequences.
func TestRenderStripProfilesPerPreset(t *testing.T) {
	for _, preset := range []string{"nerd", "unicode", "ascii"} {
		full := stripANSI(renderStrip(wideSnapshot(), 100, preset, "powerline"))
		for _, profile := range []termenv.Profile{termenv.ANSI256, termenv.ANSI} {
			out := RenderHeader(wideSnapshot(), 100, HeaderOpts{Dark: true, Profile: profile, Symbols: preset, Separator: "powerline"})
			if strings.Contains(out, "38;2") {
				t.Errorf("preset %s profile %v keeps truecolor sequences", preset, profile)
			}
			if got := stripANSI(out); got != full {
				t.Errorf("preset %s profile %v changes the text:\n%s\nwant:\n%s", preset, profile, got, full)
			}
		}
	}
}

// Narrow widths drop whole segments in a fixed order and never overflow:
// every tier renders lines that fit the width, with no replacement
// character and no split glyph.
func TestRenderStripWidthsNeverOverflow(t *testing.T) {
	longArgSnap := wideSnapshot()
	longArgSnap.Running.Arg = "/very/long/nested/path/to/some/deeply/nested/component/emberloop/pig/header_tool.go"
	for _, snap := range []HeaderSnapshot{wideSnapshot(), longArgSnap} {
		for _, preset := range []string{"nerd", "unicode", "ascii"} {
			for _, sep := range []string{"powerline", "rounded", "none"} {
				for _, width := range []int{110, 100, 95, 90, 80, 60, 40, 30, 20} {
					out := RenderHeader(snap, width, HeaderOpts{Dark: true, Profile: termenv.TrueColor, Symbols: preset, Separator: sep})
					for _, row := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
						plain := stripANSI(row)
						if lipgloss.Width(plain) > width {
							t.Errorf("preset %s sep %s width %d overflows: %q (%d cells)", preset, sep, width, plain, lipgloss.Width(plain))
						}
						if strings.ContainsRune(plain, '\uFFFD') {
							t.Errorf("preset %s sep %s width %d splits a glyph: %q", preset, sep, width, plain)
						}
					}
				}
			}
		}
	}
}

// Fixed drop order at narrowing widths (unicode): tools row and compress
// count go first, then the model, then the cost, keeping percent and state.
func TestRenderStripDropOrder(t *testing.T) {
	snap := wideSnapshot()
	mid := stripANSI(renderStrip(snap, 80, "unicode", "none"))
	for _, banned := range []string{"top", "bash", "×2"} {
		if strings.Contains(mid, banned) {
			t.Errorf("medium strip keeps %q:\n%s", banned, mid)
		}
	}
	narrow := stripANSI(renderStrip(snap, 40, "unicode", "none"))
	for _, want := range []string{"71%", "working"} {
		if !strings.Contains(narrow, want) {
			t.Errorf("narrow strip drops %q:\n%s", want, narrow)
		}
	}
	tiny := stripANSI(RenderHeader(snap, 18, HeaderOpts{Dark: true, Profile: termenv.TrueColor}))
	for _, want := range []string{"71%", "working"} {
		if !strings.Contains(tiny, want) {
			t.Errorf("tiny strip drops %q:\n%s", want, tiny)
		}
	}
	if strings.Contains(tiny, "~$") {
		t.Errorf("tiny strip keeps the cost before the percent:\n%s", tiny)
	}
}

// The strip height stays at the configured rows in every preset.
func TestRenderStripRowsPerPreset(t *testing.T) {
	for _, preset := range []string{"nerd", "unicode", "ascii"} {
		two := RenderHeader(wideSnapshot(), 100, HeaderOpts{Dark: true, Profile: termenv.TrueColor, Symbols: preset, Rows: 2})
		if got := len(strings.Split(strings.TrimRight(two, "\n"), "\n")); got != 2 {
			t.Errorf("preset %s two-row strip is %d rows, want 2", preset, got)
		}
		one := RenderHeader(wideSnapshot(), 100, HeaderOpts{Dark: true, Profile: termenv.TrueColor, Symbols: preset, Rows: 1})
		if got := len(strings.Split(strings.TrimRight(one, "\n"), "\n")); got != 1 {
			t.Errorf("preset %s single-row strip is %d rows, want 1", preset, got)
		}
	}
}

// The ascii preset stays pure ASCII at every width and separator.
func TestRenderStripAsciiPure(t *testing.T) {
	for _, sep := range []string{"powerline", "rounded", "slash", "pipe", "none"} {
		for _, width := range []int{100, 80, 60, 40, 30, 20} {
			plain := stripANSI(RenderHeader(wideSnapshot(), width, HeaderOpts{Dark: true, Profile: termenv.TrueColor, Symbols: "ascii", Separator: sep}))
			for _, r := range plain {
				if r > 127 && r != '\n' {
					t.Errorf("ascii strip (%s/%d) leaks %q:\n%s", sep, width, r, plain)
					break
				}
			}
		}
	}
}
