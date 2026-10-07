package wick

// Header strip (ticket 04): every agent pane shows its context use against
// the window with a gradient bar, an estimated cost, the top tools, the
// model, the compress count and the state. This file holds the pure render
// function: a snapshot and a width in, a styled string out. The PTY wrapper
// that draws it around a live child lives in header_wrap.go; the mockup in
// .scratch/context-compress/mockup.html is the visual reference.

import (
	"fmt"
	"hash/fnv"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"example.com/wick/tools"
)

// HeaderState is the agent state the strip's pill shows.
type HeaderState string

const (
	HeaderWorking        HeaderState = "working"
	HeaderIdle           HeaderState = "idle"
	HeaderQueued         HeaderState = "queued"
	HeaderCompressing    HeaderState = "compressing"
	HeaderCompressFailed HeaderState = "compress failed"
)

// Header tiers: a wide pane shows both rows, a medium pane drops the tools
// row, a narrow pane keeps only the percent and the cost.
const (
	headerTierWide   = "wide"
	headerTierMedium = "medium"
	headerTierNarrow = "narrow"
)

// WideTierMinWidth is the pane width from which the strip shows both rows;
// MediumTierMinWidth is where it keeps one full row instead of the narrow
// percent-and-cost line. Below either, or when the numbers do not fit, the
// layout degrades one step rather than cutting a number in half.
const (
	WideTierMinWidth   = 90
	MediumTierMinWidth = 55
)

// DefaultStripRows is the strip height in rows when no setting says
// otherwise: the mockup's two rows.
const DefaultStripRows = 2

// HeaderSnapshot is everything the strip draws for one agent.
type HeaderSnapshot struct {
	AgentID        string // stable identity: the hue hash input
	AgentType      string // empty for the top-level session: always mauve
	AgentName      string // the chip: the agent type name, as in the pane name
	Model          string
	ContextTokens  int64
	Window         int64
	Cost           float64 // dashboard-reported sum, rendered as an estimate
	HasCost        bool
	TopTools       []ToolSize
	CompressCount  int
	Pending        int  // prompts waiting in the wrapper's queue: hidden at 0
	Raw            bool // raw passthrough mode: the keyboard goes to the child
	State          HeaderState
	CompressFailed string    // non-empty shows "compress failed: <reason>" in the pill
	Warning        string    // non-empty replaces the state pill, e.g. no compressModel
	Usage          UsageSet  // the account's usage windows; Has false hides them
	UsageKey       string    // which account Usage is; empty for none
	Running        ToolCall  // the tool running now, empty when none
	Last           ToolCall  // the last finished tool, with its result size
	Now            time.Time // the clock the reset countdowns read
}

// HeaderOpts tunes the render: Rows caps the strip height (0 means the
// default two), Dark picks the Mocha or Latte palette, Profile drives Lip
// Gloss's downgrade to 256 and 16 colors. Symbols picks the glyph set
// (nerd, unicode default, ascii) and Separator the between-segment
// separator (powerline, powerline-thin, rounded, slash, pipe, none
// default); unknown values read as the defaults.
type HeaderOpts struct {
	Rows      int
	Dark      bool
	Profile   termenv.Profile
	Pulse     bool // pulse phase: the compressing pill rests on idle
	Symbols   string
	Separator string
}

// headerColor is one adaptive color: the Mocha hex for dark terminals, the
// Latte hex for light ones.
type headerColor struct {
	dark, light string
}

