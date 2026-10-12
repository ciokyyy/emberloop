package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// Context compression is Handoff restart on every Backend. The Daemon writes
// one Handoff document from the Event log, starts a fresh harness handle for
// the same Session, and sends the Handoff as its first prompt. The Session
// ID, Event log and owner stay. The Daemon exposes manual compress; the
// automatic pass lands in ticket 14.

// compressSettings are the knobs stored in the settings file. They mirror the
// terminal system's auto_compress and compress_cooldown names.
type compressSettings struct {
	Auto             bool `json:"auto_compress"`
	ThresholdPercent int  `json:"compress_threshold_percent"`
	// ThresholdTokens is the absolute token count that also trips
	// auto-compress, OR'd with the percent gate; 0 means off. The percent
	// gate alone never fires on 1M-window models (claude reports size
	// 1,000,000, so 80% is 800k, far past anything seen in prod), while an
	// absolute cap fires on every harness.
	ThresholdTokens int    `json:"compress_threshold_tokens"`
	Cooldown        int    `json:"compress_cooldown"`
	Instructions    string `json:"compress_instructions"`
	Model           string `json:"compress_model"`
}

const (
	defaultCompressPercent = 80
	// defaultCompressTokens is the out-of-the-box absolute auto-compress
	// cap: ~30% of a 1M claude window, past the percent gate on smaller
	// pig windows (80% of 272k is 218k), so it only ever hurries
	// large-window models. It acts only with a compress model set and auto
	// on, so no surprise Handoff runs without configuration.
	defaultCompressTokens   = 300000
	defaultCompressCooldown = 5
	compressTimeout         = 5 * time.Minute
)

// readCompressSettings reads the settings file on every call, so a change
// applies to the next turn without a restart.
func readCompressSettings() compressSettings {
	all, err := readSettingsMap()
	if err != nil {
		return compressSettings{Auto: true, ThresholdPercent: defaultCompressPercent, ThresholdTokens: defaultCompressTokens, Cooldown: defaultCompressCooldown}
	}
	return parseCompressSettings(all)
}

