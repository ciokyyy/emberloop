# 01: One-off session titles for the left panel

**What to build:** Sessions get a short title shown in the left panel instead of `harness · id8`. Claude Sessions title through a one-off ACP call (haiku, string system prompt, no tools, no MCP). Pig Sessions title through a bare SDK completion (`ai.StreamSimple`, tiny `MaxTokens`, no Runtime, no tools). Titling runs in the background after the first prompt; any failure leaves the current label.

**Blocked by:** None

**Status:** done

## Findings (live probe, 2026-10-09, no repo code changed)

Probe: Python script outside the repo, pinned `claude-agent-acp@0.88.0` over stdio, `model: haiku` (resolved to `claude-haiku-5-5`), `tools: []`, no MCP servers, one title prompt.

| | string `_meta.systemPrompt` | default `claude_code` preset |
|---|---|---|
| reply | `Retry with backoff for Go HTTP client` | `Here's a title for this chat:\n\n**Add retry ... daemon.go**` |
| tokens (total) | 630 (17 out) | 2526 (37 out) |
| reported cost | $0.00013 | $0.00052 |
| `session/new` | 0.43s | 0.42s |
| `session/prompt` | 1.07s | 1.10s |

- A string `systemPrompt` replaces the preset: about 4x fewer tokens, and the model obeys the "title only" instruction. Use it.
- Cold one-off total: adapter spawn + `initialize` 1.5s (npx cache warm) + `session/new` 0.4s + prompt 1.1s, about 3s. Fine in the background.
- A second `session/new` on the same adapter connection works (both probe sessions shared one process). So a title call can reuse a live Session's adapter instead of spawning Node. The code treats a connection as "exactly one Daemon Session"; check that `claudeState`/`DropSession` stay correct if the title session shares it.
- The adapter's own `session_info_update` fired in the default-preset run, even with `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`. It did not fire with the string system prompt. `mapACPUpdate` (`claude.go`) ignores it today. It is a free title for real claude Sessions, but only after the first turn ends. Not verified: its payload or its wording.
- Not probed: pig path (needs a chosen title model and its credentials), Anthropic key availability in pig's auth store.

## Plan

Built (see `emberloop-web/titles.go`):

- [x] `title` role in settings (`{harness, model}`, via existing `roles`). Deviation: unset still titles claude Sessions with haiku (pig stays untitled); `session_titles: false` or `EMBERLOOP_SESSION_TITLES=0` turns titles off
- [x] `SessionMeta.Title` persisted in `meta.json`; `persistMeta` already publishes the live "sessions" topic, so no new Event kind
- [x] Claude one-off: fresh adapter per title, not shared with the live Session. ACP callbacks are per connection, so a second session on a live connection would clobber the Session own callbacks
- [x] Pig one-off: `Provider.Stream` on the BuildModel-bound model (credentials resolve through the SDK), MaxTokens 64, thinking off; stop reason error is an error
- [x] `cleanTitle` trims to one plain line, at most 60 runes; empty keeps the label
- [x] `SessionList.svelte` shows the title, falls back to `harness · id8`
- [x] Tests: `titles_test.go` (claude stub, failure, no role, pig faux, switch)
- [x] `emberloop-web/AGENTS.md` updated

Live check: real adapter, haiku, string system prompt: title in 2.8s.

Known limits: a Session resumed after a Daemon restart without a title gets titled from its next prompt, not its first. GLOSSARY.md not touched.
