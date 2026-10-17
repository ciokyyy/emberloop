# UI overhaul

One spec, from the 2026-10-17 conversation with the User. It redesigns the Emberloop browser UI as one coherent product on desktop and phone, fixes the scroll defect, adds message actions, and turns Subagent reports into dismissible notices. Work splits into `code` Tickets (behaviour, with plain semantic UI) and `design` Tickets (layout, styling, icons, copy, states), per the Designer agents spec. Design Tickets run on the Designer Chain and are reviewed by the design Reviewer, who inspects the rendered page through the `agent-browser` CLI at 390 px and 1280 px.

## Problem Statement

1. The UI looks like a good developer tool, not a product. It has no deliberate visual system: tokens exist, but spacing, type scale, elevation and icon use vary page to page.
2. Many controls are text where an icon would read faster (`Working…`, `Compress`, `Retry`, `Change compress model`, `Dismiss`, `Send`, `Git`, `% ctx`). Other screens carry too much prose at once (Memory, Settings, Providers, Runs preview, Ticket page), which overloads the reader.
3. Scrolling up from the bottom of a long Session sometimes flings the reader away. Several mechanisms compete for the scroll position: a glide loop that follows streaming growth, global smooth scrolling, animated `details` height, virtualised block measuring and a ResizeObserver that re-glides.
4. A sent message offers no actions. The User cannot copy it, share it, edit it and resend it, or edit or cancel a queued Steer. Only the whole queue can be cleared.
5. When a Subagent group finishes, the Daemon delivers the result to the parent as a Steer. The transcript shows it as a card that stays in the flow, with no way to clear it. The User wants a quiet notification, not a message that looks like their own.
6. The phone experience is a squeezed desktop. Several screens have no breakpoint at all (Profile, Notifications, GitPanel, PromotionQueue, the auth screen), the Runs graph traps touch scroll, and the chat header shows two lines of text on a small screen.
7. Pages do not share a layout grammar, so each new page re-decides headers, sections, empty states and loading states.

## Solution

A single design language, "calm density": Apple's restraint and spacing, Revolut's confident numbers, status colour and card clarity, ChatGPT's conversation-first layout, and Matt Pocock's skills-and-tickets vocabulary kept as plain, terse engineering language rather than decoration. The product is dark by default and keeps the current dark palette as its base; the Designer refines it, it does not replace it.

- **Every screen answers one question first.** A page shows its headline state (what is happening, what needs the User) at the top and moves detail behind progressive disclosure: a row opens a sheet or an expandable section, not a wall of text.
- **Icons carry actions, words carry meaning.** Actions that are common and well known are icon buttons with a tooltip and an accessible name. Status is a dot or glyph plus one short word. Long help text moves to a tap-to-reveal hint.
- **Phone is a first-class layout, not a smaller desktop.** It has a bottom action zone, sheets instead of side panels, 44 px touch targets, safe-area insets and no hover-only affordances. It does not need every feature: dense tools (graph pan, diff, chain editing) may be view-only or deferred on phone, and each Ticket says which.
- **The transcript is calm.** Scrolling is predictable. Messages have actions on long-press (phone) or hover (desktop). Subagent reports arrive as notices, not as the User's own Steer.

## User Stories

