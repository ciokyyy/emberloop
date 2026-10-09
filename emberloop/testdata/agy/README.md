# agy probe fixtures (ticket 03)

Captured 2026-10-05 with `agy` 1.2.17, model `gemini-3.8-flash-low`
unless noted. See the ticket
(`.scratch/archive/harnesses-agy/issues/03-agy-probe.md`) for the question each
answers. Paths inside payloads point at the probe machine; tests should
parse structure, not absolute paths.

- `models.txt` — verbatim `agy models` stdout (`Fetching...` goes to
  stderr). Every Gemini id carries an effort suffix; `claude-sonnet-4-6`
  has none. `gpt-oss-120b-medium` is a lone non-Gemini `-medium` id:
  ticket 04's `Models()` must state whether the suffix groups it or is
  part of the id.
- `stream-simple.json` — `--output-format stream-json` for one `OK`
  turn: `init` (model, cwd, full tool catalog, permission_mode),
  `step_update` (`user_input`, `agent_response` with `text_delta` and
  per-step `usage`), terminal `result` (`status`, `response`,
  `duration_seconds`, `num_turns`, total `usage`).
- `stream-mcp-25s.json` / `stream-mcp-150s.json` — a `call_mcp_tool`
  step (`ServerName`, `ToolName`, `Arguments`) staying DONE/foreground
  after 25 s / 150 s, result in the same turn. ServerName is namespaced
  `emberloop-probe_emberloop-probe` (plugin_server), not the bare config name.
- `stream-queue.json` — `--input-format stream-json` takes
  `{"event":"user","message":{"content":"..."}}` lines; turn 2 runs
  after turn 1 (`num_turns` counts per result).
- `stream-stop-continue.json` — a `Stop` hook answering
  `{"decision":"continue"}` re-enters the loop (extra `system_message`
  + model steps), the second `Stop` allows it. Verbose by nature.
- `transcript-*.jsonl` — `<appDataDir>/brain/<conversation-id>/`
  `.system_generated/logs/transcript.jsonl` (appDataDir
  `~/.gemini/antigravity-cli`). JSONL steps: `USER_INPUT` (prompt,
  wrapped in `<USER_REQUEST>` + metadata), `PLANNER_RESPONSE`
  (`content`/`thinking`/`tool_calls`, per-request `input_tokens`,
  `output_tokens`, `cache_read_tokens`), `GENERIC` (tool results).
  `transcript-queued` is the interactive queue-mode session;
  `transcript-stop-continue` holds the injected
  `Stop hook blocked termination: ...` SYSTEM_MESSAGE.
- `hooks/` — stdin payloads captured from `.agents/hooks.json`
  handlers. Every payload carries `conversationId`, `workspacePaths`,
  `transcriptPath` (points at `transcript_full.jsonl`) and
  `artifactDirectoryPath` (`brain/<id>`, no `logs/` suffix).
  `PreToolUse`/`PostToolUse` add `toolCall` + `stepIdx` (`PostToolUse`
  adds `error`, never the tool output). `Stop` adds `executionNum`
  (0, then 1 after a `continue`), `terminationReason` (`NO_TOOL_CALL`
  here, not `model_stop`) and `fullyIdle`.
- `agent/` — the WORKING live shapes, laid out as
  `dot-agents/` mirrors a workspace `.agents/` (`dot-` so checkout
  tooling never mistakes fixtures for config; live absolute paths are
  replaced with `<ABS ...>` placeholders ticket 04 substitutes):
  `agents/mcponly/agent.md` (the agent behind every live `stream-mcp`
  run) needs `name`, `mainAgent: true`, `excludeDefaultComponents:
  true`, `inheritCustomizations: false`, `inheritMcp: false`, a
  plain-string `tools` list and an `# Agent System Instructions` H1
  body. The MCP server arrives via the sibling
  `plugins/emberloop-probe/` (`plugin.json` + `mcp_config.json`); the
  agent's `plugins` entry must be an ABSOLUTE path — bare names,
  agent-relative (`../plugins/...`) and workspace-relative
  (`.agents/plugins/...`) paths all fail with `Cannot resolve
  agent-relative path ...: AgentBasePath is not set` in `-p` runs (the
  agent still loads, the plugin just never attaches). `tools` entries
  must be plain strings: map forms and `call_mcp_tool` /
  `mcp(server/tool)` entries fail startup with `unknown component:
  tool ... not found in registry` (exit 3, `AGY_ERROR` JSON on
  stderr). MCP tools surface as `call_mcp_tool` regardless of the
  `tools` filter; `enabledTools: [...]` on the server rejects other
  tools at call time (`tool X is not enabled for server Y`).
- `mcp-shim/` — minimal stdio MCP shim that agy 1.2.17 completes its
  handshake with: answer `server/discover` (with `tools`), then
  standard `initialize` / `tools/list` / `tools/call`.
  `mcp-shim.sh` execs the sibling `.py`; logging goes beside the
  script (`PROBE_LOG` overrides, never fatal); `probe_slow` sleeps
  `PROBE_SLOW_SECS` (default 5; the 150 s evidence run used 150).
  Self-test:
  `printf ... | PROBE_SECRET=... PROBE_SLOW_SECS=1 sh mcp-shim.sh`.
