# 02: El can report an agent's context use

**What to build:** For a claude or pig session, el computes the current context size (latest request input plus cache read plus cache write plus that message's output), the number of tool calls still outstanding, the size of tool results per tool name, and the model. A model window table gives each model's window, with settings overrides. New settings carry the compress threshold (default 200k, global with a per-harness override) and window overrides. A `el` command prints these numbers for a session, so the slice is verifiable on its own. Both harness parsers feed one shared formula.

**Blocked by:** None (can start immediately)

**Status:** done

- [x] Context size uses the shared formula for claude and pig transcripts, tested on fixtures
- [x] Outstanding tool calls are detected: a tool call without its result counts, a completed one does not
- [x] Top three tools by result size are reported
- [x] Window table with settings override; threshold setting default 200k with per-harness override
- [x] A `el` command prints context size, window, percent, outstanding tools and top tools for a session
- [x] A new harness needs only a parser to plug in
- [x] Spec stories 1, 2, 4, 5, 11, 30

Spec: `.scratch/archive/context-compress/spec.md`. Read it for the decisions this ticket implements and its Out of Scope section.
