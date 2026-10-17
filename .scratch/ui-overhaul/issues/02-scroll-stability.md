# 02: Scroll stability

**Type:** code

**What to build:** Scrolling up from the bottom of a Session moves only as far as the reader moved. Remove global `scroll-behavior: smooth`. Rely on browser scroll anchoring for height changes; keep manual correction only for prepended history. Stop the follow loop fighting the reader: release the pin on any upward wheel, touch or key input before the next frame, and do not re-engage until the reader is within the threshold again. Disclosure (`details`) height animation and tool-strip growth must not shift content above the viewport. Add a "Latest" button when unpinned. Re-read `lib/Transcript.svelte` (glide, pin, ResizeObserver, measure), `lib/transcript.js`, `app.css` (smooth scroll, details animation).

**Blocked by:** none

**Status:** open

- [ ] A test streams growth into the last block while the reader scrolls up; scrollTop never moves down or jumps beyond the user's delta
- [ ] Opening and closing a disclosure above the viewport does not move visible content
- [ ] Prepending earlier history keeps the visible block in place
- [ ] "Latest" button appears when unpinned and returns to bottom with one tap or click
- [ ] Wheel, touch and keyboard unpin verified in vitest; phone-size e2e check in `web/e2e`
- [ ] `npm test`, `npm run build`, `npm run test:e2e` pass in `emberloop-web/web/`
