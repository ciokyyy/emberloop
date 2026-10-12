# 21: Live probes

**What to build:** Live probe scripts prove on real Backends: a Reviewer `submit`, a Chain advance on a real limit or auth failure, a Switch of a paused Ticket, and a two-spec Run.

**Blocked by:** reviewer-submit-tool (02), spec-branch-core (09), fallback-chain-run (14), pause-and-switch-api (15)

**Status:** ready-for-agent

- [ ] One probe script per behavior under `scripts/`, documented in `scripts/AGENTS.md`
- [ ] Each probe run once; outcome recorded in this ticket
- [ ] Never wired into `.emberloop/check`

## Live probe outcomes (2026-10-12)

- `run-upgrade-reviewer-submit-probe.sh` — **FAIL / unproven** (single acceptance run `16e46102db827c573ee056012ef3f047`, Pig `openrouter/anthropic/claude-haiku-4.5`, 360s). The fake Worker submitted and the fake standards Reviewer passed, but the real Pig spec Reviewer remained working and emitted no typed `submit` Event before timeout; cleanup paused the Run. The behavior remains unproven; follow-up `23-reviewer-submit-live-probe-follow-up.md` owns diagnosis and any retry.
- `run-upgrade-chain-probe.sh` — **PASS** (run `87d28f777ab75f265c23dec35029c440`). The intentionally invalid Claude API key produced a real auth failure; the Worker Chain advanced to Pig on the same branch, received a Handoff and paused.
- `run-upgrade-switch-probe.sh` — **PASS** (run `1c8cb39e3fb754d385ad0930a684bdd2`). A paused Ticket switched from Pig `openrouter/openai/gpt-4o-mini` to Pig `openrouter/anthropic/claude-haiku-4.5`; Switch did not start a Session, and manual Resume kept the branch and delivered a Handoff.
- `run-upgrade-two-specs-probe.sh` — **PASS** (runs `dd882965bbb47ef6dcac4ff8af87666d` and `fd09549cfdb0e0cbc6864f4ff903ff50`). Both real Pig Workers ran concurrently on `spec/probe-north` and `spec/probe-south`; both Runs paused cleanly.
