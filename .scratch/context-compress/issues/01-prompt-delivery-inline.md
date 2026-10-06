# 01: Prompts reach agents as message text

**What to build:** Every steering and follow-up prompt wick sends to a running agent arrives as the message itself, so the agent spends no turn reading a file. This covers reviewer follow-up reviews and steering of an already running agent. A single shared delivery helper sends the text; compress, late prompts and these cases all use it. The incremental review diff goes inline in the follow-up, always, with no size cap and no path. The prompt and brief files are still written, but only as archive copies; no message tells an agent to read them.

**Blocked by:** None (can start immediately)

**Status:** done

- [x] A reviewer follow-up sends the full follow-up text, including the incremental diff, as the message; no message contains a "read this file" instruction
- [x] Steering a running agent sends the prompt text as the message
- [x] One shared helper delivers prompt text; existing late-prompt delivery uses it
- [x] Archive copies of prompts are still written
- [x] Tests through the existing tell and nudge seams assert the text sent, never a file path
- [x] Spec stories 18, 27, 28, 29

Spec: `.scratch/context-compress/spec.md`. Read it for the decisions this ticket implements and its Out of Scope section.
