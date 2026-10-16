# AGENTS.md — .scratch/acp-bench

Discovery and benchmark area for ACP-capable coding agents/harnesses against
the Daemon's ACP spawn path (stdio, per-session MCP servers, cancel,
load/resume, model selection, images, low RAM, custom catalogs, subscription
logins). Benchmarks ran only against the fake LLM (`scripts/fakellm`) with the
probe `scripts/acpbench`; never a real model API.

- `candidates.md` — human-readable survey: master table, interop/config
  table, top-15 ranking, unknowns. Verified 2026-10-14 from primary sources
  (official ACP agents page + registry JSON, repo READMEs/docs, npm/PyPI/
  crates.io metadata, GitHub API).
- `candidates.json` — same candidates machine-readable (one object per
  candidate, `unknown` where unverified, `sources` per entry).
- `results/` — benchmark output of the 2026-10-14/15 rounds:
  `results/final/<agent>.md` and `.json` (uniform final round), earlier
  per-agent files beside them, `results/_ranking.md` (synthesis and
  recommendation), `results/_skipped.md` (skipped candidates).

Refresh rules: re-fetch `registry.json`
(`https://cdn.agentclientprotocol.com/registry/v1/latest/registry.json`) and
the official agents page first; GitHub API is rate-limited (60 req/hr
unauthenticated) so batch `stars/pushed_at/license/releases` lookups and reuse
`candidates.json` values when fresh. Discovery only surveys; benchmark runs
install candidates only under `/tmp/acpbench/work/<agent>/` with an isolated
HOME. No check wiring (scratch area, not built or tested).
