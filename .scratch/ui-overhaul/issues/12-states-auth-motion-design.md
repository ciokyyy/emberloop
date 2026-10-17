# 12: States, auth and motion

**Type:** design

**What to build:** Replace every text-only `Loading…` with `Skeleton`, every bare empty and error text with `EmptyState` and `ErrorState` (icon, one line, one action). Polish AuthScreen and first-run. Apply the motion scale consistently (page enter, sheet, disclosure) and honour reduced-motion. Re-read all "Loading", "No ", "Could not" strings in `web/src`.

**Blocked by:** 07-chat-chrome-design, 09-shell-and-dashboard-design, 10-runs-and-ticket-design, 11-settings-memory-providers-design

**Status:** open

- [ ] No text-only loading, empty or error state remains in routed pages
- [ ] AuthScreen polished at both sizes
- [ ] Motion durations come only from the scale
- [ ] Console and a11y clean; `npm test`, `npm run build`, `.emberloop/check` pass