// The strip palette reads the shared theme source (theme.go): Catppuccin
// Mocha and Latte, one definition for the strip, the generators and the
// OSC setter. Threshold colors stay green, yellow and red; no agent hue
// uses them.
var (
	headerStripBg = headerColor{WickMocha().StripBg, WickLatte().StripBg}
	headerText    = headerColor{WickMocha().Text, WickLatte().Text}
	headerDim     = headerColor{WickMocha().Dim, WickLatte().Dim}
	headerAccent  = headerColor{WickMocha().Accent, WickLatte().Accent}
	headerCalm    = headerColor{WickMocha().Calm, WickLatte().Calm}
	headerWarm    = headerColor{WickMocha().Warm, WickLatte().Warm}
	headerHot     = headerColor{WickMocha().Hot, WickLatte().Hot}
	headerCost    = headerColor{WickMocha().Cost, WickLatte().Cost}
	headerTool    = headerColor{WickMocha().Tool, WickLatte().Tool}
	headerIdle    = headerColor{WickMocha().Idle, WickLatte().Idle}
	headerBarDim  = headerColor{WickMocha().BarDim, WickLatte().BarDim}
)

// agentHues is the fixed identity set from the shared palette: mauve first
// for the orchestrator, then one hue per other agent by hash. Green, yellow
// and red stay out, so identity never reads as status.
var agentHues = []headerColor{
	{WickMocha().Hues[0].Dark, WickMocha().Hues[0].Light}, // mauve: the orchestrator
	{WickMocha().Hues[1].Dark, WickMocha().Hues[1].Light}, // pink
	{WickMocha().Hues[2].Dark, WickMocha().Hues[2].Light}, // lavender
	{WickMocha().Hues[3].Dark, WickMocha().Hues[3].Light}, // sapphire
	{WickMocha().Hues[4].Dark, WickMocha().Hues[4].Light}, // sky
	{WickMocha().Hues[5].Dark, WickMocha().Hues[5].Light}, // flamingo
	{WickMocha().Hues[6].Dark, WickMocha().Hues[6].Light}, // maroon
}

// AgentHue picks an agent's stable hue from the identity set: the hash of
// the agent ID, so it survives restarts and compressions. The orchestrator
// (and the top-level session, which has no type) is always mauve.
func AgentHue(agentID, agentType string) (dark, light string) {
	c := agentHueColor(agentID, agentType)
	return c.dark, c.light
}

func agentHueColor(agentID, agentType string) headerColor {
	if agentType == "" || agentType == "orchestrator" {
		return agentHues[0]
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(agentID))
	return agentHues[1+int(h.Sum32()%uint32(len(agentHues)-1))]
}

// ThresholdClass is the bar color for a context percent: calm below 60%,
// warm from 60 to 80%, hot above.
func ThresholdClass(pct int64) string {
	if pct < 60 {
		return "calm"
	}
	if pct < 80 {
		return "warm"
	}
	return "hot"
}

// HumanizeTokens rounds a token or byte count for the strip: 142381 reads
// 142k. Never a raw float, never more precise than the strip needs.
func HumanizeTokens(n int64) string {
	if n < 1000 {
		return strconv.FormatInt(n, 10)
	}
	return strconv.FormatInt((n+500)/1000, 10) + "k"
}

// HeaderTier is the layout tier for a pane width: wide keeps both rows,
// medium drops the tools row, narrow keeps only percent and cost.
func HeaderTier(width int) string {
	if width >= WideTierMinWidth {
		return headerTierWide
	}
	if width >= MediumTierMinWidth {
		return headerTierMedium
	}
	return headerTierNarrow
}

// headerCtx is the resolved render state: the palette hexes for the theme,
// the renderer for the profile, and the snapshot numbers. Segment colors
// map to named palette tokens: model reads the accent, the used-against-
// window numbers the text, cost the cost token, tools the tool token, the
// ctx label the agent hue, the state the threshold or idle token, and the
// separator the dim token — all from the one palette source, so a theme
// change recolors the strip with claude, pig and the pane.
type headerCtx struct {
	r        *lipgloss.Renderer
	bg       string
	text     string
	dim      string
	accent   string
	calm     string
	warm     string
	hot      string
	cost     string
	tool     string
	idle     string
	barDim   string
	hue      string
	pulse    bool
	pct      int64
	pctHex   string
	snap     HeaderSnapshot
	symbols  string
	sepGlyph string
	seph     string
}

