# crush-acp 0.5.0 + crush v0.98.1 — bench result (longtail worker, 2026-10-15)

Verdict: **USABLE with harness workarounds** — the adapter (0.5.0) is
stale relative to crush (0.98.1): its `crush models` discovery misses
file-configured custom providers so every session defaults to a bogus
`zai/glm-5.1` (no key → empty turns). With a `-m` rewrite shim + config in
the adapter's data dir, everything works: 30/30 soak, clean cancel/crash,
resume, images, `set_config_option` accepted, ~68 MB idle / ~113 MB per
session. Client MCP servers are silently DROPPED (never forwarded).
Subscription story is a plus: `crush login` covers copilot / openai(chatgpt)
/ grok / hyper.

## Install

- Adapter: `npm crush-acp@0.5.0`
  tarball `https://registry.npmjs.org/crush-acp/-/crush-acp-0.5.0.tgz`
  integrity `sha512-xIsoZLR38RXWBSbYkOIHgwBrJEosomCCilsZe9O7jO+t6k7HcXA9yttzdSLdfmxS5AxXpOWxWLirLf9/ZePE7g==`
  (9.4 MB; `node …/dist/index.js`, needs `crush` on PATH).
- Engine: charmbracelet/crush **v0.98.1** (latest tag per ls-remote),
  `crush_0.98.1_linux_x86_64.tar.gz` (26 MB download, 87 MB binary,
  sha256 `d85d913effef350f0152f594a70bd974…`; NO published hash to verify
  against — GitHub API rate-limited, asset name guessed from GoReleaser
  convention and confirmed HTTP 200).
- Total: ~122 MB. Isolated HOME `/tmp/acpbench/work/crush-acp/home`.

## Provider / model config (no rebuild)

`crush.json` custom provider (verified):

```json
{"$schema": "https://charm.land/crush.json",
 "providers": {"fake": {"type": "openai-compat",
   "base_url": "http://127.0.0.1:18600/v1", "api_key": "fake-test-key",
   "models": [{"id": "fake-tools", "name": "Fake Tools",
     "cost_per_1m_in": 0, "cost_per_1m_out": 0,
     "context_window": 128000, "default_max_tokens": 4096}]}}}
```

- **Location matters:** crush v0.98.1 merges `crush.json` from the data dir
  AND `~/.config/crush/`; identical provider ids in both → hard error
  (`found in multiple providers`). The adapter ALWAYS passes
  `--data-dir <adapter-data-dir>`, which changes discovery: config must live
  at `<adapter-data-dir>/crush.json`
  (`~/.crush-acp/zed-crush/crush.json` here) and NOWHERE else.
- `crush run -m fake-tools` / `-m fake/fake-tools` both work; NO `-m`
  (unset large slot) → misleading `No providers configured` error.
  (This version has no `model`/`provider` CLI commands; SKILL.md docs
  describe a different version. `crush models` lists 1791 built-ins but
  NEVER custom file providers.)
- Remote catalog: `crush update-providers [path-or-url]` refreshes the
  built-in Catwalk list (default remote; custom URL allowed) — host CAN
  point it at a local file. Count: ~1791 built-in model ids + custom.
- Adapter discovery bug: `fetchAvailableModels` runs `crush models` (which
  omits custom providers) → `availableModels: []`, `currentModelId`
  falls back to hardcoded `zai/glm-5.1`. Bench shim (documented harness
  adaptation, `/tmp/acpbench/scripts/longtail/crush-wrap.sh`): a `crush`
  wrapper rewriting `-m zai/glm-5.1` → `-m fake-tools`. With it, fakellm
  confirms `model=fake-tools`, `UA=Charm-Crush/v0.98.1`.
- `session/set_model` accepts any id into the session (result `{}`) but the
  spawned crush still needs the provider visible per above;
  `session/set_config_option` ACCEPTED the probe's option (only agent
  observed to accept) though effect unverifiable from transcript.
- Client MCP: **dropped** — session/new succeeds but stub tools never reach
  the model (fakellm tool lists: 26 native, no echo/slow; manual
  `TOOL:echo` → no tool call).

## Scenario results (acpbench WITH -m shim, --mcp-stub)

| scenario | verdict | notes |
|---|---|---|
| initialize | ok | 2–2.9 s; fork/list/resume, image+embeddedContext; NO authMethods |
| new_session | ok | 6.9–8 s; mcpReady not-advertised |
| prompt_text | wrong-result | real text ("OK"), TTFC ~100 ms, no tool call (probe-bug signature) |
| cancel | ok | streaming cancel even on fast turns, reusable |
| steer_followup | ok | `concurrent`, 2nd cancelled (no steering advertised) |
| load_resume | ok | resume USED |
| set_model_config | ok | set_config ACCEPTED; set_model Invalid-params on probe shape |
| images | ok | accepted with text |
| permission | skipped | never asks on ECHO turns |
| crash_recovery | ok | 5–9 ms detection |
| concurrency N=3/10 | ok | 3/3 and 10/10 |
| soak 5 | wrong-result | 0 hangs (probe-bug signature) |

Without the shim: all turn scenarios return `silent-empty-turn`
(`end_turn`, 0 bytes) — the adapter bug, not crush.

## Flakiness / RAM (under flock)

- ECHO soak: **30/30 ok, 0 hangs**, p50 1327 ms, p95 1571 ms (slowest of
  the working set — per-turn crush process spawn).
- RAM: idle-init ~68 MB, idle-new ~72 MB; N=3 peak 411 MB
  (marginal ~113 MB); N=10 peak 1363 MB / last 682 MB.
- Startup: ~1.3–3.4 s init. TTFC ~100 ms.

## Subscription logins (from `crush login --help`, no login attempted)

`crush login [hyper|copilot|openai(chatgpt)|grok(xai)]` — covers Copilot,
ChatGPT-sub and SuperGrok angles; plus per-provider API keys
(ZAI/ANTHROPIC/OPENAI/GEMINI/GROQ/OPENROUTER/CEREBRAS). No Antigravity /
OpenCode-Go / Claude-sub. Token storage: crush data dir (not enumerated).

## Gotchas for the Go daemon

1. Pin adapter+engine together; adapter discovery assumes older crush.
   Better: fix `fetchAvailableModels` to read crush.json directly (it
   already parses it for key filtering) instead of `crush models`.
2. Daemon must write `crush.json` into the ADAPTER's data dir (or pass a
   crush wrapper); `~/.config/crush/` alone is not reliably seen under
   `--data-dir`, and duplicates across both files are fatal.
3. Always set an explicit model (`-m` equivalent / set_model); unset slot
   errors misleadingly. `session/new.models` is `[]` — don't trust it.
4. No client-MCP forwarding — daemon-provided tools won't reach the model.
5. Each turn spawns a `crush run` child (~1.3 s/turn floor).

## Raw commands

```
npm install --prefix ./pkg --no-audit --no-fund crush-acp@0.5.0
curl -sSL -o crush.tar.gz https://github.com/charmbracelet/crush/releases/download/v0.98.1/crush_0.98.1_linux_x86_64.tar.gz
# crush.json (above) at $HOME/.crush-acp/zed-crush/crush.json ; wrapper bin/crush first in PATH
acpbench --cmd 'node …/dist/index.js' --cwd …/_proj --mcp-stub \
  --fake-llm-url http://127.0.0.1:18600 --scenario all
```
