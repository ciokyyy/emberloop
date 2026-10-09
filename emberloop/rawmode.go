package el

import (
	"io"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Raw passthrough mode (input-queue ticket 04): a toggle key hands the
// keyboard to the child for its own dialogs and menus. While raw mode is on
// the wrapper keeps a conservative dirty flag: any forwarded byte except a
// bare Enter marks the child's box dirty; Enter, Ctrl+C or a el clear
// resets it. Injections hold while dirty and deliver right away when clean.
// After 10 s with no keystroke the wrapper sends six Ctrl+U, then delivers.
// Ctrl+C is never a clear sequence.

// rawToggleByte is the mode toggle: Ctrl+O. It is free in the row editor
// (a filtered control byte, never inserted) and distinct from the row's
// Enter, Alt+Enter, Shift+Enter, Up, Ctrl+C and Esc keys.
const rawToggleByte = 0x0f

// rawClearByte is Ctrl+U: the child clear. Sent six times it clears a
// three-line draft in both claude and pig and is harmless on an empty box.
const rawClearByte = 0x15

// rawClearRepeats is how many Ctrl+U form one el clear.
const rawClearRepeats = 6

// rawStaleTimeout is how long raw mode may sit dirty with no keystroke
// before the wrapper clears and delivers.
const rawStaleTimeout = 10 * time.Second

// harnessKeys is one harness's input sequences: how el clears its box,
// which key inserts a newline in raw passthrough, which key interrupts,
// and whether it accepts bracketed paste. The stale path consumes clear
// today; newline, interrupt and bracketedPaste pin the probed raw-handoff
// keys per harness (spec story 27) for the delivery tickets that use them.
type harnessKeys struct {
	clear          []byte
	newline        []byte
	interrupt      []byte
	bracketedPaste bool
}

func repeatByte(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}

// harnessInputKeys holds the per-harness input sequences for claude and pig
// from the spec probes. A harness with no entry gets the conservative
// default: hold injections while raw and dirty, never clear.
var harnessInputKeys = map[string]harnessKeys{
	"claude": {
		clear:          repeatByte(rawClearByte, rawClearRepeats),
		newline:        []byte("\x1b\r"),
		interrupt:      []byte{0x03},
		bracketedPaste: true,
	},
	"pig": {
		clear:          repeatByte(rawClearByte, rawClearRepeats),
		newline:        []byte("\x1b[13;2u"),
		interrupt:      []byte{0x03},
		bracketedPaste: true,
	},
}

// harnessKeysFor reports one harness's input sequences. The second result
// is false for a harness with no entry.
func harnessKeysFor(harness string) (harnessKeys, bool) {
	keys, ok := harnessInputKeys[harness]
	return keys, ok
}

// rawClearSequence reports the bytes one stale raw clear writes. Unknown
// harnesses get no sequence and are never cleared.
func rawClearSequence(harness string) ([]byte, bool) {
	keys, ok := harnessInputKeys[harness]
	if !ok || len(keys.clear) == 0 {
		return nil, false
	}
	return keys.clear, true
}

// isRawToggle says whether one input chunk is the mode toggle key.
func isRawToggle(p []byte) bool {
	key, ok := singleKey(p)
	return ok && key == string(rune(rawToggleByte))
}

// isRawReset says whether one forwarded chunk resets the dirty flag: a bare
// Enter or Ctrl+C, as a legacy byte or a CSI-u report. A el clear resets
// through elClear.
func isRawReset(p []byte) bool {
	key, ok := singleKey(p)
	return ok && (key == "\r" || key == "\x03")
}

// rawMode tracks one wrapper's raw dirty flag and last keystroke time. The
// enabled flag lives here too, under the same lock, so the pump gate and
// the strip read it without the wrapper lock.
type rawMode struct {
	mu      sync.Mutex
	harness string
	enabled bool
	isDirty bool
	lastKey time.Time
	now     func() time.Time
}

func newRawMode(harness string, now func() time.Time) *rawMode {
	if now == nil {
		now = time.Now
	}
	return &rawMode{harness: harness, now: now}
}

func (m *rawMode) timeNow() time.Time {
	return m.now()
}

// toggle flips raw passthrough and clears the dirty flag both ways, so
// leaving raw never leaves a hold behind.
func (m *rawMode) toggle() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.enabled = !m.enabled
	m.isDirty = false
	m.lastKey = m.timeNow()
	return m.enabled
}

// isEnabled reports whether raw passthrough is on.
func (m *rawMode) isEnabled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.enabled
}

// observeForward folds one chunk forwarded to the child into the dirty flag
// and stamps the keystroke time.
func (m *rawMode) observeForward(p []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastKey = m.timeNow()
	if isRawReset(p) {
		m.isDirty = false
		return
	}
	m.isDirty = true
}

// elClear resets the flag after the wrapper sent the clear sequence.
func (m *rawMode) elClear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.isDirty = false
}

// dirty reports whether the child's box may hold a draft.
func (m *rawMode) dirty() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.isDirty
}

// stale reports whether a dirty box sat with no keystroke past the timeout.
func (m *rawMode) stale() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.isDirty {
		return false
	}
	return !m.timeNow().Before(m.lastKey.Add(rawStaleTimeout))
}

// staleIn reports how long until a dirty box counts as stale. ok is false
// when the box is clean. The result floors at zero.
func (m *rawMode) staleIn() (d time.Duration, ok bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.isDirty {
		return 0, false
	}
	d = m.lastKey.Add(rawStaleTimeout).Sub(m.timeNow())
	if d < 0 {
		d = 0
	}
	return d, true
}

// canClear says whether this harness gets a stale clear.
func (m *rawMode) canClear() bool {
	_, ok := harnessInputKeys[m.harness]
	return ok
}

// renderRawHint draws the row area while raw passthrough is on: where keys
// go and which key returns to the row.
func renderRawHint(width int, dark bool, profile termenv.Profile) string {
	renderer := lipgloss.NewRenderer(io.Discard, termenv.WithProfile(profile))
	renderer.SetColorProfile(profile)
	pal := ElMocha()
	if !dark {
		pal = ElLatte()
	}
	hintStyle := renderer.NewStyle().Foreground(lipgloss.Color(pal.Dim))
	bgStyle := renderer.NewStyle().Background(lipgloss.Color(pal.StripBg))
	content := hintStyle.Render("raw — keys go to agent · Ctrl+O to return")
	contentLen := utf8.RuneCountInString("raw — keys go to agent · Ctrl+O to return")
	if pad := width - contentLen; pad > 0 {
		content += strings.Repeat(" ", pad)
	}
	return bgStyle.Render(content)
}
