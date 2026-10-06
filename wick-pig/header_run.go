package wick

// Header strip PTY runner (ticket 04): WrapChild runs an agent command on a
// PTY sized to the terminal minus the strip rows and its top rule, forwards
// input, resize and alt-screen traffic untouched, and draws the strip in
// the reserved rows. The child sees a normal terminal: scrolling stays
// inside its rows through a scroll region the filter repairs after every
// reset, and columns stay inside the frame through left/right margins the
// filter repairs after the resets that wipe them. The shared theme
// (ticket 06) reaches the pane the same way: the wrapper sets the pane
// palette at start and resets it on exit, on signals and across restarts
// (each restart is a new WrapChild, so it sets again).
//
// Signal note: SIGINT, SIGTERM or SIGHUP on wick-pig reset the pane palette
// and re-raise to the default disposition, so the process still exits with
// the signal. A relaunch after a compress restart sets the palette again.

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
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
	// ChildStderr is the wrapped child's stderr sink: stdin and stdout
	// stay on the PTY under the strip while stderr keeps its own pipe
	// here. Nil merges stderr into the PTY, as before. The plain
	// fallback reads it first, so headless pig sessions still log;
	// wrapper warnings keep Stderr.
	ChildStderr io.Writer
	TermW       int // terminal width; 0 detects from Stdout
	TermH       int // terminal height; 0 detects from Stdout
	Poll        time.Duration
	ThemeDir    string // generated theme files; default wick's state dir
	ThemeDark   *bool  // variant override; default detects from Stdout
	// Start launches the child on a terminal of the size, with its
	// stderr on the pipe when non-nil and merged into the terminal
	// when nil; the default starts a real PTY. Wait reaps the child
	// and reports its exit.
	Start    func(bin string, args, env []string, w, h int, stderr io.Writer) (child wrapPty, wait func() error, err error)
	Snapshot func(HeaderSource, tools.Settings) (HeaderSnapshot, error)
	// Report runs one herdr command that reports a claude child as an agent;
	// the default runs herdr. PaneID names the pane; the default reads
	// HERDR_PANE_ID when HERDR_ENV is 1. Both exist for tests.
	Report func(args []string)
	PaneID string
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
	pad := FitPad(termW, termH, rows, frame, Pad{
		Left:   cfg.Settings.EffectivePadLeft(),
		Right:  cfg.Settings.EffectivePadRight(),
		Bottom: cfg.Settings.EffectivePadBottom(),
	})
	childW, childH := ChildSizePad(termW, termH, rows, frame, pad)
	dark := detectDark(stdout)
	if cfg.ThemeDark != nil {
		dark = *cfg.ThemeDark
	}
	bin, args, themeEnv := themedChildCommand(cfg, dark)
	start := cfg.Start
	if start == nil {
		start = realStart
	}
	snapshot := cfg.Snapshot
	if snapshot == nil {
		snapshot = BuildHeaderSnapshot
	}
	child, wait, err := start(bin, args, themeEnv, childW, childH, cfg.ChildStderr)
	if err != nil {
		return err
	}
	defer child.Close()
	filter := &wrapFilter{childH: childH, margins: wrapMarginSetupPad(termW, frame, pad)}
	w := &wrapper{
		cfg: cfg, stdin: stdin, stdout: stdout,
		termW: termW, termH: termH, childW: childW, childH: childH,
		rows: rows, frame: frame, rule: rule, pad: pad,
		dark:    dark,
		profile: detectProfile(stdout),
		child:   child, snapshot: snapshot, filter: filter,
		reporter: newAgentReporter(cfg),
	}
	w.syncMouseLocked()
	defer w.reporter.release()
	w.reporter.report(HeaderIdle)
	if f, ok := stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) && cfg.Start == nil {
		if state, err := term.MakeRaw(int(f.Fd())); err == nil {
			defer term.Restore(int(f.Fd()), state)
		}
	}
	w.init()
	done := make(chan struct{})
	defer close(done)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	go watchWrapSignals(sigs, done, func() { w.write(ThemeResetSequence()) }, func(s os.Signal) { resignalSelf(s, sigs) })
	go w.forwardInput()
	if cfg.Snapshot == nil {
		go w.pollUsage(done)
	}
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
			w.reporter.observe(out)
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
// rows, so the child inherits the stdio directly. A set ChildStderr stays
// the child's sink, so headless pig sessions still reach the tools log.
func runPlain(cfg WrapConfig, stdin io.Reader, stdout io.Writer) error {
	stderr := cfg.ChildStderr
	if stderr == nil {
		stderr = cfg.Stderr
	}
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
	return wrapMarginSetupPad(termW, frame, Pad{})
}

