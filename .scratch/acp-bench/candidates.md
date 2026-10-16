# ACP agent/harness candidates (surveyed 2026-10-14)

Primary sources only. `unknown` = unverified, not guessed. Machine-readable
copy: `candidates.json`. Survey-only: nothing installed or run.

Counts: **58 candidates** — 23 benchmark-worthy, 19 second-tier, 9
record-only, 7 skip. Languages: ~14 Rust, ~15 TypeScript, ~10 Python, 3 Go,
2 Zig, 1 Elixir, 1 Guile, rest proprietary/binary.

## A. Master table

| # | Name | Repo | Lang | License | Stars | Pushed | Release | ACP mode | Start (stdio) | Install (pinned, no installer scripts) | Verdict |
|---|---|---|---|---|---|---|---|---|---|---|---|
| 1 | codex-acp | agentclientprotocol/codex-acp | TS | Apache-2.0 | 456 | 2026-10-14 | v2.2.2 (2026-10-10) | adapter | `npx -y @agentclientprotocol/codex-acp` | npm 2.2.2, sha512 `vNdeNV3j…w==` | benchmark |
| 2 | claude-agent-acp | agentclientprotocol/claude-agent-acp | TS | Apache-2.0 | 2649 | 2026-10-14 | v0.90.0 (2026-10-13) | adapter | `npx -y @agentclientprotocol/claude-agent-acp` | npm 0.90.0 tgz | benchmark |
| 3 | opencode | anomalyco/opencode (was sst/opencode) | TS | MIT | 213513 | 2026-10-14 | v1.19.0 (~10-11) | native | `opencode acp` | npm `opencode-ai@1.19.0` / GH zips | benchmark |
| 4 | goose | aaif-goose/goose (was block/goose) | Rust | Apache-2.0 | 55287 | 2026-10-14 | v1.54.0 (~10-08) | native | `goose acp` | GH `goose-*-{apple-darwin,unknown-linux-gnu,windows-msvc}.tar.bz2/zip` | benchmark |
| 5 | kimi-code | MoonshotAI/kimi-code | TS | MIT | 7871 | 2026-10-14 | 2.2.0 (2026-10-14) | native | `kimi acp` | GH `kimi-1.52.0-*.tar.gz/zip` | benchmark |
| 6 | fast-agent | evalstate/fast-agent | Py | Apache-2.0 | 3936 | 2026-10-14 | v0.10.44 (2026-10-10) | native | `uvx fast-agent-acp==0.10.1 -x --model <n>` | uvx/PyPI `fast-agent-acp==0.10.1` | benchmark |
| 7 | vtcode | vinhnx/VTCode | Rust | MIT/Apache | 881 | 2026-10-14 | 0.177.0 (2026-10-13) | native | `vtcode acp` | crates.io `vtcode 0.177.0` | benchmark |
| 8 | code-assistant | stippi/code-assistant | Rust | MIT | 183 | 2026-10-14 | v0.2.16 (~09-23) | native | `code-assistant acp [--model N]` | GH `code-assistant-linux-x86_64.zip` et al | benchmark |
| 9 | crow-cli | crow-cli/crow-cli | Rust | Apache-2.0 | unknown | active | v0.1.48 (~10-13) | native | `crow-cli acp` | GH `crow-cli-{darwin,linux,windows}-*.tar.gz/zip` v0.1.24 | benchmark |
| 10 | docker-agent | docker/docker-agent (was docker/cagent) | Go | Apache-2.0 | 4491 | 2026-10-14 | v1.150.0 (2026-10-09) | native | `docker agent serve acp <agent.yaml>` | GH `docker-agent-<os>-<arch>` binaries | benchmark |
| 11 | gemini-cli | google-gemini/gemini-cli | TS | Apache-2.0 | 107368 | 2026-10-14 | v0.64.0 (~10-13) | native | `npx -y @google/gemini-cli@0.64.0 --acp` | npm 0.64.0 tgz | benchmark |
| 12 | zerostack | gi-dellav/zerostack | Rust | GPL-3.0-only | 1708 | 2026-10-14 | v1.8.5 (~10-10) | native (feat-gated) | unknown (`--features acp`; no one-liner verified) | GH `zerostack-x86_64-unknown-linux-musl.tar.gz` / crates `zerostack` | benchmark* |
| 13 | fx | vercel-labs/fx | Zig | Apache-2.0 | ~3.4k | unknown | unknown | native | `fx acp` | GH Releases (asset name unverified; NOT setup.sh) | benchmark* |
| 14 | stakpak | stakpak/agent | Rust | Apache-2.0 | 1823 | 2026-07-06 | v0.3.88 (2026-06-10) | native | `stakpak acp` | GH `stakpak-*.tar.gz/zip` v0.3.88 | benchmark (stale) |
| 15 | mistral-vibe | mistralai/mistral-vibe | Py | Apache-2.0 | 5122 | 2026-10-09 | v2.26.1 (~10-09) | native | `vibe-acp` | PyPI `mistral-vibe==2.26.1` + `vibe-acp-*.tar.gz/zip` | benchmark |
| 16 | cline | cline/cline | TS | Apache-2.0 | 70333 | 2026-10-14 | desktop-v0.0.46 (10-13) | native | `npx -y cline@3.0.71 --acp` | npm `cline@3.0.71` | benchmark |
| 17 | openhands | OpenHands/OpenHands | TS | MIT | 90916 | 2026-10-14 | v1.27.1 (2026-10-12) | native | `openhands acp [flags]` | OpenHands CLI installer | benchmark |
| 18 | kiro-cli | proprietary (kiro.dev) | bin | proprietary | — | — | unknown | native | `kiro-cli acp [--agent N]` | `kiro-cli` binary | benchmark |
| 19 | cursor-agent | proprietary (cursor.com) | TS | proprietary | — | — | 2026.10.14 | native | `agent acp` | `agent` binary / registry tarball | benchmark |
| 20 | copilot-cli | github/copilot-cli (unverified) | TS | proprietary | — | — | npm 1.0.97 | native preview | `copilot --acp` | Copilot CLI | benchmark |
| 21 | pi-acp | svkozak/pi-acp (+ earendil-works/pi) | TS | MIT | 733 | 2026-10-06 | npm 0.0.34 | adapter | `npx -y pi-acp` | npm `pi-acp@0.0.34` | benchmark |
| 22 | claude-code-acp-rs | soddygo/claude-code-acp-rs | Rust | unknown | unknown | active | unknown | adapter | `claude-code-acp-rs` | `cargo install claude-code-acp-rs` | benchmark |
| 23 | auggie | augmentcode/auggie (proprietary) | TS | proprietary | — | — | 0.36.0 | native | `auggie --acp` | npm `@augmentcode/auggie@0.36.0` | benchmark |
| 24 | qwen-code | QwenLM/qwen-code | TS | Apache-2.0 | 28485 | 2026-10-14 | npm `@qwen-code/qwen-code` | native/bridge | unknown (daemon `qwen serve` + `packages/acp-bridge/`; no one-liner verified) | npm `@qwen-code/qwen-code@latest` | benchmark* |
| 25 | hermes-agent | NousResearch/hermes-agent | Py | MIT | 253784† | 2026-10-14 | unknown | native gw | `hermes acp` | source + venv (pip artifact unverified) | second-tier |
| 26 | amp-acp | tao12345666333/amp-acp | Go | Apache-2.0 | 99 | 2026-10-09 | v0.10.0 | adapter | `./amp-acp` | GH Releases | second-tier |
| 27 | crush-acp | willbnu/crush-acp (bridge; crush native: NO) | TS | MIT | unknown | active | unknown | adapter | `crush-acp` | npm `crush-acp` + crush CLI | second-tier |
| 28 | junie | JetBrains (proprietary) | JVM | proprietary | — | — | 3659.4.0 | native† | `junie --acp true` | Junie app / registry bundle | second-tier |
| 29 | factory-droid | factory.ai (proprietary) | TS | proprietary | — | — | npm 0.241.1 | native | `npx droid exec --output-format acp` | npm `droid@0.241.1` | second-tier |
| 30 | blackbox-cli | proprietary | ? | proprietary | — | — | unknown | native exp | `blackbox --experimental-acp` | install.sh (unpinned — avoid) | second-tier |
| 31 | glm-acp-agent | stefandevo/glm-acp-agent | TS | Apache-2.0 | unknown | active | npm 1.15.0 | native | `npx -y glm-acp-agent@1.15.0` | npm 1.15.0 | second-tier |
| 32 | zcode-acp | Liang-HZ/zcode-acp | Py | Apache-2.0 | 0 | 2026-10-14 | unknown | adapter | `uvx zcode-acp` | PyPI `zcode-acp` | second-tier |
| 33 | localharness | compusophy/localharness | Rust | Apache-2.0 | 9 | 2026-08-14 | v0.82.0 | native | unknown (quickstart unfetched) | crates `localharness 0.82.0` | second-tier |
| 34 | sigit | getsigit/sigit | Rust | NOASSERTION | 55 | 2026-10-14 | crates 1.6.9 | native | `sigit --acp` | crates `sigit 1.6.9` | second-tier |
| 35 | bub | bubbuild/bub (+ bub-acp-server) | Py | Apache-2.0 | 1685 | 2026-10-14 | unknown | adapter | `bub acp` | npm `bub-acp-server` | second-tier |
| 36 | raven | evermind-ai (repo unfetched) | ? | unknown | unknown | unknown | unknown | native | `uv run raven acp [--config P]` | `raven` binary | second-tier |
| 37 | construct | construct-worlds/construct | Rust | MIT | 18 | 2026-10-03 | unknown | native | `construct acp` | source build (no prebuilt) | second-tier |
| 38 | agentpool | phil65/agentpool | Py | MIT | 196 | 2026-04-25 | PyPI 2.9.18 | native | `uvx agentpool@latest` | PyPI `agentpool==2.9.18` | second-tier |
| 39 | cuga | cuga-project/cuga-agent | Py | Apache-2.0 | 898 | 2026-10-14 | unknown | native | `cuga-acp` | pip `cuga[acp]` | second-tier |
| 40 | zeph | bug-ops/zeph | Rust | MIT | 64 | 2026-10-06 | v0.22.5 (09-18) | native | unknown | GH musl tarballs | second-tier |
| 41 | kaagum | git.systemreboot.net/kaagum | Guile | GPL-3.0 | — | — | unknown | native | `kaagum --api-key-command=… --model=…` | Guix `kaagum` | second-tier |
| 42 | poolside | poolsideai/pool | Go | proprietary | 438 | 2026-08-18 | v1.0.16 | native | `pool acp` | `pool-*.tar.gz` (downloads.poolside.ai) | second-tier |
| 43 | pi-rust (rpi) | bigfish1913/pi-rust | Rust | unknown | unknown | unknown | unknown | native† | unknown | source (21.8 MiB claimed) | second-tier |
| 44 | kimi-cli | MoonshotAI/kimi-cli | Py | Apache-2.0 | 11423 | 2026-09-22 | ARCHIVED | — | n/a | n/a | skip |
| 45 | pig | MichaelKinsy/PiG | Go | MIT | 601 | 2026-10-13 | v0.4.1 | none found | unknown | npm `@pi-in-go/pig` | skip |
| 46 | stdio-bus | stdiobus/stdiobus | Shell | Apache-2.0 | 19 | 2026-09-04 | v2.0.3 | router | n/a | GH releases | skip |
| 47 | roy | strelov1/roy | Rust | none | 3 | 2026-05-30 | unknown | client-only | n/a | source | skip |
| 48 | minion-code | femto/minion-code | Py | AGPL-3.0 | 40 | 2026-10-05 | PyPI 0.1.45 | disputed | `mcode acp` | PyPI `minion-code==0.1.45` | skip |
| 49 | aether | NOT-FOUND | — | — | — | — | — | — | — | — | skip |
| 50 | harnx | NOT-FOUND (cf. registry `harn`) | — | — | — | — | — | — | — | — | skip |
| 51 | awaken | awakenworks/awaken | Rust | NOASSERTION | 98 | 2026-09-04 | unknown | native | unknown | source | record-only |
| 52 | cortex-code | Snowflake (proprietary) | bin | proprietary | — | — | ~1.0.73 | native | `./cortex acp serve` | sfc-repo tarballs | record-only |
| 53 | banjo | joelreymont/banjo | Zig | unknown | unknown | 2026 | unknown | adapter | unknown | source | record-only |
| 54 | muse-acp | BrokkAi/muse-acp | Rust | unknown | 19 | 2026-10-14 | unknown | adapter | `muse-acp` (verify) | source | record-only |
| 55 | hydra-acp | smagnuso/hydra-acp | TS | unknown | 37 | 2026-10-14 | unknown | native daemon | unknown | source | record-only |
| 56 | droid-acp | kingsword09/droid-acp | ? | unknown | unknown | 2026 | unknown | adapter | unknown | source | record-only |
| 57 | aider-acp | jorgejhms/aider-acp | Py | unknown | unknown | 2026 | unknown | adapter | unknown | source | record-only |
| 58 | acpira | hotic/acpira | Rust | unknown | 4 | 2026-10-14 | unknown | native | unknown | source | record-only |

