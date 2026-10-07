# 14: Model windows resolve for provider-prefixed names, and a refused auto-compress is visible

**What to build:** Auto-compress never runs for a worker on a large-window model because the window lookup misses the model name. `ModelWindow` matches the model name by prefix against a small family table. The compress model `openrouter/deepseek/deepseek-v4-flash-0731` and the worker model `antigravity-gemini-3.8-flash` match no entry, so both fall back to `DefaultContextWindow` (200k). `SummarizeSession` then refuses a transcript past that window (`transcript is ~N tokens, past the "..." window 200000: compression refused`), `recordAutoFailure` parks the pane for 10 minutes, and nothing tells the user. The strip also shows the wrong window for the worker (`210k / 200k`, 105%) when the real window is 1M. Fix all three parts in this ticket.

Evidence from the live check: ticket 13's worker (session `ab2572d7`) sat at 228,364 tokens (114% of the 200k threshold) with 0 outstanding tool calls, and `~/.pig/state/emberloop/compress/` was never created. Vendor docs give about 1M tokens for DeepSeek V4 Flash 0731 (OpenRouter) and Gemini 3.8 Flash (Google AI docs).

1. **Name matching.** The window lookup strips a leading `provider/` path (any number of segments) and a harness prefix such as `antigravity-`, then matches the family table, case-insensitive, longest match wins. Add `deepseek-v4` (1M) and `gemini-` already covers Gemini once the prefix is stripped. A settings override in `context.windows` still wins over the table. An unknown model still gets the default.
2. **Seed the settings.** Document and test `context.windows` overrides for the two models above so an unlisted model can be fixed without code. The strip and the compress check read the same resolved window.
3. **Visible refusal.** An auto-compress that is refused or fails (window, summarizer error, restart error) leaves a trace: the strip shows a short `compress failed: <reason>` state for the pane until the next attempt or success, and the reason is appended to the agent's tools log or state. The 10-minute failure cooldown stays. A success clears the state.

**Blocked by:** None (can start immediately)

**Status:** done

- [x] `ModelWindow("openrouter/deepseek/deepseek-v4-flash-0731", nil)` and `ModelWindow("antigravity-gemini-3.8-flash", nil)` return 1,000,000
- [x] Table test for provider-prefixed, harness-prefixed, upper-case and unknown names; unknown names still return the default
- [x] A `context.windows` override beats the table for the exact model name
- [x] The strip shows the 1M window for the Gemini worker (about 22% at 228k, not 114%)
- [x] A 228k transcript with the compress model above passes the window check in `SummarizeSession` and reaches the summarizer call (fake exec)
- [x] A refused or failed auto-compress shows a `compress failed` state in the strip and records its reason; a later success clears it
- [x] The failure cooldown still parks the pane; the state text does not trigger new model calls
- [x] Docs: `emberloop/AGENTS.md` names the window rules and the override setting

Spec: `.scratch/input-queue/spec.md`. Read it for the decisions this ticket implements and its Out of Scope section. Context: `.scratch/context-compress/spec.md` (auto-compress).