// wrapMarginSetupPad is wrapMarginSetup with the padding columns: the child
// screen starts after the left frame and padding columns and ends before the
// right ones. Empty when nothing constrains the columns.
func wrapMarginSetupPad(termW int, frame bool, pad Pad) string {
	left, right := pad.Left, pad.Right
	if frame {
		left++
		right++
	}
	if left == 0 && right == 0 {
		return ""
	}
	// Origin mode (?6) makes absolute cursor addresses relative to the
	// margins, so a child that positions with CUP (pig) lands inside the
	// padding like one that moves relatively (claude).
	return fmt.Sprintf("\x1b[?69h\x1b[%d;%ds\x1b[?6h", left+1, termW-right)
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
	pad      Pad
	dark     bool
	profile  termenv.Profile
	child    wrapPty
	snapshot func(HeaderSource, tools.Settings) (HeaderSnapshot, error)
	filter   *wrapFilter
	reporter *agentReporter
	mouse    mouseMapper
	// lastPlain is the painted strip rows without colors, for copying a
	// drag across them.
	lastPlain []string
	usageKey  atomic.Value // string: the account the strip's usage reads
	mu        sync.Mutex
	last      string
	phase     bool
	closed    bool
}

// sideCols are the columns the strip and the child lose on the left and the
// right: the frame column (when on) plus the padding.
func (w *wrapper) sideCols() (left, right int) {
	left, right = w.pad.Left, w.pad.Right
	if w.frame {
		left++
		right++
	}
	return left, right
}

// contentW is the strip width: the terminal minus the frame and padding
// columns.
func (w *wrapper) contentW() int {
	l, r := w.sideCols()
	return w.termW - l - r
}

// ruleRow is the terminal row of the strip's top rule: the first row past
// the child area.
func (w *wrapper) ruleRow() int { return w.childH + 1 }

// init claims the strip rows and sets the pane palette: the scroll region
// and the column margins end above the strip rows, the frame goes up, the
// OSC palette lands first so the child paints into themed colors, and the
// first snapshot paints immediately.

func (w *wrapper) init() {
	snap, ok := w.readSnap()
	w.mu.Lock()
	defer w.mu.Unlock()
	var b strings.Builder
	b.WriteString(ThemeSetSequence(w.dark))
	// Wipe the screen and home the cursor first: the shell that started the
	// wrapper leaves its greeting and command line behind, and a child that
	// redraws only the cells it changes (claude) would show them through.
	b.WriteString("\x1b[2J\x1b[H")
	b.WriteString(fmt.Sprintf("\x1b[1;%dr", w.childH))
	b.WriteString(w.filter.marginStr())
	w.writeLocked(b.String())
	w.drawFrameLocked()
	if ok {
		w.paintLocked(w.renderSnapLocked(snap))
	}
}

// usagePollEvery is how often a pane asks whether its account's usage cache
// is due for a refresh; the cache's own age, shared by every pane, decides
// whether a request goes out.
const usagePollEvery = 30 * time.Second

// pollUsage keeps the usage cache of this pane's account fresh. The account
// is known only once a transcript names the provider, so it re-reads the key
// every round.
func (w *wrapper) pollUsage(done <-chan struct{}) {
	wait := 3 * time.Second
	for {
		select {
		case <-done:
			return
		case <-time.After(wait):
		}
		key, _ := w.usageKey.Load().(string)
		if key == "" {
			wait = 3 * time.Second // no transcript names the account yet
			continue
		}
		wait = usagePollEvery
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		_ = RefreshUsage(ctx, w.cfg.Source.StateDir, key, time.Now())
		cancel()
	}
}

