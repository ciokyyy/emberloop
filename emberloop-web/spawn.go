package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SpawnAgent is one requested Subagent: its first prompt plus its kind.
// Type is worker or scout; empty means worker. Scouts are read-only.
type SpawnAgent struct {
	Prompt string `json:"prompt"`
	Type   string `json:"type,omitempty"`
}

// spawnGroup is the Subagents of one spawn call: the parent they report to
// and every member in spawn order. Delivered marks the grouped result as
// sent, so a restart or a racing finish never sends it twice.
type spawnGroup struct {
	ID        string   `json:"id"`
	Parent    string   `json:"parent"`
	Members   []string `json:"members"`
	Delivered bool     `json:"delivered"`
}

// Spawn creates one Subagent Session per requested agent under parentID.
// Each child inherits the parent's harness, model and working directory,
// records its parent and Group, and runs its prompt at once. The shared
// Group delivers one grouped result to the parent when the last member
// finishes. It returns the HTTP status to answer with when non-zero.
func (d *Daemon) Spawn(parentID string, agents []SpawnAgent) (string, []SessionMeta, int, string) {
	d.mu.Lock()
	parent, ok := d.sessions[parentID]
	if !ok {
		d.mu.Unlock()
		return "", nil, http.StatusNotFound, "not found"
	}
	if parent.meta.Status == "ended" {
		d.mu.Unlock()
		return "", nil, http.StatusGone, "session ended"
	}
	if parent.meta.Scout {
		d.mu.Unlock()
		return "", nil, http.StatusForbidden, "scout sessions are read-only"
	}
	for _, a := range agents {
		if a.Prompt == "" {
			d.mu.Unlock()
			return "", nil, http.StatusBadRequest, "each agent needs a prompt"
		}
		if t := a.Type; t != "" && t != "worker" && t != "scout" {
			d.mu.Unlock()
			return "", nil, http.StatusBadRequest, "unknown type (want worker or scout)"
		}
	}
	// Reserve every child slot before starting any: over the cap the whole
	// call is refused and no half Group is left behind.
	max := d.maxRunningLocked()
	if d.liveCountLocked()+d.starting+len(agents) > max {
		running := d.liveCountLocked()
		d.mu.Unlock()
		return "", nil, http.StatusTooManyRequests, capMessage(running, max)
	}
	harness, model, workDir := parent.meta.Harness, parent.meta.Model, parent.meta.WorkDir
	d.mu.Unlock()

	gid := newID()
	members := make([]string, len(agents))
	metas := make([]SessionMeta, len(agents))
	for i, a := range agents {
		members[i] = newID()
		h, m := harness, model
		role := "worker"
		if a.Type == "scout" {
			role = "scout"
		}
		if rh, rm := roleFor(role); rh != "" {
			h, m = rh, rm
		}
		metas[i] = SessionMeta{
			ID:        members[i],
			Harness:   h,
			Model:     m,
			WorkDir:   workDir,
			ProjectID: parent.meta.ProjectID,
			Status:    "idle",
			CreatedAt: time.Now().UTC(),
			ParentID:  parentID,
			GroupID:   gid,
			Scout:     a.Type == "scout",
		}
	}
	// Publish the Group before any child can finish: a fast child must
	// find its Group when its first turn settles. The pointer stays local
	// so the persist below never reads the map outside the lock.
	g := &spawnGroup{ID: gid, Parent: parentID, Members: append([]string(nil), members...)}
	d.mu.Lock()
	d.groups[gid] = g
	d.mu.Unlock()
	d.persistGroup(g)

	type started struct {
		meta SessionMeta
		be   Backend
	}
	okStarts := make([]started, 0, len(agents))
	for i := range metas {
		be, serr := d.startSession(metas[i])
		if serr != nil {
			// Best-effort rollback: close what started, drop the Group.
			for _, s := range okStarts {
				if s.be != nil {
					s.be.CloseSession(s.meta.ID)
				}
				d.removeSession(s.meta.ID)
			}
			d.mu.Lock()
			delete(d.groups, gid)
			d.mu.Unlock()
			_ = os.Remove(d.groupFile(gid))
			return "", nil, serr.code, serr.msg
		}
		okStarts = append(okStarts, started{meta: metas[i], be: be})
	}
	for _, s := range okStarts {
		d.persistResumeToken(s.be, s.meta.ID)
		d.appendEvent(s.meta.ID, "status", "", "idle")
		d.appendEvent(parentID, "subagent_spawned", s.meta.ID, "")
	}
	// Launch each first prompt like an idle send: busy and active under
	// the lock, the turn on its own goroutine.
	for i, s := range okStarts {
		d.mu.Lock()
		if cs, ok := d.sessions[s.meta.ID]; ok {
			cs.busy = true
			cs.active++
		}
		d.mu.Unlock()
		d.appendModeEvent(s.meta.ID, "prompt", agents[i].Prompt, "", "")
		go d.runTurn(s.meta.ID, s.be, agents[i].Prompt, nil)
	}
	d.mu.Lock()
	kids := make([]SessionMeta, 0, len(members))
	for _, m := range members {
		if s, ok := d.sessions[m]; ok {
			kids = append(kids, s.meta)
		}
	}
	d.mu.Unlock()
	return gid, kids, 0, ""
}

