package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"example.com/emberloop"
)

// pal is the dashboard palette: the same Catppuccin source the el
// header strip and the pane colors use. Mocha until SetDark says otherwise.
var pal = el.ElMocha()

// darkTheme is whether the palette is the dark one; Markdown follows it.
var darkTheme = true

// SetDark picks the Mocha (dark) or Latte (light) palette and rebuilds the
// package styles that read it.
func SetDark(dark bool) {
	darkTheme = dark
	if dark {
		pal = el.ElMocha()
	} else {
		pal = el.ElLatte()
	}
	styleSel = lipgloss.NewStyle().Bold(true).
		Foreground(lipgloss.Color(pal.Accent)).Background(lipgloss.Color(pal.BarDim))
}

// painter renders row segments on one shared background. A background set
// once around a whole row is lost at the first ANSI reset inside it, so each
// segment carries the background itself.
type painter struct{ bg string }

// s renders text in the foreground colour hex, bold on request.
func (p painter) s(hex string, bold bool, text string) string {
	st := lipgloss.NewStyle().Foreground(lipgloss.Color(hex)).Bold(bold)
	if p.bg != "" {
		st = st.Background(lipgloss.Color(p.bg))
	}
	return st.Render(text)
}

// fill cuts row to width with an ellipsis, or pads it with the background
// out to width, so a selected row reads as one solid bar.
func (p painter) fill(row string, width int) string {
	if width <= 0 {
		return row
	}
	w := lipgloss.Width(row)
	if w > width {
		return ansi.Truncate(row, width, "…")
	}
	if p.bg == "" {
		return row
	}
	return row + lipgloss.NewStyle().Background(lipgloss.Color(p.bg)).Render(strings.Repeat(" ", width-w))
}

// miniBar is a thin progress bar of cells columns: filled part in fill,
// the rest in the dim bar colour.
func (p painter) miniBar(frac float64, cells int, fill string) string {
	n := int(frac*float64(cells) + 0.5)
	if n < 0 {
		n = 0
	}
	if n > cells {
		n = cells
	}
	return p.s(fill, false, strings.Repeat("━", n)) + p.s(pal.BarDim, false, strings.Repeat("━", cells-n))
}

// selBg is the background of the selected row in the focused pane. The
// selected row in an unfocused pane keeps only its marker.
func selBg(selected, focused bool) string {
	if selected && focused {
		return pal.BarDim
	}
	return ""
}

// marker is the row's left edge: a thick accent bar on the selected row,
// dimmed when its pane has no focus.
func marker(p painter, selected, focused bool) string {
	switch {
	case selected && focused:
		return p.s(pal.Accent, true, "▌")
	case selected:
		return p.s(pal.Dim, false, "▌")
	}
	return p.s(pal.Dim, false, " ")
}

// pill is one tab label: solid accent when active, dim on the strip when not.
func pill(label string, active bool) string {
	if active {
		return lipgloss.NewStyle().Bold(true).Padding(0, 1).
			Foreground(lipgloss.Color(pal.StripBg)).Background(lipgloss.Color(pal.Accent)).Render(label)
	}
	return lipgloss.NewStyle().Padding(0, 1).
		Foreground(lipgloss.Color(pal.Dim)).Background(lipgloss.Color(pal.StripBg)).Render(label)
}
