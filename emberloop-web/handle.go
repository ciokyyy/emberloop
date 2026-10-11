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
	// inject is the memory bundle the next turn carries ahead of its
	// prompt. Empty until memory exists.
	inject injection
}

// injection is one memory bundle: the texts the next turn carries ahead of
// its prompt, in order (Profile, then recall), with the Profile version and
// Rule refs the injection Event records. The three travel as one value so
// the texts and their provenance cannot disagree: an injection Event always
// describes exactly what the model saw.
type injection struct {
	// texts holds the memory texts, in order: Profile, then recall.
	texts []string
	// profile is the Profile version the texts carry, 0 when none.
	profile int
	// refs are the Rule IDs and versions the texts carry. Empty until
	// recall exists.
	refs []InjectedRule
}

// empty reports whether the bundle carries nothing.
func (in injection) empty() bool {
	return len(in.texts) == 0 && in.profile == 0 && len(in.refs) == 0
}

// block joins the texts for the turn, in order.
func (in injection) block() string { return strings.Join(in.texts, "\n\n") }

// prepend puts Profile texts first and records their version. The Daemon
// calls it with profileBlockFor at every handle start.
func (in *injection) prepend(texts []string, profile int) {
	in.texts = append(texts, in.texts...)
	in.profile = profile
}

// appendRecall adds the recall block after the Profile and records its Rule
// refs. The recall block carries no preamble of its own: when the bundle has
// no Profile the caller leads it with the preamble so every injection still
// opens with the precedence rules.
func (in *injection) appendRecall(texts []string, refs []InjectedRule) {
	if len(texts) == 0 {
		return
	}
	if in.profile == 0 {
		texts[0] = profilePreamble + "\n\n" + texts[0]
	}
	in.texts = append(in.texts, texts...)
	in.refs = append(in.refs, refs...)
}

// memoryInjection is the injection hook. It returns the memory bundle for
// one start. Production recall is built by recallBlockFor at turn time from
// the task prompt (create) or the Handoff (compress), so the hook stays empty
// outside tests. Tests replace it to simulate Profile and recall texts.
var memoryInjection = func(meta SessionMeta, op handleOp) injection { return injection{} }

// startHandle derives the owner, role and injections of one handle start from
// the Session record. Create, resume and compress all call it.
func startHandle(meta SessionMeta, op handleOp) handle {
	return handle{owner: meta.Owner, role: elRoleFor(meta), inject: memoryInjection(meta, op)}
}

// takeInjection returns the memory bundle that the next turn of the Session
// carries ahead of its prompt, with the op that set it, and clears both. A
// fresh handle sets them once, so they go out once per start. The op tells
// the turn whether recall applies: create turns build fresh recall from the
// prompt, resume turns carry the Profile only, compress turns already carry
// fresh recall from the Handoff.
func (d *Daemon) takeInjection(id string) (injection, handleOp) {
	d.mu.Lock()
	defer d.mu.Unlock()
	s, ok := d.sessions[id]
	if !ok {
		return injection{}, handleResume
	}
	in, op := s.inject, s.injectOp
	s.inject = injection{}
	s.injectOp = handleResume
	return in, op
}

