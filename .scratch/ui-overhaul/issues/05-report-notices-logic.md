# 05: Subagent reports as notices (behaviour)

**Type:** code

**What to build:** The Daemon keeps steering the parent with the result text. The UI renders a report Event as a notice (never as a Steer or User message, no sender badge). Notices can be expanded, collapsed and dismissed. Dismissal is per User and Session, stored in the browser and keyed by Event seq, and survives reload. A Reports tray per Session lists all reports, dismissed ones included, and restores any. A new report sets an unread marker on the Session in lists; the existing push path sends "Subagents finished" when the User is away. Re-read `lib/events.js` (`parseReport`, kinds), `lib/Card.svelte` (report), `lib/GroupCard.svelte`, `lib/subagents.js`, `lib/SessionList.svelte`, `push.go`, `push.js`.

**Blocked by:** none

**Status:** open

- [ ] Report Event renders as a notice kind with group name, counts and a one-line preview
- [ ] Dismiss hides it; reload keeps it hidden; tray restores it
- [ ] Unread marker shows on the Session row until the Session is opened
- [ ] Push body "Subagents finished" is sent for group completion when no client is watching, and not for each-mode children
- [ ] No Steer title or sender badge ever appears on a report
- [ ] `events.test.js`, `Card.test.js`, `GroupCard.test.js`, `SessionList.test.js` and push trigger Go test cover this; `npm test`, `npm run build` and `.emberloop/check` pass
