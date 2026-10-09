package el

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// rowAction tells the wrapper what action the input triggered.
type rowAction int

const (
	rowActionNone rowAction = iota
	rowActionSubmit
	rowActionQueue
	rowActionInterrupt
	rowActionEsc
)

// draftStaleTimeout is how long an unsubmitted draft may sit with no keystrokes
// before it is cleared and held injections are delivered.
const draftStaleTimeout = 10 * time.Second

// inputRow manages user prompt editing between the child PTY and the strip.
// Normal typing collects in the row buffer; Enter submits as steer; Alt+Enter
// queues; Shift+Enter inserts a newline; bracketed paste arrives as one edit;
// Up recalls history; Ctrl+C / Esc forward interrupts to the child.
type inputRow struct {
	mu            sync.Mutex
	buf           []rune
	cursor        int
	history       []string
	historyIdx    int
	savedDraft    string
	lastKeystroke time.Time
	now           func() time.Time
	pasting       bool
	pasteBuf      bytes.Buffer
	pending       []byte // incomplete escape sequence held for the next chunk
}

func newInputRow() *inputRow {
	return &inputRow{
		historyIdx: -1,
		now:        time.Now,
	}
}

func (r *inputRow) timeNow() time.Time {
	if r.now != nil {
		return r.now()
	}
	return time.Now()
}

func (r *inputRow) draft() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return string(r.buf)
}

func (r *inputRow) hasDraft() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.buf) > 0
}

// staleIn reports how long until the draft counts as stale. ok is false when
// the row holds no draft. The result floors at zero.
func (r *inputRow) staleIn() (d time.Duration, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.buf) == 0 {
		return 0, false
	}
	d = r.lastKeystroke.Add(draftStaleTimeout).Sub(r.timeNow())
	if d < 0 {
		d = 0
	}
	return d, true
}

func (r *inputRow) checkStale() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.buf) == 0 {
		return false
	}
	if !r.timeNow().Before(r.lastKeystroke.Add(draftStaleTimeout)) {
		r.buf = nil
		r.cursor = 0
		r.historyIdx = -1
		r.savedDraft = ""
		return true
	}
	return false
}

// insertRunes inserts text at the current cursor position.
func (r *inputRow) insertRunes(runes []rune) {
	if len(runes) == 0 {
		return
	}
	newBuf := make([]rune, 0, len(r.buf)+len(runes))
	newBuf = append(newBuf, r.buf[:r.cursor]...)
	newBuf = append(newBuf, runes...)
	newBuf = append(newBuf, r.buf[r.cursor:]...)
	r.buf = newBuf
	r.cursor += len(runes)
}

// handleInput feeds one stdin chunk to the row. The decoder splits the chunk
// into keys; the first key that triggers an action ends the chunk.
func (r *inputRow) handleInput(p []byte) (rowAction, string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.lastKeystroke = r.timeNow()
	return r.feed(p)
}

func (r *inputRow) feed(p []byte) (rowAction, string) {
	// Bracketed paste handling
	if r.pasting {
		if idx := bytes.Index(p, []byte("\x1b[201~")); idx >= 0 {
			r.pasteBuf.Write(p[:idx])
			pasted := r.pasteBuf.String()
			r.pasteBuf.Reset()
			r.pasting = false
			r.insertRunes([]rune(pasted))
			return r.feed(p[idx+len("\x1b[201~"):])
		}
		r.pasteBuf.Write(p)
		return rowActionNone, ""
	}

	if len(r.pending) > 0 {
		p = append(r.pending, p...)
		r.pending = nil
	}

	if idx := bytes.Index(p, []byte("\x1b[200~")); idx >= 0 {
		r.feed(p[:idx])
		after := p[idx+len("\x1b[200~"):]
		if endIdx := bytes.Index(after, []byte("\x1b[201~")); endIdx >= 0 {
			// Entire bracketed paste within one chunk
			r.insertRunes([]rune(string(after[:endIdx])))
			return r.feed(after[endIdx+len("\x1b[201~"):])
		}
		r.pasting = true
		r.pasteBuf.Write(after)
		return rowActionNone, ""
	}

	for len(p) > 0 {
		key, n, complete := nextKey(p)
		if !complete {
			// A sequence split across reads: keep it for the next chunk.
			r.pending = append([]byte(nil), p...)
			break
		}
		p = p[n:]
		if act, text := r.handleKey(key); act != rowActionNone {
			return act, text
		}
	}
	return rowActionNone, ""
}

// resetDraft clears the buffer and history position.
func (r *inputRow) resetDraft() {
	r.buf = nil
	r.cursor = 0
	r.historyIdx = -1
	r.savedDraft = ""
}

// wordLeft returns the start of the word before the cursor.
func (r *inputRow) wordLeft() int {
	i := r.cursor
	for i > 0 && unicode.IsSpace(r.buf[i-1]) {
		i--
	}
	for i > 0 && !unicode.IsSpace(r.buf[i-1]) {
		i--
	}
	return i
}

// wordRight returns the end of the word after the cursor.
func (r *inputRow) wordRight() int {
	i := r.cursor
	for i < len(r.buf) && unicode.IsSpace(r.buf[i]) {
		i++
	}
	for i < len(r.buf) && !unicode.IsSpace(r.buf[i]) {
		i++
	}
	return i
}

