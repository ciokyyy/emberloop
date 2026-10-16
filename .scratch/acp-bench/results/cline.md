# cline 3.0.71 — bench result: BLOCKED (longtail worker, 2026-10-15)

Verdict: **blocked: ACP `session/new` requires interactive OAuth
(`authenticate`) even with a locally configured API-key provider.**
No headless path found; per instructions no real login attempted.

- Artifact: `npm cline@3.0.71`
  tarball `https://registry.npmjs.org/cline/-/cline-3.0.71.tgz`
  integrity `sha512-THSCuQrVPi5w48tyXaZrdUr4EnPQMMgsQN0moYxODl0LJveOeRytMyHx+WfzCY5NdPi1TzjDozFGn0uNVbK/fg==`
  (406 MB on disk; compiled Bun binary `bin/cline` → `@cline/cli-linux-x64`).
- Provider config (file-based, good design, unreachable over ACP):
  `cline auth --provider openai --apikey K --modelid M --baseurl U` writes
  `~/.cline/data/settings/providers.json`:
  `{"version":1,"lastUsedProvider":"openai-compatible",
    "providers":{"openai-compatible":{"settings":{"provider":"openai-compatible",
    "apiKey":"…","model":"fake-tools","baseUrl":"http://127.0.0.1:18600/v1"},
    "tokenSource":"manual"}}}` — verified written; custom endpoint WITHOUT rebuild.
- Protocol-only: `initialize` ok (5.2 s; caps loadSession + image;
  `authMethods`: `cline`, `cline-pass`, **`openai-codex` ("Sign in with
  ChatGPT Subscription")**).
- `session/new` → `-32000 Authentication required: Call authenticate before
  starting a session`. CLI flags (`--acp -P openai-compatible -m …`) do NOT
  bypass it. `authenticate {methodId:"cline"}` starts an interactive OAuth
  device flow (`[acp/auth] Enter this code in your browser: …`) — daemon-blocked.
- Subscription surface (from client): Cline account, ClinePass, ChatGPT
  subscription (openai-codex). No SuperGrok/Antigravity/OpenCode-Go/Copilot.
  Tokens would live under `~/.cline/data/`.
- Daemon note: IF a headless token-seed for `authenticate` is ever found
  (env-provided Cline API key honored over ACP), re-bench — the file
  provider config is otherwise ideal. acpbench also needs an `authenticate`
  step for such agents (probe gap).
