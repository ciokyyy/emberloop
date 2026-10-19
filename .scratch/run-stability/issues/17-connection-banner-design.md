# 17: Global connection banner

**Type:** design

**What to build:** Build the approved mock in `.scratch/run-stability/mock/` (open `index.html`; reference screenshot `index.png`). One strip under the app header showing the connection state from 16: a small dot and short caption-size label (Reconnecting… or Daemon unreachable) and one small Refresh action, no box and no dismiss. Refresh asks the service worker to check for a new version and then reloads the page, so an installed PWA can recover. On recovery a green "Back online" shows for about 1.5 seconds and the strip slides away. The Root offline page uses the same copy and Refresh action, centred. Phone: the strip sits under the header, above the composer and clear of the keyboard through the visual viewport, and it does not move the transcript scroll position. Use the design tokens, one motion scale, and reduced motion without slide or pulse.

**Blocked by:** 16-connection-state-store

**Status:** ready-for-agent

- [ ] The strip matches the mock in reconnecting, unreachable, back-online and phone-with-keyboard states at 390 px and 1280 px
- [ ] Refresh checks for a new service worker version and reloads the page
- [ ] The strip hides by itself on recovery and never stays after the Daemon returns
- [ ] The strip takes its own height in the layout and never covers transcript text or the Reports tray at 390 px
- [ ] Transcript scroll position holds when the strip appears and hides on a phone
- [ ] The Root offline page shows the same copy and Refresh action
- [ ] No overflow at 390 px; console and a11y clean; screenshots at both sizes attached in the review
- [ ] `npm test`, `npm run build` pass; `.emberloop/check` passes
