# 03: Probe agy's real behaviour (PROBED 2026-10-05, agy 1.2.17)

**What to build:** A written record, in this ticket, of how the installed `agy` behaves, with captured output saved as test fixtures. The grill found these in the binary and its changelog but could not run them (the service returned 503 on eligibility). Answer each:

Probed 2026-10-05 with `agy` 1.2.17 (live service; 503s came and went — every error below with zero usage spent no quota). Fixtures: `wick-pig/testdata/agy/` (README maps files to questions). TDD note: this ticket writes no code, so there are no seams to agree; the seams (harness transcript parsing, `Models()` grouping, usage records) are tested by tickets 04/07 against these fixtures.

## 1. `--agent` does NOT accept a path outside the workspace

- `--agent /tmp/agy-probe/outside/mywrap/agent.md` (absolute file or dir) silently runs the DEFAULT agent: reply `I am Antigravity...`, `input_tokens` 12403 (full default context). `init` echoes the path, but the agent file is not loaded.
- `--agent mywrap` with `.agents/agents/mywrap/agent.md` in the worktree DOES load: reply `PROBE-ZEBRA-42` (system prompt from the file), `input_tokens` 1441.
- The agent file needs `name:`, `mainAgent: true`, and the system prompt as the H1 body (`# Agent System Instructions` works; without `name` the file is ignored and the default agent runs).
- `--add-dir /tmp/agy-probe/outside` + `--agent mywrap` hung past 90 s (killed); not pursued.
- **Spec fallback CONFIRMED and REQUIRED: generate the agent under the worktree's `.agents/agents/wick-<role>/` and exclude it via the repo's local git exclude. `~/.wick-pig/agy/` as an `--agent` path does not work.**

## 2. `excludeDefaultComponents` + `inheritCustomizations: false` strips everything

- Stripped agent (`excludeDefaultComponents: true`, `inheritCustomizations: false`, `inheritMcp: false`, `tools: [run_command]`): 1441–1468 input tokens vs ~12394 default.
- Built-in tools really gone: asked to `view_file`, stripped agent replies `NO-VIEW-TOOL`; same prompt on the default agent reads the file.
- Workspace `GEMINI.md`/`AGENTS.md` NOT auto-injected when stripped: the stripped agent had to `ls` + `cat GEMINI.md` via `run_command` to find the marker; the default agent answered it directly (12521 tokens). (Global `~/.gemini/*` covered by the same switch per the `inheritCustomizations` docs — "skills, rules, plugins, subagents and MCP servers" — plus the 1441-token budget leaving no room for them.)
- `tools:` must be plain strings; map forms fail startup (`unknown component ... not found in registry`, exit 3, `AGY_ERROR` JSON on stderr).

## 3. One MCP server via a workspace plugin; system prompt is the H1 body

- Agent-level `mcpServers:` in frontmatter BREAKS loading (falls back to default). The working path: workspace plugin `.agents/plugins/<name>/{plugin.json,mcp_config.json}` with `{"mcpServers": {"wick-probe": {command, args, env}}}` (same shape as global `mcp_config.json`, verified via `agy mcp add --env`), plus `inheritMcp: false` (drops global servers) and agent `plugins: [<ABSOLUTE path to the plugin dir>]`.
- Bare names, agent-relative (`../plugins/...`) and workspace-relative (`.agents/plugins/...`) plugin refs all fail in `-p` runs: `Cannot resolve agent-relative path ...: AgentBasePath is not set` (agent still loads, plugin never attaches). Absolute paths attach.
- `env` reaches the server process (spy log: `ENV: PROBE_SECRET=QUASAR-9`; tool call returned `SECRET-IS-QUASAR-9`).
- System prompt = the Markdown H1 body after frontmatter.
- MCP tools surface as `call_mcp_tool` (+ `list_resources`, `read_resource`), regardless of the `tools` filter; call shape is `{ServerName, ToolName, Arguments}`. **ServerName is namespaced `<plugin>_<server>`** (`wick-probe_wick-probe`); the bare config name errors. `enabledTools: ["probe_echo"]` on the server rejects other tools at call time (`tool probe_slow is not enabled for server ...`).
- Caveat (ticket 04): in ONE interactive session the model claimed `call_mcp_tool` unavailable and confabulated the answer — print vs interactive tool surfacing needs a recheck in the harness live test.

