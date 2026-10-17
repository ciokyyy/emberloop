# 07: Chat chrome design

**Type:** design

**What to build:** Header, context meter, Working indicator, Compress banner, composer controls and tool strip. Header: title, one status glyph, menu; Git and Plus New as icon buttons. Context meter: small ring with tap-open detail sheet (model, tokens, Compress). Working indicator: activity glyph plus current tool in a few characters. Compress banner: one line with icon actions (Retry, Change model, Dismiss) and tooltips. Composer: attach, mode switch (Send, Steer, Follow-up) and send as icons with the active mode obvious; Remember as a toggle icon. Tool strip: icon per tool kind, collapsed by default, summary line. Re-read `App.svelte` header, `ContextMeter`, `WorkingIndicator`, `CompressBanner`, `Composer`, `ToolStrip`.

**Blocked by:** 01-design-foundation, 02-scroll-stability

**Status:** open

- [ ] No visible text button remains where an icon with tooltip carries the same meaning
- [ ] Header fits one line at 360 px width
- [ ] Composer modes distinguishable without reading; keyboard shortcuts unchanged
- [ ] Long tool runs collapse to one summary line with expand
- [ ] Console and a11y clean; screenshots at 390 and 1280 px in the review
- [ ] `npm test`, `npm run build`, `.emberloop/check` pass
