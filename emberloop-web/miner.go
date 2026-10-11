package web

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"example.com/emberloop-web/memory"
)

// MinerSettleWindow is how quiet a Session must be before the miner reads it:
// the settle window from the spec, shared with auto-compress and the nightly
// job through session.Idle. Ended Sessions mine regardless of quiet time.
const MinerSettleWindow = 10 * time.Minute

// minerContextKinds are the Event kinds stored with a correction as the thing
// it corrected: the preceding tool call or diff hunk.
var minerContextKinds = map[string]bool{
	"tool_call_started": true,
	"tool_call_updated": true,
	"diff":              true,
}

// MineResult counts one MineSession call.
type MineResult struct {
	SessionID  string
	Scanned    int // new log lines read
	Kept       int // miner-readable user prompts among them
	Stored     int // observations inserted
	Duplicates int // duplicate hits folded into evidence_count
	Reset      bool
}

// minerCursorPath is the cursor file beside the Session's events.jsonl.
func (d *Daemon) minerCursorPath(id string) string {
	return filepath.Join(d.sessionDir(id), "miner.cursor.json")
}

// minerDeciderDoc is the decider_json of a heuristic observation: what fired
// and the redacted thing it corrected. Ticket 20's Decider extends this.
type minerDeciderDoc struct {
	Miner      string  `json:"miner"`
	Type       string  `json:"type"`
	Patterns   string  `json:"patterns"`
	Confidence float64 `json:"confidence"`
	Sentiment  string  `json:"sentiment"`
	Context    string  `json:"context,omitempty"`
}