## 4. `background`: per-tool `toolConfig.<tool>.background`; default already foreground

- Proto (`McpServerToolConfig`) has per-tool `background` (`McpToolBackgroundMode` OFF/ALWAYS/UNSPECIFIED), plus `eager` and `task_options`, set as `toolConfig` on the MCP server entry. `toolConfig`/`background`/`background_mode` (server level or per tool, valid or bogus values) are all ACCEPTED silently — no startup or call-time error — so the honored key is NOT nailed down; `eager` / `forceAllToolsEager` are untested alternatives. **Follow-up for ticket 05.**
- Behaviour, live: a 25 s AND a 150 s `call_mcp_tool` call both stayed foreground (`DONE` after 25.1 s / 150.0 s, result in the same turn; fixtures `stream-mcp-25s.json`, `stream-mcp-150s.json`). `run_command` by contrast backgrounds within seconds (`Tool is running as a background task with task id: .../task-6`, `manage_task status/kill` to follow it). So the spec's "never backgrounded" is belt-and-braces on top of already-foreground MCP calls in `-p` mode; the interactive threshold is untested.
- Side finding: with an unconnectable MCP server, `-p` hangs forever (killed at timeouts) instead of failing fast — the harness must not ship a dead server entry.

## 5. All five hooks exist; `Stop` blocks, reason is injected

- Events (all in `.agents/hooks.json` named-hook entries; `PreToolUse` / `PostToolUse` grouped under `matcher`, the rest flat): `PreToolUse`, `PostToolUse`, `PreInvocation`, `PostInvocation`, `Stop`.
- stdin payloads (fixtures `hooks/`): every payload carries `conversationId`, `workspacePaths`, `transcriptPath` (points at `transcript_full.jsonl`, NOT `transcript.jsonl`) and `artifactDirectoryPath` (`brain/<id>`, no `logs/` suffix). `PreToolUse`/`PostToolUse` add `toolCall{name,args}` + `stepIdx` (`PostToolUse` adds `error`, never the tool output). `Stop` adds `executionNum` (0, then 1 after a `continue`), `terminationReason` (`NO_TOOL_CALL` here — not the documented `model_stop`) and `fullyIdle`. Empty stdout (`{}` or nothing) is accepted as allow.
- **Stop gate PROVEN**: first `Stop` → `{"decision":"continue", "reason":"..."}` re-entered the loop (hook fired twice, `executionNum` 0→1); the reason arrived as a `SYSTEM_MESSAGE` step (`Stop hook blocked termination: ...`); second `Stop` → `{}` let it `SUCCESS`. **The spec's nudge-and-give-up fallback is NOT needed — use the Stop gate (ticket 06).**
- From binary docs only (not run live): `PreToolUse` outputs `decision: allow|deny|ask|force_ask` (+ `reason`, `permissionOverrides`, arg-rewriting `overwrite`); `PreInvocation` outputs `injectSteps` (`toolCall`/`userMessage`/`ephemeralMessage`); `PostInvocation` adds `terminationBehavior: force_continue|terminate`.

## 6. Transcript: `brain/<id>/.system_generated/logs/transcript.jsonl`

- Full path `~/.gemini/antigravity-cli/brain/<conversation-id>/.system_generated/logs/transcript.jsonl` (plus `transcript_full.jsonl`; `truncated_fields` selects full lines — docs only, small runs never truncate).
- JSONL steps: `USER_INPUT` (prompt wrapped in `<USER_REQUEST>` + local-time metadata + model-selection notices), `PLANNER_RESPONSE` (`content`/`thinking`/`tool_calls[{name,args}]`, per-request `input_tokens`, `output_tokens`, `cache_read_tokens` — NO `thinking_tokens`/totals in the transcript), `GENERIC` (tool results), plus `error_message` / `system_message` step types.
- Covers prompts, tool calls, results and per-request usage — the harness's LastReply/TurnState source (ticket 04).
- Open point for ticket 04: `-p`/interactive runs mint their own conversation id (seen in `init` / the exit resume hint `agy --conversation=<id>`); there is no `--session-id` flag, so the harness must capture the id (stdout `init` in `-p`, pane output interactively) to find the transcript.

## 7. `stream-json` usage: per-step and total, no cost