1. As a User reading a long Session, I want scrolling up from the bottom to move only as far as my finger or wheel moved, so that I never lose my place.
2. As a User, I want the view to follow new output only while I am at the bottom, and a clear "Latest" button when I am not, so that I control the position.
3. As a User, I want to copy any message (mine or the agent's) with one tap, so that I can reuse it.
4. As a User, I want to copy a message as Markdown or as plain text, so that pasting elsewhere keeps or drops formatting as I choose.
5. As a User, I want to share a message or a whole turn through the OS share sheet on phone, and as a copied link-free text block on desktop, so that I can send it to someone.
6. As a User, I want to edit a message I sent and resend the edited text as a new prompt, so that I can fix a typo or sharpen a request without retyping.
7. As a User, I want to edit a Steer or Follow-up that is still queued, so that I can change it before the agent sees it.
8. As a User, I want to cancel a single queued Steer without clearing the others, so that one mistake costs one message.
9. As a User, I want a queued message to look different from a delivered one, so that I know which I can still change.
10. As a User, I want the Subagent finish report to appear as a quiet notice that I can open, collapse or dismiss, so that it never reads like something I said.
11. As a User, I want dismissed reports to stay dismissed after reload, so that the transcript stays clean.
12. As a User, I want all reports for a Session in one place I can reopen, so that dismissing is never destructive.
13. As a User, I want a gentle indicator and, when I am away, a push notification when a Subagent group finishes, so that I know without watching.
14. As a User on a phone, I want the chat header to show the Session title, one status glyph and a menu, so that the screen is not full of text.
15. As a User, I want the context-window meter to be a small ring with a tap-to-open detail sheet, so that the header stays light.
16. As a User, I want Compress, Retry and Dismiss to be icon actions with tooltips, so that the banner takes one line.
17. As a User, I want a Working indicator that shows activity and the current tool in a few characters, so that I can tell a stalled agent from a busy one.
18. As a User, I want the composer to have an attach button, a mode switch (Send, Steer, Follow-up) and a primary send button as icons, so that its state is clear without reading.
19. As a User on a phone, I want the composer pinned above the keyboard and the safe area, so that I can type without layout jumps.
20. As a User, I want tool calls grouped and collapsed into a short strip with an icon per tool kind, so that long runs do not bury the answer.
21. As a User, I want the Dashboard to open on what needs me (Needs you first), then what is running, with big status numbers, so that I can act in seconds.
22. As a User, I want Project and Session lists to show a status glyph, a title and one line of meta, so that scanning is fast.
23. As a User, I want the Runs preview to show Ticket type, blockers and warnings as chips, not sentences, so that a Run is readable at a glance.
24. As a User, I want the Ticket page to lead with status and the next action, and put the transcript and facts behind tabs on phone, so that I am not shown everything at once.
25. As a User on a phone, I want the Runs graph to be a list by default with a full-screen graph view I opt into, so that scrolling is never trapped.
26. As a User, I want Settings pages to group controls into short sections with one-line descriptions and a tap-to-reveal "why", so that I can configure without reading paragraphs.
27. As a User, I want Memory to open on a summary (counts, pending approvals) and put Rules, Profile and ledgers behind sections, so that the page is not a wall of text.
28. As a User, I want Providers and Quota to share one card style with percent first, so that I can compare providers quickly.
29. As a User, I want empty, loading and error states to share one pattern with an icon, one line and one action, so that no screen is ever blank or cryptic.
30. As a User, I want animations to be short and to respect reduced-motion, so that the app feels alive without being slow.
31. As a User, I want sign-in and first-run to feel as polished as the rest, so that the first impression matches.
32. As a User with a keyboard, I want focus rings, shortcuts and tooltips that match the icon actions, so that icon-only buttons stay discoverable.
33. As a User, I want the app to look the same in the PWA on phone, desktop Chrome and Safari, so that I can trust it everywhere.
34. As a future Designer agent, I want a documented token set and primitive list, so that I extend the system instead of inventing styles.

## Implementation Decisions

- **Design language.** Base palette stays the existing dark set. The foundation Ticket defines semantic tokens (surface levels 0 to 3, text levels, status colours for working, needs-you, done, failed, paused, owner palette), a type scale (a headline, title, body, caption and numeric style with tabular figures for quota, cost and counts), a spacing scale on a 4 px base, three elevation levels, radii kept as they are, and a motion scale (instant 100 ms, standard 180 ms, emphasis 320 ms; one easing curve). A light theme is not part of this spec (see Further Notes).
- **Icon rule.** The icon library stays as it is. A shared icon map names each domain concept once (Session, Run, Ticket, Subagent, Steer, Follow-up, Report, Memory, Quota, Provider, Project, Git, Compress, Copy, Share, Edit, Retry, Dismiss) so the same concept never has two icons. Icon-only buttons always carry an accessible name and a tooltip.
- **Disclosure rule.** At most one paragraph of help text is visible by default on any page; the rest sits behind an info control or a collapsed section.
- **Responsive rule.** Every design Ticket specifies both layouts. Phone: single column, bottom-anchored primary action, sheets for secondary panels, 44 px touch targets, safe-area insets, no hover-only controls. Desktop: persistent navigation, side panels, hover affordances with keyboard equivalents. A Ticket states what phone omits.
- **Scroll model.** Stick-to-bottom is the only autoscroll; it engages when the reader is within a small threshold of the bottom and releases on any upward input before the next frame. Browser scroll anchoring is the default for height changes; manual position correction remains only for prepending earlier history. Global smooth scrolling is removed so programmatic and user scrolls do not blend. Height animation of disclosure content must not change the scroll position of content above the viewport. Streaming growth never moves the reader unless they are pinned. A regression test fixture reproduces scrolling up while the last block grows.
- **Message actions.** A shared per-message action set: Copy (text or Markdown), Share, and for the User's own messages Edit and resend. The action surface is a hover toolbar on desktop and a long-press sheet on phone, both reachable by keyboard. Copy and Share are client-side (clipboard and the Web Share API with a copy fallback). "Edit and resend" loads the original text and attachments into the composer as a new prompt; it never rewrites history, because the Event log is append-only and a delivered message has already been seen by the agent. A confirmation line says so once. Rewinding or forking a Session at a message is out of scope.
- **Queued Steers.** The Daemon gains per-message queue control: list items with stable ids, replace a queued item's text, and remove one item. Today only the whole queue can be cleared. Pig's native queue and the Daemon's pending queue must both honour it. A claude Steer degrades to a Follow-up and is stored as such; the queue UI shows the mode actually used. An item already delivered returns a conflict. The transcript marks queued items distinctly until delivery.
- **Subagent reports.** The Daemon keeps delivering the result to the parent agent as a Steer, because the parent needs the text. The change is entirely in how the UI treats it. A report Event renders as a notice, not a User message: icon, group name, counts, a one-line summary, and an expand control. It can be dismissed. Dismissal is a per-User, per-Session state stored in the browser and keyed by Event sequence, so it survives reload on that device. A Session-level Reports tray lists all reports, dismissed ones included, and can restore any. A new report shows a brief unread dot on the Session in lists and, when the User is away, the existing push path with a "Subagents finished" body. A report never appears with the Steer title or the sender badge.
- **Chat chrome.** Header, context meter, Working indicator, Compress banner, composer controls and tool strip become compact icon-led components with tooltips, per the icon and disclosure rules.
- **Pages.** Dashboard leads with Needs you, then Running, with large numerals. Runs preview and Ticket page lead with status and next action; type, blockers and warnings become chips; transcript and facts sit behind tabs on phone. Settings, Memory, Providers and Quota use shared section, row and card components. Providers and Quota share one card.
- **States.** One empty, one loading (skeleton, not text) and one error pattern, built once and used by every page.
- **Motion.** Short, purposeful, reduced-motion safe. No decorative loops other than the Working indicator.
- **Seams.** The Daemon queue control is one new Daemon seam. Everything else is browser UI.

## Testing Decisions

- A good test drives external behaviour: what the User sees and can do, not component internals.
- **Daemon seam** (`api_test.go`, `steer_test.go`, fake Backend): per-message queue list, replace, remove, delivered-conflict, pig native queue and claude degrade.
- **Browser unit seam** (vitest, `Transcript`, `Card`, `Composer`, `events`): scroll pin and release under streaming growth, message actions, Edit and resend filling the composer, queued item edit and cancel, report notice rendering, dismiss, restore, reload persistence, unread dot.
- **Layout seam** (`agent-browser`, Chromium at 390 px and 1280 px, in the e2e suite): no horizontal overflow on any route, 44 px touch targets for primary controls on phone, no hover-only control, the Runs graph does not trap touch scroll, composer stays above the keyboard inset.
- **Design review.** Each design Ticket is judged by the design Reviewer from snapshots and capped JPEG screenshots at both sizes; its checklist is in each Ticket.
- **Prior art:** `Transcript.test.js`, `Composer.test.js`, `Card.test.js`, `GroupCard.test.js`, `RunsView.test.js`, `SettingsHub.test.js`, `steer_test.go`, the e2e specs under `web/e2e`.

## Out of Scope

- A light theme, user-selectable accent colours and theming settings.
- Rewinding or forking a Session at a message; editing delivered history.
- Exporting a whole Session to a file or a public share link.
- New product features beyond those named (no new pages, no new agent capabilities).
- Native mobile apps; the PWA stays the phone product.
- Backend or Run driver behaviour, except the queue control seam.
- Replacing the icon library or the component library.

## Further Notes

- Decisions made here without the User and open to change before Tickets start: report dismissal lives in the browser (not the Daemon); "Edit and resend" is a new prompt, not a rewrite; no light theme.
- The Designer and design Reviewer need a vision-capable model. Design Tickets are judged mostly by the Reviewer's screenshots, so each Ticket lists concrete, checkable outcomes rather than taste words.
- Ticket order follows the `Blocked by` lines. Code Tickets (02 to 05) land first so design Tickets style real behaviour; the foundation Ticket (01) lands before every other design Ticket so they share one system.
- The two Matt Pocock influences are tone, not look: terse labels, domain words (Session, Run, Ticket, Steer, Subagent), and no marketing copy.