func pick(c headerColor, dark bool) string {
	if dark {
		return c.dark
	}
	return c.light
}

// stripSymbols normalizes a glyph set name: nerd, unicode or ascii,
// defaulting to unicode so a terminal without a Nerd Font looks right.
func stripSymbols(v string) string {
	switch tools.NormalizeStripSymbols(v) {
	case tools.StripSymbolsNerd, tools.StripSymbolsUnicode, tools.StripSymbolsASCII:
		return tools.NormalizeStripSymbols(v)
	}
	return tools.StripSymbolsUnicode
}

// stripSeparator normalizes a separator name, defaulting to none (the
// current spaced look).
func stripSeparator(v string) string {
	switch tools.NormalizeStripSeparator(v) {
	case tools.StripSeparatorPowerline, tools.StripSeparatorPowerlineThin, tools.StripSeparatorRounded, tools.StripSeparatorSlash, tools.StripSeparatorPipe, tools.StripSeparatorNone:
		return tools.NormalizeStripSeparator(v)
	}
	return tools.StripSeparatorNone
}

// gapWidth is the cell width of the between-segment gap on bar rows: the
// current double space when no separator is set, the styled glyph with a
// space each side otherwise.
func (h *headerCtx) gapWidth() int {
	if h.sepGlyph == "" {
		return 2
	}
	return 2 + lipgloss.Width(h.sepGlyph)
}

// ngapWidth is the gap width on narrow rows, which join with one space
// today instead of two.
func (h *headerCtx) ngapWidth() int {
	if h.sepGlyph == "" {
		return 1
	}
	return 2 + lipgloss.Width(h.sepGlyph)
}

// gap joins bar-row cells: the current double space, or the separator
// glyph in the separator token color with a space each side.
func (h *headerCtx) gap() string {
	if h.sepGlyph == "" {
		return h.fill("  ")
	}
	return h.fill(" ") + h.style(cell{text: h.sepGlyph, hex: h.seph}) + h.fill(" ")
}

// ngap joins narrow-row cells: the current single space, or the separator
// glyph with a space each side.
func (h *headerCtx) ngap() string {
	if h.sepGlyph == "" {
		return h.fill(" ")
	}
	return h.fill(" ") + h.style(cell{text: h.sepGlyph, hex: h.seph}) + h.fill(" ")
}

// RenderHeader draws the strip: the snapshot at the width, in at most Rows
// rows (0 means the default two). Narrow panes drop the second row first;
// Rows 1 forces the single-row layout on any width. The bar flexes to the
// room the numbers leave; when even the minimum bar leaves no room, the
// layout degrades one tier rather than cutting a number in half. The output
// is one or two full-width lines on the strip background.
func RenderHeader(snap HeaderSnapshot, width int, opts HeaderOpts) string {
	if width < 1 {
		width = 1
	}
	rows := opts.Rows
	if rows <= 0 {
		rows = DefaultStripRows
	}
	profile := opts.Profile
	if profile == termenv.Ascii {
		profile = termenv.ANSI
	}
	r := lipgloss.NewRenderer(io.Discard, termenv.WithProfile(profile))
	r.SetColorProfile(profile)
	h := &headerCtx{
		r:        r,
		bg:       pick(headerStripBg, opts.Dark),
		text:     pick(headerText, opts.Dark),
		dim:      pick(headerDim, opts.Dark),
		accent:   pick(headerAccent, opts.Dark),
		calm:     pick(headerCalm, opts.Dark),
		warm:     pick(headerWarm, opts.Dark),
		hot:      pick(headerHot, opts.Dark),
		cost:     pick(headerCost, opts.Dark),
		tool:     pick(headerTool, opts.Dark),
		idle:     pick(headerIdle, opts.Dark),
		barDim:   pick(headerBarDim, opts.Dark),
		hue:      pick(agentHueColor(snap.AgentID, snap.AgentType), opts.Dark),
		pulse:    opts.Pulse,
		snap:     snap,
		symbols:  stripSymbols(opts.Symbols),
		sepGlyph: StripSeparatorGlyph(stripSymbols(opts.Symbols), stripSeparator(opts.Separator)),
		seph:     pick(headerDim, opts.Dark),
	}
	if snap.Window > 0 {
		h.pct = snap.ContextTokens * 100 / snap.Window
	}
	switch ThresholdClass(h.pct) {
	case "calm":
		h.pctHex = h.calm
	case "warm":
		h.pctHex = h.warm
	default:
		h.pctHex = h.hot
	}
	tier := HeaderTier(width)
	if tier == headerTierWide && rows < 2 {
		tier = headerTierMedium
	}
	if tier == headerTierWide {
		if first, ok := h.barRow(width, h.wideSegs(), nil, 24, 8); ok {
			return first + "\n" + h.secondRow(width)
		}
		tier = headerTierMedium
	}
	if tier == headerTierMedium {
		pill := h.pill()
		if first, ok := h.barRow(width, h.mediumSegs(), &pill, 16, 6); ok {
			return first
		}
	}
	return h.narrowRow(width)
}

