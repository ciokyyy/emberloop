package wick

// Header strip PTY runner (ticket 04): WrapChild runs an agent command on a
// PTY sized to the terminal minus the strip rows and its top rule, forwards
// input, resize and alt-screen traffic untouched, and draws the strip in
// the reserved rows. The child sees a normal terminal: scrolling stays
// inside its rows through a scroll region the filter repairs after every
// reset, and columns stay inside the frame through left/right margins the
// filter repairs after the resets that wipe them.
//
// Signal note: SIGTERM or SIGHUP on wick-pig ends the process without the
// exit restore below or the raw-mode restore. The owner of the process
// lifetime (herdr, the user's shell) handles those; the wrapper restores
// the terminal on every return path of its own.

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/creack/pty"
	"github.com/muesli/termenv"
	"golang.org/x/term"

	"example.com/wick/tools"
)

// wrapPty is the surface the runner needs from a terminal: the real PTY in
// production, the fake child in tests.
type wrapPty interface {
	io.Reader
	io.Writer
	Close() error
	Resize(w, h int)
}

// WrapConfig runs one child under the strip.
type WrapConfig struct {
	Source   HeaderSource
	Settings tools.Settings
	Bin      string
	Args     []string
	Env      []string
	Stdin    io.Reader // default os.Stdin
	Stdout   io.Writer // default os.Stdout
	Stderr   io.Writer // plain-fallback error sink; default os.Stderr
	TermW    int       // terminal width; 0 detects from Stdout
	TermH    int       // terminal height; 0 detects from Stdout
	Poll     time.Duration
	// Start launches the child on a terminal of the size; the default
	// starts a real PTY. Wait reaps the child and reports its exit.
	Start    func(bin string, args, env []string, w, h int) (child wrapPty, wait func() error, err error)
	Snapshot func(HeaderSource, tools.Settings) (HeaderSnapshot, error)
}

// DefaultWrapPoll is the snapshot refresh while the child runs: usage rows
// land often enough that two seconds keeps the strip current without
// repainting over every burst of child output.
const DefaultWrapPoll = 2 * time.Second

// WrapChild runs Bin under the header strip and returns when the child
// ends, with the child's exit error. Input flows to the child, the child's
// output flows back with strip-breaking sequences repaired, and resizes
// reach the child at the reduced height. Without a terminal on Stdout it
// runs the child plainly: headless sessions have no rows to reserve.
func WrapChild(cfg WrapConfig) error {
	stdin := cfg.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}
	stdout := cfg.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	termW, termH := cfg.TermW, cfg.TermH
	if termW <= 0 || termH <= 0 {
		w, h, ok := detectTermSize(stdout)
		if !ok {
			return runPlain(cfg, stdin, stdout)
		}
		if termW <= 0 {
			termW = w
		}
		if termH <= 0 {
			termH = h
		}
	}
	rows := cfg.Settings.EffectiveStripRows()
	frame := cfg.Settings.StripFrameEnabled()
	rule := cfg.Settings.StripRuleEnabled()
	childW, childH := ChildSize(termW, termH, rows, frame)
	start := cfg.Start
	if start == nil {
		start = realStart
	}
	snapshot := cfg.Snapshot
	if snapshot == nil {
		snapshot = BuildHeaderSnapshot
	}
	child, wait, err := start(cfg.Bin, cfg.Args, cfg.Env, childW, childH)
	if err != nil {
		return err
	}
	defer child.Close()
	filter := &wrapFilter{childH: childH, margins: wrapMarginSetup(termW, frame)}
	w := &wrapper{
		cfg: cfg, stdin: stdin, stdout: stdout,
		termW: termW, termH: termH, childW: childW, childH: childH,
		rows: rows, frame: frame, rule: rule,
		dark:    detectDark(stdout),
		profile: detectProfile(stdout),
		child:   child, snapshot: snapshot, filter: filter,
	}
	if f, ok := stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) && cfg.Start == nil {
		if state, err := term.MakeRaw(int(f.Fd())); err == nil {
			defer term.Restore(int(f.Fd()), state)
		}
	}
	w.init()
	done := make(chan struct{})
	defer close(done)
	go w.forwardInput()
	if f, ok := stdout.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		go w.watchResize(done)
	}
	poll := cfg.Poll
	if poll <= 0 {
		poll = DefaultWrapPoll
	}
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	pulse := time.NewTicker(time.Second)
	defer pulse.Stop()
	// The snapshot ticker runs beside the read loop: an idle child blocks
	// in Read, and the strip must still follow state, cost and the pulse.
	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				w.repaint()
			case <-pulse.C:
				w.pulseTick()
			}
		}
	}()
	buf := make([]byte, 32*1024)
	for {
		n, err := child.Read(buf)
		if n > 0 {
			out, repaint := filter.push(buf[:n])
			w.write(string(out))
			if repaint {
				w.forced()
			}
		}
		if err != nil {
			break
		}
	}
	w.restore()
	return wait()
}

