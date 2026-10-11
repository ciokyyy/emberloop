package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/MichaelKinsy/PiG/agent"
)

// Context compression for pig Sessions. The pig SDK compacts a Session in
// place: it summarizes old history, keeps recent turns, and keeps the same
// Session id, so nothing restarts. The Daemon exposes a manual action and an
// automatic pass that runs when a turn settles above a threshold.

// compressSettings are the knobs stored in the settings file. They mirror the
// terminal system's auto_compress and compress_cooldown names.
type compressSettings struct {
	Auto             bool   `json:"auto_compress"`
	ThresholdPercent int    `json:"compress_threshold_percent"`
	Cooldown         int    `json:"compress_cooldown"`
	Instructions     string `json:"compress_instructions"`
}

const (
	defaultCompressPercent  = 80
	defaultCompressCooldown = 5
	compressTimeout         = 5 * time.Minute
)

// readCompressSettings reads the settings file on every call, so a change
// applies to the next turn without a restart.
func readCompressSettings() compressSettings {
	s := compressSettings{Auto: true, ThresholdPercent: defaultCompressPercent, Cooldown: defaultCompressCooldown}
	all, err := readSettingsMap()
	if err != nil {
		return s
	}
	if raw, ok := all["auto_compress"]; ok {
		var b bool
		if json.Unmarshal(raw, &b) == nil {
			s.Auto = b
		}
	}
	if raw, ok := all["compress_threshold_percent"]; ok {
		var n int
		if json.Unmarshal(raw, &n) == nil && n >= 10 && n <= 99 {
			s.ThresholdPercent = n
		}
	}
	if raw, ok := all["compress_cooldown"]; ok {
		var n int
		if json.Unmarshal(raw, &n) == nil && n >= 0 {
			s.Cooldown = n
		}
	}
	if raw, ok := all["compress_instructions"]; ok {
		var str string
		if json.Unmarshal(raw, &str) == nil {
			s.Instructions = strings.TrimSpace(str)
		}
	}
	return s
}

// compactBackend is the seam a Backend implements to compress in place.
type compactBackend interface {
	// Compact summarizes the Session history; it blocks until done.
	Compact(sessionID, instructions string) error
	// ContextUsage reports tokens in context and the model window.
	ContextUsage(sessionID string) (used, size int, ok bool)
}

// CompactionEvent is the JSON Text of a "compaction" Event.
type CompactionEvent struct {
	Reason       string `json:"reason"`
	Aborted      bool   `json:"aborted,omitempty"`
	TokensBefore int    `json:"tokensBefore"`
	TokensAfter  int    `json:"tokensAfter"`
	Window       int    `json:"window,omitempty"`
	Summary      string `json:"summary,omitempty"`
	Error        string `json:"error,omitempty"`
}

func (c CompactionEvent) text() string {
	b, _ := json.Marshal(c)
	return string(b)
}

// Compact compresses one live pig Session in place.
func (b *PigBackend) Compact(sessionID, instructions string) error {
	b.mu.Lock()
	ps, ok := b.sessions[sessionID]
	b.mu.Unlock()
	if !ok {
		return fmt.Errorf("pig: unknown session")
	}
	ctx, cancel := context.WithTimeout(ps.ctx, compressTimeout)
	defer cancel()
	return ps.sess.Compact(ctx, instructions)
}

// ContextUsage reports the projected context tokens and the model window.
// Tokens are unknown right after a compaction until the next reply.
func (b *PigBackend) ContextUsage(sessionID string) (int, int, bool) {
	b.mu.Lock()
	ps, ok := b.sessions[sessionID]
	b.mu.Unlock()
	if !ok {
		return 0, 0, false
	}
	u := ps.sess.ContextUsage()
	if u == nil || u.ContextWindow <= 0 {
		return 0, 0, false
	}
	used := 0
	if u.Tokens != nil {
		used = *u.Tokens
	}
	return used, u.ContextWindow, u.Tokens != nil
}

func usageText(used, size int) string {
	b, _ := json.Marshal(map[string]int{"used": used, "size": size})
	return string(b)
}

// compactionNormEvents maps the SDK compaction events. The end event also
// refreshes the context meter from the SDK's estimate after compaction.
func compactionNormEvents(e agent.AgentEvent, window int) []normEvent {
	switch e := e.(type) {
	case agent.CompactionStartEvent:
		return []normEvent{{kind: "compaction_started", text: CompactionEvent{Reason: e.Reason}.text()}}
	case agent.CompactionEndEvent:
		ev := CompactionEvent{
			Reason: e.Reason, Aborted: e.Aborted, TokensBefore: e.TokensBefore,
			TokensAfter: e.EstimatedTokensAfter, Window: window, Summary: e.Summary, Error: e.ErrorMessage,
		}
		out := []normEvent{{kind: "compaction", text: ev.text()}}
		if !e.Aborted && e.ErrorMessage == "" && window > 0 && e.EstimatedTokensAfter > 0 {
			out = append(out, normEvent{kind: "usage", text: usageText(e.EstimatedTokensAfter, window)})
		}
		return out
	}
	return nil
}

// autoCompressDue reports whether usage crosses the threshold.
func autoCompressDue(used, size int, s compressSettings) bool {
	if !s.Auto || size <= 0 || used <= 0 {
		return false
	}
	return used*100 >= size*s.ThresholdPercent
}

