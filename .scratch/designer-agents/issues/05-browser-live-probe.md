# 05: Browser CLI live probe

**What to build:** An env-gated live probe, never part of the repo check, proves on real agents that the browser CLI works end to end. On claude and on pig (ACP and SDK) a Session is asked to: confirm `AGENT_BROWSER_SESSION` and `PATH` in `bash` and in `execute`; open a local static page; take a snapshot; take a phone-width screenshot to the session screenshot dir; `read` the file; and describe what it shows. The probe records per Backend whether the screenshot image reached the model (the one fact the research left unverified: `anthropics/claude-code#54737` reports MCP image blocks dropped; this path uses `read`, not MCP). Findings go in the ticket's closing notes and `.scratch/designer-agents/research-browser-tooling.md` open question 1. Follow `scripts/AGENTS.md` and an existing probe such as `scripts/compress-pig-acp` for shape.

**Blocked by:** 03-browser-cli-install, 04-browser-session-env-and-cleanup

**Status:** done

- [x] `scripts/browser-probe.sh` (or the repo's probe convention) runs against a real Daemon, gated by an `EL_LIVE_*` variable, with a cheap vision-capable model per Backend
- [x] The probe passes or reports a per-Backend verdict: env inherited, snapshot works, screenshot `read` reaches the model
- [x] If a Backend drops the image, the probe says so and a follow-up note names the fallback (snapshot-only Designer on that Backend); no silent pass
- [x] `scripts/AGENTS.md` lists the probe and its env gate
