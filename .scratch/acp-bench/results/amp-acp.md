# amp-acp 0.10.0 — bench result: BLOCKED (longtail worker, 2026-10-15)

Verdict: **blocked: Amp cloud only — no endpoint override; needs real
`amp login` / `AMP_API_KEY`.** Protocol-only results below.

- Artifact: `npm amp-acp@0.10.0`
  tarball `https://registry.npmjs.org/amp-acp/-/amp-acp-0.10.0.tgz`
  integrity `sha512-6wa/Jxr29Bf1sqWVnPxnQAg4wN2XaxEI2ULUIHux6PR707jyFjwy4BhKcX6FboAB+Zz3HfNCmwXg3rRr0RXV8A==`
  (149 MB; bundles `@ampcode/sdk` + `@ampcode/cli-linux-x64` — Amp CLI
  `0.0.1792008042` runs, but `amp login` is a real account login).
- v0.10.0 uses the Amp CLI as default transport (`AMP_ACP_TRANSPORT=sdk`
  fallback); SDK shells out to the CLI — no `*_URL/*_ENDPOINT` override
  anywhere in adapter or SDK (only `AMP_SDK_CLI_BASE_URL` for the CLI
  *download*). Redirecting to fakellm is impossible short of TLS MITM.
- Protocol-only (with fake `AMP_API_KEY`, sdk transport):
  `initialize` ok (1.3 s; caps: loadSession, session resume, image+
  embeddedContext, MCP http+sse; `_meta.amp-acp/thread-lifecycle` native
  thread extensions; `authMethods`: interactive `setup`);
  `new_session` ok (9 s; mcpReady not-advertised).
- No prompt attempted (would call the real Amp API — forbidden).
- Providers/catalog: Amp-side only (modes low/medium/high/ultra + plugin
  agent modes); env surface `AMP_API_KEY/AMP_CLI_PATH/AMP_HOME/
  AMP_SETTINGS_FILE/AMP_MODE/AMP_MODELS/AMP_ACP_STATE_DIR/…`; durable
  session store `$XDG_STATE_HOME/amp-acp/sessions` with thread-ID mapping,
  `session/load` replays via `amp threads export`. No custom provider
  without rebuild; no remote-catalog override for the host.
- Subscriptions: Amp subscription only (`amp login`; headless
  `AMP_API_KEY`). No SuperGrok / ChatGPT-sub / Antigravity / OpenCode-Go /
  Claude-sub / Copilot.
