# 16: Gated live probe of real quota endpoints

**What to build:** A gated live probe proves the Claude and ChatGPT Plus readers against the real upstreams, read-only: it exercises the pollers, records what the real responses carry, and never runs as part of the check.

**Blocked by:** 07-claude-poller, 08-chatgpt-plus-card

**Status:** done

- [x] One probe script per behaviour under the live-probe scripts area, documented in its module doc
- [x] Each probe touches only read-only quota endpoints; no login is refreshed, rewritten or deleted
- [x] Each probe run once; outcome recorded in this ticket
- [x] Never wired into the repo check

## Probe run outcomes (2026-10-14)

- Claude: `EL_LIVE_CLAUDE_QUOTA=1 scripts/claude-quota-live-probe.sh` ran once. The poller returned `state=expired`, `plan=Pro`, with no windows. The host login's `expiresAt` was already past, so the poller correctly made no upstream request; the login file was unchanged. Live endpoint confirmation needs a valid host Claude login and was not attempted by refreshing it.
- ChatGPT Plus: `EL_LIVE_CHATGPT_QUOTA=1 scripts/chatgpt-quota-live-probe.sh` ran once and the real usage request returned `state=ok`, `plan=Plus`: 5-hour 72% (`resets_at=1791999569`) and Weekly 28% (`resets_at=1792487763`). The pig login file was unchanged.

Both probes used an isolated temporary Daemon and quota cache; neither login was refreshed, rewritten or deleted.
