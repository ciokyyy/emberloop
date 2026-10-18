# 09: Shell, navigation and Dashboard

**Type:** design

**What to build:** Rail, sidebar, drawer, Project switcher, Session and Run lists, Dashboard. Dashboard leads with Needs you, then Running, with large numerals and status glyphs. Session and Project rows: status glyph, title, one line of meta, unread dot. Project switcher as a compact popover with search. Consistent page header pattern (title, scope chip, actions) for every area. Re-read `App.svelte`, `Rail`, `DrawerNav`, `ProjectSwitcher`, `SessionList`, `DashboardPage`, `RunsSidebar`, `QuotaSidebar`.

**Blocked by:** 01-design-foundation

**Status:** done

- [x] Dashboard answers "what needs me" in the first viewport on both sizes
- [x] Rows share one structure across Sessions, Runs and Projects
- [x] Every area uses the same page header pattern
- [x] No horizontal overflow; 44 px targets on phone; Back closes drawer
- [x] Console and a11y clean; `npm test`, `npm run build`, `.emberloop/check` pass