`*` = benchmark-worthy modulo one verification (see Unknowns). `†` hermes
star count anomalous in API snapshot (verify identity). `†` junie announced/
coming-soon; pi-rust ACP via writeup only.

Registry-only mentions (not tabled, see `candidates.json` notes / registry):
Antigravity (`agy_acp_server.par`), Devin, Bob (IBM), Codebuddy, DimCode,
MiniMax, Nova, Qoder, Kilo, Kimchi, Corust (GPL-3.0), Harn, Dirac, DeepAgents,
Flixa, Intentic bridge, XENO, Prism, WrongStack, Martty, QLogic, DeepSeek
harness, OpenClaw, AutoDev, fount, Raxol, Muse Spark Code (unofficial),
Claw Orchestrator, S1Code (unverified), Prux (unverified), chump, rustcode,
aar, bobby-browser, codehalter, coma, mow, agent-wire, mjolnir, OpenACP,
a2acode, qodo-acp, dsh-acp-replay, deepseek-harness-aionui, claude-agent-tui.

## B. Interop / config table (daemon needs)

| Name | session/new mcpServers | cancel / load / resume / set_config | Provider config (path, keys) | OpenAI-baseURL? | Model catalog | Subscription logins |
|---|---|---|---|---|---|---|
| codex-acp | YES | cancel YES; subagent sessions YES | `CODEX_CONFIG` JSON env; `MODEL_PROVIDER` | YES (custom gateway) | Codex app-server list | ChatGPT, API key, gateway |
| claude-agent-acp | PARTIAL (MCP OAuth) | full session ops claimed + v2 | Agent SDK auth | NO | Claude via SDK | Claude sub |
| opencode | YES (via config) | unknown | `opencode.json` | unknown | models.dev + Zen | Zen sub |
| goose | unknown | unknown | unknown | unknown | GDK models | unknown |
| kimi-code | YES (documented shape) | load/resume/list/fork/set_mode/set_model YES | `/login` OAuth / platform key | YES (claimed) | Kimi + compatibles | Kimi OAuth |
| fast-agent | YES (client attach) | sessions list; rest unknown | `fast-agent.yaml` + `.secrets.yaml` | YES | multi-provider + `--model` | keys + keyring OAuth + HF |
| vtcode | unknown | cancel YES; resume via CLI | `vtcode.toml` | unknown | hosted/local | keys via config/env |
| code-assistant | unknown | resume `--continue-task` | `~/.config/code-assistant/providers.json`+`models.json`, `base_url` | YES | hand `models.json` | ChatGPT (`codex_auth`) |
| crow-cli | YES (documented shape) | unknown | `~/.agents/crow/config.yaml`, `base_url` | YES | `crow-cli models` | e.g. OpenRouter key |
| docker-agent | unknown | SQLite persistence | `agent.yaml`; `--models-gateway URL` | YES | any id; gateways | cloud keys / DMR local |
| gemini-cli | YES (bidir) | cancel/new/load | `~/.gemini/settings.json` | unknown | Gemini/Vertex | Google OAuth† |
| zerostack | unknown | unknown | `~/.local/share/zerostack/config.yaml`, base URL / key env | YES | unknown | key env |
| fx | unknown | unknown | unknown | unknown | unknown | unknown |
| stakpak | YES (config) | unknown | `~/.stakpak/config.toml` BYOK | unknown | profiles | `auth login` |
| mistral-vibe | unknown | unknown | `config.toml`; `MISTRAL_API_KEY` | unknown | Mistral | key / browser auth |
| cline | unknown | cancel/load claimed | unknown | unknown | Cline roster | unknown |
| openhands | YES (stdio JSON-RPC) | resume flags | `~/.openhands/settings.json` | unknown | CLI roster | CLI creds reuse |
| kiro-cli | YES (example) | load YES; set_mode/set_model YES | unknown | unknown | `--agent` list | AWS/Kiro (unver.) |
| cursor-agent | YES (example) | resume YES | unknown | unknown | Cursor models | `cursor_login` / key |
| copilot-cli | YES (`newSession`) | session/new opts few | unknown | unknown | Copilot; `--effort` | GH sub; BYOK |
| pi-acp | ACCEPTED NOT WIRED | load/fork YES; no fs/terminal | `~/.pi/agent/settings.json` | unknown | pi roster | pi-side (unver.) |
| claude-code-acp-rs | unknown | unknown | Claude Code auth | NO | via CLI | Claude sub via CLI |
| auggie | unknown | unknown | CLI flags | unknown | Augment | Augment sub |
| qwen-code | unknown | unknown | `/auth`; settings ref | YES (claimed) | Qwen/OpenAI/Anth/Gemini | provider keys |
| hermes-agent | YES (gateway passthrough) | model picker; auto-approval | `~/.hermes/config.yaml` custom endpoints | YES | custom endpoints | keys via config |
| kaagum | unknown | NO resume (roadmap) | CLI flags; any base_url | YES (incl. llama.cpp) | any id | any key cmd |
| agentpool | unknown | new_session YES | YAML + env keys | YES (LiteLLM) | multi-agent/LiteLLM | keys via env |
| cuga | inbound unknown | NO load/resume; text-only | supervisor YAML (Dynaconf) | unknown | supervisor YAML | unknown |
| sigit | unknown | session/new YES | unknown | unknown | local Qwen | none (local) |

