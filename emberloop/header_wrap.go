package emberloop

// Header wrapper geometry, child-output filter and snapshot builder
// (ticket 04). The PTY runner that draws the strip around a live child is
// further below: it sizes the child with ChildSize, passes the child's
// output through FilterChildOutput, and repaints snapshots from
// BuildHeaderSnapshot.

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"example.com/emberloop/statedb"
	"example.com/emberloop/tools"
)

// StripFootprint is the terminal rows the strip costs above its content
// rows: one agent-hue top rule. A two-row strip occupies three rows; the
// rule reads as the frame's top edge, so the mockup's two content rows
// stay exact.
const StripFootprint = 1

// InputRowHeight is the terminal rows reserved for the emberloop input row (ticket 02).
const InputRowHeight = 1

// ChildSize is the terminal size the agent child runs at: the full width
// minus one column per side when the frame is on, and the full height minus
// the strip rows and its top rule. The child is told this size and gets it
// again on every resize, so its own output never reaches the strip rows.
func ChildSize(termW, termH, stripRows int, frame bool) (w, h int) {
	return ChildSizePadRow(termW, termH, stripRows, frame, Pad{}, false)
}

// Pad is the empty space around the wrapped agent screen and the strip:
// columns on the left and right, rows below the strip. It is background
// only; nothing is drawn in it.
type Pad struct{ Left, Right, Bottom int }

// minPadChildW and minPadChildH are the smallest child screen padding may
// leave: padding shrinks first, before the strip drops a row or a segment.
const (
	minPadChildW = 20
	minPadChildH = 5
)

// FitPadRow shrinks pad until the child screen keeps minPadChildW columns and
// minPadChildH rows (or padding is gone).
func FitPadRow(termW, termH, stripRows int, frame bool, pad Pad, row bool) Pad {
	extra := 0
	if row {
		extra = InputRowHeight
	}
	frameCols := 0
	if frame {
		frameCols = 2
	}
	for termW-frameCols-pad.Left-pad.Right < minPadChildW && pad.Left+pad.Right > 0 {
		if pad.Left >= pad.Right {
			pad.Left--
		} else {
			pad.Right--
		}
	}
	for termH-stripRows-StripFootprint-extra-pad.Bottom < minPadChildH && pad.Bottom > 0 {
		pad.Bottom--
	}
	return pad
}

// FitPad shrinks pad until the child screen keeps minPadChildW columns and
// minPadChildH rows (or padding is gone).
func FitPad(termW, termH, stripRows int, frame bool, pad Pad) Pad {
	return FitPadRow(termW, termH, stripRows, frame, pad, false)
}

// ChildSizePadRow is ChildSize with padding and optional input row: the child
// also loses the padding columns on each side, the padding rows below the strip,
// and the input row between the child and strip.
func ChildSizePadRow(termW, termH, stripRows int, frame bool, pad Pad, row bool) (w, h int) {
	extra := 0
	if row {
		extra = InputRowHeight
	}
	w, h = termW-pad.Left-pad.Right, termH-stripRows-StripFootprint-extra-pad.Bottom
	if frame {
		w -= 2
	}
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return w, h
}

// ChildSizePad is ChildSize with padding.
func ChildSizePad(termW, termH, stripRows int, frame bool, pad Pad) (w, h int) {
	return ChildSizePadRow(termW, termH, stripRows, frame, pad, false)
}

// Sequences a child emits that would destroy the strip: scroll-region
// resets (re-apply the region inline, and the margins after the resets that
// wipe them), full clears (repaint right after) and cursor addresses past
// the child area (clamp to its last row). The wrapper's own cursor
// save/restore uses ESC 7 and ESC 8, never CSI s: with the column margins
// enabled CSI s would set margins instead of saving. A child sharing the
// one DECSC slot can still land one restore off across a repaint, so
// repaints stay rare (only on changed numbers) to keep the window small.
var (
	wrapRegionReset = regexp.MustCompile("\x1b\\[r|\x1b\\[!p|\x1bc")
	wrapFullReset   = regexp.MustCompile("\x1bc")
	wrapClear       = regexp.MustCompile("\x1b\\[2J")
	wrapCup         = regexp.MustCompile("\x1b\\[(\\d*);?(\\d*)[Hf]")
	wrapVPA         = regexp.MustCompile("\x1b\\[(\\d+)d")
	wrapRegionSet   = regexp.MustCompile("\x1b\\[(\\d*);(\\d+)r")
)