// readSnap builds the current strip snapshot outside the lock: transcript
// and database IO must never hold it while child output waits to write.
func (w *wrapper) readSnap() (HeaderSnapshot, bool) {
	snap, err := w.snapshot(w.cfg.Source, w.cfg.Settings)
	if err != nil {
		return HeaderSnapshot{}, false
	}
	w.usageKey.Store(snap.UsageKey)
	return snap, true
}

// renderSnapLocked renders a snapshot under the lock, so geometry and
// pulse phase stay consistent with the paint.
func (w *wrapper) renderSnapLocked(snap HeaderSnapshot) string {
	inner := w.contentW() - 2*stripInset
	if inner < 1 {
		return RenderHeader(snap, w.contentW(), HeaderOpts{Rows: w.rows, Dark: w.dark, Profile: w.profile, Pulse: w.phase})
	}
	strip := RenderHeader(snap, inner, HeaderOpts{Rows: w.rows, Dark: w.dark, Profile: w.profile, Pulse: w.phase})
	side := w.edgeStyle().Render(strings.Repeat(" ", stripInset))
	lines := strings.Split(strip, "\n")
	for i, line := range lines {
		lines[i] = side + line + side
	}
	return strings.Join(lines, "\n")
}

// stripInset is the blank strip-colored columns inside each side of the
// strip, so its text does not touch the block's edge.
const stripInset = 2

// edgeStyle is the strip background alone, for blank cells and edge glyphs.
func (w *wrapper) edgeStyle() lipgloss.Style {
	r := lipgloss.NewRenderer(io.Discard, termenv.WithProfile(w.profile))
	r.SetColorProfile(w.profile)
	return r.NewStyle().Background(lipgloss.Color(w.stripBg()))
}

// repaint redraws the strip when the numbers changed.
func (w *wrapper) repaint() {
	snap, ok := w.readSnap()
	if !ok {
		return
	}
	w.reporter.report(snap.State)
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
	w.lastPlain = strings.Split(ansiRe.ReplaceAllString(strip, ""), "\n")
	var b strings.Builder
	b.WriteString("\x1b7\x1b[?6l")
	col, _ := w.sideCols()
	col++
	b.WriteString(fmt.Sprintf("\x1b[%d;%dH%s", w.ruleRow(), col, w.topRule()))
	for i, line := range strings.Split(strip, "\n") {
		b.WriteString(fmt.Sprintf("\x1b[%d;%dH%s", w.ruleRow()+1+i, col, line))
	}
	if w.pad.Bottom >= 1 && !w.rule {
		if row := w.ruleRow() + 1 + len(strings.Split(strip, "\n")); row <= w.termH {
			b.WriteString(fmt.Sprintf("\x1b[%d;%dH%s", row, col, w.bottomEdge()))
		}
	}
	b.WriteString("\x1b8")
	w.writeLocked(b.String())
}

// bottomEdge is the strip's soft lower edge: upper half blocks in the strip
// color, with quadrant glyphs at both ends that leave the outer lower
// corner empty, so the block reads as rounded.
func (w *wrapper) bottomEdge() string {
	return w.roundedEdge("▜", "▀", "▛")
}

// roundedEdge draws one full-width edge row from a left corner, a fill and a
// right corner glyph, colored as the strip background.
func (w *wrapper) roundedEdge(left, fill, right string) string {
	n := w.contentW()
	if n < 2 {
		return ""
	}
	r := lipgloss.NewRenderer(io.Discard, termenv.WithProfile(w.profile))
	r.SetColorProfile(w.profile)
	return r.NewStyle().Foreground(lipgloss.Color(w.stripBg())).Render(left + strings.Repeat(fill, n-2) + right)
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
	return w.roundedEdge("▟", "▄", "▙")
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
	b.WriteString("\x1b7\x1b[?6l")
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
// back to full-screen, the strip rows are cleared, the cursor lands on
// the bottom row, and the pane palette resets to the terminal's own. Every
// return path after the read loop runs through here, so the user's shell
// never inherits a broken region or a recolored terminal.

func (w *wrapper) restore() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	var b strings.Builder
	b.WriteString("\x1b[r")
	if w.filter.marginStr() != "" {
		b.WriteString("\x1b[?6l\x1b[?69l")
	}
	for r := w.ruleRow(); r <= w.termH; r++ {
		b.WriteString(fmt.Sprintf("\x1b[%d;1H\x1b[2K", r))
	}
	b.WriteString(fmt.Sprintf("\x1b[%d;1H", w.termH))
	b.WriteString(ThemeResetSequence())
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
			w.mu.Lock()
			in := w.mouse.translate(buf[:n])
			w.mu.Unlock()
			if len(in) > 0 {
				if _, werr := w.child.Write(in); werr != nil {
					return
				}
			}
		}
		if err != nil {
			return
		}
	}
}