// cell is one styled span of a strip row.
type cell struct {
	text string
	hex  string
	bold bool
	raw  bool // text is already styled
}

// wideSegs is the mockup's row 1 without the bar: chip, ctx label, percent,
// used against the window, cost, model and compress count. The pill lives on
// row 2 at this tier.
func (h *headerCtx) wideSegs() []cell {
	segs := h.mediumSegs()
	if h.snap.CompressCount > 0 {
		segs = append(segs, cell{text: stripIcon(h.symbols, "compress", h.compressText()), hex: h.dim})
	}
	return segs
}

// compressText is the compress count label: the ascii preset spells the
// times sign with an x, so the preset stays pure ASCII.
func (h *headerCtx) compressText() string {
	if h.symbols == tools.StripSymbolsASCII {
		return fmt.Sprintf("compressed x%d", h.snap.CompressCount)
	}
	return fmt.Sprintf("compressed ×%d", h.snap.CompressCount)
}

// mediumSegs is row 1 without the bar and without the compress count: the
// count needs the most room and goes first when space runs short.
func (h *headerCtx) mediumSegs() []cell {
	segs := []cell{h.chip(), {text: stripIcon(h.symbols, "ctx", "ctx"), hex: h.hue, bold: true}}
	segs = append(segs, cell{text: fmt.Sprintf("%d%%", h.pct), hex: h.pctHex, bold: true})
	used := HumanizeTokens(h.snap.ContextTokens)
	if h.snap.Window > 0 {
		used += " / " + HumanizeTokens(h.snap.Window)
	}
	segs = append(segs, cell{text: stripIcon(h.symbols, "tokens", used), hex: h.text})
	if h.snap.HasCost && h.snap.Cost > 0 {
		segs = append(segs, cell{text: stripIcon(h.symbols, "cost", fmt.Sprintf("~$%.2f", h.snap.Cost)), hex: h.cost})
	}
	if h.snap.Model != "" {
		segs = append(segs, cell{text: stripIcon(h.symbols, "model", h.snap.Model), hex: h.accent})
	}
	return segs
}

// barRow builds a row with the gradient bar past the chip and ctx label.
// The bar takes the room the numbers leave, up to maxBar and down to
// minBar; pill pins to the right edge when set. It reports false when even
// the minimum bar leaves no room, so the caller degrades one tier.
func (h *headerCtx) barRow(width int, segs []cell, pill *cell, maxBar, minBar int) (string, bool) {
	fixed := 0
	for _, s := range segs {
		fixed += lipgloss.Width(s.text)
	}
	pw := 0
	if pill != nil {
		pw = lipgloss.Width(pill.text) + 1
	}
	bar := width - fixed - h.gapWidth()*len(segs) - pw - 1
	if bar > maxBar {
		bar = maxBar
	}
	if bar < minBar {
		return "", false
	}
	cells := []cell{segs[0], segs[1], {text: h.bar(bar), hex: h.text}}
	cells = append(cells, segs[2:]...)
	if pill != nil {
		cells = append(cells, *pill)
		return h.row(width, cells), true
	}
	return h.styled(width, h.join(cells)), true
}

