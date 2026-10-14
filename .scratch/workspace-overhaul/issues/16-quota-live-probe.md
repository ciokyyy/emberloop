# 16: Gated live probe of real quota endpoints

**What to build:** A gated live probe proves the Claude and ChatGPT Plus readers against the real upstreams, read-only: it exercises the pollers, records what the real responses carry, and never runs as part of the check.

**Blocked by:** 07-claude-poller, 08-chatgpt-plus-card

**Status:** ready-for-agent

- [ ] One probe script per behaviour under the live-probe scripts area, documented in its module doc
- [ ] Each probe touches only read-only quota endpoints; no login is refreshed, rewritten or deleted
- [ ] Each probe run once; outcome recorded in this ticket
- [ ] Never wired into the repo check
