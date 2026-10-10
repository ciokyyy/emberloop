# 01: Handoff swap on the fake Backend

**What to build:** A Compress button on a Session. Pressing it builds a deterministic Handoff from the Event log (no LLM), starts a fresh harness handle for the same Session, and sends the Handoff as its first prompt. The Session ID and full history stay. The UI shows a collapsed Handoff block and a `compressed` divider. The first slice runs end to end on the fake Backend.

**Blocked by:** None (can start immediately)

**Status:** done

- [x] `POST` compress on a Session rotates the resume token and keeps the Session ID and all prior Events
- [x] The Handoff prompt Event is flagged as a Handoff and renders as a collapsed block, not a user message
- [x] A `compressed` Event records compress number, tokens before, model and fallback step
- [x] The Handoff is built without an LLM: first user prompt, last 3 user prompts, files touched, last assistant message, open tool errors
- [x] A copy of the Handoff is stored in the Session directory per compress
- [x] The old handle is released only after the first turn on the new handle starts cleanly
- [x] The context meter resets from the next `usage` Event
- [x] Go tests at the Daemon HTTP seam against the fake Backend cover each case above
