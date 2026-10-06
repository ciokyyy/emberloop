package wick

// The strip's activity label: the tool an agent is running now, or the one
// it just finished, in a few words with a glyph per tool. wick's own tools
// arrive as mcp__wick__read and the like; the label drops that plumbing and
// says what the agent is doing: `▤ reading header.go`, `⑂ spawning scout`.

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

// toolStyle is how one tool reads: its glyph, the verb while it runs and the
// verb once it is done.
type toolStyle struct{ icon, running, done string }

var toolStyles = map[string]toolStyle{
	"read":       {"▤", "reading", "read"},
	"edit":       {"✎", "editing", "edited"},
	"write":      {"✎", "writing", "wrote"},
	"bash":       {"❯", "running", "ran"},
	"grep":       {"⌕", "searching", "searched"},
	"find":       {"⌕", "finding", "found"},
	"ctx_search": {"⌕", "searching output", "searched output"},
	"websearch":  {"◎", "searching the web", "searched the web"},
	"spawn":      {"⑂", "spawning", "spawned"},
	"git":        {"⎇", "git", "git"},
	"check":      {"✓", "checking", "checked"},
	"execute":    {"λ", "executing", "executed"},
	"submit":     {"⇧", "submitting", "submitted"},
	"background": {"◌", "background", "background"},
	"Read":       {"▤", "reading", "read"},
	"Edit":       {"✎", "editing", "edited"},
	"Write":      {"✎", "writing", "wrote"},
	"Bash":       {"❯", "running", "ran"},
	"Grep":       {"⌕", "searching", "searched"},
	"Glob":       {"⌕", "finding", "found"},
}

// bareToolName drops the mcp__<server>__ prefix a harness puts on MCP tools.
func bareToolName(name string) string {
	if strings.HasPrefix(name, "mcp__") {
		if i := strings.LastIndex(name, "__"); i > len("mcp__")-1 {
			return name[i+2:]
		}
	}
	return name
}

// shortArg trims a tool argument to fit a strip: a path keeps its last two
// parts, anything else its first line, all cut to max runes with an ellipsis.
func shortArg(arg string, max int) string {
	arg = strings.TrimSpace(arg)
	if i := strings.IndexByte(arg, '\n'); i >= 0 {
		arg = arg[:i]
	}
	if strings.HasPrefix(arg, "/") || strings.HasPrefix(arg, "~") || strings.HasPrefix(arg, "./") {
		parts := strings.Split(strings.TrimRight(arg, "/"), "/")
		if len(parts) > 2 {
			arg = strings.Join(parts[len(parts)-2:], "/")
		}
	}
	if max < 1 {
		return ""
	}
	if utf8.RuneCountInString(arg) > max {
		r := []rune(arg)
		arg = string(r[:max-1]) + "…"
	}
	return arg
}

// toolLabel is the glyph and words for one call: the glyph, then the verb,
// then the key argument. Unknown tools keep their bare name.
func toolLabel(c ToolCall, running bool, argMax int) (icon, verb, arg string) {
	name := bareToolName(c.Name)
	st, ok := toolStyles[name]
	if !ok {
		st = toolStyle{"▸", name, name}
	}
	verb = st.done
	if running {
		verb = st.running
	}
	return st.icon, verb, shortArg(c.Arg, argMax)
}

// activityCell is the left of the strip's second row. A pending warning,
// queued compress or running compress wins. Otherwise: the tool running now
// (bright), else while the turn is still going the last finished tool with
// its result size (dim), else `thinking`. An idle agent shows nothing.
func (h *headerCtx) activityCell(max int) cell {
	switch {
	case h.snap.Warning != "":
		return h.fit(h.pill(), max)
	case h.snap.State == HeaderQueued, h.snap.State == HeaderCompressing:
		return h.fit(h.pill(), max)
	case h.snap.State == HeaderIdle:
		if h.snap.Pending > 0 {
			return h.fit(cell{text: strconv.Itoa(h.snap.Pending) + " pending", hex: h.idle}, max)
		}
		return cell{}
	}
	if h.snap.Running.Name != "" {
		icon, verb, arg := toolLabel(h.snap.Running, true, max)
		return h.activity(icon, verb, arg, "", true, max)
	}
	if h.snap.Last.Name != "" {
		icon, verb, arg := toolLabel(h.snap.Last, false, max)
		size := ""
		if h.snap.Last.Bytes > 0 {
			size = HumanizeTokens(h.snap.Last.Bytes)
		}
		return h.activity(icon, verb, arg, size, false, max)
	}
	return h.activity("◐", "thinking", "", "", false, max)
}

// activity styles one label and cuts its argument so the whole fits max
// columns.
func (h *headerCtx) activity(icon, verb, arg, size string, live bool, max int) cell {
	fixed := len([]rune(icon)) + 1 + len([]rune(verb))
	if size != "" {
		fixed += 3 + len(size)
	}
	if h.snap.Pending > 0 {
		fixed += 3 + len(strconv.Itoa(h.snap.Pending)) + len(" pending")
	}
	if arg != "" {
		room := max - fixed - 1
		if room < 4 {
			arg = ""
		} else {
			arg = shortArg(arg, room)
		}
	}
	iconHex, verbHex, argHex := h.dim, h.dim, h.dim
	if live {
		iconHex, verbHex, argHex = h.hue, h.text, h.tool
	}
	out := h.style(cell{text: icon, hex: iconHex, bold: live}) + h.fill(" ") + h.style(cell{text: verb, hex: verbHex, bold: live})
	if arg != "" {
		out += h.fill(" ") + h.style(cell{text: arg, hex: argHex})
	}
	if size != "" {
		out += h.fill(" ") + h.style(cell{text: "· " + size, hex: h.dim})
	}
	if h.snap.Pending > 0 {
		out += h.fill(" ") + h.style(cell{text: "· " + strconv.Itoa(h.snap.Pending) + " pending", hex: h.dim})
	}
	return cell{text: out, raw: true}
}

// fit cuts a plain cell to max columns.
func (h *headerCtx) fit(c cell, max int) cell {
	if max > 0 && utf8.RuneCountInString(c.text) > max {
		r := []rune(c.text)
		c.text = string(r[:max-1]) + "…"
	}
	return c
}

// splitRow lays a row out as a left cell and a right cell with the gap
// between them on the strip background.
func (h *headerCtx) splitRow(width int, left, right cell) string {
	gap := width - lipgloss.Width(left.text) - lipgloss.Width(right.text)
	if gap < 1 {
		gap = 1
	}
	var l, r string
	if left.text != "" {
		l = h.style(left)
	}
	if right.text != "" {
		r = h.style(right)
	}
	return h.styled(width, l+h.fill(strings.Repeat(" ", gap))+r)
}