func (r *inputRow) deleteRange(from, to int) {
	if from >= to {
		return
	}
	r.buf = append(r.buf[:from:from], r.buf[to:]...)
	r.cursor = from
}

// handleKey acts on one canonical key from the decoder.
func (r *inputRow) handleKey(s string) (rowAction, string) {
	switch s {
	case "":
		return rowActionNone, ""
	case "\x03": // Ctrl+C interrupt
		r.buf = nil
		r.cursor = 0
		r.historyIdx = -1
		return rowActionInterrupt, ""
	case "\x1b": // Esc interrupt
		return rowActionEsc, ""
	case "\x1b\r", "\x1b\n": // Alt+Enter (queue)
		text := strings.TrimSpace(string(r.buf))
		if text == "" {
			return rowActionNone, ""
		}
		r.history = append(r.history, text)
		r.resetDraft()
		return rowActionQueue, text
	case "\n": // Shift+Enter (insert newline)
		r.insertRunes([]rune{'\n'})
	case "\r": // Enter (submit as steer)
		text := strings.TrimSpace(string(r.buf))
		if text == "" {
			return rowActionNone, ""
		}
		r.history = append(r.history, text)
		r.resetDraft()
		return rowActionSubmit, text
	case "\x1b[A": // Up arrow (history recall)
		if len(r.history) > 0 {
			if r.historyIdx == -1 {
				r.savedDraft = string(r.buf)
				r.historyIdx = len(r.history) - 1
			} else if r.historyIdx > 0 {
				r.historyIdx--
			}
			r.buf = []rune(r.history[r.historyIdx])
			r.cursor = len(r.buf)
		}
	case "\x1b[B": // Down arrow (history forward)
		if r.historyIdx >= 0 {
			r.historyIdx++
			if r.historyIdx < len(r.history) {
				r.buf = []rune(r.history[r.historyIdx])
			} else {
				r.historyIdx = -1
				r.buf = []rune(r.savedDraft)
			}
			r.cursor = len(r.buf)
		}
	case "\x1b[D": // Left arrow
		if r.cursor > 0 {
			r.cursor--
		}
	case "\x1b[C": // Right arrow
		if r.cursor < len(r.buf) {
			r.cursor++
		}
	case "\x1bb": // Ctrl/Alt+Left, Alt+B
		r.cursor = r.wordLeft()
	case "\x1bf": // Ctrl/Alt+Right, Alt+F
		r.cursor = r.wordRight()
	case "\x1b[H", "\x01": // Home / Ctrl+A
		r.cursor = 0
	case "\x1b[F", "\x05": // End / Ctrl+E
		r.cursor = len(r.buf)
	case "\x7f", "\x08": // Backspace
		if r.cursor > 0 {
			r.deleteRange(r.cursor-1, r.cursor)
		}
	case "\x17", "\x1b\x7f": // Ctrl+W, Ctrl/Alt+Backspace (delete previous word)
		r.deleteRange(r.wordLeft(), r.cursor)
	case "\x1b[3~": // Delete
		if r.cursor < len(r.buf) {
			r.deleteRange(r.cursor, r.cursor+1)
		}
	case "\x1bd": // Ctrl/Alt+Delete, Alt+D (delete next word)
		r.deleteRange(r.cursor, r.wordRight())
	case "\x15": // Ctrl+U (clear draft)
		r.buf = nil
		r.cursor = 0
	default:
		// Printable text only. Unhandled control bytes and Alt chords are
		// dropped, never typed into the row.
		if first, _ := utf8.DecodeRuneInString(s); first >= 32 && first != 0x7f || first == '\t' {
			r.insertRunes([]rune(s))
		}
	}
	return rowActionNone, ""
}

// render formats the input row with lipgloss styles matching the palette.
func (r *inputRow) render(width int, dark bool, profile termenv.Profile) string {
	r.mu.Lock()
	defer r.mu.Unlock()

	renderer := lipgloss.NewRenderer(io.Discard, termenv.WithProfile(profile))
	renderer.SetColorProfile(profile)

	pal := ElMocha()
	if !dark {
		pal = ElLatte()
	}

	promptStyle := renderer.NewStyle().Foreground(lipgloss.Color(pal.Accent)).Bold(true)
	textStyle := renderer.NewStyle().Foreground(lipgloss.Color(pal.Text))
	bgStyle := renderer.NewStyle().Background(lipgloss.Color(pal.StripBg))

	// Display newlines in single row as ↵. The cursor draws as one reversed
	// cell: the rune under it, or a space at the end of the draft.
	cursor := min(max(r.cursor, 0), len(r.buf))
	before := strings.ReplaceAll(string(r.buf[:cursor]), "\n", " ↵ ")
	under, after := " ", ""
	if cursor < len(r.buf) {
		under = strings.ReplaceAll(string(r.buf[cursor]), "\n", "↵")
		after = strings.ReplaceAll(string(r.buf[cursor+1:]), "\n", " ↵ ")
	}
	cursorStyle := renderer.NewStyle().Foreground(lipgloss.Color(pal.Text)).Reverse(true)

	content := promptStyle.Render("❯ ") + textStyle.Render(before) + cursorStyle.Render(under) + textStyle.Render(after)
	contentLen := utf8.RuneCountInString("❯ " + before + under + after)
	if pad := width - contentLen; pad > 0 {
		content += strings.Repeat(" ", pad)
	}

	return bgStyle.Render(content)
}