// HeaderStripDisabled is the rollout escape hatch: WICK_HEADER_STRIP=0 runs
// every child plainly, with no reserved rows and no strip. The default is
// the strip everywhere; the spike in scripts/ decides whether it stays.
func HeaderStripDisabled() bool {
	return os.Getenv("WICK_HEADER_STRIP") == "0"
}

// RunPlainCommand runs a child with the stdio directly: the no-terminal
// fallback the wrapper and the wrap command share. Stderr stays its own
// sink: callers that read stdout must never see error text in it.
func RunPlainCommand(bin string, args, env []string, stdin io.Reader, stdout, stderr io.Writer) error {
	c := exec.Command(bin, args...)
	c.Env = append(os.Environ(), env...)
	c.Stdin, c.Stdout, c.Stderr = stdin, stdout, stderr
	return c.Run()
}

// runPlain runs the child without a strip: no terminal means no reserved
// rows, so the child inherits the stdio directly.
func runPlain(cfg WrapConfig, stdin io.Reader, stdout io.Writer) error {
	stderr := cfg.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	return RunPlainCommand(cfg.Bin, cfg.Args, cfg.Env, stdin, stdout, stderr)
}

// wrapMarginSetup is the column constraint for the child screen, enable
// included: one column per side when the frame is on, empty (no
// constraint) when it is off. A full reset wipes the enable with the
// margins, so repairs send the whole setup again.
func wrapMarginSetup(termW int, frame bool) string {
	if !frame {
		return ""
	}
	return fmt.Sprintf("\x1b[?69h\x1b[2;%ds", termW-1)
}

// wrapper holds one wrapped child: its sizes, theme and the last strip it
// painted, so repaints happen only when the numbers change.
type wrapper struct {
	cfg      WrapConfig
	stdin    io.Reader
	stdout   io.Writer
	termW    int
	termH    int
	childW   int
	childH   int
	rows     int
	frame    bool
	rule     bool
	dark     bool
	profile  termenv.Profile
	child    wrapPty
	snapshot func(HeaderSource, tools.Settings) (HeaderSnapshot, error)
	filter   *wrapFilter
	mu       sync.Mutex
	last     string
	phase    bool
	closed   bool
}

// contentW is the strip width: the terminal minus the frame columns.
func (w *wrapper) contentW() int {
	if w.frame {
		return w.termW - 2
	}
	return w.termW
}

// ruleRow is the terminal row of the strip's top rule: the first row past
// the child area.
func (w *wrapper) ruleRow() int { return w.childH + 1 }

// init claims the strip rows: the scroll region and the column margins end
// above them, the frame goes up, and the first snapshot paints
// immediately.
func (w *wrapper) init() {
	snap, ok := w.readSnap()
	w.mu.Lock()
	defer w.mu.Unlock()
	var b strings.Builder
	b.WriteString(fmt.Sprintf("\x1b[1;%dr", w.childH))
	if w.frame {
		b.WriteString(w.filter.marginStr())
	}
	w.writeLocked(b.String())
	w.drawFrameLocked()
	if ok {
		w.paintLocked(w.renderSnapLocked(snap))
	}
}

