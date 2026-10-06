package wick

import (
	"testing"

	"github.com/muesli/termenv"
)

// Golden layout for the wide tier: the exact two rows for the mockup's
// numbers (142k of 200k at 71%, ~$1.82, opus, compressed twice, bash/read/
// grep on top). Locks the layout the substring tests only probe.
func TestRenderHeaderWideGolden(t *testing.T) {
	out := stripANSI(renderTrue(wideSnapshot(), 100))
	want := `orchestrator  ctx  ▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▱▱▱▱▱▱▱  71%  142k / 200k  ~$1.82  opus  compressed ×2          
top  bash  31k  read  22k  grep  9k                                                        ● working`
	if out != want {
		t.Errorf("wide golden differs:\ngot:\n%q\nwant:\n%q", out, want)
	}
}

// The compressing pill breathes: the pulse phase rests on the idle color
// instead of the accent, so a one-second repaint visibly alternates.
func TestRenderHeaderPulse(t *testing.T) {
	snap := wideSnapshot()
	snap.State = HeaderCompressing
	plain := RenderHeader(snap, 100, HeaderOpts{Dark: true, Profile: termenv.TrueColor})
	pulsed := RenderHeader(snap, 100, HeaderOpts{Dark: true, Profile: termenv.TrueColor, Pulse: true})
	if stripANSI(plain) != stripANSI(pulsed) {
		t.Error("pulse changes the text, want only the color")
	}
	if plain == pulsed {
		t.Error("pulse phase renders identically, want the idle rest color")
	}
}

// Golden layout for the medium tier: one full row with the smaller bar and
// the state pill on it.
func TestRenderHeaderMediumGolden(t *testing.T) {
	out := stripANSI(renderTrue(wideSnapshot(), 80))
	want := `orchestrator  ctx  ▰▰▰▰▰▰▰▰▰▰▰▱▱▱▱▱  71%  142k / 200k  ~$1.82  opus    ● working`
	if out != want {
		t.Errorf("medium golden differs:\ngot:\n%q\nwant:\n%q", out, want)
	}
}

// Golden layout for the narrow tier: percent and cost beside the state.
func TestRenderHeaderNarrowGolden(t *testing.T) {
	out := stripANSI(renderTrue(wideSnapshot(), 40))
	want := `orchestrator ctx 71% · ~$1.82  ● working`
	if out != want {
		t.Errorf("narrow golden differs:\ngot:\n%q\nwant:\n%q", out, want)
	}
}
