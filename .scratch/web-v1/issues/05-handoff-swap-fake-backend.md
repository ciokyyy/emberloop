# 05: Handoff swap on the fake Backend

**What to build:** A Compress button on a Session. Pressing it builds a deterministic Handoff from the Event log (no LLM), starts a fresh harness handle for the same Session, and sends the Handoff as its first prompt. The Session ID and full history stay. Carried over from the old Handoff compress spec. The `sender=system` and `owner` assertions for the Handoff prompt live in ticket 02.

**Blocked by:** none

**Status:** done

- [x] `POST` compress rotates the resume token and keeps the Session ID and all prior Events
- [x] The Handoff prompt Event is flagged as a Handoff and renders as a collapsed block, not a user message
- [x] A `compressed` Event records compress number, tokens before, model and fallback step
- [x] The Handoff is built without an LLM
- [x] A copy of the Handoff is stored in the Session directory per compress
- [x] The old handle is released only after the first turn on the new handle starts cleanly
- [x] The context meter resets from the next `usage` Event
- [x] Go tests at the Daemon HTTP seam cover each case
