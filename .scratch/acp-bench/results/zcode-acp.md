# zcode-acp 0.9.1 — bench result: BLOCKED (longtail worker, 2026-10-15)

Verdict: **blocked: requires the ZCode macOS desktop app engine
(`/Applications/ZCode.app/…/zcode.cjs`) + ZCode subscription; no Linux
engine, no HTTP endpoint to redirect.**

- Artifact: `pip zcode-acp==0.9.1` in an isolated venv (26 MB; binary
  `zcode-acp`, module `zcode_acp.py` ~4.8k lines, Apache-2.0).
- Protocol-only: `initialize` ok (1.1 s; `authMethods`: `zcode-login`
  "Sign in through the ZCode desktop app… no API keys are proxied";
  `loadSession: false`). `session/new` → **timeout-hang** (30 s): bridge
  tries to spawn `node $ZCODE_CJS app-server`, which doesn't exist on
  Linux; the failure is silent, no error reply.
- Env levers (all engine-oriented, none fakellm-compatible): `ZCODE_CJS`
  (engine path), `ZCODE_ACP_MODEL`, `ZCODE_ACP_APPROVAL_BRIDGE/DIR`
  (file-based permission bridge), `ZCODE_ACP_THOUGHT`, `ZCODE_PLAN_BALANCE_URL`.
  Modes default/acceptEdits/plan/bypassPermissions; `_meta.steering`
  outcomes enumerated. Interesting MCP note in source: engine-side stdio
  entries REQUIRE `env` as an array (acpbench sends `env: []` — compatible).
- Gotcha: bare `--help`/tty startup crashes with
  `PermissionError: … selectors EVENT_READ` (works under pipes).
- Subscriptions: ZCode subscription via desktop app only. Nothing else.
