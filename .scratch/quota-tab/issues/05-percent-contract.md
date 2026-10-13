# 05: Percent contract with word labels and states

**What to build:** Every quota card speaks one comparable language: percent used first, money and tokens as secondary text, window names in words. The Daemon produces the percent; the UI never computes one from dollars. OpenCode-Go and Antigravity keep their existing fetchers and gain the new labels.

Response contract (from the spec): each provider carries `key`, `label`, `plan`, `state` (`ok`, `stale`, `expired`, `not_configured`, `error`), `updated_at`, `error`, optional `hint`, and `windows[]`. Each window carries `label` (word form: 5-hour, Weekly, Monthly), `pct` (integer 0-100, percent used), `resets_at`, and optional `detail` (secondary text such as `$59.16 of $63.00 · $3.84 left`). Antigravity returns two groups. `pct` is the only field the bar and the Live status list read.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] HTTP seam returns the contract above for every card, verified against local stubs
- [ ] All window labels are word form; no short labels remain
- [ ] Every window carries `pct` 0-100 (percent used); `detail` carries dollars/tokens where they exist
- [ ] `stale` keeps last values with their age; `expired` keeps last values dimmed with a hint; `not_configured` names the missing login in `hint` and is never an error
- [ ] Antigravity returns both groups, each with a 5-hour and a weekly bar
- [ ] Providers register through one small interface (fetch to windows and state) so the cache, the endpoint and the refresher stay provider-agnostic; tickets 06, 07 and 08 each add one provider through it without touching the others
- [ ] Go tests at the HTTP seam; repo check passes
