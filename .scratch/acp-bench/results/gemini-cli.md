# gemini-cli @ 0.64.0 (`--acp`) — benchmark result (2026-10-14, worker: node)

**Verdict: fit with integration tax.** Fastest steady-state (p50 28 ms),
arbitrary custom model ids honored, `set_mode`/`set_model`/load all work —
but the client MUST `authenticate` (probe never does; shim required), the
default `auto` model burns ~90 s/classifier-retries per turn against any
non-schema-conformant endpoint, command-stdio MCP servers are silently
dropped, and a folder-trust gate disables agents/hooks/privileged modes.

Version: `@google/gemini-cli@0.64.0`
(npm integrity `sha512-FHrZ1NPf9QszyVMaa/BP3SsPy40ogBLnbZNWkv/PMi1Uz2zoDNMLGiBzBGmx4Bl7iLRq01K9PY/5kmloXFTpsA==`).
Install: `npm install --prefix /tmp/acpbench/work/gemini-cli/pkg
@google/gemini-cli@0.64.0`, isolated HOME. Size: **101 MB** (smallest).
Start: `gemini --acp` (stdio; UA `GeminiCLI-acp/0.64.0/...`).

## Provider = fakellm (verified, with caveats)

`GEMINI_API_KEY=fake-test-key`,
`GOOGLE_GEMINI_BASE_URL=http://127.0.0.1:18202`,
`settings.json {"model":{"name":"gemini-2.5-pro"}}` (**object form — string
form hard-errors at startup**: `Expected object, received string`),
`TMPDIR=<isolated-home>/tmp`, plus `authenticate {methodId:gemini-api-key}`
before anything else. fakellm JSONL confirms
`POST /v1beta/models/gemini-2.5-pro:streamGenerateContent` with 15 native
tools. **Without `authenticate`: prompt → `Internal error` or silent hang**
(probe gap; worked around with pass-through proxy
`/tmp/acpbench/scripts/nodeworker/acp_proxy.py` injecting `authenticate` +
waiting for its result).

## Scenario results (proxy + `--mcp-stub`, report `gm-func.json`)

| Scenario | Verdict | Latency | Notes |
|---|---|---|---|
| initialize | ok | 5–10 s | caps: loadSession, image+audio+embeddedContext, MCP http+sse only; authMethods=[oauth-personal, gemini-api-key, vertex-ai, gateway] |
| new_session | ok | 9–14 s | mcpReady not-advertised; models list + modes in result |
| prompt_text | wrong-result | ~6 s | PROBE+FAKE ARTIFACT; end_turn, TTFC **2 ms**, 5 updates |
| cancel | skipped | — | turns too fast to catch (honest skip; `raced`) |
| steer_followup | skipped | — | same (`raced`) |
| load_resume | ok | 9 s | `session/resume` unsupported → `session/load` fallback works |
| set_model_config | ok* | 6 s | *probe shape wrong; manual `modelId` incl. **arbitrary** ids → ACCEPTED (see below) |
| images | ok | 5 s | accepted |
| permission | skipped | — | never asks on fake turns |
| crash_recovery | skipped | — | turn finished before kill (honest skip) |
| concurrency N=5 | ok | 26 s | 5/5 + 5/5 |

`session/set_config_option`: **Method not found** (unsupported).
`session/set_mode`: `plan` accepted; `yolo` rejected in untrusted folder
(`Cannot enable privileged approval modes in an untrusted folder`).

## RAM

| Phase | RSS |
|---|---|
| startup-init | 404 MB (5 procs) |
| idle-after-initialize / new-session | 405–415 MB |
| prompt-turn / post-turn | 417–424 MB |
| 5 parallel peak | 2106 MB (17 procs) |
| **marginal/session (N=5)** | **338 MB** (354,790,604 B) |

N=10 not run (would need ~3.5 GB; box too loaded).

## Flakiness / latency

soak50: 50/50 completed in 9.8 s total, **0 hangs, 0 crashes**,
p50 28 ms / p95 63 ms — fastest steady-state of the three.
BUT this requires an explicit model. With the default `auto` model, every
turn opens with `ModelRouterService → generateJson` classifier calls
(`Answer with your current model name`); fakellm's `OK` fails schema
validation → `API returned invalid content after all retries`, ~90 s backoff
per turn, error reports written to `$TMPDIR`. `OverrideStrategy` bypasses all
classifier calls (0 ms) the moment the model is non-auto — set it via
settings `model.name` or per-session `set_model`.