// secondRow is the strip's second line: what the agent is doing on the left
// (the tool running now, or the last one finished), the account usage pinned
// right. An idle agent shows no state word; compress and warnings still do.
func (h *headerCtx) secondRow(width int) string {
	right := h.usageCell()
	room := width
	if right.text != "" {
		room = width - lipgloss.Width(right.text) - 3
	}
	if room < 1 {
		room, right = width, cell{}
	}
	return h.splitRow(width, h.activityCell(room), right)
}

// narrowRow is the narrow tier: percent and cost beside the state, with the
// chip when it fits. Segments drop whole in a fixed order — chip and ctx
// label first, then the cost — and the percent and the state always remain,
// so a glyph is never split and the row never overflows.
func (h *headerCtx) narrowRow(width int) string {
	pill := h.pill()
	pct := cell{text: fmt.Sprintf("%d%%", h.pct), hex: h.pctHex, bold: true}
	cost := cell{text: stripIcon(h.symbols, "cost", h.costText()), hex: h.cost}
	full := []cell{h.chip(), {text: stripIcon(h.symbols, "ctx", "ctx"), hex: h.hue, bold: true}, pct}
	if h.sepGlyph == "" {
		dot := "·"
		if h.symbols == tools.StripSymbolsASCII {
			dot = "-"
		}
		full = append(full, cell{text: dot, hex: h.dim})
	}
	if h.snap.HasCost && h.snap.Cost > 0 {
		full = append(full, cost)
	}
	if h.fits(width, full, pill) {
		return h.rowSep(width, append(full, pill), h.ngap())
	}
	short := []cell{pct}
	if h.snap.HasCost && h.snap.Cost > 0 {
		short = append(short, cost)
		if h.fits(width, short, pill) {
			return h.rowSep(width, append(short, pill), h.ngap())
		}
		short = short[:1]
	}
	if h.fits(width, short, pill) {
		return h.rowSep(width, append(short, pill), h.ngap())
	}
	return h.rowSep(width, []cell{pill}, h.ngap())
}

func (h *headerCtx) costText() string {
	return fmt.Sprintf("~$%.2f", h.snap.Cost)
}

func (h *headerCtx) fits(width int, cells []cell, pill cell) bool {
	w := lipgloss.Width(pill.text)
	for _, c := range cells {
		w += lipgloss.Width(c.text) + h.ngapWidth()
	}
	return w <= width
}

// row joins cells with the between-segment gap, pins the last cell (the
// state pill) to the right edge, and pads the row to the full width on the
// strip background, so the strip reads as a bar without a border line.
// Narrow rows join with the narrow gap instead; both read as the current
// spacing when no separator is set.
func (h *headerCtx) row(width int, cells []cell) string {
	return h.rowSep(width, cells, h.gap())
}

func (h *headerCtx) rowSep(width int, cells []cell, sep string) string {
	if len(cells) == 0 {
		return h.styled(width, "")
	}
	body := cells[:len(cells)-1]
	last := cells[len(cells)-1]
	parts := make([]string, 0, len(body))
	for _, c := range body {
		parts = append(parts, h.style(c))
	}
	left := strings.Join(parts, h.fill(sep))
	gap := width - lipgloss.Width(left) - lipgloss.Width(last.text)
	if gap < 1 {
		gap = 1
	}
	return h.styled(width, left+h.fill(strings.Repeat(" ", gap))+h.style(last))
}

func (h *headerCtx) join(cells []cell) string {
	parts := make([]string, 0, len(cells))
	for _, c := range cells {
		parts = append(parts, h.style(c))
	}
	return strings.Join(parts, h.gap())
}