† unpaid/Google One users moved to Antigravity CLI (2026-06-18).

## C. Top 15 (start command + pinned artifact)

1. **codex-acp** — `npx -y @agentclientprotocol/codex-acp@2.2.2`
   (`https://registry.npmjs.org/@agentclientprotocol/codex-acp/-/codex-acp-2.2.2.tgz`,
   sha512 `vNdeNV3jxbqaH7MYvVWzyB6cMOz+YGIV9W1xyBDsQfoii92DulZLM+ahCL2T2G6uohlaB1Yma347p2kLfAab2w==`).
   mcpServers YES, ChatGPT sub + gateway, images YES.
2. **claude-agent-acp** — `npx -y @agentclientprotocol/claude-agent-acp@0.90.0`
   (`…/-/claude-agent-acp-0.90.0.tgz`). Full session ops + v2 draft.
3. **opencode** — `opencode acp` (npm `opencode-ai@1.19.0`,
   `https://registry.npmjs.org/opencode-ai/-/opencode-ai-1.19.0.tgz`).
   NOTE: upstream moved `sst/opencode` → `anomalyco/opencode`; confirm before pin.
4. **goose** — `goose acp` (GH v1.54.0 `goose-*-{apple-darwin,unknown-linux-gnu,windows-msvc}.tar.bz2/zip`).
   NOTE: moved `block/goose` → `aaif-goose/goose`.