// handleSpawn serves POST /api/sessions/{id}/spawn.
func (d *Daemon) handleSpawn(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Agents []SpawnAgent `json:"agents"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Agents) == 0 {
		http.Error(w, "agents is required", http.StatusBadRequest)
		return
	}
	gid, kids, code, msg := d.Spawn(id, req.Agents)
	if code != 0 {
		if code == http.StatusNotFound {
			http.NotFound(w, r)
			return
		}
		if code == http.StatusForbidden || code == http.StatusTooManyRequests {
			writeJSON(w, code, map[string]any{"error": msg})
			return
		}
		http.Error(w, msg, code)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"group": gid, "sessions": kids})
}

// SteerChild messages one Subagent on behalf of its parent: a running child
// is steered at its next safe step, an idle one starts as a follow-up, and a
// released one resumes first. Only the recorded parent may message a child;
// scouts stay read-only. It reuses resumeReleased plus routeLive, the same
// path sendMessage serves the UI with, so no queue logic is duplicated. It
// returns the tool result text on success, else the HTTP code and why.
func (d *Daemon) SteerChild(parentID, childID, text string) (string, int, string) {
	if strings.TrimSpace(text) == "" {
		return "", http.StatusBadRequest, "message is required"
	}
	d.mu.Lock()
	child, ok := d.sessions[childID]
	if !ok {
		d.mu.Unlock()
		return "", http.StatusNotFound, "not found"
	}
	if child.meta.ParentID != parentID {
		d.mu.Unlock()
		return "", http.StatusForbidden, "not a child of this session"
	}
	if child.meta.Scout {
		d.mu.Unlock()
		return "", http.StatusForbidden, "scout sessions are read-only"
	}
	if child.meta.Status == "ended" {
		d.mu.Unlock()
		return "", http.StatusGone, "session ended"
	}
	d.mu.Unlock()
	if code, msg := d.resumeReleased(childID); code != 0 {
		return "", code, msg
	}
	d.mu.Lock()
	s, ok := d.sessions[childID]
	if !ok {
		d.mu.Unlock()
		return "", http.StatusNotFound, "not found"
	}
	mode := "followup"
	if s.busy {
		mode = "steer"
	}
	d.mu.Unlock()
	res, code := d.routeLive(childID, mode, text)
	if code != http.StatusAccepted {
		if code == http.StatusNotFound {
			return "", http.StatusNotFound, "not found"
		}
		if code == http.StatusGone {
			return "", http.StatusGone, "session ended"
		}
		return "", code, "session is resuming; send the message again"
	}
	if !res.queued {
		if mode == "steer" {
			return fmt.Sprintf("started steer on %s (seq %d)", childID, res.seq), 0, ""
		}
		return fmt.Sprintf("started follow-up on %s (seq %d)", childID, res.seq), 0, ""
	}
	if res.seq != 0 {
		if mode == "steer" {
			return fmt.Sprintf("steered %s (queued natively)", childID), 0, ""
		}
		return fmt.Sprintf("queued as follow-up on %s", childID), 0, ""
	}
	d.mu.Lock()
	eff := mode
	if cur, ok := d.sessions[childID]; ok {
		if res.position > 0 && res.position <= len(cur.pending) {
			eff = cur.pending[res.position-1].mode
		} else {
			for i := len(cur.pending) - 1; i >= 0; i-- {
				if cur.pending[i].text == text {
					eff = cur.pending[i].mode
					break
				}
			}
		}
	}
	pos := res.position
	d.mu.Unlock()
	if eff == "steer" {
		return fmt.Sprintf("steered %s (queued at position %d)", childID, pos), 0, ""
	}
	return fmt.Sprintf("queued as follow-up on %s (position %d)", childID, pos), 0, ""
}

// CheckChildren peeks at the caller's own Subagents without messaging them:
// one summary line per child, oldest first. Only the recorded parent may
// peek; scouts have no children, so theirs is always empty. Results already
// arrive as one grouped result, so callers use check to peek, never to poll.
func (d *Daemon) CheckChildren(parentID string) (string, int, string) {
	d.mu.Lock()
	type snap struct {
		meta   SessionMeta
		events []Event
		live   bool
	}
	var kids []snap
	for _, s := range d.sessions {
		if s.meta.ParentID == parentID {
			kids = append(kids, snap{s.meta, append([]Event(nil), s.events...), s.live})
		}
	}
	d.mu.Unlock()
	sort.Slice(kids, func(i, j int) bool { return kids[i].meta.CreatedAt.Before(kids[j].meta.CreatedAt) })
	if len(kids) == 0 {
		return "no subagents", 0, ""
	}
	var b strings.Builder
	for _, k := range kids {
		b.WriteString(summarizeChild(k.meta, k.events, k.live))
		b.WriteByte('\n')
	}
	return strings.TrimSuffix(b.String(), "\n"), 0, ""
}

// CheckChild peeks at one of the caller's own Subagents: its summary plus
// the last twenty Events, one line each. A non-child id is refused; scout
// children read fine, since a peek is not a message.
func (d *Daemon) CheckChild(parentID, childID string) (string, int, string) {
	d.mu.Lock()
	s, ok := d.sessions[childID]
	if !ok {
		d.mu.Unlock()
		return "", http.StatusNotFound, "not found"
	}
	if s.meta.ParentID != parentID {
		d.mu.Unlock()
		return "", http.StatusForbidden, "not a child of this session"
	}
	meta, events, live := s.meta, append([]Event(nil), s.events...), s.live
	d.mu.Unlock()
	var b strings.Builder
	b.WriteString(summarizeChild(meta, events, live))
	for _, l := range recentEventLines(events, 20) {
		b.WriteByte('\n')
		b.WriteString(l)
	}
	return b.String(), 0, ""
}

// CancelChild stops one of the caller's own Subagents without touching the
// parent turn: it reuses interrupt, the same stop the UI route uses, so the
// child settles idle with its queue dropped. Kill also ends the child
// Session through endSession, the same end DELETE serves. A non-child id is
// refused; scouts may be stopped, since a stop is not a message.
func (d *Daemon) CancelChild(parentID, childID string, kill bool) (string, int, string) {
	d.mu.Lock()
	child, ok := d.sessions[childID]
	if !ok {
		d.mu.Unlock()
		return "", http.StatusNotFound, "not found"
	}
	if child.meta.ParentID != parentID {
		d.mu.Unlock()
		return "", http.StatusForbidden, "not a child of this session"
	}
	if child.meta.Status == "ended" {
		d.mu.Unlock()
		return "", http.StatusGone, "session ended"
	}
	d.mu.Unlock()
	stopped, code := d.interrupt(childID)
	if code != http.StatusOK {
		if code == http.StatusNotFound {
			return "", http.StatusNotFound, "not found"
		}
		return "", http.StatusGone, "session ended"
	}
	if !kill {
		if stopped {
			return fmt.Sprintf("interrupted %s", childID), 0, ""
		}
		return fmt.Sprintf("%s already idle", childID), 0, ""
	}
	if _, code := d.endSession(childID); code != http.StatusOK {
		if code == http.StatusNotFound {
			return "", http.StatusNotFound, "not found"
		}
		return "", code, "session ended"
	}
	return fmt.Sprintf("killed %s", childID), 0, ""
}

// endSession ends one Session the way DELETE /api/sessions/{id} does: it
// marks ended under the lock, closes a live agent process outside it, and
// appends the ended Event. Two concurrent ends emit one ended Event.
func (d *Daemon) endSession(id string) (SessionMeta, int) {
	d.mu.Lock()
	s, ok := d.sessions[id]
	if !ok {
		d.mu.Unlock()
		return SessionMeta{}, http.StatusNotFound
	}
	if s.meta.Status == "ended" {
		m := s.meta
		d.mu.Unlock()
		return m, http.StatusOK
	}
	s.meta.Status = "ended"
	wasLive := s.live
	s.live = false
	d.persistMeta(s.meta)
	be := d.backendForLocked(s.meta.Harness)
	d.mu.Unlock()
	if wasLive && be != nil {
		be.CloseSession(id)
	}
	d.appendEvent(id, "status", "", "ended")
	d.mu.Lock()
	m := d.sessions[id].meta
	d.mu.Unlock()
	return m, http.StatusOK
}

// summarizeChild renders one peek line: id, kind, status, the running tool
// and token count when the log shows them, and the last assistant reply tail.
func summarizeChild(meta SessionMeta, events []Event, live bool) string {
	kind := "worker"
	if meta.Scout {
		kind = "scout"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s (%s): status=%s", shortID(meta.ID), kind, displayStatus(meta, live))
	if tool := currentToolOf(events); tool != "" {
		fmt.Fprintf(&b, " tool=%s", tool)
	}
	if toks, ok := tokensOf(events); ok {
		fmt.Fprintf(&b, " tokens=%d", toks)
	}
	if tail := assistantTail(events, 300); tail != "" {
		fmt.Fprintf(&b, " reply=%s", tail)
	}
	return b.String()
}

// displayStatus names the peek status: ended wins, a Session holding no
// agent process is released, otherwise the recorded turn status.
func displayStatus(meta SessionMeta, live bool) string {
	if meta.Status == "ended" {
		return "ended"
	}
	if !live {
		return "released"
	}
	return meta.Status
}

// currentToolOf names the tool still running: the newest tool_call_started
// whose id no later tool_call_updated completed or failed. Anything
// unparseable or settled reports nothing.
func currentToolOf(events []Event) string {
	done := map[string]bool{}
	for _, e := range events {
		if e.Kind != "tool_call_updated" {
			continue
		}
		var tc ToolCall
		if json.Unmarshal([]byte(e.Text), &tc) != nil {
			continue
		}
		if tc.Status == "completed" || tc.Status == "failed" {
			done[tc.ID] = true
		}
	}
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Kind != "tool_call_started" {
			continue
		}
		var tc ToolCall
		if json.Unmarshal([]byte(events[i].Text), &tc) != nil || tc.Name == "" {
			return ""
		}
		if !done[tc.ID] {
			return tc.Name
		}
		return ""
	}
	return ""
}

// tokensOf reports the newest usage count in the log, false when none.
func tokensOf(events []Event) (int64, bool) {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Kind != "usage" {
			continue
		}
		var u struct {
			Used int64 `json:"used"`
		}
		if json.Unmarshal([]byte(events[i].Text), &u) != nil {
			return 0, false
		}
		return u.Used, true
	}
	return 0, false
}

// assistantTail joins the assistant chunks after the last prompt and keeps
// the last n characters, so a peek shows how the child answered.
func assistantTail(events []Event, n int) string {
	start := 0
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Kind == "prompt" {
			start = i + 1
			break
		}
	}
	var b strings.Builder
	for _, e := range events[start:] {
		if e.Kind == "assistant_chunk" {
			b.WriteString(e.Text)
		}
	}
	tail := strings.Join(strings.Fields(b.String()), " ")
	if len(tail) > n {
		tail = tail[len(tail)-n:]
	}
	return tail
}

// recentEventLines renders the last n Events, one compact line each.
func recentEventLines(events []Event, n int) []string {
	if len(events) > n {
		events = events[len(events)-n:]
	}
	out := make([]string, 0, len(events))
	for _, e := range events {
		out = append(out, eventLine(e))
	}
	return out
}

// eventLine compacts one Event: its seq, kind, delivery mode when set, and
// the text flattened to one line (status Events show their status).
func eventLine(e Event) string {
	text := strings.Join(strings.Fields(e.Text), " ")
	if e.Kind == "status" {
		text = e.Status
	}
	if len(text) > 120 {
		text = text[:120] + "…"
	}
	if e.Mode != "" {
		return fmt.Sprintf("#%d %s/%s %s", e.Seq, e.Kind, e.Mode, text)
	}
	return fmt.Sprintf("#%d %s %s", e.Seq, e.Kind, text)
}

// maybeDeliverGroup sends the Group's single grouped result when id's turn
// settled it as the last unfinished member. The result goes to the parent
// as a Steer, so a working parent queues it and an idle one runs it at once.
func (d *Daemon) maybeDeliverGroup(id string) {
	d.mu.Lock()
	s, ok := d.sessions[id]
	if !ok || s.meta.GroupID == "" {
		d.mu.Unlock()
		return
	}
	g, ok := d.groups[s.meta.GroupID]
	if !ok || g.Delivered {
		d.mu.Unlock()
		return
	}
	for _, m := range g.Members {
		ms, ok := d.sessions[m]
		if !ok {
			continue
		}
		if ms.busy || len(ms.pending) > 0 || ms.active > 0 {
			d.mu.Unlock()
			return
		}
		switch ms.meta.Status {
		case "idle", "failed", "ended":
		default:
			d.mu.Unlock()
			return
		}
	}
	g.Delivered = true
	d.persistGroup(g)
	parent, gid := g.Parent, g.ID
	d.mu.Unlock()

	d.appendEvent(parent, "subagent_finished", gid, "")
	text := d.groupResultText(gid)
	// A settled parent released its agent; resume it so the Steer has a
	// process to run on. A refusal (cap, ended, unwired) drops the
	// delivery: the Group stays marked, exactly once either way.
	if code, _ := d.resumeReleased(parent); code != 0 {
		return
	}
	d.routeLive(parent, "steer", text)
}

// groupResultText combines every member's result, oldest member first, into
// the one message the parent receives. A member that submitted reports its
// submit text; one that never submitted reports its last assistant message
// (the whole final reply, not chunk fragments); a failed one keeps its mark,
// and a member that gave up without submitting (gate bound, flag on) is
// marked. Callers hold no lock; it takes its own.
func (d *Daemon) groupResultText(gid string) string {
	d.mu.Lock()
	g, ok := d.groups[gid]
	if !ok {
		d.mu.Unlock()
		return "Subagents finished."
	}
	members := append([]string(nil), g.Members...)
	d.mu.Unlock()
	var b strings.Builder
	fmt.Fprintf(&b, "Subagents %s finished:\n", gid)
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, m := range members {
		reply := "(no reply)"
		kind := "worker"
		if s, ok := d.sessions[m]; ok {
			if s.meta.Scout {
				kind = "scout"
			}
			if rec := d.submits[m]; rec != nil && rec.Status != "" && rec.Status != "stopped" {
				reply = submitResultText(rec)
			} else if last := lastAssistantReply(s.events); last != "" {
				reply = last
			}
			if s.meta.Status == "failed" {
				reply += " [failed]"
			}
			if rec := d.submits[m]; rec != nil && rec.Status == "stopped" {
				// The gate's give-up (flag on): the last turn answered a
				// reminder, so report the whole transcript like before,
				// marked, rather than the reminder reply alone.
				if whole := wholeTranscriptReply(s.events); whole != "" {
					reply = whole
				}
				reply += " [stopped without submitting]"
			}
		} else {
			reply = "(session gone)"
		}
		fmt.Fprintf(&b, "- %s (%s): %s\n", shortID(m), kind, reply)
	}
	return b.String()
}

// submitResultText renders one spawn child's submit record as its Group
// result: the report when it has one, else the completed checkbox texts,
// else the bare status.
func submitResultText(rec *submitRec) string {
	if rec.Report != "" {
		return rec.Report
	}
	if len(rec.Completed) > 0 {
		return strings.Join(rec.Completed, "; ")
	}
	if rec.Status != "" {
		return rec.Status
	}
	return "(empty submit)"
}

// lastAssistantReply joins the assistant chunks after the last prompt into
// the child's whole final reply, flattened to one line. Empty when the
// child said nothing since its last prompt. Chunks stream per delta, so the
// reply is every chunk joined, not the last chunk alone.
func lastAssistantReply(events []Event) string {
	start := 0
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Kind == "prompt" {
			start = i + 1
			break
		}
	}
	var b strings.Builder
	for _, e := range events[start:] {
		if e.Kind == "assistant_chunk" {
			b.WriteString(e.Text)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// wholeTranscriptReply joins every assistant chunk in the log, the way the
// grouped result read before settle delivery: a child re-prompted by the
// submit gate still reports its original answer, not just its reply to the
// last reminder.
func wholeTranscriptReply(events []Event) string {
	var b strings.Builder
	for _, e := range events {
		if e.Kind == "assistant_chunk" {
			b.WriteString(e.Text)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// --- group persistence: stateDir/enderbloom/groups/{id}.json ---

func (d *Daemon) groupFile(gid string) string {
	return filepath.Join(d.stateDir, "enderbloom", "groups", gid+".json")
}

func (d *Daemon) persistGroup(g *spawnGroup) {
	dir := filepath.Join(d.stateDir, "enderbloom", "groups")
	_ = os.MkdirAll(dir, 0o755)
	b, _ := json.Marshal(g)
	tmp, err := os.CreateTemp(dir, "group-*.json")
	if err != nil {
		return
	}
	name := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return
	}
	_ = os.Rename(name, d.groupFile(g.ID))
}

func (d *Daemon) loadGroups() {
	root := filepath.Join(d.stateDir, "enderbloom", "groups")
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, en := range entries {
		if en.IsDir() || !strings.HasSuffix(en.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(root, en.Name()))
		if err != nil {
			continue
		}
		var g spawnGroup
		if err := json.Unmarshal(raw, &g); err != nil || g.ID == "" {
			continue
		}
		gc := g
		d.groups[g.ID] = &gc
	}
}
