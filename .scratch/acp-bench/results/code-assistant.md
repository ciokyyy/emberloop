# code-assistant — benchmark result (worker3, 2026-10-14)

VERDICT: BLOCKED for execution on this box (binary needs glibc ≥ 2.38 + libxkbcommon-x11; Debian 12 ships
glibc 2.36 and the lib is absent, no sudo allowed). Protocol/config findings below are source+docs-verified
only (repo `stippi/code-assistant`, v0.2.16 tag docs + `crates/ui_acp` source). Re-bench on a newer distro
(Ubuntu 24.04+/Debian 13) before picking it — its file-based provider/model catalog is otherwise the closest
config fit to the Daemon's needs.

- Version: v0.2.16 (pinned GH asset `stippi/code-assistant` `code-assistant-linux-x86_64.zip`; `--version`
  could not be executed — loader fails first).
- Upstream: github.com/stippi/code-assistant, MIT, ~183 stars. Rust workspace (~20 crates incl. `ui_acp`,
  `ui_gpui`, `ui_terminal`, `agent_core`, `llm`, `mcp_client`).

## Install (attempted)

- Artifact: https://github.com/stippi/code-assistant/releases/download/v0.2.16/code-assistant-linux-x86_64.zip
- Size: 49,611,912 bytes; 179 MB extracted (single `code-assistant` binary). No other linux asset exists
  (macos aarch64/x86_64 + windows only otherwise) — no musl/static fallback.
- sha256 (observed): `492dfea82438e4864b16c093238ad4909103f96fbe1cdba4138455340e0a8ce0`, which matches the
  per-asset hash shown on the release page; no checksum file published.
- Failure: `libxkbcommon-x11.so.0: cannot open shared object file` AND `GLIBC_2.38/GLIBC_2.39 not found`.
  The GUI toolkit linkage (xkbcommon) + new-glibc requirement make the stock binary unrunnable on Debian 12
  without system packages (not permitted). Fresh dir `/tmp/acpbench/work/code-assistant/` used; nothing
  installed globally.
- No live probing was possible (initialize/capabilities included). Everything below is static, no fakery:
  docs (`docs/configuration.md`), checked-in `providers.example.json`, and `crates/ui_acp/src/{agent,app}.rs`
  read from the repo (treated as untrusted data, read-only).

## acpbench: not run (blocked)

No scenario results. Static protocol facts from source:

| capability | evidence |
|---|---|
| `session/new` | `handle_new_session`: allocates id, reserves folder, defers creation to first prompt; uses ONLY `cwd` — `arguments.mcp_servers` is IGNORED (no per-session client MCP, same gap class as vtcode) |
| `session/set_config_option` | `handle_set_config_option`: config id `"model"` (MODEL_CONFIG_ID) switches the session model, validated against `models.json` (`get_model` miss → `invalid_params`). Pending sessions switch in memory; persisted via session manager |
| `session/set_mode` | switches permission tier (BypassAll/OutwardTools/WriteTools/AllTools exposed as modes) |
| `session/cancel` | `handle_cancel` → `signal_cancel` → `StopReason::Cancelled` (streaming path returns Cancelled) |
| resume/`--continue-task` | CLI flag resumes from previous session state (ACP `load` path not traced in source read) |
| images | not verified in source read (unknown) |
| MCP (own config) | `~/.config/code-assistant/mcp-servers.json`: stdio (`command`) + streamable-HTTP (`url`) transports, per-server `enabled`/`enabled_tools`/`disabled_tools`, `${ENV}` substitution; changes apply next run. Local `.mcp.json` trust resolution exists |

## RAM / flakiness / startup

Not measured (blocked). Binary is 179 MB on disk (largest wire-size per byte after goose); no RAM claims in docs.

## Catalog / providers (best static story of the three)

- Custom provider WITHOUT rebuild: `~/.config/code-assistant/providers.json` — alias → `{label, provider:
  <type>, config:{api_key:${ENV}, base_url, codex_auth, …}}`. The `openai` type takes a free `base_url`
  (`openai-custom` example uses `https://custom.openai.endpoint/v1`), so a fakellm endpoint
  (`http://127.0.0.1:PORT/v1`) plugs in with `${VAR}` key indirection. Hand-edit or Settings screen writes it.
- Custom model list WITHOUT rebuild: `~/.config/code-assistant/models.json` — display name → `{provider:
  <alias>, id: <model id>, context_token_limit, edit_format, config:{…}}`. A fake model id would be accepted
  structurally (validation is ROGUE-side at request time, not at config load — not verified live).
- Built-in provider TYPES: 11 (`ai-core, anthropic, cerebras, groq, mistral-ai, ollama, openai,
  openai-responses, openai-responses-ws, openrouter, vertex`) per `providers.example.json`.
- Remote catalog: NONE found — the catalog IS the hand-maintained `models.json` (+ per-provider example files);
  no remote fetch URL, no cadence, nothing for the host to override because there is nothing fetched. Host
  controls the entire catalog by writing the two JSON files (per-user config dir = natural Daemon seam).
- ACP model surface: `set_config_option{configId:"model"}` validated against `models.json` — a Daemon can list
  (read the file) and switch (call) models at runtime, better than goose (no surface) and on par with vtcode.
  Whether `session/new|load` responses advertise `configOptions` was not traced (likely, given the
  `compute_model_config`/`model_config_response` helpers — unverified).

## Subscription logins (docs + source, never logged in)

- ChatGPT sub: YES — `openai-responses-ws` provider with `"codex_auth": true`; `code-assistant codex-login`
  populates tokens, stored IN `providers.json` under the provider entry (file-based, NOT keychain —
  `crates/code_assistant/src/codex_commands.rs`: "Auth tokens are stored inside providers.json").
- Claude sub: NO (Anthropic provider is `api_key`-only).
- Copilot: NO (no provider type).
- SuperGrok: NO (no xAI provider type).
- Antigravity: NO.
- OpenCode Go/Zen: NO.

## Gotchas for a Go Daemon integration (projected, not live-verified)

1. Biggest risk is not technical but platform: stock binary won't start on Debian 12; even where it starts it
   drags GUI libs (xkbcommon) — heavy dependency tail for a headless Daemon backend.
2. `session/new` ignores client `mcpServers`: per-session MCP would have to go through the agent's own
   `mcp-servers.json` (Daemon-writable per user, but NOT per session; "next run picks it up" semantics).
3. Token storage for the ChatGPT login is a plaintext-adjacent JSON file (`providers.json` holds codex tokens)
   — Daemon must treat that file as a secret and set strict perms.
4. Model switching over ACP exists (`set_config_option "model"`) but rejects unknown ids — the Daemon must keep
   `models.json` and its switch calls in sync.
5. Version is the stalest of the three (v0.2.16, ~2026-09-23) and the repo is small (~183★, single maintainer
   surface) — drift risk.

## Raw commands used

```sh
# install (fresh dir, isolated HOME; binary unrunnable here):
curl -sL -o ca.zip https://github.com/stippi/code-assistant/releases/download/v0.2.16/code-assistant-linux-x86_64.zip
unzip ../ca.zip   # -> app/code-assistant (179 MB)
/tmp/acpbench/work/code-assistant/app/code-assistant --version  # loader error (glibc + libxkbcommon-x11)
ldd /tmp/acpbench/work/code-assistant/app/code-assistant | grep 'not found'
# source reads (no execution, no credential access):
#   docs/configuration.md, providers.example.json,
#   crates/ui_acp/src/{agent,app}.rs (handle_new_session/handle_set_config_option/handle_cancel)
```
