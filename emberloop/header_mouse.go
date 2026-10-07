package emberloop

import (
	"fmt"
	"regexp"
	"strconv"
)

// The child draws inside the padding and above the strip, but the terminal
// reports mouse positions for the whole pane. Without a translation a
// fullscreen child (claude, pig) reads every click shifted right by the left
// padding, so a drag selects the wrong text. mouseMapper rewrites SGR mouse
// reports (CSI < b ; x ; y M|m) into the child's own coordinates.
var (
	sgrMouse        = regexp.MustCompile("\x1b\\[<(\\d+);(\\d+);(\\d+)([Mm])")
	sgrMousePartial = regexp.MustCompile("\x1b\\[<[0-9;]*$")
)

// mouseMapper holds the geometry of the child screen and the drag state. A
// press outside the child (padding, strip) is dropped: the child never sees
// a click it cannot own. A drag that starts inside keeps going outside, with
// the position clamped to the child, so selecting past an edge still works.
type mouseMapper struct {
	offX, w, h int // columns before the child screen; child size
	dragging   bool
	held       []byte
	// stripDown is true while a drag that began on the strip rows runs: the
	// wrapper owns it, the child never sees it. onStrip gets the terminal
	// cells of the press and release once the button comes up.
	stripDown      bool
	stripX, stripY int
	onStrip        func(x1, y1, x2, y2 int)
}

// set updates the geometry after a resize.
func (m *mouseMapper) set(offX, w, h int) { m.offX, m.w, m.h = offX, w, h }

// translate rewrites the mouse reports in data and returns the bytes for the
// child. A report split across reads is held until the rest arrives.
func (m *mouseMapper) translate(data []byte) []byte {
	if len(m.held) > 0 {
		data = append(m.held, data...)
		m.held = nil
	}
	if loc := sgrMousePartial.FindIndex(data); loc != nil {
		m.held = append([]byte(nil), data[loc[0]:]...)
		data = data[:loc[0]]
	}
	return sgrMouse.ReplaceAllFunc(data, func(seq []byte) []byte {
		p := sgrMouse.FindSubmatch(seq)
		b, _ := strconv.Atoi(string(p[1]))
		x, _ := strconv.Atoi(string(p[2]))
		y, _ := strconv.Atoi(string(p[3]))
		release := string(p[4]) == "m"
		motion := b&32 != 0
		wheel := b&64 != 0
		if m.stripDown && !wheel {
			if release {
				m.stripDown = false
				if m.onStrip != nil {
					m.onStrip(m.stripX, m.stripY, x, y)
				}
			}
			return nil
		}
		if !wheel && !release && !motion && !m.dragging && y > m.h && m.onStrip != nil {
			m.stripDown, m.stripX, m.stripY = true, x, y
			return nil
		}
		cx, cy := x-m.offX, y
		inside := cx >= 1 && cx <= m.w && cy >= 1 && cy <= m.h
		if !inside && !m.dragging {
			return nil
		}
		if !wheel {
			switch {
			case release:
				m.dragging = false
			case !motion:
				m.dragging = true
			}
		}
		cx = min(max(cx, 1), m.w)
		cy = min(max(cy, 1), m.h)
		return []byte(fmt.Sprintf("\x1b[<%d;%d;%d%s", b, cx, cy, p[4]))
	})
}
