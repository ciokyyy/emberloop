# 07: Ladder editor, routing badge and credit cards

**Type:** design

**What to build:** Settings section for Ladders: rungs with provider and model pickers, schedule editor (UTC weekly windows, default peak hours), the active rung marked, balance and credit-flag status, and a model-picker entry for each Ladder. The transcript turn badge ("via Relace · peak"). The credits quota cards (dollars, "credits unknown" and warning states). Phone and desktop. Re-read the Settings sections, the Chain editor, the quota page and the transcript turn components.

**Blocked by:** 04-credit-quota-cards, 06-ladder-settings-and-chain, 03-usage-provider-and-cost

**Status:** ready-for-agent

- [ ] Both rungs of the seeded Ladder are visible with the active one marked at 390 px and 1280 px
- [ ] A schedule edits without a text file
- [ ] Credits cards read as dollars with no percent
- [ ] No horizontal overflow; 44 px targets on phone
- [ ] Console and a11y clean; `npm test`, `npm run build` and `.emberloop/check` pass

Spec: `../spec.md`.
