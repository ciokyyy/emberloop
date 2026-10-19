# 14: Babysitter timeline lines and needs-you diagnosis

**Type:** design

**What to build:** Style how the Babysitter shows in the UI. Each diagnosis and action is one muted line on the Ticket timeline in the Run page, with a quiet icon and the time. Shadow-mode actions read as would-have, visibly different from executed ones. An escalation shows its diagnosis in the needs-you reason where the User already looks. Use the foundation primitives and the icon map. Do not change logic.

**Blocked by:** 10-babysitter-action-queue

**Status:** ready-for-agent

- [ ] A Babysitter line never reads as a Worker or Reviewer message at 390 px or 1280 px
- [ ] Would-have lines are distinguishable from executed lines in under a second
- [ ] An escalation's diagnosis is readable in the needs-you reason without opening a log
- [ ] No overflow with a long diagnosis; console and a11y clean
- [ ] `npm test`, `npm run build` pass; `.emberloop/check` passes