// resizeSettle is the quiet time after the last SIGWINCH before the child
// is resized.
const resizeSettle = 150 * time.Millisecond

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
			// A layout change sends a burst of sizes. Wait for the burst
			// to settle: every size the child sees is a full redraw that
			// lands in scrollback.
			for settled := false; !settled; {
				select {
				case <-ch:
				case <-done:
					return
				case <-time.After(resizeSettle):
					settled = true
				}
			}
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
	if termW == w.termW && termH == w.termH {
		return
	}
	w.termW, w.termH = termW, termH
	w.pad = FitPad(w.termW, w.termH, w.rows, w.frame, Pad{
		Left:   w.cfg.Settings.EffectivePadLeft(),
		Right:  w.cfg.Settings.EffectivePadRight(),
		Bottom: w.cfg.Settings.EffectivePadBottom(),
	})
	w.childW, w.childH = ChildSizePad(w.termW, w.termH, w.rows, w.frame, w.pad)
	w.child.Resize(w.childW, w.childH)
	w.filter.setSize(w.childH, wrapMarginSetupPad(w.termW, w.frame, w.pad))
	w.syncMouseLocked()
	var b strings.Builder
	b.WriteString(fmt.Sprintf("\x1b[1;%dr", w.childH))
	b.WriteString(w.filter.marginStr())
	w.writeLocked(b.String())
	w.clearBelowChildLocked()
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
// that ignores the query costs a timeout, never a hang. It runs before the
// stdin raw-mode switch below, while the tty still answers.
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

// realStart launches the child on a real PTY at the size. Stdin and stdout
// stay on the terminal; a non-nil stderr keeps its own pipe instead of
// merging into the PTY, so the tools log still reads raw stderr under the
// strip. StartWithAttrs only assigns the terminal where the command left
// the stream unset, so a preset stderr survives while stdin and stdout
// take the slave.
func realStart(bin string, args, env []string, w, h int, stderr io.Writer) (wrapPty, func() error, error) {
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), env...)
	if filepath.Base(bin) == "claude" {
		cmd.Env = withoutHerdrClaudeHook(cmd.Env)
	}
	if stderr != nil {
		cmd.Stderr = stderr
	}
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: uint16(h), Cols: uint16(w)})
	if err != nil {
		return nil, nil, err
	}
	return &ptyChild{f: f}, cmd.Wait, nil
}

// themedChildCommand adds the per-run theme launch when the child is the
// harness binary itself (wrap passes the harness through): pig gains its
// theme file in the args, claude its theme reference in the launch settings
// plus two wick-named theme files in its config directory. Any other binary runs
// untouched, and a theme failure runs the child unthemed rather than
// refusing the session.
func themedChildCommand(cfg WrapConfig, dark bool) (string, []string, []string) {
	if cfg.Source.Harness == "" || filepath.Base(cfg.Bin) != cfg.Source.Harness {
		return cfg.Bin, cfg.Args, cfg.Env
	}
	if Registered(cfg.Source.Harness) == nil {
		return cfg.Bin, cfg.Args, cfg.Env
	}
	args, err := ThemeLaunchArgs(cfg.Source.Harness, cfg.Args, dark, cfg.ThemeDir)
	if err != nil {
		stderr := cfg.Stderr
		if stderr == nil {
			stderr = os.Stderr
		}
		fmt.Fprintf(stderr, "wick-pig: theme disabled: %v\n", err)
		return cfg.Bin, cfg.Args, cfg.Env
	}
	env := append(append([]string{}, cfg.Env...), mustThemeEnv(cfg, dark)...)
	return cfg.Bin, args, env
}

