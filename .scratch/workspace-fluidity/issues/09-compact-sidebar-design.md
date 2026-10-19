# 09: Compact sidebar rows and Active/All control

**Type:** design

**What to build:** Single-line rows at about 32 px on desktop (status glyph, title, unread dot; meta in a tooltip and on the selected row), hover actions menu, time-group headers, the Active/All segmented control, search at the top. Phone rows stay 44 px. Style cue: DeepSeek's chat list. Re-read `App.svelte` rail2, `OwnerGroups`, `ProjectList`, `SessionList`, `ui/row.svelte` and the design tokens.

**Blocked by:** 08-sidebar-active-and-time-groups

**Status:** ready-for-agent

- [ ] At least twice as many rows fit in the desktop sidebar as before
- [ ] Active/All control and time groups read clearly at 390 px and 1280 px
- [ ] Needs-you attention and unread still stand out in one line
- [ ] 44 px targets on phone; Back closes the drawer
- [ ] Console and a11y clean; `npm test`, `npm run build` and `.emberloop/check` pass

Spec: `../spec.md`.
