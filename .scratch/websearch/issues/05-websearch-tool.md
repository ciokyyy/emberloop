# 05: `websearch` tool with Brave and Exa

**What to build:** A `websearch` tool (query, count, freshness, domains, provider, fresh) served through MCP and the pig adapter. `provider` is `brave`, `exa` or `auto`; the default comes from `search.provider` in settings, edited with `wick-pig settings`. Under `auto` it tries Brave then Exa on error, timeout, 429 or quota, 5xx and zero results; a bad-query 4xx does not fall back; a named provider never falls back. Missing keys are skipped under `auto` and an error when named. Output is a numbered list (title, URL, snippet, date, sources) with a footer naming the answering provider, fallbacks, cache status and latest quota. Results are cached for 15 minutes in the database (`fresh: true` bypasses). Each live call records Brave's rate-limit headers or Exa's reported cost in the quota table. A `search.mode: parallel` setting runs providers concurrently and merges by reciprocal rank fusion; it is off by default and not on the tool schema.

**Blocked by:** 03

**Status:** ready-for-agent

- [ ] Chain order, no-fallback for a named provider and bad-query handling are tested against fake provider servers
- [ ] Missing-key behaviour for both `auto` and a named provider
- [ ] Cache hit, expiry and `fresh` bypass
- [ ] Quota rows written from Brave headers and Exa cost, nothing invented when absent
- [ ] Parallel mode merges and tags `sources`, behind the setting
- [ ] `search.provider` and `search.mode` settable through `wick-pig settings`
- [ ] Tool is available to every role that can read; repo check passes
