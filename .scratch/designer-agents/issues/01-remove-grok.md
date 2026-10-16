# 01: Remove the grok Backend

**What to build:** The grok Backend and everything that exists only for it are gone (spec, Grok removal). No Backend, Chain, setting, UI option, probe or doc names grok as a supported harness. Stored Sessions that used grok stay readable and cannot be resumed or prompted: the Daemon refuses with a message that grok support was removed and never falls through to the default Backend. Saved Chain entries and role settings that name grok are dropped on read with a notice; a Run's stored override or Chain position that named it is cleared and the Ticket uses the configured defaults. Re-read `emberloop-web/grokacp.go`, `daemon.go` (`backendForLocked`, `resumeBackendLocked`, `handleModels`, `modelEffortOptions`), `chains.go` (`validateChainEntry`, `lookupChain`), `settings.go`, `claude.go` (plain-adapter hooks: keep any that pig-acp still uses, delete only grok-only branches), `acpretry.go`, `prompts.go`, `cmd/emberloop-web/main.go`, `web/src/lib/harnesses.js` and the settings and runs tests first.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] `grokacp.go`, `grokacp_test.go`, `grok_handoff_test.go` and `grokacp_live_test.go` are deleted; the `grok` Backend registration, `GROK_HOME` and `EL_LIVE_GROK` are gone
- [ ] `/api/models`, Settings PUT and Chain validation reject `grok` with a "harness must be claude or pig" style error; the web harness list, icon and model entry for grok are gone
- [ ] A saved Chain entry or role setting naming grok is dropped on read with a notice; a Run override or Chain position that referenced it is cleared; tests cover Chains, legacy role keys and a restored Run
- [ ] A stored Session with harness `grok` loads, shows its history, and refuses resume or prompt with "grok support was removed"; it never reaches the fake or default Backend (test at the Daemon API seam)
- [ ] `GLOSSARY.md` (Backend), `emberloop-web/AGENTS.md` (Purpose, adapter retry), the web README if it names grok, and `docs/adr/0005-designer-tickets-agent-browser-cli.md` (replace the grok Backend sentence with one line saying grok support was removed) carry no stale grok text
- [ ] No grep hit for `grok` remains outside `.scratch/` history, the ADR line above, and the refusal message and its test
- [ ] `.emberloop/check` passes; in `emberloop-web/web/`, `npm test` and `npm run build` pass
