package wick

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"time"
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

func (r *inputRow) handleInput(p []byte) (rowAction, string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.lastKeystroke = r.timeNow()

	// Bracketed paste handling
	if r.pasting {
		if idx := bytes.Index(p, []byte("\x1b[201~")); idx >= 0 {
			r.pasteBuf.Write(p[:idx])
			pasted := r.pasteBuf.String()
			r.pasteBuf.Reset()
			r.pasting = false
			r.insertRunes([]rune(pasted))
			// Handle any trailing bytes after the paste marker
			if rest := p[idx+len("\x1b[201~"):]; len(rest) > 0 {
				r.mu.Unlock()
				act, text := r.handleInput(rest)
				r.mu.Lock()
				return act, text
			}
			return rowActionNone, ""
		}
		r.pasteBuf.Write(p)
		return rowActionNone, ""
	}

	if idx := bytes.Index(p, []byte("\x1b[200~")); idx >= 0 {
		// Insert any prefix before paste start
		if idx > 0 {
			r.insertRunes([]rune(string(p[:idx])))
		}
		after := p[idx+len("\x1b[200~"):]
		if endIdx := bytes.Index(after, []byte("\x1b[201~")); endIdx >= 0 {
			// Entire bracketed paste within one chunk
			pasted := string(after[:endIdx])
			r.insertRunes([]rune(pasted))
			if rest := after[endIdx+len("\x1b[201~"):]; len(rest) > 0 {
				r.mu.Unlock()
				act, text := r.handleInput(rest)
				r.mu.Lock()
				return act, text
			}
			return rowActionNone, ""
		}
		r.pasting = true
		r.pasteBuf.Write(after)
		return rowActionNone, ""
	}

	s := string(p)

	// Trailing Alt+Enter with preceding text: e.g. "my text\x1b\r"
	if strings.HasSuffix(s, "\x1b\r") && len(s) > 2 {
		r.insertRunes([]rune(strings.TrimSuffix(s, "\x1b\r")))
		s = "\x1b\r"
	}

	// Trailing Enter with preceding text: e.g. "my text\r" or "my text\r\n"
	if (strings.HasSuffix(s, "\r") || strings.HasSuffix(s, "\r\n")) && !strings.HasPrefix(s, "\x1b") {
		trimmed := strings.TrimSuffix(strings.TrimSuffix(s, "\r\n"), "\r")
		if len(trimmed) > 0 {
			r.insertRunes([]rune(trimmed))
			s = "\r"
		}
	}
	// Ctrl+C interrupt
	if s == "\x03" {
		r.buf = nil
		r.cursor = 0
		r.historyIdx = -1
		return rowActionInterrupt, ""
	}

	// Esc interrupt
	if s == "\x1b" {
		return rowActionEsc, ""
	}

	// Alt+Enter (queue)
	if s == "\x1b\r" || s == "\x1b\n" || s == "\x1b[13;3u" || s == "\x1b[27;3;13~" {
		text := strings.TrimSpace(string(r.buf))
		if text == "" {
			return rowActionNone, ""
		}
		r.history = append(r.history, text)
		r.buf = nil
		r.cursor = 0
		r.historyIdx = -1
		r.savedDraft = ""
		return rowActionQueue, text
	}

	// Shift+Enter (insert newline)
	if s == "\x1b[13;2u" || s == "\x1b[27;2;13~" || s == "\n" {
		r.insertRunes([]rune{'\n'})
		return rowActionNone, ""
	}

	// Enter (submit as steer)
	if s == "\r" || s == "\r\n" || s == "\x1b[13;1u" || s == "\x1b[13u" {
		text := strings.TrimSpace(string(r.buf))
		if text == "" {
			return rowActionNone, ""
		}
		r.history = append(r.history, text)
		r.buf = nil
		r.cursor = 0
		r.historyIdx = -1
		r.savedDraft = ""
		return rowActionSubmit, text
	}

	// Up arrow (history recall)
	if s == "\x1b[A" || s == "\x1bOA" {
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
		return rowActionNone, ""
	}

	// Down arrow (history forward)
	if s == "\x1b[B" || s == "\x1bOB" {
		if r.historyIdx >= 0 {
			r.historyIdx++
			if r.historyIdx < len(r.history) {
				r.buf = []rune(r.history[r.historyIdx])
				r.cursor = len(r.buf)
			} else {
				r.historyIdx = -1
				r.buf = []rune(r.savedDraft)
				r.cursor = len(r.buf)
			}
		}
		return rowActionNone, ""
	}

	// Left arrow
	if s == "\x1b[D" || s == "\x1bOD" {
		if r.cursor > 0 {
			r.cursor--
		}
		return rowActionNone, ""
	}

	// Right arrow
	if s == "\x1b[C" || s == "\x1bOC" {
		if r.cursor < len(r.buf) {
			r.cursor++
		}
		return rowActionNone, ""
	}

	// Home / Ctrl+A
	if s == "\x1b[H" || s == "\x1b[1~" || s == "\x01" {
		r.cursor = 0
		return rowActionNone, ""
	}

	// End / Ctrl+E
	if s == "\x1b[F" || s == "\x1b[4~" || s == "\x05" {
		r.cursor = len(r.buf)
		return rowActionNone, ""
	}

	// Backspace
	if s == "\x7f" || s == "\x08" {
		if r.cursor > 0 {
			r.buf = append(r.buf[:r.cursor-1], r.buf[r.cursor:]...)
			r.cursor--
		}
		return rowActionNone, ""
	}

	// Delete
	if s == "\x1b[3~" {
		if r.cursor < len(r.buf) {
			r.buf = append(r.buf[:r.cursor], r.buf[r.cursor+1:]...)
		}
		return rowActionNone, ""
	}

	// Ctrl+U (clear draft)
	if s == "\x15" {
		r.buf = nil
		r.cursor = 0
		return rowActionNone, ""
	}

	// Regular text insertion: filter out non-printable control characters
	var runes []rune
	for _, rn := range s {
		if rn >= 32 || rn == '\t' || rn == '\n' {
			runes = append(runes, rn)
		}
	}
	r.insertRunes(runes)
	return rowActionNone, ""
}

// render formats the input row with lipgloss styles matching the palette.
func (r *inputRow) render(width int, dark bool, profile termenv.Profile) string {
	r.mu.Lock()
	defer r.mu.Unlock()

	renderer := lipgloss.NewRenderer(io.Discard, termenv.WithProfile(profile))
	renderer.SetColorProfile(profile)

	pal := WickMocha()
	if !dark {
		pal = WickLatte()
	}

	promptStyle := renderer.NewStyle().Foreground(lipgloss.Color(pal.Accent)).Bold(true)
	textStyle := renderer.NewStyle().Foreground(lipgloss.Color(pal.Text))
	bgStyle := renderer.NewStyle().Background(lipgloss.Color(pal.StripBg))

	// Display newlines in single row as ↵
	displayBuf := strings.ReplaceAll(string(r.buf), "\n", " ↵ ")

	content := promptStyle.Render("❯ ") + textStyle.Render(displayBuf)
	contentLen := utf8.RuneCountInString("❯ ") + utf8.RuneCountInString(displayBuf)
	if pad := width - contentLen; pad > 0 {
		content += strings.Repeat(" ", pad)
	}

	return bgStyle.Render(content)
}