## Catalog / providers

- `session/new` lists **6 static entries** (`auto`, `gemini-3.1-pro-preview
  (-customtools)`, `gemini-3-flash-preview`, `gemini-2.5-pro`,
  `gemini-3.8-flash`, `gemini-3.5-flash-lite`) bundled in 0.64.0; changing
  settings `model.name` changes `currentModelId` (verified `gemini-2.5-pro`).
  No remote catalog fetch observed.
- Custom model WITHOUT rebuild: `session/set_model {modelId: <anything>}`
  is passed straight to the endpoint URL — `fake-small` verified end-to-end
  (`/v1beta/models/fake-small:streamGenerateContent`, 0.4 s turn). No custom
  *provider* baseURL except `GOOGLE_GEMINI_BASE_URL` env (Gemini protocol
  only) and the untested `gateway` authenticate method
  (`{protocol:google, restartRequired:false}`).
- `providers/list|set`: **Method not found** (no client-driven routing).
- Env knobs observed in bundle: `GEMINI_API_KEY`, `GOOGLE_API_KEY`,
  `GOOGLE_GEMINI_BASE_URL`, `GOOGLE_VERTEX_BASE_URL`,
  `GOOGLE_GENAI_USE_VERTEXAI`, `GEMINI_MODEL`.

## Subscriptions (docs + strings, no login attempted)

Google OAuth (`oauth-personal` → `~/.gemini/oauth_creds.json` per binary
strings), Gemini API key, Vertex AI, custom gateway. ChatGPT sub / Claude
sub / Copilot / SuperGrok / Antigravity / OpenCode Go: none.

## Gotchas for the Go Daemon

1. MUST `authenticate` first (api-key method works headless with env key);
   budget for the proxy/shim until the probe learns auth.
2. Always set an explicit model (settings or post-`session/new`
   `set_model`); never leave `auto` against constrained endpoints.
3. `session/new mcpServers` with command-stdio entries are **silently
   dropped** (never reach the model) — only http/sse servers usable.
4. Folder trust: untrusted cwd disables project agents, hooks, and privileged
   modes (`Skipping project agents due to untrusted folder`). Daemon-owned
   project dirs must be pre-trusted.
5. Set `TMPDIR` inside the isolated HOME (error reports + chats default
   elsewhere); sessions persist chats under `~/.gemini/tmp|history`.
6. Turns complete in ms — cancel/permission/crash paths are untestable
   against instant models (all honest-skips, not failures).

## Raw commands

```sh
FAKE=http://127.0.0.1:18202
/tmp/acpbench/bin/fakellm --addr 127.0.0.1:18202 --log /tmp/acpbench/work/fakellm-18202.jsonl &
npm install --prefix /tmp/acpbench/work/gemini-cli/pkg @google/gemini-cli@0.64.0
export HOME=/tmp/acpbench/work/gemini-cli/home XDG_CONFIG_HOME=$HOME/.config \
  XDG_DATA_HOME=$HOME/.local/share XDG_CACHE_HOME=$HOME/.cache TMPDIR=$HOME/tmp
echo '{"model":{"name":"gemini-2.5-pro"}}' > $HOME/.gemini/settings.json
P=/tmp/acpbench/scripts/nodeworker/acp_proxy.py
/tmp/acpbench/bin/acpbench --cmd "python3 -I $P /tmp/acpbench/work/gemini-cli/pkg/node_modules/.bin/gemini --acp" \
  --cwd /tmp/acpbench/work/_proj --mcp-stub --scenario all \
  --env GEMINI_API_KEY=fake-test-key --env GOOGLE_GEMINI_BASE_URL=$FAKE \
  --env PROXY_AUTH_METHOD=gemini-api-key --env TMPDIR=$HOME/tmp --fake-llm-url $FAKE
# manual checks: acp_manual.py / acp_setmodel.py / acp_setprompt.py / acp_raw.py in same dir
```

Reports: `/tmp/acpbench/work/gm-{func,soak50,conc5,px,p,ns,init}.json`.
