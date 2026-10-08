package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
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
		go d.runTurn(s.meta.ID, s.be, agents[i].Prompt)
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

// groupResultText combines every member's last assistant reply, oldest
// member first, into the one message the parent receives.
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
			// Chunks stream per delta, so the reply is every assistant
			// chunk of the last turn joined, not the last chunk alone.
			start := 0
			for i := len(s.events) - 1; i >= 0; i-- {
				if s.events[i].Kind == "prompt" {
					start = i + 1
					break
				}
			}
			var rb strings.Builder
			for _, e := range s.events[start:] {
				if e.Kind == "assistant_chunk" {
					rb.WriteString(e.Text)
				}
			}
			if rb.Len() > 0 {
				reply = rb.String()
			}
			if s.meta.Status == "failed" {
				reply += " [failed]"
			}
		} else {
			reply = "(session gone)"
		}
		fmt.Fprintf(&b, "- %s (%s): %s\n", shortID(m), kind, reply)
	}
	return b.String()
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
