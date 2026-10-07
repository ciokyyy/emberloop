package emberloop

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestMouseMapperShiftsByPadding(t *testing.T) {
	m := &mouseMapper{}
	m.set(4, 92, 25)
	if got := string(m.translate([]byte("\x1b[<0;10;3M"))); got != "\x1b[<0;6;3M" {
		t.Errorf("press = %q", got)
	}
	if got := string(m.translate([]byte("a\x1b[<0;10;3mb"))); got != "a\x1b[<0;6;3mb" {
		t.Errorf("release in text = %q", got)
	}
}

func TestMouseMapperDropsPressesOutsideChild(t *testing.T) {
	m := &mouseMapper{}
	m.set(4, 92, 25)
	for _, in := range []string{"\x1b[<0;2;3M", "\x1b[<0;50;27M", "\x1b[<0;99;3M", "\x1b[<0;50;27m"} {
		if got := m.translate([]byte(in)); len(got) != 0 {
			t.Errorf("%q passed as %q", in, got)
		}
	}
}

func TestMouseMapperClampsDragOutsideChild(t *testing.T) {
	m := &mouseMapper{}
	m.set(4, 92, 25)
	m.translate([]byte("\x1b[<0;10;3M"))
	if got := string(m.translate([]byte("\x1b[<32;99;27M"))); got != "\x1b[<32;92;25M" {
		t.Errorf("drag outside = %q", got)
	}
	if got := string(m.translate([]byte("\x1b[<0;99;27m"))); got != "\x1b[<0;92;25m" {
		t.Errorf("release outside = %q", got)
	}
	if got := m.translate([]byte("\x1b[<0;99;27M")); len(got) != 0 {
		t.Errorf("press after drag ended = %q", got)
	}
}

func TestMouseMapperHoldsSplitReport(t *testing.T) {
	m := &mouseMapper{}
	m.set(4, 92, 25)
	if got := m.translate([]byte("x\x1b[<0;1")); string(got) != "x" {
		t.Errorf("first half = %q", got)
	}
	if got := string(m.translate([]byte("0;3M"))); got != "\x1b[<0;6;3M" {
		t.Errorf("second half = %q", got)
	}
}

func TestMouseMapperStripDragIsOwnedByWrapper(t *testing.T) {
	m := &mouseMapper{}
	m.set(4, 92, 25)
	var got [4]int
	m.onStrip = func(x1, y1, x2, y2 int) { got = [4]int{x1, y1, x2, y2} }
	for _, in := range []string{"\x1b[<0;10;27M", "\x1b[<32;20;27M", "\x1b[<0;30;27m"} {
		if out := m.translate([]byte(in)); len(out) != 0 {
			t.Errorf("%q reached the child as %q", in, out)
		}
	}
	if got != [4]int{10, 27, 30, 27} {
		t.Errorf("strip drag = %v", got)
	}
}

func TestWrapperCopiesStripSelectionOverOSC52(t *testing.T) {
	var out strings.Builder
	w := &wrapper{termW: 100, termH: 30, childH: 25, pad: Pad{Left: 4, Right: 4, Bottom: 2}, stdout: &out,
		lastPlain: []string{"plan  ctx  62%   claude-sonnet-5-5"}}
	// strip starts at terminal column 5; "ctx" is cells 7-9 of the row
	w.copyStripSelection(11, 27, 13, 27)
	want := "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte("ctx")) + "\x07"
	if out.String() != want {
		t.Errorf("osc = %q, want %q", out.String(), want)
	}
}
