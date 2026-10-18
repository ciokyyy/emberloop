# 01: Design foundation

**Type:** design

**What to build:** One documented design system in `emberloop-web/web`. Semantic tokens (surface 0-3, text levels, status colours for working, needs-you, done, failed, paused, owner palette), type scale with tabular numerals, 4 px spacing scale, three elevation levels, motion scale (100/180/320 ms, one easing), and a shared icon map that names each domain concept once. New shared primitives built on `lib/ui`: `Section`, `Row`, `StatCard`, `Chip`, `IconButton` (tooltip plus accessible name), `EmptyState`, `Skeleton`, `ErrorState`, `InfoHint` (tap-to-reveal help). Apply them to one reference page (Quota) at 390 px and 1280 px. Write `web/DESIGN.md` listing tokens, primitives and the rules in the spec (icon, disclosure, responsive). Re-read `app.css`, `lib/ui/*`, `settingsIcons.js`, `areas.js`.

**Blocked by:** none

**Status:** done

- [x] Tokens, scales and the icon map exist and are used by the new primitives
- [x] All primitives render at 390 px and 1280 px with no overflow; icon-only buttons have tooltip and accessible name
- [x] Quota page rebuilt on the primitives, percent first, no horizontal scroll
- [x] `web/DESIGN.md` documents tokens, primitives, icon map and the three rules
- [x] Reduced-motion disables non-essential motion
- [x] `npm test` and `npm run build` pass in `emberloop-web/web/`; `web/README.md` maps new tests; `.emberloop/check` passes
