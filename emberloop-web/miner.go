package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

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
	Dropped    int // heuristic hits the Decider dropped below its cut
	Reset      bool
}

// minerCursorPath is the cursor file beside the Session's events.jsonl.
func (d *Daemon) minerCursorPath(id string) string {
	return filepath.Join(d.sessionDir(id), "miner.cursor.json")
}

// minerDeciderDoc is the decider_json of a mined observation: the Decider
// that judged it (mode, model, version), its durable probability, kind and
// scope, whether it is queue-eligible (ticket 22), and the heuristic detail
// (what fired and the redacted thing it corrected).
type minerDeciderDoc struct {
	Miner         string  `json:"miner"`
	Type          string  `json:"type"`
	Patterns      string  `json:"patterns"`
	Confidence    float64 `json:"confidence"`
	Sentiment     string  `json:"sentiment"`
	Context       string  `json:"context,omitempty"`
	Decider       string  `json:"decider"`
	Model         string  `json:"model"`
	Version       string  `json:"version"`
	Durable       float64 `json:"durable"`
	Kind          string  `json:"kind"`
	Confirmed     bool    `json:"confirmed"`
	Scope         string  `json:"scope_hint"`
	QueueEligible bool    `json:"queue_eligible"`
}

// MineSession mines one settled Session's new events.jsonl bytes into the
// sender's User bank. A Session mines only when ended or idle through the
// settle window. It reads only up to the last newline through a byte-offset
// cursor (offset, file size, hash of the first 4 KB); a rewritten head or a
// shrunk file resets the cursor and a torn last line waits. Only
// FilterMinerEvents prompts (MinerReadable: sender=user prompts, steers and
// follow-ups) are classified, then confirmed by the Decider (decideMine):
// heuristic by default, the memory model in llm and ensemble modes. A drop
// below its cut stores nothing. Handoffs, injection blocks and every
// sender=system Event never reach the classifier. Each kept hit is redacted,
// grounded, then stored with dedup in the Event sender's bank: a steer from
// User B inside User A's Session lands in B's bank only. Duplicates fold into
// evidence_count; refines and contradicts are stored with the relation and
// the related observation ID in decider_json, nothing is dropped. A transient
// bank error aborts the scan without advancing the cursor, so the bytes mine
// again; permanently invalid records are skipped.
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
	// Context for a correction comes only from Events before it: the
	// in-memory history cut where this chunk starts, then earlier fresh
	// Events as the scan advances.
	prior := minerPriorEvents(memCopy, fresh)
	for i, e := range fresh {
		// The miner reads Sessions only through MinerReadable, the
		// per-Event predicate of FilterMinerEvents: injections, Handoffs
		// and Daemon-made prompts are dropped here, never mined raw.
		if !MinerReadable(e) {
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
		bank, err := d.profileBankFor(author)
		if err != nil {
			if errors.Is(err, memory.ErrInvalid) {
				prior = append(prior, e)
				continue // permanent: no bank for this author; later bytes still mine.
			}
			return res, fmt.Errorf("miner: bank for %s: %w", author, err)
		}
		// A replayed Event (cursor reset) is found here, before the Decider,
		// so it never costs a confirming LLM call; the store below would
		// return the same row without bumping or inserting.
		if d.minedEventAlready(ctx, bank, id, eventID) {
			res.Duplicates++
			prior = append(prior, e)
			continue
		}
		// The heuristic proposes; the confirming Decider (decider.go) judges
		// durable and offers kind and scope. A drop below its cut stores
		// nothing; an unavailable confirmer leaves the heuristic keep
		// standing, but never queue-eligible.
		dec := d.decideMine(e.Text, contextText, hit)
		if !dec.keep {
			res.Dropped++
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
			Miner:         "heuristic-v1",
			Type:          hit.Type,
			Patterns:      hit.Patterns,
			Confidence:    hit.Confidence,
			Sentiment:     hit.Sentiment,
			Context:       memory.Redact(truncateMinerContext(contextText)),
			Decider:       dec.decider,
			Model:         dec.model,
			Version:       dec.version,
			Durable:       dec.durable,
			Kind:          dec.kind,
			Scope:         dec.scope,
			QueueEligible: dec.eligible,
			Confirmed:     dec.confirmed,
		})
		_, rel, err := memory.DedupAndStore(ctx, bank, memory.Observation{
			SessionID:   id,
			EventID:     eventID,
			Author:      author,
			Quote:       redactedQuote,
			Kind:        dec.kind,
			DeciderJSON: string(doc),
		})
		if err != nil {
			if errors.Is(err, memory.ErrInvalid) {
				prior = append(prior, e)
				continue // permanent: the record itself is invalid.
			}
			return res, fmt.Errorf("miner: store session %s event %s: %w", id, eventID, err)
		}
		if rel == memory.RelDuplicate {
			res.Duplicates++
		} else {
			res.Stored++
		}
		prior = append(prior, e)
	}
	// The cursor advances only after a clean scan. A crash or a returned
	// error re-reads the bytes; same-Event replays return without bumping,
	// so neither path loses a hit nor inflates evidence.
	if err := memory.SaveCursor(d.minerCursorPath(id), next); err != nil {
		return res, err
	}
	return res, nil
}

// minedEventAlready reports whether this Session Event already has a stored
// observation: a cursor reset replays it. The lookup is by Session ID and
// Event ID, so it is exact whatever else the quote matches; the store would
// return the same row without bumping or inserting.
func (d *Daemon) minedEventAlready(ctx context.Context, bank memory.Memory, id, eventID string) bool {
	_, err := bank.ObservationByEvent(ctx, id, eventID)
	return err == nil
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

// minerPriorEvents returns the history before the first fresh Event: the
// in-memory log cut where the new chunk starts. The scan appends earlier
// fresh Events as it advances, so a correction's context comes from before
// it, never from the Session tail. The chunk start is matched by sequence
// number; when it carries none the fallback drops the trailing overlap
// (memory and file are written together, so a full in-memory history ends
// with the fresh Events).
func minerPriorEvents(mem, fresh []Event) []Event {
	if len(fresh) == 0 {
		return mem
	}
	if fresh[0].Seq != 0 {
		for j := range mem {
			if mem[j].Seq == fresh[0].Seq {
				return append([]Event(nil), mem[:j]...)
			}
		}
	}
	if len(mem) >= len(fresh) {
		return append([]Event(nil), mem[:len(mem)-len(fresh)]...)
	}
	return nil
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

// truncateMinerContext caps stored context at maxMinerContextRunes runes so
// one giant tool result cannot bloat the bank. It cuts on a rune boundary,
// so CJK text stays valid UTF-8.
func truncateMinerContext(s string) string {
	const maxMinerContextRunes = 2000
	if utf8.RuneCountInString(s) <= maxMinerContextRunes {
		return s
	}
	n := 0
	for i := range s {
		if n >= maxMinerContextRunes {
			return s[:i]
		}
		n++
	}
	return s
}