// MineSession mines one settled Session's new events.jsonl bytes into the
// sender's User bank. A Session mines only when ended or idle through the
// settle window. It reads only up to the last newline through a byte-offset
// cursor (offset, file size, hash of the first 4 KB); a rewritten head or a
// shrunk file resets the cursor and a torn last line waits. Only
// FilterMinerEvents prompts (MinerReadable: sender=user prompts, steers and
// follow-ups) are classified; Handoffs, injection blocks and every
// sender=system Event never reach the classifier. Each hit is redacted, then
// grounded, then stored with dedup in the Event sender's bank: a steer from
// User B inside User A's Session lands in B's bank only. Duplicates fold into
// evidence_count; refines and contradicts are stored, nothing is dropped.
func (d *Daemon) MineSession(id string) (MineResult, error) {
	res := MineResult{SessionID: id}
	d.mu.Lock()
	s, ok := d.sessions[id]
	var settled bool
	var memCopy []Event
	if ok {
		settled = s.meta.Status == "ended" || s.Idle(d.now(), MinerSettleWindow, d.groupOutstandingLocked(id))
		memCopy = append([]Event(nil), s.events...)
	} else {
		// A Session the Daemon never loaded (canned file, reload gap) mines
		// only when its meta says it ended; idle needs live state.
		if raw, err := os.ReadFile(filepath.Join(d.sessionDir(id), "meta.json")); err == nil {
			var meta SessionMeta
			if json.Unmarshal(raw, &meta) == nil && meta.Status == "ended" {
				settled = true
			}
		}
	}
	d.mu.Unlock()
	if !ok && !settled {
		return res, fmt.Errorf("miner: session %s not loaded", id)
	}
	if !settled {
		return res, nil
	}
	logPath := filepath.Join(d.sessionDir(id), "events.jsonl")
	if _, err := os.Stat(logPath); err != nil {
		if os.IsNotExist(err) {
			return res, nil
		}
		return res, err
	}
	cur, err := memory.LoadCursor(d.minerCursorPath(id))
	if err != nil {
		return res, err
	}
	chunk, next, reset, err := memory.ReadNew(logPath, cur)
	if err != nil {
		return res, err
	}
	res.Reset = reset
	if len(chunk) == 0 {
		if err := memory.SaveCursor(d.minerCursorPath(id), next); err != nil {
			return res, err
		}
		return res, nil
	}
	var fresh []Event
	for _, line := range strings.Split(string(chunk), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var e Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		fresh = append(fresh, e)
	}
	res.Scanned = len(fresh)
	ctx := context.Background()
	prior := append([]Event(nil), memCopy...)
	for i, e := range fresh {
		// The miner reads Sessions only through this filter, never the raw
		// log: injections, Handoffs and Daemon-made prompts are dropped here.
		// FilterMinerEvents keeps the same set MinerReadable admits per Event.
		if !MinerReadable(e) || len(FilterMinerEvents([]Event{e})) == 0 {
			prior = append(prior, e)
			continue
		}
		res.Kept++
		hit, ok := memory.ClassifyHeuristic(e.Text)
		if !ok {
			prior = append(prior, e)
			continue
		}
		author := strings.TrimSpace(e.User)
		if author == "" {
			prior = append(prior, e)
			continue
		}
		eventID := strconv.FormatInt(e.Seq, 10)
		if e.Seq == 0 {
			eventID = "line-" + strconv.Itoa(i)
		}
		contextText := minerContextOf(prior)
		redactedQuote := memory.Redact(strings.TrimSpace(e.Text))
		if redactedQuote == "" {
			prior = append(prior, e)
			continue
		}
		if err := memory.CheckGrounding(eventID, redactedQuote, memory.GroundEvent{
			Found:  true,
			Sender: e.Sender,
			Text:   memory.Redact(e.Text),
		}); err != nil {
			prior = append(prior, e)
			continue
		}
		doc, _ := json.Marshal(minerDeciderDoc{
			Miner:      "heuristic-v1",
			Type:       hit.Type,
			Patterns:   hit.Patterns,
			Confidence: hit.Confidence,
			Sentiment:  hit.Sentiment,
			Context:    memory.Redact(truncateMinerContext(contextText)),
		})
		bank, err := d.profileBankFor(author)
		if err != nil {
			prior = append(prior, e)
			continue
		}
		stored, rel, err := memory.DedupAndStore(ctx, bank, memory.Observation{
			SessionID:   id,
			EventID:     eventID,
			Author:      author,
			Quote:       redactedQuote,
			Kind:        memory.KindPreference,
			DeciderJSON: string(doc),
		})
		_ = stored
		if err != nil {
			prior = append(prior, e)
			continue
		}
		if rel == memory.RelDuplicate {
			res.Duplicates++
		} else {
			res.Stored++
		}
		prior = append(prior, e)
	}
	// The cursor advances after the scan, so a crash re-reads (same-Event
	// hits return without bumping) rather than losing bytes.
	if err := memory.SaveCursor(d.minerCursorPath(id), next); err != nil {
		return res, err
	}
	return res, nil
}

// MineSettledSessions mines every settled Session once, for the nightly job.
// One Session failing does not stop the others; the errors join.
func (d *Daemon) MineSettledSessions() ([]MineResult, error) {
	d.mu.Lock()
	ids := make([]string, 0, len(d.sessions))
	for id := range d.sessions {
		ids = append(ids, id)
	}
	d.mu.Unlock()
	var out []MineResult
	var errs []error
	for _, id := range ids {
		r, err := d.MineSession(id)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		out = append(out, r)
	}
	if len(errs) > 0 {
		return out, errs[0]
	}
	return out, nil
}

// minerContextOf returns the text of the nearest preceding tool call or diff
// hunk: the thing a correction corrected. Empty when none precedes.
func minerContextOf(prior []Event) string {
	for i := len(prior) - 1; i >= 0; i-- {
		if minerContextKinds[prior[i].Kind] {
			return prior[i].Text
		}
		if prior[i].Kind == "prompt" && MinerReadable(prior[i]) {
			break
		}
	}
	return ""
}

// truncateMinerContext caps stored context so one giant tool result cannot
// bloat the bank.
func truncateMinerContext(s string) string {
	const cap = 2000
	if len(s) <= cap {
		return s
	}
	return s[:cap]
}
