# 06: Message, queue and report design

**Type:** design

**What to build:** Style the behaviour from 04 and 05. User bubbles, assistant messages, queued items (dashed or dimmed with clock glyph), action toolbar and long-press sheet (icons: Copy, Share, Edit, Cancel, Retry), report notice (icon, group name, counts, one-line summary, expand, dismiss), Reports tray and unread dot. Phone: long-press sheet at bottom, 44 px targets; desktop: hover toolbar with tooltips. Use the foundation primitives and icon map. Do not change logic or tests' behaviour.

**Blocked by:** 01-design-foundation, 04-message-actions-logic, 05-report-notices-logic

**Status:** open

- [ ] A report never looks like a User message at 390 px or 1280 px
- [ ] Actions are icon buttons with tooltip and accessible name; no hover-only access on phone
- [ ] Queued versus delivered is readable in under a second
- [ ] Reports tray readable with 1 and with 10 reports; no overflow
- [ ] Console and a11y clean; screenshots at both sizes attached in the review
- [ ] `npm test`, `npm run build` pass; `.emberloop/check` passes