// FilterChildOutput passes a chunk of child output through to the terminal,
// rewriting the sequences that would overwrite the strip. Margins carries
// the wrapper's column constraint (empty when the frame is off): it is
// re-applied after the resets that wipe it. It returns the bytes to write
// and whether the wrapper must repaint the strip right after: a
// full-screen clear or reset erases the strip with the rest, while a plain
// region reset is repaired inline and needs no repaint.
func FilterChildOutput(data []byte, childH int, margins string) (out []byte, repaint bool) {
	region := []byte(fmt.Sprintf("\x1b[1;%dr", childH))
	out, full := applyRegionReset(data, region, margins)
	if wrapClear.Match(out) || full {
		repaint = true
	}
	out = wrapCup.ReplaceAllFunc(out, func(m []byte) []byte {
		parts := wrapCup.FindSubmatch(m)
		row, _ := strconv.Atoi(string(parts[1]))
		if row == 0 {
			row = 1
		}
		if row > childH {
			col := string(parts[2])
			if col == "" {
				col = "1"
			}
			return []byte(fmt.Sprintf("\x1b[%d;%s%s", childH, col, m[len(m)-1:]))
		}
		return m
	})
	out = wrapVPA.ReplaceAllFunc(out, func(m []byte) []byte {
		parts := wrapVPA.FindSubmatch(m)
		row, _ := strconv.Atoi(string(parts[1]))
		if row > childH {
			return []byte(fmt.Sprintf("\x1b[%dd", childH))
		}
		return m
	})
	out = wrapRegionSet.ReplaceAllFunc(out, func(m []byte) []byte {
		parts := wrapRegionSet.FindSubmatch(m)
		bottom, _ := strconv.Atoi(string(parts[2]))
		if bottom > childH {
			return []byte(fmt.Sprintf("\x1b[%s;%dr", parts[1], childH))
		}
		return m
	})
	return out, repaint
}

// applyRegionReset re-applies the wrapper's scroll region after every child
// sequence that resets it, so the strip rows stay outside the child's
// scrolling area. A full reset also wipes the column margins, so they go
// back up with it; a plain region reset leaves the child's own margins
// alone. It reports a full reset, which clears the screen and needs a
// repaint.
func applyRegionReset(data, region []byte, margins string) ([]byte, bool) {
	full := false
	out := wrapRegionReset.ReplaceAllFunc(data, func(m []byte) []byte {
		res := make([]byte, 0, len(m)+len(region)+len(margins))
		res = append(res, m...)
		res = append(res, region...)
		if wrapFullReset.Match(m) {
			full = true
			res = append(res, margins...)
		}
		return res
	})
	return out, full
}

// HeaderSource locates everything one strip repaint reads: whose numbers
// they are, which harness's transcript holds them, and which state
// directory holds the usage rows.
type HeaderSource struct {
	AgentID        string
	AgentType      string
	AgentName      string
	Harness        string
	TranscriptPath string // exact file when known; else resolved per poll
	SessionID      string // harness session id, for transcript lookup
	SessionDir     string // session directory for harnesses that take one
	StateDir       string
	CompressCount  int
}

// resolveTranscript is the transcript a repaint reads: the exact path when
// the caller knows it, else the harness's session lookup, retried every
// poll so a session that starts writing late still appears.
func resolveTranscript(src HeaderSource) string {
	if src.TranscriptPath != "" {
		return src.TranscriptPath
	}
	if src.Harness == "claude" {
		if p := readLiveTranscript(src.StateDir, src.AgentID); p != "" {
			return p
		}
	}
	if src.SessionID == "" {
		return ""
	}
	if h := Registered(src.Harness); h != nil {
		return FindSession(h.SessionRoot(Session{ID: src.SessionID, Dir: src.SessionDir}), src.SessionID)
	}
	return ""
}

