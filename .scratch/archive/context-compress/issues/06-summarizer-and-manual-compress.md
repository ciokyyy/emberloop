# 06: `el compress` summarizes and restarts a session

**What to build:** A new `compressModel` setting per harness names the model for compression. `el compress <pane>` renders the session transcript as text in full (user messages, assistant text, tool calls and results), runs a pig one-off with no tools using the pinned combined prompt (Matt Pocock handoff structure with caveman ultra for prose; code, paths, errors and IDs verbatim), writes the handoff, and hands it to the supervisor. The summary must pass a sanity check (non-empty, expected sections) before the swap; on failure the old session stays untouched. The old transcript and the handoff copy are kept. If the transcript exceeds the compress model's window, or `compressModel` is unset, compression is refused with a clear message. Settings validation checks the window is large enough.

**Blocked by:** 05-supervisor-restart-loop

**Status:** done

- [x] `compressModel` setting per harness, in the settings CLI
- [x] Combined prompt is a pinned copy stored with el, not read from the skill library at runtime
- [x] Summarizer runs as a pig one-off with no tools and no extra system prompt
- [x] Failed or empty summary: old session continues, no restart
- [x] Handoff copy and old transcript kept for audit and rollback
- [x] Refusal with a message for unset model or input larger than the window
- [x] After an interrupted step the handoff ends with a line to continue it; a manual compress resumes only if a task was open
- [x] Tests with a fake summarizer through the exec seam
- [x] Spec stories 18 to 22

Spec: `.scratch/archive/context-compress/spec.md`. Read it for the decisions this ticket implements and its Out of Scope section.
