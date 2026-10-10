package web

import (
	"encoding/json"
	"net/url"
	"strconv"
)

// Session snapshot paging and replay compaction.
//
// A turn starts at every prompt Event, so cutting the log at prompt
// boundaries never splits a rendered card. The UI opens a Session with
// ?tail=6 and pages earlier turns with ?before=<seq>&turns=<n>; omitting
// both returns the full log for old clients. ?at=<seq> widens a tail
// window back to the turn holding that seq, for deep links.
//
// Snapshot responses (full, tail and page) carry only the last
// tool_call_updated per tool call id: the transcript folds updates for one
// id into a single card where the last non-empty input/output wins, so the
// dropped intermediates change nothing on screen. The live SSE tail stays
// uncompacted.

// defaultPageTurns is the turn count a ?before= page takes when ?turns= is
// absent, matching the UI tail size.
const defaultPageTurns = 6

// promptIndexes returns the event indexes that start a turn.
func promptIndexes(events []Event) []int {
	var out []int
	for i, e := range events {
		if e.Kind == "prompt" {
			out = append(out, i)
		}
	}
	return out
}

// tailStart returns the slice index of the first event of the last n turns.
// n <= 0, no prompts, or fewer turns than n all mean the whole log.
func tailStart(events []Event, n int) int {
	if n <= 0 {
		return 0
	}
	prompts := promptIndexes(events)
	if len(prompts) == 0 || n >= len(prompts) {
		return 0
	}
	return prompts[len(prompts)-n]
}

// beforeWindow pages earlier turns: the last n turns among events with Seq
// below before, as a [start, end) window the UI prepends.
func beforeWindow(events []Event, before int64, n int) (start, end int) {
	if n <= 0 {
		n = defaultPageTurns
	}
	end = len(events)
	for i, e := range events {
		if e.Seq >= before {
			end = i
			break
		}
	}
	return tailStart(events[:end], n), end
}

// toolUpdateID reports the tool call id of a tool_call_updated Event.
func toolUpdateID(e Event) (string, bool) {
	if e.Kind != "tool_call_updated" {
		return "", false
	}
	var t struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(e.Text), &t); err != nil || t.ID == "" {
		return "", false
	}
	return t.ID, true
}

// compactToolUpdates keeps every event except superseded tool_call_updated
// ones: per tool call id only the last update survives. Updates without a
// parseable id are kept; the live SSE tail never passes through here.
func compactToolUpdates(events []Event) []Event {
	last := map[string]int{}
	for i, e := range events {
		if id, ok := toolUpdateID(e); ok {
			last[id] = i
		}
	}
	if len(last) == 0 {
		return events
	}
	out := make([]Event, 0, len(events))
	for i, e := range events {
		if id, ok := toolUpdateID(e); ok && i != last[id] {
			continue
		}
		out = append(out, e)
	}
	return out
}

// atStart widens a tail window back to the turn holding seq at, so a deep
// link target is in the snapshot. at <= 0, a full window, an unknown seq,
// or at already inside all keep start.
func atStart(events []Event, start int, at int64) int {
	if at <= 0 || start <= 0 {
		return start
	}
	pos := -1
	for i, e := range events {
		if e.Seq == at {
			pos = i
			break
		}
	}
	if pos < 0 || pos >= start {
		return start
	}
	turn := 0
	for _, p := range promptIndexes(events[:pos+1]) {
		turn = p
	}
	return turn
}

// foldToolCalls reduces tool events to one record per call id the way the
// transcript does: ordered merge where the last non-empty field wins. Tests
// compare the fold before and after compaction to prove it lossless.
func foldToolCalls(events []Event) map[string]ToolCall {
	out := map[string]ToolCall{}
	for _, e := range events {
		if e.Kind != "tool_call_started" && e.Kind != "tool_call_updated" {
			continue
		}
		var t ToolCall
		if err := json.Unmarshal([]byte(e.Text), &t); err != nil || t.ID == "" {
			continue
		}
		cur := out[t.ID]
		if t.Name != "" {
			cur.Name = t.Name
		}
		if t.Server != "" {
			cur.Server = t.Server
		}
		if t.Input != "" {
			cur.Input = t.Input
		}
		if t.Output != "" {
			cur.Output = t.Output
		}
		if t.Status != "" {
			cur.Status = t.Status
		}
		cur.ID = t.ID
		out[t.ID] = cur
	}
	return out
}

// snapshotWindow slices events[start:end] and compacts the window for the
// response. firstSeq is the window's first sequence number in the
// uncompacted log, so the UI can page with ?before=firstSeq without gaps
// even when compaction dropped that exact event.
func snapshotWindow(events []Event, start, end int) (out []Event, firstSeq int64, hasMore bool) {
	if start < 0 {
		start = 0
	}
	if end < 0 {
		end = 0
	}
	if end > len(events) {
		end = len(events)
	}
	if start > end {
		start = end
	}
	window := events[start:end]
	if len(window) > 0 {
		firstSeq = window[0].Seq
	}
	return compactToolUpdates(window), firstSeq, start > 0
}

// parseSnapshotQuery reads the tail/before/turns/at query of a session GET.
// tail=-1 means full; hasBefore selects page mode over tail mode.
func parseSnapshotQuery(q url.Values) (tail int, hasBefore bool, before int64, turns int, at int64) {
	tail = -1
	if s := q.Get("tail"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			tail = n
		}
	}
	if s := q.Get("before"); s != "" {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil && n > 0 {
			hasBefore, before = true, n
		}
	}
	turns = defaultPageTurns
	if s := q.Get("turns"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			turns = n
		}
	}
	if s := q.Get("at"); s != "" {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil && n > 0 {
			at = n
		}
	}
	return tail, hasBefore, before, turns, at
}
