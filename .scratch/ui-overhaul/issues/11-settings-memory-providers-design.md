# 11: Settings, Memory, Providers

**Type:** design

**What to build:** Settings pages, Memory, Providers, Profile, Notifications, PromotionQueue on shared `Section`/`Row`/`Card`. At most one paragraph of help visible per page; the rest behind `InfoHint`. Memory opens on a summary (counts, pending approvals) with Rules, Profile and ledgers in sections. Providers share the Quota card style, percent first. Chain editing on phone becomes a list that opens a sheet per entry. Re-read `SettingsPage`, `SettingsHub`, `SettingsNav`, `MemoryPage`, `ProvidersPage`, `ProfileSettings`, `Notifications`, `PromotionQueue`.

**Blocked by:** 01-design-foundation

**Status:** open

- [ ] Memory first viewport shows summary and pending approvals, not prose
- [ ] Providers cards match Quota cards; connect flows unchanged in behaviour
- [ ] Chain editor usable at 390 px
- [ ] Pages with no breakpoints today (Profile, Notifications, PromotionQueue) lay out correctly on phone
- [ ] Console and a11y clean; `npm test`, `npm run build`, `.emberloop/check` pass