// fill is blank space on the strip background. Every cell resets its own
// colors, so the gaps between cells must carry the background themselves or
// the strip shows through patchy.
func (h *headerCtx) fill(blank string) string {
	return h.r.NewStyle().Background(lipgloss.Color(h.bg)).Render(blank)
}

func (h *headerCtx) style(c cell) string {
	if c.raw {
		return c.text
	}
	s := h.r.NewStyle().Foreground(lipgloss.Color(c.hex)).Background(lipgloss.Color(h.bg))
	if c.bold {
		s = s.Bold(true)
	}
	return s.Render(c.text)
}

func (h *headerCtx) styled(width int, content string) string {
	return h.r.NewStyle().Background(lipgloss.Color(h.bg)).Foreground(lipgloss.Color(h.text)).Width(width).Render(content)
}

func (h *headerCtx) chip() cell {
	name := h.snap.AgentName
	if name == "" {
		name = "orchestrator"
	}
	if left, right := StripCaps(h.symbols); left != "" {
		return cell{text: left + StripNerdIcons["agent"] + " " + name + " " + right, hex: h.hue, bold: true}
	}
	return cell{text: name, hex: h.hue, bold: true}
}

// pill is the state field: working, idle, compress queued, compressing, raw,
// or the warning. A warning always wins over the state, never over the queue
// count: a waiting input queue appends `N pending` to either, so the strip
// shows it until the queue drains. In raw mode the pill reads `raw`, or
// `raw, N queued` while items wait, except compress states win: a queued or
// running compress keeps its own pill so the reason stays visible. Nerd wraps
// the icon and label in rounded caps; unicode keeps the plain symbols; ascii
// is labels only.
func (h *headerCtx) pill() cell {
	icon, label, hex := h.pillParts()
	rawMode := h.snap.Raw && h.snap.Warning == "" &&
		h.snap.State != HeaderQueued && h.snap.State != HeaderCompressing &&
		h.snap.State != HeaderCompressFailed
	if rawMode {
		label = "raw"
		icon = "raw"
		hex = h.idle
		if h.snap.Pending > 0 {
			label = "raw, " + strconv.Itoa(h.snap.Pending) + " queued"
		}
	} else if h.snap.Pending > 0 {
		dot := "·"
		if h.symbols == tools.StripSymbolsASCII {
			dot = "-"
		}
		label += " " + dot + " " + strconv.Itoa(h.snap.Pending) + " pending"
	}
	if h.symbols == tools.StripSymbolsNerd {
		left, right := StripCaps(h.symbols)
		return cell{text: left + StripNerdIcons[icon] + " " + label + " " + right, hex: hex}
	}
	if h.symbols == tools.StripSymbolsASCII {
		label = strings.ReplaceAll(label, "…", "...")
		var b strings.Builder
		for _, r := range label {
			if r <= 127 {
				b.WriteRune(r)
			} else {
				b.WriteRune('?')
			}
		}
		return cell{text: b.String(), hex: hex}
	}
	prefix := map[string]string{"working": "● ", "idle": "◌ ", "queued": "⏳ ", "sync": "⟳ ", "warning": "⚠ ", "raw": "⌨ "}[icon]
	return cell{text: prefix + label, hex: hex}
}

// pillParts is the state content without preset dressing: the nerd icon
// name, the plain label and the palette token color.
func (h *headerCtx) pillParts() (icon, label, hex string) {
	if h.snap.Warning != "" {
		return "warning", h.snap.Warning, h.hot
	}
	switch h.snap.State {
	case HeaderIdle:
		return "idle", "idle", h.idle
	case HeaderQueued:
		return "queued", "compress queued", h.idle
	case HeaderCompressing:
		if h.pulse {
			return "sync", "compressing…", h.idle
		}
		return "sync", "compressing…", h.accent
	case HeaderCompressFailed:
		text := "compress failed"
		if h.snap.CompressFailed != "" {
			text = "compress failed: " + h.snap.CompressFailed
		}
		return "warning", text, h.hot
	default:
		return "working", "working", h.pctHex
	}
}

