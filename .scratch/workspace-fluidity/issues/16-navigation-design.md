# 16: Quick switcher, scope chip and Project pages

**Type:** design

**What to build:** The quick switcher (desktop modal, phone sheet with a visible button), a scope chip or breadcrumb in every page header, and the Project-scoped Settings and Memory pages. Guidance cues for a fluid flow: clear current location, one-tap jump back, no dead ends. Re-read the page header pattern, `ProjectSwitcher`, `DrawerNav`, `Rail`, Settings and Memory pages.

**Blocked by:** 13-nav-memory-and-prefetch, 14-quick-switcher-logic, 15-project-scoped-settings-and-memory

**Status:** ready-for-agent

- [ ] The scope chip names the Project and area on every page at 390 px and 1280 px
- [ ] The switcher reaches any Session, Run, Project or page in three taps or fewer on phone
- [ ] Project-scoped Settings and Memory show clearly what is overridden or inherited
- [ ] 44 px targets on phone; Back closes the switcher
- [ ] Console and a11y clean; `npm test`, `npm run build` and `.emberloop/check` pass

Spec: `../spec.md`.
