# 13: Closeout and layout probe

**Type:** code

**What to build:** An e2e layout check in `web/e2e` that visits every routed page at 390 px and 1280 px and asserts no horizontal overflow, 44 px primary targets on phone and no hover-only control. Update `web/README.md`, `web/DESIGN.md` and `emberloop-web/AGENTS.md` for the final component map. Add `.scratch/ui-overhaul/AGENTS.md` index entry check. Mark the spec's out-of-scope list unchanged.

**Blocked by:** 06-message-and-report-design, 08-mobile-chat-design, 12-states-auth-motion-design

**Status:** done

- [x] E2E layout check covers every route in `router.js` at both sizes
- [x] Docs list the new components and tests
- [x] `npm test`, `npm run build`, `npm run test:e2e` and `.emberloop/check` pass
