# Run review: project-init (visual pass, prod Daemon on 7940)

Observed while Run `db193a3a…` was live: tickets 01 and 02 running (pig worker, `muse-spark`, effort max), 03 to 05 queued, no reviewers yet. Screenshots sit beside this file. Reviewer and merge states were not reached at the time of writing.

## Bugs

1. **Worker transcript cannot scroll (ticket page).** Reported by the user, confirmed in the DOM at 1920x1000. The transcript card (`div.overflow-hidden.rounded-2xl.border`) is 535px tall with a 6661px scrollHeight and no scrolling overflow, so older content is unreachable. Shot: `08-user-ticket-page-scroll-bug.png`, `09-ticket-02-1920.png`.
2. **Whole page scrolls instead of panes (ticket page).** The root is `h-dvh`, but the main column has `overflow: visible` and is 1145px tall in a 1000px viewport. The document scrolls, so the rail and second sidebar (both `h-dvh`) end partway down and the settings gear leaves its place. Shot: `08-user-ticket-page-scroll-bug.png`.
3. **Run page can load empty.** One fresh load of `/runs/<id>` showed "No Runs yet" in the sidebar and "No tickets yet" in the main area while the Run was running. Three reloads afterwards were fine. Cause not found: likely the Run list or selected Run races the first render. Shot: `06-run-mobile.png` (taken after a viewport resize, so it shows the desktop layout with the empty state).
4. **Ticket page loses sidebar state.** The "Repo directory" field is empty and the spec list is missing on ticket routes. Shot: `03-ticket-page-01.png`.
5. **Acceptance list shows raw backticks.** Items such as `` `ValidateSpec` returns … `` render the backticks literally. The Brief renders the same text as code. Shot: `03-ticket-page-01.png`, `08-user-ticket-page-scroll-bug.png`.
6. **Graph does not fit on load.** The rightmost node is clipped at the edge, the "2 running / 3 blocked" legend overlaps node 04, and the text of node 03 runs past its card. Shot: `01-run-overview.png`.
7. **Prompt bubble clips long paths on mobile (390px).** The worker prompt with long absolute paths runs past the right edge. Shot: `05-ticket-page-mobile.png`.
8. **Console issue.** Chrome reports "A form field element should have an id or name attribute" (2 fields).

9. **Graph canvas is narrow.** `RunsView.svelte` wraps the whole Runs page in `max-w-3xl` (line 247), so the graph never gets more than about 740px and clips its last column. The canvas should fill the available width and fit-to-view on load. Shot: `10-user-graph-clipped.png`.
10. **List view: Sessions are not clickable in practice.** Only the worker hash is a `link` button, and it looks like plain mono text. The Spec review and Standards review cells are text only (`verdict` plus `(stale)`), with no link to the Reviewer Sessions. Rows themselves do not open the ticket page. Make the whole row open the ticket, and make each agent cell a visible link or chip that opens its Session read-only. Shot: `02-list-view.png`.
11. **List view looks poor.** Eight cramped columns in 740px, wrapped titles, ids in mono at a different size than the rest, no state color or badge, no status dots. Use state badges, fixed column widths, a truncated id, and the full width.
12. **Chat sidebar shows a Subagent twice, some nested and some not.** `SessionList.svelte` `tierRoots` keeps a child as a root of its own tier whenever its parent sits in another tier, while the parent row also nests it (`tree.kids`). Example: Worker `e7793b84` is Recent (idle), its child `tests-reviewer` is working. The child shows under Working as a loose row and again under the parent as `2 sub`. The comment above `tierRoots` says children always render nested, so the filter should drop any child whose parent is in the list. Show the parent's tier by its busiest child (or the Run row, see Design recommendation) so a working child still pulls attention. Shot: `11-user-sidebar-duplicate-sub.png`.

## Polish

1. The Run is listed as `spec.md` in the sidebar. Use the spec title (`Project init and validate`).
2. The preview table and the Live Tickets table show the same five tickets on one page. Hide the preview table once a Run exists.
3. "Restart Run" sits in the header of a running Run next to Pause. Show it only for finished or failed Runs.
4. The Run header shows no branch, harness, model or elapsed time.
5. Workers show as a hash (`9df042f8`). Name them after the ticket (`Worker 01`).
6. The graph is tiny in the centered 740px column. Let it use the full width.
7. The List view's last column ("Refusal") sits at the right edge with no padding.

## Good

- State track (queued, running, in review, merging, merged) reads clearly.
- Agents, Acceptance, Gate and Follow-ups side panels are clear. Gate shows `pending@ <tip>` per reviewer.
- Steer, Retry, Park and Discard are one click away on every ticket page.
- Live transcript updates with tool strips and timings. Graph and List toggles work, and node colors match state.
- Sessions of the Run appear in Chat nested under the Run id.

## Design recommendation

Run Sessions (Workers and Reviewers, up to 3 per ticket) show as loose Sessions in the Chat lists. Hide them there and show one collapsed row per Run under its Project: `Project init and validate · 2 running · 3/5`. Click opens `/runs/<id>`. A chevron lists the Sessions, collapsed by default. A needs-you ticket turns the row amber. `lib/sidebar.js` holds the tier helpers, and the Run lookup comes from `/api/runs`, since a Run is not a Session.

## Run behaviour seen

- Ticket 01's worker went to needs-you with "no worker activity for 10 minutes". The Daemon had already sent it "Nudge: you have not submitted. Spawn your reviewers and submit." The worker is still `working`, so this is a real stall, not a UI error.
- Workers spawn their own scouts (`tests-reviewer`, `spec-reviewer`) before submit. These are not the Run's Reviewers: `reviewerSessions` is still empty for every ticket. The two kinds look alike in the sidebar, so mark Run Reviewers differently from worker-spawned scouts.
- The reviewer prompt carries no skill text (see the reply on injected skills).

## Not yet seen

Reviewer cards, pass and fail verdicts, "pass held", rework rounds, needs-you banner, merge and merged state, paused Run.

## Fixed after this review (verified on a dev Daemon, screenshots `after-*.png`)

- Bug 1, 2: transcript scrolls inside its card; Runs pages scroll in their own container, so the rails keep full height.
- Bug 3: the Runs sidebar says "Loading Runs…" until the first list answers, and a ticket route seeds the repo and spec list from its Run.
- Bug 4, 5, 7, 8: sidebar state on ticket routes, inline code in acceptance items, long paths wrap in prompt bubbles, form fields carry names.
- Bug 6, 9: graph fits and centers on load, fills the page width (`max-w-[1600px]`), legend moved top-left, blocker labels shortened to ticket numbers.
- Bug 10, 11: List view rewritten (state badges, agent chips, verdict badges, box counts, whole-row click).
- Bug 12: `rootTiers` in `sidebar.js`; a Subagent shows only nested under its parent, and the parent takes the busiest tier.
- Polish 1 to 5: Run title in sidebar and header, preview table only before a Run exists, Restart Run only for finished Runs, header shows harness, model and effort, discarded and parked tickets no longer raise needs-you.
- Workers: the start prompt and the stall nudge no longer tell a Run Worker to spawn reviewers. Reviewer prompts carry the code-review skill axis briefs.