- `init`: `model`, `cwd`, `agent?`, full `tools` catalog, `permission_mode`. (The catalog is NOT the effective set — the stripped agent still lists every built-in.)
- `step_update`: `user_input` / `agent_response` (`text_delta` chunks; terminal chunk carries `duration_seconds` + `usage`) / `tool` (`tool_name`, `tool_info{name,parameters,output|error}`, `ACTIVE`→`DONE`/`ERROR` + `duration_seconds`).
- `usage` shape: `{input_tokens, output_tokens, thinking_tokens, cache_read_tokens, total_tokens}` per model step and summed in `result` (`status`, `response`, `duration_seconds`, `num_turns`). No `cache_write`, no cost field (ticket 07 maps `thinking_tokens`→reasoning, `cache_read_tokens`→cacheRead; `reportedCost` stays empty).
- `--input-format stream-json` consumes `{"event":"user","message":{"content":"..."}}` per line, one turn per message, sequentially.
- Exit codes: 0 ok; 3 on config errors with `AGY_ERROR: {...}` JSON on stderr; eligibility 503s print plain errors with zero usage.

## 8. `--effort` conflicts with effort-suffixed ids; valid levels include `xhigh`

- `--model gemini-3.8-flash-low --effort high` → `invalid model selection ... conflicts with --effort=high` (no run). `--effort low` + `-low` is accepted (no-op). `--effort` never re-tiers a suffixed id.
- Valid levels: `low, medium, high, xhigh, max` — **the spec's effort list (low/medium/high/max) must gain `xhigh`**.
- Ungrouped ids (`claude-sonnet-4-6`) accept any of the five levels at validation (`--effort bogus` rejected pre-run); a live ungrouped run was not spent (quota) — ticket 04's live test covers it.
- So: grouped models launch as `<base>-<effort>` with NO `--effort` flag; ungrouped as model + `--effort <level>` (spec stands).

## 9. Queue mode: typed text waits, then auto-submits

- Interactive pane, turn running: typed text + Enter shows `▸ <text>` above the prompt with `Press up to edit queued messages`; the running turn is NOT interrupted.
- At turn end the queued text auto-submits as the next `USER_INPUT` (transcript steps 10–11 in `transcript-queued.jsonl`).
- Matches the changelog (`Queued Messages: Queue` default) — spec's "never change queue mode; notices queue till turn end" stands, and long MCP calls staying foreground means a notice can't detach them in `-p` mode.
- First interactive launch stops at a folder-trust prompt (`Do you trust the contents of this project?`) — ticket 04's `Trust` must handle it.

**Blocked by:** None (can start immediately)

**Status:** done

- [x] Every question answered with evidence (command, output) in this ticket
- [x] Fixtures saved: `agy models` output, a transcript sample, a stream-json result, hook stdin payloads
- [x] Each answer that changes the harnesses spec's fixed fallbacks noted explicitly

### Spec changes required

- Fallback 1 (worktree `.agents/agents/wick-<role>/` + git exclude) confirmed REQUIRED (§1). The agent's `plugins` entry must be an ABSOLUTE path (relative plugin refs fail, §3) — ticket 04's generator must write absolute plugin paths even though the agent itself lives in the worktree.
- Fallback 2 (nudge-and-give-up) NOT needed: `Stop` blocks (§5) — ticket 06 wires the real gate.
- Effort levels gain `xhigh` (§8).
- No `--effort` flag alongside a suffixed model id (§8).

### Follow-ups (for later tickets, not this one)

- Ticket 05: pin the honored never-background key (`toolConfig.<tool>.background` vs `eager`/`forceAllToolsEager`) with a >60 s interactive MCP call; verify `off` keeps it foreground. The committed `mcp-shim` supports this via `PROBE_SLOW_SECS` (the 150 s evidence run used 150).
- Ticket 04: harness session→conversation-id correlation (no `--session-id`; capture `init` / resume hint); recheck MCP tool surfacing in interactive sessions (§3 caveat); `Trust` for the folder-trust prompt; ungrouped-model `--effort` live run; grouping rule for the lone non-Gemini suffixed id `gpt-oss-120b-medium` (suffix levels `-max`/`xhigh` have no fixture coverage either — only `-low`/`-medium`/`-high` were listed).
- Ticket 07: usage parser over `stream-*.json` + `transcript-*.jsonl` fixtures (`thinking_tokens`→reasoning, `cache_read_tokens`→cacheRead).
