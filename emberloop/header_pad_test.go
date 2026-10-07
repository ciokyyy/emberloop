package emberloop

// Ticket 12 red slice 1: padding defaults come from settings.
// Empty settings mean two columns left/right and one row at the bottom;
// explicit zero restores the edge-to-edge layout.
import (
	"github.com/muesli/termenv"
	"strings"
	"testing"

	"example.com/emberloop/tools"
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
	if got := wrapMarginSetupPad(100, false, Pad{Left: 4, Right: 4}); got != "\x1b[?69h\x1b[5;96s\x1b[?6h" {
		t.Errorf("margins = %q", got)
	}
	if got := wrapMarginSetupPad(100, true, Pad{Left: 4, Right: 4}); got != "\x1b[?69h\x1b[6;95s\x1b[?6h" {
		t.Errorf("framed margins = %q", got)
	}
	if got := wrapMarginSetupPad(100, false, Pad{}); got != "" {
		t.Errorf("zero pad margins = %q, want none", got)
	}
}

func TestBottomEdgeIsRounded(t *testing.T) {
	w := &wrapper{termW: 40, dark: true, profile: termenv.TrueColor}
	if got := stripANSI(w.bottomEdge()); got != "▜"+strings.Repeat("▀", 38)+"▛" {
		t.Errorf("bottom edge = %q", got)
	}
}

func TestStripRowsKeepBackgroundBetweenCells(t *testing.T) {
	out := RenderHeader(HeaderSnapshot{AgentName: "implementer", State: HeaderIdle, Window: 1000, ContextTokens: 100},
		60, HeaderOpts{Rows: 2, Dark: true, Profile: termenv.TrueColor})
	bg := "48;2;" // truecolor background code
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, bg) {
			t.Errorf("row without background: %q", line)
		}
		// every reset must be followed by a background set, never a bare gap
		for _, seg := range strings.Split(line, "\x1b[0m")[1:] {
			if seg != "" && !strings.HasPrefix(seg, "\x1b[") {
				t.Errorf("unstyled text after reset: %q", seg)
			}
		}
	}
}

// TestStripPlacementWithPadding is the golden for child size and strip
// placement: the strip starts inside the left padding, spans the terminal
// minus both side paddings, and sits right under the child with the bottom
// padding below it.
func TestStripPlacementWithPadding(t *testing.T) {
	cases := []struct {
		name                          string
		termW, termH                  int
		pad                           Pad
		wantW, wantH, wantRule, wantC int
	}{
		{"default", 100, 30, Pad{4, 4, 2}, 92, 25, 26, 92},
		{"zero", 100, 30, Pad{}, 100, 27, 28, 100},
		{"oversized", 40, 12, Pad{30, 30, 20}, 20, 5, 6, 20},
	}
	for _, c := range cases {
		pad := FitPad(c.termW, c.termH, 2, false, c.pad)
		cw, ch := ChildSizePad(c.termW, c.termH, 2, false, pad)
		w := &wrapper{termW: c.termW, termH: c.termH, pad: pad, childW: cw, childH: ch}
		if cw != c.wantW || ch != c.wantH || w.ruleRow() != c.wantRule || w.contentW() != c.wantC {
			t.Errorf("%s: pad %+v child %dx%d rule %d content %d, want child %dx%d rule %d content %d",
				c.name, pad, cw, ch, w.ruleRow(), w.contentW(), c.wantW, c.wantH, c.wantRule, c.wantC)
		}
		if l, _ := w.sideCols(); l != pad.Left {
			t.Errorf("%s: strip starts after %d columns, want %d", c.name, l, pad.Left)
		}
	}
}
