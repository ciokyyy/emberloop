# ACP benchmark synthesis (final round, 2026-10-15)

Supersedes the earlier merged ranking, which was written before the baselines
finished and recommended glm-acp-agent without checking subscription support.
Per-agent evidence: `final/<agent>.md` and `.json`. Tools: `scripts/acpbench`
(v2) and `scripts/fakellm` (v2) against a fake LLM; no real model API was called.

## Headline findings

1. **No agent survives a stalled provider stream.** A truly held-open stream
   (one chunk, then silence) leaves `session/prompt` unresolved for every agent
   tested: pi-acp+PiG, pi-acp+Node pi, codex-acp, goose, opencode, fast-agent,
   mistral-vibe, kimi-code, glm-acp-agent, fx, crow-cli, docker-agent, zerostack
   (90 to 130 s probe timeouts, one upstream request, no retry). Qwen-code
   surfaces provider hangs only through the client timeout. This is the failure
   behind the Daemon's `watchPigStall`; it is a provider-layer problem, not
   an ACP framing problem and not specific to pig. The fix is harness-agnostic:
   a turn-idle watchdog in the Daemon's generic ACP client (no update for N s,
   then `session/cancel`, then respawn/resume).
2. **ACP framing is stable.** Controlled soaks (50 to 100 turns) and
   cancel-mix runs show 0 hangs and 0 crashes for pi-acp+PiG, pi-acp+Node pi,
   claude-agent-acp, codex-acp, goose, opencode, gemini-cli, mistral-vibe,
   fast-agent, kimi-code and glm-acp-agent. Cancel works on all of them.
3. **Error surfacing differs a lot** (see fault matrix below). pig-acp swallows
   HTTP failures into quiet `end_turn` replies after ~14 s of retries.
4. **Subscription support decides the harness**, not RAM. Only pig covers
   SuperGrok OAuth, ChatGPT sub (`openai-codex`), OpenCode Go and Copilot in one
   place; `agy` is a ToS-banned path everywhere.

## RAM (marginal per Session, RSS summed over process tree)

pi-acp+PiG about 105 MB (N=5: 663 MB total) | goose about 100 MB | docker-agent 85
MB | pi-acp+Node about 170 MB | fast-agent about 185 MB | kimi-code 130 to 150 MB
| crow-cli about 149 MB | codex-acp about 262 MB | gemini-cli about 338 MB |
claude-agent-acp 370 to 450 MB | opencode about 445 MB | qwen-code about 314 MB.
Lightest (but see MCP column): zerostack about 11 MB, fx about 13 MB, vtcode about
43 MB, stakpak about 47 MB. N=10 was skipped on most agents (box too small).

## MCP forwarding (decides usability for el-mcp)

Forwarded and verified: pi-acp (via `mcp.json`, not `session/new`), claude-agent-acp,
codex-acp, goose, opencode, fast-agent, kimi-code, glm-acp-agent, crow-cli,
docker-agent. Silently dropped: vtcode, stakpak, crush-acp, zerostack, qwen-code
(discovered, not forwarded), gemini-cli (stdio dropped). mistral-vibe: only with
`--legacy-harness`.

## Fault matrix highlights (3 runs per class)

- Fast clean errors: goose, docker-agent, zerostack, glm-acp-agent, codex-acp (429 in about 1 s,
  5xx in 25 s; `truncate` reported as success text with `stopReason=end_turn`).
- Slow tails: opencode (5xx, reset, midstream 60 to 90 s), fast-agent (about 40 s),
  qwen-code (about 100 s, 429 never returns), crow-cli (empty reply: 7.5 min retry storm),
  mistral-vibe (about 300 s on 429/5xx).
- Swallowers (error becomes empty `end_turn`): kimi-code (429/5xx/empty), pi-acp
  (after retry chatter), glm-acp-agent (empty).
- Retry storms: fx on 429 and 500 never surfaces.

## Recommendation

Keep pig (over ACP) and Claude. Build one generic ACP client with a small quirk
layer per harness (spawn argv/env, MCP delivery, model selection via
`session/set_config_option`, effort mapping, auth/catalog source). Move the stall
watchdog out of `pig.go` into that generic layer so every harness gets it. Treat
`end_turn` with error-shaped text or empty text as a provider failure. A second
lane worth adding later: codex-acp (official ChatGPT-sub path, about 262 MB, fast
fail on 429) or goose (about 100 MB, fail-fast errors, ChatGPT+Copilot OAuth, no
runtime model switch).

## Caveats

- fakellm2 caps `stall-5s`/`stall-forever` at 0.5 s and lacks a `truncate` alias; true
  stalls used `HANG` or by-counter faults. Several matrix rows are marked VOID in the
  per-agent files.
- Soak numbers with COOPERATE fire-once only exercise the tool on iteration 0.
- N=10 RAM runs skipped; the box had under 4 GB free.
- Fake-LLM results do not capture real provider behavior (latency, real rate limits).