// readSnap builds the current strip snapshot outside the lock: transcript
// and database IO must never hold it while child output waits to write.
func (w *wrapper) readSnap() (HeaderSnapshot, bool) {
	snap, err := w.snapshot(w.cfg.Source, w.cfg.Settings)
	if err != nil {
		return HeaderSnapshot{}, false
	}
	return snap, true
}

// renderSnapLocked renders a snapshot under the lock, so geometry and
// pulse phase stay consistent with the paint.
func (w *wrapper) renderSnapLocked(snap HeaderSnapshot) string {
	return RenderHeader(snap, w.contentW(), HeaderOpts{Rows: w.rows, Dark: w.dark, Profile: w.profile, Pulse: w.phase})
}

// repaint redraws the strip when the numbers changed.
func (w *wrapper) repaint() {
	snap, ok := w.readSnap()
	if !ok {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.paintLocked(w.renderSnapLocked(snap))
}

// pulseTick advances the compressing pill one phase and repaints. Other
// states render identically in both phases, so the equality check below
// makes the tick a no-op for them.
func (w *wrapper) pulseTick() {
	snap, ok := w.readSnap()
	if !ok {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.phase = !w.phase
	w.paintLocked(w.renderSnapLocked(snap))
}

// forced repaints unconditionally: a clear or full reset erased the strip,
// so the equality check must not skip the redraw.
func (w *wrapper) forced() {
	snap, ok := w.readSnap()
	if !ok {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.last = ""
	w.paintLocked(w.renderSnapLocked(snap))
}

// paintLocked draws the top rule and the strip rows at their reserved
// rows, then restores the child's cursor. Equal output stays unpainted, so
// the strip never flickers over child output that did not change it. The
// save uses ESC 7, not CSI s: with the column margins enabled CSI s would
// set margins instead of saving the cursor. A paint after restore is a
// no-op: the terminal is already handed back.
func (w *wrapper) paintLocked(strip string) {
	if w.closed || strip == "" || strip == w.last {
		return
	}
	w.last = strip
	var b strings.Builder
	b.WriteString("\x1b7")
	col := 1
	if w.frame {
		col = 2
	}
	b.WriteString(fmt.Sprintf("\x1b[%d;%dH%s", w.ruleRow(), col, w.topRule()))
	for i, line := range strings.Split(strip, "\n") {
		b.WriteString(fmt.Sprintf("\x1b[%d;%dH%s", w.ruleRow()+1+i, col, line))
	}
	b.WriteString("\x1b8")
	w.writeLocked(b.String())
}

// topRule is the strip's top edge. By default it is a full-width row of
// lower half blocks in the strip background, so the strip reads as a tinted
// block with a soft edge and no drawn line. The strip-rule setting restores
// the drawn line instead: one full-width row of dashes in the agent hue,
// above the content rows, on the strip background. Either way the agent hue
// stays in the name chip and ctx label.
func (w *wrapper) topRule() string {
	r := lipgloss.NewRenderer(io.Discard, termenv.WithProfile(w.profile))
	r.SetColorProfile(w.profile)
	if w.rule {
		pair := agentHueColor(w.cfg.Source.AgentID, w.cfg.Source.AgentType)
		hex := pair.dark
		if !w.dark {
			hex = pair.light
		}
		return r.NewStyle().Foreground(lipgloss.Color(hex)).Background(lipgloss.Color(w.stripBg())).Width(w.contentW()).Render(strings.Repeat("─", w.contentW()))
	}
	return r.NewStyle().Foreground(lipgloss.Color(w.stripBg())).Width(w.contentW()).Render(strings.Repeat("▄", w.contentW()))
}

func (w *wrapper) stripBg() string {
	if w.dark {
		return headerStripBg.dark
	}
	return headerStripBg.light
}

// drawFrameLocked paints the agent-colored frame beside the child screen:
// one hue column per side, full height. The child's own columns stay
// inside the margins, so it can never overwrite the frame.
func (w *wrapper) drawFrameLocked() {
	if !w.frame {
		return
	}
	var b strings.Builder
	b.WriteString("\x1b7")
	for r := 1; r <= w.termH; r++ {
		b.WriteString(fmt.Sprintf("\x1b[%d;1H%s\x1b[%d;%dH%s", r, w.edge(), r, w.termW, w.edge()))
	}
	b.WriteString("\x1b8")
	w.writeLocked(b.String())
}

// edge is one frame cell in the agent hue.
func (w *wrapper) edge() string {
	r := lipgloss.NewRenderer(io.Discard, termenv.WithProfile(w.profile))
	r.SetColorProfile(w.profile)
	pair := agentHueColor(w.cfg.Source.AgentID, w.cfg.Source.AgentType)
	hex := pair.dark
	if !w.dark {
		hex = pair.light
	}
	return r.NewStyle().Foreground(lipgloss.Color(hex)).Render("│")
}

// restore hands the terminal back: the scroll region and the margins go
// back to full-screen, the strip rows are cleared, and the cursor lands on
// the bottom row. Every return path after the read loop runs through here,
// so the user's shell never inherits a broken region.
func (w *wrapper) restore() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	var b strings.Builder
	b.WriteString("\x1b[r")
	if w.frame {
		b.WriteString("\x1b[?69l")
	}
	for r := w.ruleRow(); r <= w.termH; r++ {
		b.WriteString(fmt.Sprintf("\x1b[%d;1H\x1b[2K", r))
	}
	b.WriteString(fmt.Sprintf("\x1b[%d;1H", w.termH))
	w.writeLocked(b.String())
}

// forwardInput copies typed input to the child until stdin ends or a write
// fails. A post-exit write fails against the closed child and ends the
// loop; a terminal stdin outlives the child only until the process exits.
func (w *wrapper) forwardInput() {
	buf := make([]byte, 32*1024)
	for {
		n, err := w.stdin.Read(buf)
		if n > 0 {
			if _, werr := w.child.Write(buf[:n]); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// watchResize forwards terminal resizes to resized: the child keeps the
// reduced height and the strip repaints around the new size.
func (w *wrapper) watchResize(done <-chan struct{}) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	defer signal.Stop(ch)
	for {
		select {
		case <-done:
			return
		case <-ch:
			if f, ok := w.stdout.(*os.File); ok {
				if ws, err := pty.GetsizeFull(f); err == nil {
					w.resized(int(ws.Cols), int(ws.Rows))
				}
			}
		}
	}
}

// resized applies a new terminal size: the child is re-sized to the reduced
// size, the filter learns the new height and margins, and the strip repaints
// around it. Split from the signal loop so tests drive it directly.
func (w *wrapper) resized(termW, termH int) {
	snap, ok := w.readSnap()
	w.mu.Lock()
	defer w.mu.Unlock()
	w.termW, w.termH = termW, termH
	w.childW, w.childH = ChildSize(w.termW, w.termH, w.rows, w.frame)
	w.child.Resize(w.childW, w.childH)
	w.filter.setSize(w.childH, wrapMarginSetup(w.termW, w.frame))
	var b strings.Builder
	b.WriteString(fmt.Sprintf("\x1b[1;%dr", w.childH))
	if w.frame {
		b.WriteString(w.filter.marginStr())
	}
	w.writeLocked(b.String())
	w.drawFrameLocked()
	w.last = ""
	if ok {
		w.paintLocked(w.renderSnapLocked(snap))
	}
}

func (w *wrapper) write(s string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.writeLocked(s)
}

func (w *wrapper) writeLocked(s string) {
	_, _ = io.WriteString(w.stdout, s)
}

// wrapFilter is the stateful side of the child filter: it holds a trailing
// partial escape for the next chunk and watches the alt screen, so a reset
// split across reads is still repaired and leaving alt-screen repaints.
// The height and margins track resizes through setSize.
type wrapFilter struct {
	mu      sync.Mutex
	childH  int
	margins string
	buf     []byte
	alt     bool
}

// setSize learns a resize: later chunks repair regions and clamp against
// the new height.
func (f *wrapFilter) setSize(childH int, margins string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.childH, f.margins = childH, margins
}

// marginStr reports the column constraint for region repairs.
func (f *wrapFilter) marginStr() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.margins
}

func (f *wrapFilter) push(data []byte) (out []byte, repaint bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.buf = append(f.buf, data...)
	cut := holdCut(f.buf)
	if len(f.buf)-cut > 64 {
		// A held tail that never completes is binary output, not an
		// escape: flush it rather than growing without bound.
		cut = len(f.buf)
	}
	head := f.buf[:cut]
	f.buf = append([]byte(nil), f.buf[cut:]...)
	if len(head) == 0 {
		return nil, false
	}
	return f.process(head)
}

func (f *wrapFilter) process(head []byte) (out []byte, repaint bool) {
	if bytes.Contains(head, []byte("?1049h")) || bytes.Contains(head, []byte("?47h")) {
		f.alt = true
	}
	out, repaint = FilterChildOutput(head, f.childH, f.margins)
	if f.alt && (bytes.Contains(head, []byte("?1049l")) || bytes.Contains(head, []byte("?47l"))) {
		f.alt = false
		repaint = true
	}
	return out, repaint
}

// holdCut is the split point holding a trailing partial escape: the index
// of the last ESC when nothing after it completes a sequence.
func holdCut(b []byte) int {
	i := bytes.LastIndexByte(b, 0x1b)
	if i < 0 {
		return len(b)
	}
	if completeSuffix(b[i:]) {
		return len(b)
	}
	return i
}

// completeSuffix says whether the bytes after the last ESC already form a
// complete sequence: a CSI with its final byte, a two-byte sequence, or an
// OSC terminated by BEL or ESC backslash.
func completeSuffix(tail []byte) bool {
	if len(tail) < 2 {
		return false
	}
	if tail[1] == ']' {
		return bytes.IndexByte(tail, 0x07) >= 0 || bytes.Contains(tail, []byte("\x1b\\"))
	}
	if tail[1] == '[' {
		for _, c := range tail[2:] {
			if c >= 0x40 && c <= 0x7e {
				return true
			}
		}
		return false
	}
	return tail[1] != 0x1b
}

// detectTermSize reads the terminal size from a TTY stdout.
func detectTermSize(stdout io.Writer) (int, int, bool) {
	f, ok := stdout.(*os.File)
	if !ok || !term.IsTerminal(int(f.Fd())) {
		return 0, 0, false
	}
	ws, err := pty.GetsizeFull(f)
	if err != nil || ws.Cols == 0 || ws.Rows == 0 {
		return 0, 0, false
	}
	return int(ws.Cols), int(ws.Rows), true
}

// detectDark says whether the terminal is dark. Termenv reads COLORFGBG
// first and otherwise asks the terminal with a bounded wait, so a terminal
// that ignores the query costs a timeout, never a hang. It runs before raw
// mode, while the tty still answers.
func detectDark(stdout io.Writer) bool {
	if f, ok := stdout.(*os.File); ok {
		if out := termenv.NewOutput(f); out != nil {
			return out.HasDarkBackground()
		}
	}
	return true
}

// detectProfile is the color profile for the strip: the terminal's own
// when Stdout is a TTY, else truecolor, so Lip Gloss downgrades the
// palette itself on limited terminals.
func detectProfile(stdout io.Writer) termenv.Profile {
	if f, ok := stdout.(*os.File); ok {
		return termenv.NewOutput(f).Profile
	}
	return termenv.TrueColor
}

// realStart launches the child on a real PTY at the size.
func realStart(bin string, args, env []string, w, h int) (wrapPty, func() error, error) {
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), env...)
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: uint16(h), Cols: uint16(w)})
	if err != nil {
		return nil, nil, err
	}
	return &ptyChild{f: f}, cmd.Wait, nil
}

// ptyChild adapts a PTY master to the runner's surface.
type ptyChild struct {
	f *os.File
}

func (p *ptyChild) Read(b []byte) (int, error)  { return p.f.Read(b) }
func (p *ptyChild) Write(b []byte) (int, error) { return p.f.Write(b) }
func (p *ptyChild) Close() error                { return p.f.Close() }
func (p *ptyChild) Resize(w, h int) {
	_ = pty.Setsize(p.f, &pty.Winsize{Rows: uint16(h), Cols: uint16(w)})
}