5. **kimi-code** — `kimi acp` (GH `kimi-1.52.0-*.tar.gz/zip`; release `2.2.0`).
   Best session/new + set_model docs. Legacy `kimi-cli` archived — ignore.
6. **fast-agent** — `uvx fast-agent-acp==0.10.1 -x --model <name>` (PyPI).
   Client MCP attach YES; `-x` maps to fs/terminal delegation.
7. **code-assistant** — `code-assistant acp [--model "<name>"]`
   (GH v0.2.16 `code-assistant-linux-x86_64.zip` et al).
   `providers.json` base_url + ChatGPT sub = closest config fit.
8. **vtcode** — `vtcode acp` (crates `vtcode==0.177.0`). Rust, verified ACP smoke doc.
9. **crow-cli** — `crow-cli acp` (GH v0.1.24 tarballs; repo v0.1.48).
   Only small native documenting the exact `mcpServers` session/new shape.
10. **docker-agent** — `docker agent serve acp <agent.yaml>` (GH v1.150.0 binaries).
    `--models-gateway <url>` = fake-endpoint lever. Renamed ex-`cagent`.
11. **gemini-cli** — `npx -y @google/gemini-cli@0.64.0 --acp`. Bidir MCP; auth-migration caveat.
12. **zerostack** — start TBD (GH v1.8.5 musl tarball / `cargo install zerostack --features acp`).
    Verify stdio form first; GPL-3.0-only.
