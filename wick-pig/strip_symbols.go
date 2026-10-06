package wick

// Strip symbols (ticket 10): the header strip reads as a row of colored
// segments in the manner of omp's status line — one icon per segment, a
// separator between segments, rounded caps on the chip and the state field.
// Only the structure comes from omp, never its code.
//
// Glyph sets: nerd needs a Nerd Font, unicode is plain symbols (the
// default, so a terminal without a Nerd Font looks right), ascii is plain
// ASCII. Separators: powerline, powerline-thin, rounded, slash, pipe, none.
//
// Nerd Font codepoints below are Nerd Fonts v3, checked against the Glyph
// Sets wiki (Font Awesome U+ED00-U+F2FF, Powerline U+E0A0-U+E0A2 and
// U+E0B0-U+E0B3, Powerline Extra U+E0B4-U+E0C8 and friends). No Material
// Design icon is used on purpose: v3 moved mdi out of the legacy
// U+F500-U+FD46 range to U+F0001+, so a legacy codepoint renders as a wrong
// glyph under v3. strip_symbols_test.go pins every value here.

import (
	"example.com/wick/tools"
)

// StripNerdIcons is the one nerd glyph table: segment or state name to the
// single Nerd Fonts v3 glyph drawn for it. Font Awesome names in comments.
var StripNerdIcons = map[string]string{
	"agent":    "", // U+F007 fa-user: the agent chip
	"ctx":      "", // U+F200 fa-chart-pie: the context label
	"tokens":   "", // U+F1C0 fa-database: the used-against-window numbers
	"cost":     "", // U+F155 fa-dollar-sign: the cost estimate
	"model":    "", // U+F2DB fa-microchip: the model name
	"tools":    "", // U+F0AD fa-wrench: the top-tools row
	"compress": "", // U+F066 fa-compress: the compress count
	"working":  "", // U+F111 fa-circle: the working state
	"idle":     "", // U+F10C fa-circle regular: the idle state
	"queued":   "", // U+F017 fa-clock: the compress-queued state
	"sync":     "", // U+F021 fa-sync: the compressing state
	"warning":  "", // U+F071 fa-warning: the warning pill
}

// StripNerdIconTable returns the nerd glyph table copy: callers never mutate
// the shared table through it.
func StripNerdIconTable() map[string]string {
	out := make(map[string]string, len(StripNerdIcons))
	for k, v := range StripNerdIcons {
		out[k] = v
	}
	return out
}

// StripSeparatorGlyph is the between-segment separator glyph for the preset
// and separator setting. Nerd uses Powerline codepoints, unicode plain
// symbols, ascii plain ASCII. Unknown input reads as none.
func StripSeparatorGlyph(symbols, separator string) string {
	switch tools.NormalizeStripSymbols(symbols) {
	case tools.StripSymbolsNerd:
		switch tools.NormalizeStripSeparator(separator) {
		case tools.StripSeparatorPowerline:
			return "" // U+E0B0 powerline right hard
		case tools.StripSeparatorPowerlineThin:
			return "" // U+E0B1 powerline right thin
		case tools.StripSeparatorRounded:
			return "" // U+E0B4 powerline extra rounded right
		case tools.StripSeparatorSlash:
			return "/"
		case tools.StripSeparatorPipe:
			return "│" // U+2502, no Nerd Font needed
		}
	case tools.StripSymbolsASCII:
		switch tools.NormalizeStripSeparator(separator) {
		case tools.StripSeparatorPowerline, tools.StripSeparatorPowerlineThin:
			return ">"
		case tools.StripSeparatorRounded:
			return ")"
		case tools.StripSeparatorSlash:
			return "/"
		case tools.StripSeparatorPipe:
			return "|"
		}
	default: // unicode and anything unknown: plain symbols
		switch tools.NormalizeStripSeparator(separator) {
		case tools.StripSeparatorPowerline:
			return "▶" // U+25B6
		case tools.StripSeparatorPowerlineThin:
			return "›" // U+203A
		case tools.StripSeparatorRounded:
			return "»" // U+00BB
		case tools.StripSeparatorSlash:
			return "/"
		case tools.StripSeparatorPipe:
			return "│" // U+2502
		}
	}
	return ""
}

// StripCaps wraps the agent chip and the state field: rounded caps in nerd,
// a plain edge in every other preset.
func StripCaps(symbols string) (left, right string) {
	if tools.NormalizeStripSymbols(symbols) != tools.StripSymbolsNerd {
		return "", ""
	}
	return "", "" // U+E0B6 and U+E0B4, powerline extra rounded caps
}

// stripIcon prefixes a segment label with its nerd icon. Other presets show
// the label alone: unicode already carries its plain symbols (the working
// dot, the warning triangle), ascii is labels only.
func stripIcon(symbols, name, label string) string {
	if tools.NormalizeStripSymbols(symbols) != tools.StripSymbolsNerd {
		return label
	}
	icon, ok := StripNerdIcons[name]
	if !ok || label == "" {
		return label
	}
	return icon + " " + label
}
