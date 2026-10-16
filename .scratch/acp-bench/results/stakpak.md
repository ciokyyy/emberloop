# stakpak 0.3.88 — bench result (longtail worker, 2026-10-15)

Verdict: **USABLE, lightest of the set** — 83 MB Rust binary, ~64 MB idle,
~46 MB/session (N=10 = 459 MB total), 30/30 soak at p50 13 ms, clean
cancel + 3 ms crash detection, non-interactive BYOK with custom endpoint.
Model roster is NOT customizable (provider-known ids only; unknown ids
route to stakpak cloud and fail) — the one catalog wart. STALE upstream
(v0.3.88, 2026-06-10; a v1.0 tag exists but was not benched).

## Install

- Artifact: GitHub release `stakpak/agent@v0.3.88`
  `…/releases/download/v0.3.88/stakpak-linux-x86_64.tar.gz` (31 MB;
  sha256 `1e923d6b632e6bda743b3f48c40914f6…`; asset name guessed
  GoReleaser-style, HTTP 200 confirmed; NO published hash — GitHub API
  rate-limited, could not cross-check).
- Size: 109 MB total (83 MB binary + 31 MB tarball). `stakpak 0.3.88`,
  `stakpak acp` starts the ACP server.
- Note: phones home on every start (`Update check failed: Failed to fetch
  release info` in this sandbox) — daemon hosts want that disabled/firewalled.

## Provider / model config (no rebuild)

Non-interactive (verified):
`stakpak auth login --provider openai --api-key fake-test-key --endpoint http://127.0.0.1:18600/v1`
writes `~/.stakpak/config.toml`:

```toml
[profiles.default]
provider = "local"
model = "gpt-4.1"
[profiles.default.providers.openai]
type = "openai"
api_endpoint = "http://127.0.0.1:18600/v1"
[profiles.default.providers.openai.auth]
type = "api"
key = "fake-test-key"
```

- Custom ENDPOINT ✓ (fakellm confirms hits). Custom MODEL ID ✗: setting
  `model = "fake-tools"` yields `currentModelId: fake-tools` in session/new
  but prompts fail with `Bad Request: InvalidAgentInput("Provider not found:
  stakpak")` — unknown ids route to the stakpak-cloud provider. Use a
  provider-known id (gpt-4.1 works against any OpenAI-compatible endpoint;
  fakellm accepts any id).
- `auth login --provider` covers anthropic/openai/gemini/stakpak/amazon-bedrock
  + OAuth access/refresh flags; `--endpoint` is provider-specific.
- `session/new` returns `models: {currentModelId, availableModels: []}`
  (empty list — roster not exposed).
- No `session/set_model` (`Invalid params`), no `session/set_config_option`
  (`Method not found`) — model = config file + restart.
- No remote catalog observed (api_endpoint is per-provider; no models.dev
  polling seen). Built-in providers: per `auth login` list above.
- Client MCP tools NOT forwarded (fakellm sees only `stakpak__*` native tools).

## Scenario results (acpbench, --mcp-stub)

| scenario | verdict | notes |
|---|---|---|
| initialize | ok | 1.2–1.7 s; loadSession, http+sse MCP, image+embeddedContext; empty sessionCapabilities/authMethods |
| new_session | ok | 6.7–6.9 s (mostly the 5 s mcpReady wait); mcpReady not-advertised |
| prompt_text | wrong-result | completes, TTFC ~10 ms, no tool call (probe-bug signature) |
| cancel | ok (slow turn) | streaming, reusable |
| steer_followup | skipped | turns too fast (honest skip) |
| load_resume | ok | resume unsupported → session/load fallback WORKED, next prompt ok |
| set_model_config | ok* | both methods absent (*no-op ok) |
| images | ok | accepted |
| permission | skipped | never asks on ECHO turns |
| crash_recovery | ok | 3 ms detection |
| concurrency N=3/10 | ok | 3/3 and 10/10 |
| soak 5 | wrong-result | 0 hangs (probe-bug signature) |

## Flakiness / RAM (under flock)

- ECHO soak: **30/30 ok, 0 hangs**, p50 13 ms, p95 30 ms. Fastest turns benched.
- RAM: idle-init ~64 MB, idle-new ~66 MB, post-turn ~64 MB; N=3 peak
  208 MB (marginal ~47 MB); N=10 peak 459 MB. Lowest footprint in this batch.
- Startup: ~1–1.7 s init.

## Subscription logins (docs/source)

Stakpak account (`auth login` default provider; `stakpak account`), provider
OAuth/API keys; keys live in `~/.stakpak/config.toml`. No SuperGrok /
ChatGPT-sub / Antigravity / OpenCode-Go / Claude-sub / Copilot paths seen.

## Gotchas for the Go daemon

1. Model ids are provider-roster-bound — a generic backend can redirect the
   endpoint but cannot invent model ids; map daemon model names to
   provider-known ids.
2. Update-check phone-home on every spawn; check for a disable flag/env
   before production use.
3. `session/new.models.availableModels` is empty — daemon model pickers
   must come from its own catalog, not ACP.
4. No client-MCP forwarding; no runtime model switching.

## Raw commands

```
curl -sSL -o stakpak-linux-x86_64.tar.gz …/v0.3.88/stakpak-linux-x86_64.tar.gz && tar xzf …
stakpak auth login --provider openai --api-key fake-test-key --endpoint http://127.0.0.1:18600/v1
acpbench --cmd '/tmp/acpbench/work/stakpak/stakpak acp' --cwd …/_proj \
  --mcp-stub --fake-llm-url http://127.0.0.1:18600 --scenario all
```