13. **fx** — `fx acp` (GH Releases pinned asset; do NOT use `setup.sh` for bench).
    Zig ~6 MB; newest/least-verified of the 15.
14. **stakpak** — `stakpak acp` (GH v0.3.88 zips). STALE since 2026-07 — pin and note drift.
15. **mistral-vibe** — `vibe-acp` (PyPI `mistral-vibe==2.26.1` / `vibe-acp-*.tar.gz/zip`).

## D. Biggest unknowns (in bench-blocking order)

1. **session/new `mcpServers` support**: verified YES for only 8
   (codex-acp, kimi-code, fast-agent, crow-cli, kiro, cursor, copilot,
   hermes-gateway, + opencode/stakpak via config). Unknown for goose, gemini
   (partial), vtcode, code-assistant, zerostack, fx, qwen — needs a live
   `initialize` + `session/new` probe per agent.
2. **cancel/load/set_config_option matrix**: only kimi-code, kiro, cursor,
   copilot, openhands, pi-acp document parts; the rest need probing.
3. **Provider baseURL keys** (fake-local-LLM endpoint): confirmed for
   codex-acp (gateway), code-assistant (`base_url`), crow-cli (`base_url`),
   docker-agent (`--models-gateway`), hermes (`config.yaml`), kaagum
   (any URL), agentpool (LiteLLM), kimi-code/qwen (claimed). Unknown
   everywhere else.