// mustThemeEnv resolves the extra theme environment, logging and falling
// back to none when the theme files cannot be written.
func mustThemeEnv(cfg WrapConfig, dark bool) []string {
	env, err := ThemeLaunchEnv(cfg.Source.Harness, dark, cfg.ThemeDir)
	if err != nil {
		stderr := cfg.Stderr
		if stderr == nil {
			stderr = os.Stderr
		}
		fmt.Fprintf(stderr, "wick-pig: theme env disabled: %v\n", err)
		return nil
	}
	return env
}

// watchWrapSignals resets the pane palette on SIGINT, SIGTERM and SIGHUP,
// then re-raises through resignal so the process still exits with the
// signal. The reset travels the locked writer, so it never interleaves a
// strip repaint. A test feeds its own channel; production passes the real
// signal channel.
func watchWrapSignals(ch <-chan os.Signal, done <-chan struct{}, reset func(), resignal func(os.Signal)) {
	for {
		select {
		case <-done:
			return
		case s, ok := <-ch:
			if !ok {
				return
			}
			reset()
			resignal(s)
			return
		}
	}
}

// resignalSelf restores the default disposition and delivers the signal to
// this process again: the exit still reads as the signal, after the palette
// reset above already landed.
func resignalSelf(s os.Signal, ch chan os.Signal) {
	signal.Stop(ch)
	signal.Reset(s)
	if p, err := os.FindProcess(os.Getpid()); err == nil {
		_ = p.Signal(s)
	}
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

// clearBelowChildLocked blanks every row under the child screen, padding
// included, so a resize leaves no ghost of the strip at its old rows or
// columns. The strip repaints right after.
func (w *wrapper) clearBelowChildLocked() {
	var b strings.Builder
	b.WriteString("\x1b7\x1b[?6l")
	for r := w.childH + 1; r <= w.termH; r++ {
		b.WriteString(fmt.Sprintf("\x1b[%d;1H\x1b[2K", r))
	}
	b.WriteString("\x1b8")
	w.writeLocked(b.String())
}

// withoutHerdrClaudeHook drops HERDR_ENV from a wrapped claude's env. Herdr's
// own claude hook runs only when HERDR_ENV=1 and, on SessionStart, takes the
// pane's agent session. Once taken, herdr silently refuses every later
// report-agent from the wrapper, which is the only way herdr can see a
// wrapped claude (the foreground process is wick-pig), so the pane freezes
// at its first state and then shows no agent. The wrapper reports instead.
func withoutHerdrClaudeHook(env []string) []string {
	out := make([]string, 0, len(env))
	for _, e := range env {
		if strings.HasPrefix(e, "HERDR_ENV=") {
			continue
		}
		out = append(out, e)
	}
	return out
}

// syncMouseLocked tells the mouse mapper where the child screen sits.
func (w *wrapper) syncMouseLocked() {
	off := w.pad.Left
	if w.frame {
		off++
	}
	w.mouse.set(off, w.childW, w.childH)
	w.mouse.onStrip = w.copyStripSelection
}

// copyStripSelection copies the strip text between two terminal cells to the
// clipboard through OSC 52, the way claude and pig copy their own selections.
// A child that captures the mouse never lets the terminal select the strip,
// so a drag over it is handled here. It runs inside translate, which holds
// w.mu.
func (w *wrapper) copyStripSelection(x1, y1, x2, y2 int) {
	if y2 < y1 || (y2 == y1 && x2 < x1) {
		x1, y1, x2, y2 = x2, y2, x1, y1
	}
	col, _ := w.sideCols()
	var lines []string
	for y := y1; y <= y2; y++ {
		i := y - w.ruleRow() - 1
		if i < 0 || i >= len(w.lastPlain) {
			continue
		}
		cells := []rune(w.lastPlain[i])
		from, to := 0, len(cells)
		if y == y1 {
			from = x1 - col - 1
		}
		if y == y2 {
			to = x2 - col
		}
		from, to = min(max(from, 0), len(cells)), min(max(to, 0), len(cells))
		if from < to {
			lines = append(lines, strings.TrimRight(string(cells[from:to]), " "))
		}
	}
	text := strings.Join(lines, "\n")
	if strings.TrimSpace(text) == "" {
		return
	}
	w.writeLocked("\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(text)) + "\x07")
}
