# 08: Mobile chat

**Type:** design

**What to build:** The chat on a phone as its own layout. Composer pinned above the keyboard and safe-area; bottom-sheet menus for Session actions, model and Project; swipe-back or Back closes sheets; drawer holds Project switcher, Session list and the five areas; scroll-to-latest button clears the composer; no hover-only affordance; 44 px primary targets. Review the `visualViewport` behaviour for keyboard open. Desktop layout must not regress.

**Blocked by:** 06-message-and-report-design, 07-chat-chrome-design

**Status:** done

- [x] At 390 px: no horizontal overflow, composer never covered by keyboard (emulated inset), all primary targets at least 44 px
- [x] Sheets close with Back and with an explicit close icon
- [x] Scroll-to-latest and composer do not overlap
- [x] Desktop layout screenshots unchanged within the Ticket's intent
- [x] Console and a11y clean; `npm test`, `npm run build`, `.emberloop/check` pass
