package wick

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The key decoder turns raw terminal bytes into canonical key strings. The
// input row and the raw-mode helpers act on those strings, never on the raw
// bytes. A canonical key is either printable text (one run), a single
// control byte ("\r", "\x03", "\x7f"), a legacy cursor key ("\x1b[A"), an
// Alt chord ("\x1b" plus the rune), or "" for input that is consumed with no
// effect (focus reports, key releases, unknown sequences).
//
// Terminals that run the kitty keyboard protocol (or xterm modifyOtherKeys)
// report keys as CSI sequences: Ctrl+C arrives as "\x1b[99;5u". The decoder
// maps those onto the same canonical keys the legacy bytes produce.

const (
	modShift = 1
	modAlt   = 2
	modCtrl  = 4
	// modLocks are caps lock and num lock, which never change the key.
	modLocks = 64 | 128
)

// nextKey splits the first key off p. n is the byte count consumed. When
// complete is false, p ends inside an escape sequence and the caller keeps
// the bytes until more input arrives.
func nextKey(p []byte) (key string, n int, complete bool) {
	if len(p) == 0 {
		return "", 0, true
	}
	switch {
	case p[0] == 0x1b:
		return nextEscape(p)
	case p[0] < 0x20 && p[0] != '\t', p[0] == 0x7f:
		return string(p[:1]), 1, true
	}
	end := 1
	for end < len(p) && (p[end] >= 0x20 && p[end] != 0x7f || p[end] == '\t') {
		end++
	}
	return string(p[:end]), end, true
}

func nextEscape(p []byte) (string, int, bool) {
	if len(p) == 1 {
		return "\x1b", 1, true
	}
	switch p[1] {
	case '[':
		for i := 2; i < len(p); i++ {
			if p[i] >= 0x40 && p[i] <= 0x7e {
				return canonCSI(p[2:i], p[i]), i + 1, true
			}
		}
		return "", 0, false
	case 'O':
		if len(p) < 3 {
			return "", 0, false
		}
		switch p[2] {
		case 'A', 'B', 'C', 'D', 'H', 'F':
			return "\x1b[" + string(p[2:3]), 3, true
		}
		return "", 3, true
	case 0x1b:
		return "\x1b", 1, true
	}
	r, size := utf8.DecodeRune(p[1:])
	if r == utf8.RuneError && size <= 1 && !utf8.FullRune(p[1:]) {
		return "", 0, false
	}
	return "\x1b" + string(p[1:1+size]), 1 + size, true
}

// csiParams splits a CSI parameter string into fields of colon-separated
// numbers. Empty or malformed numbers read as 0.
func csiParams(body []byte) [][]int {
	if len(body) == 0 {
		return nil
	}
	var out [][]int
	for _, field := range strings.Split(string(body), ";") {
		var subs []int
		for _, sub := range strings.Split(field, ":") {
			v, _ := strconv.Atoi(sub)
			subs = append(subs, v)
		}
		out = append(out, subs)
	}
	return out
}

func csiParam(params [][]int, field, sub, def int) int {
	if field >= len(params) || sub >= len(params[field]) || params[field][sub] == 0 {
		return def
	}
	return params[field][sub]
}

func canonCSI(body []byte, final byte) string {
	if len(body) > 0 && (body[0] == '<' || body[0] == '?' || body[0] == '>' || body[0] == '=') {
		return ""
	}
	params := csiParams(body)
	switch final {
	case 'u':
		if csiParam(params, 1, 1, 1) == 3 {
			return ""
		}
		return canonKeycode(csiParam(params, 0, 0, 0), csiParam(params, 1, 0, 1), csiParam(params, 0, 1, 0))
	case '~':
		mods := csiParam(params, 1, 0, 1)
		switch csiParam(params, 0, 0, 0) {
		case 27:
			return canonKeycode(csiParam(params, 2, 0, 0), mods, 0)
		case 1, 7:
			return "\x1b[H"
		case 4, 8:
			return "\x1b[F"
		case 3:
			if csiParam(params, 1, 1, 1) == 3 {
				return ""
			}
			if lockless(mods)&(modCtrl|modAlt) != 0 {
				return "\x1bd"
			}
			return "\x1b[3~"
		}
		return ""
	case 'A', 'B', 'C', 'D', 'H', 'F':
		if csiParam(params, 1, 1, 1) == 3 {
			return ""
		}
		bits := lockless(csiParam(params, 1, 0, 1))
		if bits&(modCtrl|modAlt) != 0 {
			switch final {
			case 'C':
				return "\x1bf"
			case 'D':
				return "\x1bb"
			}
		}
		return "\x1b[" + string(final)
	}
	return ""
}

// lockless returns the modifier bits of a CSI modifier parameter, without
// the lock bits.
func lockless(mods int) int {
	bits := mods - 1
	if bits < 0 {
		bits = 0
	}
	return bits &^ modLocks
}

// canonKeycode maps a CSI-u key (code point, modifier parameter, shifted
// code point) to its canonical key.
func canonKeycode(code, mods, shifted int) string {
	bits := lockless(mods)
	if bits&^(modShift|modAlt|modCtrl) != 0 {
		return ""
	}
	shift, alt, ctrl := bits&modShift != 0, bits&modAlt != 0, bits&modCtrl != 0
	switch code {
	case 13, 57414:
		switch {
		case ctrl:
			return ""
		case alt:
			return "\x1b\r"
		case shift:
			return "\n"
		}
		return "\r"
	case 27:
		if bits == 0 {
			return "\x1b"
		}
		return ""
	case 9:
		if bits == 0 {
			return "\t"
		}
		return ""
	case 8, 127:
		if ctrl || alt {
			return "\x1b\x7f"
		}
		return "\x7f"
	}
	if code < 32 || code > unicode.MaxRune || (code >= 0xE000 && code <= 0xF8FF) {
		return ""
	}
	r := rune(code)
	if shift {
		if shifted > 0 {
			r = rune(shifted)
		} else {
			r = unicode.ToUpper(r)
		}
	}
	switch {
	case ctrl && alt:
		return ""
	case ctrl:
		lower := unicode.ToLower(r)
		switch {
		case lower >= 'a' && lower <= 'z':
			return string(rune(lower-'a') + 1)
		case r >= '@' && r <= '_':
			return string(r & 0x1f)
		}
		return ""
	case alt:
		return "\x1b" + string(r)
	}
	return string(r)
}

// singleKey decodes a chunk that holds exactly one key. ok is false for
// empty, partial or multi-key chunks.
func singleKey(p []byte) (key string, ok bool) {
	key, n, complete := nextKey(p)
	if !complete || n == 0 || n != len(p) {
		return "", false
	}
	return key, true
}
