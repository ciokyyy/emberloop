# 16: Chain and switch ui

**What to build:** The Run view shows each Ticket's active Chain entry, a pause control per Ticket and for the Run, a Backend and model picker on paused items, and a resume button. Settings edit the Chains.

**Blocked by:** fallback-chain-run (14), pause-and-switch-api (15)

**Status:** ready-for-agent

- [ ] Active Chain entry visible per Ticket
- [ ] Pause, picker and resume work from the UI
- [ ] Settings page edits per-role Chains
- [ ] `RunsView.test.js` and settings tests cover each; dist is not committed; `.emberloop/check` passes