// BuildHeaderSnapshot folds one agent's current numbers into a strip
// snapshot: context size, model and top tools from the transcript, the
// window from settings, the cost from the usage rows, the state from the
// harness's turn reader (or the queued compress mark, or `compressing…`
// while a compress runs), and a warning when no compress model is set. A
// transcript or database that is not there yet is an empty snapshot, not an
// error: the strip appears once the session starts writing.
func BuildHeaderSnapshot(src HeaderSource, s tools.Settings) (HeaderSnapshot, error) {
	snap := HeaderSnapshot{
		AgentID:       src.AgentID,
		AgentType:     src.AgentType,
		AgentName:     src.AgentName,
		State:         HeaderWorking,
		CompressCount: src.CompressCount,
	}
	path := resolveTranscript(src)
	provider := ""
	// No transcript yet means no turn has started: the agent is waiting for
	// its first message, not working. herdr's wait for idle depends on it.
	if path == "" {
		snap.State = HeaderIdle
	}
	if h := Registered(src.Harness); h != nil && path != "" {
		if state, _ := h.TurnState(path); state == TurnDone || state == TurnFailed {
			snap.State = HeaderIdle
		}
	}
	// A live compress mark wins over the queue: the summary and restart run
	// now, so the strip shows why the agent paused.
	if path != "" {
		if f, err := os.Open(path); err == nil {
			core, err := SummarizeContext(src.Harness, f)
			_ = f.Close()
			if err != nil {
				return HeaderSnapshot{}, err
			}
			snap.Model = core.Model
			snap.ContextTokens = core.ContextTokens
			snap.TopTools = core.TopTools
			snap.Running, snap.Last = core.Running, core.Last
			provider = core.Provider
			snap.Window = SnapshotWindow(core, s)
		}
	}
	if cost, ok := agentCost(src.StateDir, src.AgentID); ok {
		snap.Cost, snap.HasCost = cost, true
	}
	snap.Now = time.Now()
	snap.UsageKey = UsageKeyFor(src.Harness, provider, snap.Model)
	snap.Usage = ReadUsage(src.StateDir, snap.UsageKey, snap.Now)
	if s.EffectiveCompressModel(src.Harness) == "" {
		snap.Warning = "no compressModel"
	}
	// A queued manual compress waits at the tool-call boundary: the strip
	// shows the queue instead of working or idle.
	if src.AgentID != "" && CompressQueued(src.AgentID) {
		snap.State = HeaderQueued
	}
	if src.AgentID != "" {
		if st, ok, _ := ReadAgentState(src.AgentID); ok {
			if compressingLive(st) {
				snap.State = HeaderCompressing
			} else if st.CompressFailed != "" {
				snap.State = HeaderCompressFailed
				snap.CompressFailed = shortCompressReason(st.CompressFailed)
			}
		}
	}
	return snap, nil
}

// agentCost sums one agent's reported cost the way the dashboard does:
// per-request rows add up, and each claude cost snapshot replaces the last
// one per model. A missing database or no reported cost is absent, never
// zero: the strip hides it instead of printing it.
func agentCost(stateDir, agentID string) (float64, bool) {
	if agentID == "" {
		return 0, false
	}
	if stateDir == "" {
		stateDir = statedb.StateDir()
	}
	db, err := statedb.OpenReadOnly(statedb.DBPathForDir(stateDir))
	if err != nil {
		return 0, false
	}
	defer db.Close()
	rows, err := statedb.UsageRowsSince(db, 0)
	if err != nil {
		return 0, false
	}
	var total float64
	byModel := map[string]float64{}
	hasCost := false
	for _, r := range rows {
		if r.Agent != agentID {
			continue
		}
		switch r.Kind {
		case statedb.KindRequest:
			if r.ReportedCost != nil {
				total += *r.ReportedCost
				hasCost = true
			}
		case statedb.KindCost:
			if r.ReportedCost != nil {
				byModel[r.Model] = *r.ReportedCost
				hasCost = true
			}
		}
	}
	for _, c := range byModel {
		total += c
	}
	return total, hasCost
}

// shortCompressReason condenses an auto-compress error or refusal into a brief
// pill description.
func shortCompressReason(s string) string {
	msg := strings.TrimSpace(s)
	if msg == "" {
		return ""
	}
	switch {
	case strings.Contains(msg, "past") && strings.Contains(msg, "window"):
		return "past window"
	case strings.Contains(msg, "no compressModel"):
		return "no compressModel"
	case strings.HasPrefix(msg, "summarizer failed:"):
		sub := strings.TrimSpace(strings.TrimPrefix(msg, "summarizer failed:"))
		runes := []rune(sub)
		if len(runes) > 0 && len(runes) <= 25 {
			return sub
		}
		return "summarizer error"
	case strings.HasPrefix(msg, "summarizer:"):
		sub := strings.TrimSpace(strings.TrimPrefix(msg, "summarizer:"))
		runes := []rune(sub)
		if len(runes) > 0 && len(runes) <= 25 {
			return sub
		}
		return "summarizer error"
	case strings.HasPrefix(msg, "restart:"):
		return "restart error"
	default:
		first := strings.Split(msg, "\n")[0]
		first = strings.TrimSpace(first)
		runes := []rune(first)
		if len(runes) > 25 {
			return string(runes[:22]) + "…"
		}
		return first
	}
}