func (d *Daemon) compactorFor(id string) (compactBackend, *session, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	s, ok := d.sessions[id]
	if !ok {
		return nil, nil, false
	}
	cb, _ := d.backendForLocked(s.meta.Harness).(compactBackend)
	return cb, s, cb != nil
}

// runCompaction compresses a Session that is already marked busy. The SDK
// compaction events stream into the transcript on their own; this adds only
// the failure Event the SDK did not emit and resets the cooldown counter.
func (d *Daemon) runCompaction(id string, cb compactBackend, instructions string) {
	err := cb.Compact(id, instructions)
	if err != nil {
		d.appendEvent(id, "compaction", CompactionEvent{Reason: "manual", Aborted: true, Error: err.Error()}.text(), "")
	}
	d.mu.Lock()
	if s, ok := d.sessions[id]; ok {
		s.turnsSinceCompress = 0
	}
	d.mu.Unlock()
	if used, size, ok := cb.ContextUsage(id); ok {
		d.appendEvent(id, "usage", usageText(used, size), "")
	}
}

// maybeAutoCompress runs after a turn, before the Session settles. It only
// acts for Backends that compact in place, above the threshold, outside the
// cooldown, and never for read-only scouts.
func (d *Daemon) maybeAutoCompress(id string) {
	cb, s, ok := d.compactorFor(id)
	if !ok {
		return
	}
	set := readCompressSettings()
	d.mu.Lock()
	s.turnsSinceCompress++
	skip := s.meta.Scout || s.meta.Status == "ended" || s.turnsSinceCompress < set.Cooldown
	d.mu.Unlock()
	if skip || !set.Auto {
		return
	}
	used, size, ok := cb.ContextUsage(id)
	if !ok || !autoCompressDue(used, size, set) {
		return
	}
	d.appendEvent(id, "status", "", "working")
	d.runCompaction(id, cb, set.Instructions)
	d.appendEvent(id, "status", "", "idle")
}

// handleCompress is POST /api/sessions/{id}/compress: compress now. It
// answers 202 once the compaction runs; the transcript carries the result.
func (d *Daemon) handleCompress(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Instructions string `json:"instructions"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	cb, s, ok := d.compactorFor(id)
	var hb handoffBackend
	if !ok {
		hb, s = d.handoffFor(id)
	}
	if !ok && hb == nil {
		d.mu.Lock()
		_, exists := d.sessions[id]
		d.mu.Unlock()
		if !exists {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "compression is not available for this Session"})
		return
	}
	d.mu.Lock()
	scout, ended := s.meta.Scout, s.meta.Status == "ended"
	d.mu.Unlock()
	if scout || ended {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "this Session cannot be compressed"})
		return
	}
	if code, msg := d.resumeReleased(id); code != 0 {
		writeJSON(w, code, map[string]any{"error": msg})
		return
	}
	instructions := strings.TrimSpace(req.Instructions)
	if instructions == "" {
		instructions = readCompressSettings().Instructions
	}
	d.mu.Lock()
	if !s.live {
		d.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]any{"error": "session is resuming; try again"})
		return
	}
	if s.busy {
		d.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]any{"error": "session is working; compress when it is idle"})
		return
	}
	s.busy = true
	s.active++
	d.mu.Unlock()
	if hb != nil {
		go d.runHandoff(id, hb)
		writeJSON(w, http.StatusAccepted, map[string]any{"ok": true})
		return
	}
	go func() {
		d.appendEvent(id, "status", "", "working")
		d.runCompaction(id, cb, instructions)
		d.appendEvent(id, "status", "", "idle")
		d.finishTurn(id)
	}()
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true})
}

// compressPatch is the PUT shape: every field optional.
type compressPatch struct {
	Auto             *bool   `json:"auto_compress"`
	ThresholdPercent *int    `json:"compress_threshold_percent"`
	Cooldown         *int    `json:"compress_cooldown"`
	Instructions     *string `json:"compress_instructions"`
}

func (p *compressPatch) validate() string {
	if p == nil {
		return ""
	}
	if p.ThresholdPercent != nil && (*p.ThresholdPercent < 10 || *p.ThresholdPercent > 99) {
		return "compress_threshold_percent must be between 10 and 99"
	}
	if p.Cooldown != nil && *p.Cooldown < 0 {
		return "compress_cooldown must be zero or more"
	}
	if p.Instructions != nil && len(*p.Instructions) > 2000 {
		return "compress_instructions is limited to 2000 characters"
	}
	return ""
}

// apply writes the set fields into the settings map.
func (p *compressPatch) apply(all map[string]json.RawMessage) {
	if p == nil {
		return
	}
	set := func(k string, v any) {
		raw, _ := json.Marshal(v)
		all[k] = raw
	}
	if p.Auto != nil {
		set("auto_compress", *p.Auto)
	}
	if p.ThresholdPercent != nil {
		set("compress_threshold_percent", *p.ThresholdPercent)
	}
	if p.Cooldown != nil {
		set("compress_cooldown", *p.Cooldown)
	}
	if p.Instructions != nil {
		set("compress_instructions", strings.TrimSpace(*p.Instructions))
	}
}
