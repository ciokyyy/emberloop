# 04: Message actions (behaviour)

**Type:** code

**What to build:** Per-message actions with plain semantic UI. Copy (text and Markdown) and Share on every message (Web Share API on phone, clipboard fallback elsewhere). Edit and resend on the User's own delivered messages: loads text and attachments into the composer as a new prompt and shows a one-line note that history is not rewritten. Queued Steers and Follow-ups get Edit (in place) and Cancel (single item), and look different from delivered ones. Actions open from a hover toolbar (desktop) and a long-press menu (phone), both keyboard reachable. Re-read `lib/Card.svelte`, `lib/Transcript.svelte`, `lib/Composer.svelte`, `lib/composer.js`, `lib/events.js`, `lib/api.js`.

**Blocked by:** 03-queue-item-control

**Status:** done

- [x] Copy and Share work for User, assistant and report messages; failures show a message
- [x] Edit and resend fills the composer without sending; attachments carried over
- [x] Queued item Edit and Cancel call the new API and update the row live
- [x] Queued and delivered messages are visually distinct (markup and state, styling comes in 06)
- [x] Long-press and keyboard open the action menu; no hover-only control
- [x] `Card.test.js`, `Composer.test.js`, `Transcript.test.js` cover each action; `npm test` and `npm run build` pass; `web/README.md` maps tests
