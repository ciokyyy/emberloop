# 21: Live probes

**What to build:** Live probe scripts prove on real Backends: a Reviewer `submit`, a Chain advance on a real limit or auth failure, a Switch of a paused Ticket, and a two-spec Run.

**Blocked by:** reviewer-submit-tool (02), spec-branch-core (09), fallback-chain-run (14), pause-and-switch-api (15)

**Status:** ready-for-agent

- [ ] One probe script per behavior under `scripts/`, documented in `scripts/AGENTS.md`
- [ ] Each probe run once; outcome recorded in this ticket
- [ ] Never wired into `.emberloop/check`