4. **zerostack stdio form**: ACP behind `acp` cargo feature; no `zerostack acp`
   one-liner in README — read `docs/` / `--help` before bench.
5. **qwen-code end-user command**: daemon + `packages/acp-bridge/` documented,
   no verified one-liner (do NOT assume `qwen --acp`).
6. **Image support**: claimed for codex-acp, kimi-code (passthrough limits),
   cuga (explicit NO); unverified for all other natives.
7. **RAM/startup**: only pi-rust claims numbers (12 MiB RSS, 17.5 ms; writeup
   only) and fx claims 6 MB; no independent measurements — bench must measure.
8. **Rate-limit gaps**: tail stars/pushed_at/license/releases unfetched
   (GitHub API 60/hr exhausted); `candidates.json` marks them `unknown`.
9. **Repo-identity risks**: `anomalyco/opencode` (fork or new upstream?),
   hermes-agent star-count anomaly, minion-code protocol confusion, `raven`
   repo URL unresolved, `acp-agent` name collisions.
10. **Subscription logins**: SuperGrok/ChatGPT/OpenCode-Go style logins mostly
    unverified beyond ChatGPT (codex-acp, code-assistant), Claude (adapters),
    Kimi OAuth, Cursor, Copilot — each needs an interactive-login probe.

## E. Method + sources

- Official agents page `https://agentclientprotocol.com/overview/agents`
  (47 entries) + machine-readable registry
  `https://cdn.agentclientprotocol.com/registry/v1/latest/registry.json`
  (repo `agentclientprotocol/registry`, 417★, Apache-2.0, pushed 2026-10-14).
- Zed external agents (`zed.dev/docs/ai/external-agents`, `zed.dev/acp`),
  JetBrains ACP (`jetbrains.com/acp`, Junie doc), vendor docs (docker, Kiro,
  Cursor, Copilot, OpenHands, fast-agent, Kimi, Gemini, Blackbox, Augment).
- GitHub API `stars/pushed_at/license/releases` for 30+ repos (rate limit hit
  at 60/hr); npm (`registry.npmjs.org`), PyPI (`pypi.org/pypi/…/json`),
  crates.io (`/api/v1/crates/…`, UA header) for pinned versions.
- GitHub topic/search sweeps (`agent-client-protocol` 446 tagged, 1244 code
  hits; `acp-agent` 1668; rust 236 / go 136), awesome-acp lists
  (`tabtabtabai/awesome-acp`), HN Algolia (no new 2026 lightweight stories —
  only 2025 ACP threads), fresh-month web searches (found fx, zerostack,
  pi-rust, S1Code, Prux).
