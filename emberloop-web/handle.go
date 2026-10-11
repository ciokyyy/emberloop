package web

import "strings"

// handleOp names the start a harness handle serves: a new Session or
// Subagent, a resume of a released Session, or a Handoff swap on a live one.
type handleOp int

const (
	handleCreate handleOp = iota
	handleResume
	handleCompress
)

// handle is the identity one harness handle starts with. startHandle derives
// it from the Session record on every create, resume and compress. No Backend
// keeps identity state of its own, so a new handle never inherits a stale one.
type handle struct {
	// owner is the User who created the Session. A resume-token rotation keeps it.
	owner string
	// role is the el MCP role, which sets the tool set: web, web-worker,
	// run-worker or scout (elRoleFor).
	role string
	// inject holds the memory texts the next turn carries ahead of its
	// prompt, in order: Profile, then recall. Empty until memory exists.
	inject []string
}

// memoryInjections is the injection hook. It returns the memory texts for one
// start. Memory is not built yet, so it returns nothing and every start is a
// no-op for injections. Tests replace it.
var memoryInjections = func(meta SessionMeta, op handleOp) []string { return nil }

// startHandle derives the owner, role and injections of one handle start from
// the Session record. Create, resume and compress all call it.
func startHandle(meta SessionMeta, op handleOp) handle {
	return handle{owner: meta.Owner, role: elRoleFor(meta), inject: memoryInjections(meta, op)}
}

// takeInjection returns the memory texts that the next turn of the Session
// carries ahead of its prompt, joined in order, and clears them. A fresh
// handle sets them once, so they go out once per start.
func (d *Daemon) takeInjection(id string) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	s, ok := d.sessions[id]
	if !ok || len(s.inject) == 0 {
		return ""
	}
	text := strings.Join(s.inject, "\n\n")
	s.inject = nil
	return text
}