// bar draws the gradient bar: each filled cell blends calm to warm to hot
// by its position; unfilled cells use the dim color. The percent, not the
// bar, carries the threshold state. The ascii preset draws with number
// signs and dashes, so the preset stays pure ASCII.
func (h *headerCtx) bar(n int) string {
	fillGlyph, emptyGlyph := "▰", "▱"
	if h.symbols == tools.StripSymbolsASCII {
		fillGlyph, emptyGlyph = "#", "-"
	}
	filled := 0
	if h.pct > 0 {
		filled = int((int64(n)*h.pct + 50) / 100)
		if filled < 1 {
			filled = 1
		}
		if filled > n {
			filled = n
		}
	}
	var b strings.Builder
	for i := 0; i < n; i++ {
		hex := h.barDim
		glyph := emptyGlyph
		if i < filled {
			hex = gradientCell(h.calm, h.warm, h.hot, i, n)
			glyph = fillGlyph
		}
		b.WriteString(h.r.NewStyle().Foreground(lipgloss.Color(hex)).Background(lipgloss.Color(h.bg)).Render(glyph))
	}
	return b.String()
}

// gradientCell blends calm to warm to hot by the cell's position in the bar.
func gradientCell(calm, warm, hot string, i, n int) string {
	t := 0.0
	if n > 1 {
		t = float64(i) / float64(n-1)
	}
	if t < 0.6 {
		return mixHex(calm, warm, t/0.6)
	}
	return mixHex(warm, hot, (t-0.6)/0.4)
}

func mixHex(a, b string, t float64) string {
	ar, ag, ab := hexParts(a)
	br, bg, bb := hexParts(b)
	return fmt.Sprintf("#%02x%02x%02x",
		roundFloat(lerp(float64(ar), float64(br), t)),
		roundFloat(lerp(float64(ag), float64(bg), t)),
		roundFloat(lerp(float64(ab), float64(bb), t)))
}

func hexParts(h string) (int, int, int) {
	h = strings.TrimPrefix(h, "#")
	if len(h) != 6 {
		return 0, 0, 0
	}
	r, _ := strconv.ParseInt(h[0:2], 16, 0)
	g, _ := strconv.ParseInt(h[2:4], 16, 0)
	b, _ := strconv.ParseInt(h[4:6], 16, 0)
	return int(r), int(g), int(b)
}

func lerp(a, b, t float64) float64 { return a + (b-a)*t }

func roundFloat(f float64) int { return int(f + 0.5) }

// usageCell is the account usage, every window in one cell:
// `5h ◔ 37% ↻1h40m   wk ◔ 14% ↻4d2h`. The pie fills with the percent and
// takes the threshold color of the context percent; the reset countdown
// stays dim. Empty when the account has reported nothing.
func (h *headerCtx) usageCell() cell {
	u := h.snap.Usage
	if !u.Has {
		return cell{}
	}
	now := h.snap.Now
	if now.IsZero() {
		now = time.Now()
	}
	var parts []string
	for _, w := range u.Windows {
		hex := h.calm
		switch ThresholdClass(int64(w.Pct)) {
		case "warm":
			hex = h.warm
		case "hot":
			hex = h.hot
		}
		pie := usagePie(w.Pct)
		resetPrefix := "↻"
		if h.symbols == tools.StripSymbolsASCII {
			pie = "*"
			resetPrefix = "~"
		}
		out := h.style(cell{text: w.Label, hex: h.dim}) + h.fill(" ") +
			h.style(cell{text: pie, hex: hex}) + h.fill(" ") +
			h.style(cell{text: fmt.Sprintf("%d%%", w.Pct), hex: hex, bold: true})
		if r := usageResetIn(w.ResetsAt, now); r != "" {
			out += h.fill(" ") + h.style(cell{text: resetPrefix + r, hex: h.dim})
		}
		parts = append(parts, out)
	}
	return cell{text: strings.Join(parts, h.fill("   ")), raw: true}
}