// parseCompressSettings decodes the compress knobs: shared by the host
// reader and the Daemon-scoped test seam so the two never diverge.
func parseCompressSettings(all map[string]json.RawMessage) compressSettings {
	s := compressSettings{Auto: true, ThresholdPercent: defaultCompressPercent, ThresholdTokens: defaultCompressTokens, Cooldown: defaultCompressCooldown}
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
	if raw, ok := all["compress_threshold_tokens"]; ok {
		var n int
		if json.Unmarshal(raw, &n) == nil && n >= 0 {
			s.ThresholdTokens = n
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
	if raw, ok := all["compress_model"]; ok {
		var str string
		if json.Unmarshal(raw, &str) == nil {
			s.Model = strings.TrimSpace(str)
		}
	}
	return s
}

// compressSettingsOf reads this Daemon's compress knobs: the override file
// when set, else the host file.
func (d *Daemon) compressSettingsOf() compressSettings {
	all, err := d.settingsMap()
	if err != nil {
		return compressSettings{Auto: true, ThresholdPercent: defaultCompressPercent, ThresholdTokens: defaultCompressTokens, Cooldown: defaultCompressCooldown}
	}
	return parseCompressSettings(all)
}

// ContextUsage reports the projected context tokens and the model window.
// The window alone counts as known (true) even when the token count is not
// (yet): right after a compaction Tokens is nil while ContextWindow is set,
// and callers seed a zero-used reading from the window instead of dropping
// the meter.
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
	return used, u.ContextWindow, true
}

func usageText(used, size int) string {
	b, _ := json.Marshal(map[string]int{"used": used, "size": size})
	return string(b)
}

// autoCompressDue reports whether the newest usage Event sits at or past
// either gate — compress_threshold_percent of the window, or the absolute
// compress_threshold_tokens cap (0 means off) — with no open tool call.
// Cooldown counts non-Handoff prompts since the last compress, except past
// the window: a Session at or over its window compresses whatever the
// cooldown says, since every further turn into a full context returns
// nothing. The idle, model, block and Group checks stay with the callers:
// this is the usage half only, shared by the turn-end check and the pre-gate
// arm.
func autoCompressDue(events []Event, set compressSettings) bool {
	used, size, ok := usageOf(events)
	if !ok || size <= 0 {
		return false
	}
	percentDue := used*100 >= int64(set.ThresholdPercent)*size
	tokensDue := set.ThresholdTokens > 0 && used >= int64(set.ThresholdTokens)
	if !percentDue && !tokensDue {
		return false
	}
	if openTask(events) {
		return false
	}
	if used >= size {
		return true
	}
	return cooldownPassed(events, set.Cooldown)
}

// armAutoCompress records a deferred automatic compress when one is due and
// runnable: usage at or past the threshold, a Handoff Backend, a set
// compress model and no block. It reports whether a wait is now armed. It
// never starts the Handoff itself, so callers use it while the Session is
// busy (the submit-gate arm, the finishTurn drain): runDeferredCompress runs
// the wait once the Session settles and re-verifies it is still due.
func (d *Daemon) armAutoCompress(id string) bool {
	set := d.compressSettingsOf()
	if !set.Auto {
		return false
	}
	d.mu.Lock()
	s, ok := d.sessions[id]
	if !ok || s.meta.Status == "ended" || s.meta.Scout || s.swapping || s.compressWait || s.meta.CompressBlock != "" {
		d.mu.Unlock()
		return false
	}
	events := append([]Event(nil), s.events...)
	d.mu.Unlock()
	if !autoCompressDue(events, set) {
		return false
	}
	hb, _ := d.handoffFor(id)
	if hb == nil {
		return false
	}
	if compressNote(set.Model) != "" {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	s, ok = d.sessions[id]
	if !ok || s.compressWait || s.swapping || s.meta.Status == "ended" || s.meta.CompressBlock != "" {
		return false
	}
	s.compressWait = true
	s.compressAuto = true
	s.compressInstr = ""
	return true
}

// compressBeforeGate arms an automatic compress when one is due, for a turn
// that just settled under the submit gate. Reminders into a nearly full
// context burn turns the compress would have saved: the gate skips them and
// the deferred compress runs once the turn settles. A wait armed earlier
// (manual or automatic) counts too, so no reminder lands while a compress
// already pends. It reports whether the gate should stand down.
func (d *Daemon) compressBeforeGate(id string) bool {
	d.mu.Lock()
	waiting := false
	if s, ok := d.sessions[id]; ok {
		waiting = s.compressWait
	}
	d.mu.Unlock()
	if waiting {
		return true
	}
	return d.armAutoCompress(id)
}

// maybeAutoCompress runs after a turn settles, before Group delivery settles
// a deferred manual compress. When auto_compress is on and the newest usage
// Event sits at or past compress_threshold_percent, it starts a Handoff
// compress through the shared requestCompress path (same swap, same fallback
// chain, same block policy). It fires only at a tool boundary (no open tool
// call, per openTask) or when the Session is idle, never mid-token: the gate
// is the one session.Idle definition (quiet 0, so a settled turn needs no
// quiet wait), and a turn, queued input or resume/release in flight skips to
// the next turn end. An outstanding Group still arms an automatic wait
// instead of skipping, so runDeferredCompress runs it at Group delivery.
// compress_cooldown counts non-Handoff prompts since the last compressed Event
// (turns to wait); the first compress is never cooled. Past the window the
// cooldown is bypassed (see autoCompressDue). An unset or unauthed
// compress model, a blocked Session, a scout or an ended Session never starts
// one: the Session GET compressNote and the Settings warning already name the
// missing model.
func (d *Daemon) maybeAutoCompress(id string) {
	set := d.compressSettingsOf()
	if !set.Auto {
		return
	}
	d.mu.Lock()
	s, ok := d.sessions[id]
	if !ok {
		d.mu.Unlock()
		return
	}
	now := d.now()
	groupOpen := d.groupOutstandingLocked(id)
	if s.meta.Status == "ended" || s.meta.Scout || s.swapping || s.compressWait || s.meta.CompressBlock != "" {
		d.mu.Unlock()
		return
	}
	if !s.Idle(now, 0, groupOpen) && !groupOpen {
		d.mu.Unlock()
		return
	}
	events := append([]Event(nil), s.events...)
	due := autoCompressDue(events, set)
	d.mu.Unlock()
	if !due {
		return
	}
	if groupOpen {
		// An outstanding Group holds the Handoff until delivery: arm an
		// automatic wait (re-verified at run) instead of starting one.
		// armAutoCompress re-checks viability without the lock held.
		d.armAutoCompress(id)
		return
	}
	hb, _ := d.handoffFor(id)
	if hb == nil {
		return
	}
	if compressNote(set.Model) != "" {
		return
	}
	d.requestCompress(id, "")
}

// usageOf reports the newest usage Event's used and size counts, false when
// the log holds none.
func usageOf(events []Event) (int64, int64, bool) {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Kind != "usage" {
			continue
		}
		var u struct {
			Used int64 `json:"used"`
			Size int64 `json:"size"`
		}
		if json.Unmarshal([]byte(events[i].Text), &u) != nil || u.Size <= 0 {
			continue
		}
		return u.Used, u.Size, true
	}
	return 0, 0, false
}

// promptsSinceCompress counts the non-Handoff prompts after the last
// compressed Event, false when no compress ran yet.
func promptsSinceCompress(events []Event) (int, bool) {
	last := -1
	for i, e := range events {
		if e.Kind == "compressed" {
			last = i
		}
	}
	if last < 0 {
		return 0, false
	}
	n := 0
	for _, e := range events[last+1:] {
		if e.Kind == "prompt" && !e.Handoff {
			n++
		}
	}
	return n, true
}

// cooldownPassed reports whether enough turns ran since the last compress.
// Cooldown counts non-Handoff prompts (the Settings "Turns to wait"); the
// first compress is never cooled and a non-positive cooldown never blocks.
func cooldownPassed(events []Event, cooldown int) bool {
	if cooldown <= 0 {
		return true
	}
	n, had := promptsSinceCompress(events)
	if !had {
		return true
	}
	return n >= cooldown
}

// handleCompress is POST /api/sessions/{id}/compress: compress now, or once
// the Session's Group is no longer outstanding. It answers 202 once the
// Handoff runs or waits; the transcript carries the result. It answers 409
// with the reason while no authed compress model is set.
func (d *Daemon) handleCompress(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Instructions string `json:"instructions"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	code, out := d.requestCompress(id, strings.TrimSpace(req.Instructions))
	if code == http.StatusNotFound {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, code, out)
}

// requestCompress starts a Handoff compress of id, or holds it while a Group
// of the Session is outstanding. It returns the HTTP code and the body to answer.
func (d *Daemon) requestCompress(id, instructions string) (int, map[string]any) {
	hb, s := d.handoffFor(id)
	if hb == nil {
		d.mu.Lock()
		_, exists := d.sessions[id]
		d.mu.Unlock()
		if !exists {
			return http.StatusNotFound, nil
		}
		return http.StatusBadRequest, map[string]any{"error": "compression is not available for this Session"}
	}
	d.mu.Lock()
	scout, ended := s.meta.Scout, s.meta.Status == "ended"
	d.mu.Unlock()
	if scout || ended {
		return http.StatusForbidden, map[string]any{"error": "this Session cannot be compressed"}
	}
	set := d.compressSettingsOf()
	if note := compressNote(set.Model); note != "" {
		return http.StatusConflict, map[string]any{"error": note}
	}
	if instructions == "" {
		instructions = set.Instructions
	}
	// The Handoff must carry the Group's result, so the compress waits for
	// the Group. runDeferredCompress resumes it at a turn end or a delivery.
	// A wait armed here is manual: the operator asked, so it runs whatever
	// the meter says when the Group clears. Automatic waits arm with
	// compressAuto set and are re-verified at run time instead.
	d.mu.Lock()
	if d.groupOutstandingLocked(id) {
		if s.meta.CompressBlock != "" {
			// A blocked Session waits on no Group: a Retry asks again once the Group reports.
			d.mu.Unlock()
			return http.StatusConflict, map[string]any{"error": "compress is blocked; retry once the Group reports"}
		}
		s.compressWait = true
		s.compressAuto = false
		s.compressInstr = instructions
		d.mu.Unlock()
		return http.StatusAccepted, map[string]any{"ok": true, "waiting": "group"}
	}
	d.mu.Unlock()
	if code, msg := d.resumeReleased(id); code != 0 {
		return code, map[string]any{"error": msg}
	}
	d.mu.Lock()
	if !s.live {
		d.mu.Unlock()
		return http.StatusConflict, map[string]any{"error": "session is resuming; try again"}
	}
	if s.busy {
		d.mu.Unlock()
		return http.StatusConflict, map[string]any{"error": "session is working; compress when it is idle"}
	}
	s.busy = true
	s.active++
	s.swapping = true
	// This run settles any wait that held a compress of the Session.
	s.compressWait = false
	s.compressAuto = false
	s.compressInstr = ""
	d.mu.Unlock()
	go d.runHandoff(id, hb, set.Model, instructions)
	return http.StatusAccepted, map[string]any{"ok": true}
}

// runDeferredCompress runs a compress that waited on a Group, once no Group
// of the Session is outstanding and no turn runs. A Session that turns busy
// again keeps the wait for its next turn end. An automatic wait re-verifies
// the meter first: usage may have fallen since it armed (a manual compress
// ran meanwhile), and a stale auto wait must not spend a Handoff on a small
// context. A manual wait always runs: the operator asked for it.
func (d *Daemon) runDeferredCompress(id string) {
	d.mu.Lock()
	s, ok := d.sessions[id]
	if !ok || !s.compressWait || s.busy || d.groupOutstandingLocked(id) {
		d.mu.Unlock()
		return
	}
	if s.meta.Status == "ended" {
		s.compressWait = false
		s.compressAuto = false
		d.mu.Unlock()
		return
	}
	if s.meta.CompressBlock != "" {
		// A blocked Session starts no deferred compress; a Retry asks again.
		s.compressWait = false
		s.compressAuto = false
		s.compressInstr = ""
		d.mu.Unlock()
		return
	}
	if s.compressAuto {
		events := append([]Event(nil), s.events...)
		d.mu.Unlock()
		if !autoCompressDue(events, d.compressSettingsOf()) {
			d.mu.Lock()
			if cur, ok := d.sessions[id]; ok && cur.compressAuto {
				cur.compressWait = false
				cur.compressAuto = false
				cur.compressInstr = ""
			}
			d.mu.Unlock()
			return
		}
		d.mu.Lock()
		s, ok = d.sessions[id]
		if !ok || !s.compressWait || s.busy || d.groupOutstandingLocked(id) || s.meta.Status == "ended" || s.meta.CompressBlock != "" {
			d.mu.Unlock()
			return
		}
	}
	s.compressWait = false
	s.compressAuto = false
	instructions := s.compressInstr
	s.compressInstr = ""
	d.mu.Unlock()
	code, out := d.requestCompress(id, instructions)
	if code == http.StatusAccepted {
		return
	}
	d.mu.Lock()
	if cur, ok := d.sessions[id]; ok && cur.busy && cur.meta.Status != "ended" {
		// A turn started after the check: wait for its end instead. The
		// wait keeps its manual or automatic kind, so the recheck still
		// applies to an automatic one.
		cur.compressWait = true
		cur.compressInstr = instructions
		d.mu.Unlock()
		return
	}
	d.mu.Unlock()
	msg, _ := out["error"].(string)
	d.appendEvent(id, "error", "compress failed: "+msg, "")
}

// compressPatch is the PUT shape: every field optional.
type compressPatch struct {
	Auto             *bool   `json:"auto_compress"`
	ThresholdPercent *int    `json:"compress_threshold_percent"`
	ThresholdTokens  *int    `json:"compress_threshold_tokens"`
	Cooldown         *int    `json:"compress_cooldown"`
	Instructions     *string `json:"compress_instructions"`
	Model            *string `json:"compress_model"`
}

func (p *compressPatch) validate() string {
	if p == nil {
		return ""
	}
	if p.ThresholdPercent != nil && (*p.ThresholdPercent < 10 || *p.ThresholdPercent > 99) {
		return "compress_threshold_percent must be between 10 and 99"
	}
	if p.ThresholdTokens != nil && *p.ThresholdTokens < 0 {
		return "compress_threshold_tokens must be zero or more"
	}
	if p.Cooldown != nil && *p.Cooldown < 0 {
		return "compress_cooldown must be zero or more"
	}
	if p.Instructions != nil && len(*p.Instructions) > 2000 {
		return "compress_instructions is limited to 2000 characters"
	}
	if p.Model != nil && strings.TrimSpace(*p.Model) != "" && !pigModelAuthed(strings.TrimSpace(*p.Model)) {
		return "compress_model must be an authed pig model"
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
	if p.ThresholdTokens != nil {
		set("compress_threshold_tokens", *p.ThresholdTokens)
	}
	if p.Cooldown != nil {
		set("compress_cooldown", *p.Cooldown)
	}
	if p.Instructions != nil {
		set("compress_instructions", strings.TrimSpace(*p.Instructions))
	}
	if p.Model != nil {
		set("compress_model", strings.TrimSpace(*p.Model))
	}
}

// usageWindowBackend is the optional Backend seam that reports the model's
// context window with no turn running: the pig SDK window, the catalog
// window, or the adapter's known size. The Daemon seeds the meter from it.
type usageWindowBackend interface {
	UsageWindow(model string) int
}

// estimateUsed guesses the context tokens from the Event log at four
// characters per token: prompts and model text so far. It sizes a seed, not
// a gate; real readings replace it.
func estimateUsed(events []Event) int {
	chars := 0
	for _, e := range events {
		switch e.Kind {
		case "prompt", "assistant_chunk", "thinking_chunk":
			chars += len(e.Text)
		}
	}
	return chars / 4
}

// usageWindowLocked resolves the context window for one Session: the live
// pig window first (authoritative past compactions), else the Backend seam.
// Callers hold d.mu: lock order is d.mu before the Backend lock.
func (d *Daemon) usageWindowLocked(id string, meta SessionMeta) int {
	be := d.backendForLocked(meta.Harness)
	if pb, ok := be.(*PigBackend); ok {
		if _, size, ok := pb.ContextUsage(id); ok && size > 0 {
			return size
		}
	}
	if wb, ok := be.(usageWindowBackend); ok {
		return wb.UsageWindow(meta.Model)
	}
	return 0
}

// seedUsage appends an estimated usage Event so the meter shows from Session
// start and from the first prompt dispatch: without it a new Session reads
// "no usage yet" until its first turn ends, and a stalled first turn (seen
// with opencode-go pig models) holds the meter dead the whole turn. It
// never overwrites a real reading: with a usage Event already in the log it
// does nothing, and the auto-compress gates keep reading the newest Event.
func (d *Daemon) seedUsage(id string) {
	d.mu.Lock()
	s, ok := d.sessions[id]
	if !ok {
		d.mu.Unlock()
		return
	}
	if _, _, ok := usageOf(s.events); ok {
		d.mu.Unlock()
		return
	}
	size := d.usageWindowLocked(id, s.meta)
	used := estimateUsed(s.events)
	d.mu.Unlock()
	if size <= 0 {
		return
	}
	d.appendEvent(id, "usage", usageText(used, size), "")
}
