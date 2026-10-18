# Design system

"Calm density": dark only, quiet surfaces, confident numbers, status colour with a word. Tokens live in `src/app.css`; primitives live in `src/lib/ui/`; the icon map is `src/lib/icons.js`. Extend these. Never hardcode a colour, size, shadow or duration, and never pick an icon for a domain concept by hand.

The reference page is Quota (`src/lib/QuotaPage.svelte`): read it for every primitive in use at 390 px and 1280 px.
Quota provider cards sit in a container-query grid (single column below `@2xl`) and flow up to three usage windows across (`sm:grid-cols-3`) so row-mate cards stay even. `e2e/tests/quota-layout.e2e.ts` probes 390, 800 and 1280 px; 800 px is extra coverage for the rail plus sidebar layout.
The final page and chat component map is summarized below; `e2e/tests/layout-sweep.e2e.ts` visits every route pattern at 390 px and 1280 px, checking overflow, one page header and h1, 44 px primary controls, and no hover-only control on phone. `e2e/tests/quota-layout.e2e.ts` adds the 800 px rail-plus-sidebar check.

## Tokens

### Surfaces

| Token | Utility | Use |
| --- | --- | --- |
| `--surface-0` | `bg-surface-0` (`bg-background`) | App canvas |
| `--surface-1` | `bg-surface-1` (`bg-card`) | Cards, rows grouped on the canvas |
| `--surface-2` | `bg-surface-2` (`bg-popover`) | Popovers, tooltips, sheets |
| `--surface-3` | `bg-surface-3` (`bg-accent`) | Hover and selected rows, skeleton blocks |

`--sidebar` (#171717) is the one sunken surface for the rail and drawer. `--muted` fills chips, icon tiles and bar tracks. `--border` draws every edge.

### Text

| Token | Utility | Use |
| --- | --- | --- |
| `--fg-1` | `text-fg-1` | Primary text, numbers |
| `--fg-2` | `text-fg-2` | Secondary text, labels |
| `--fg-3` | `text-fg-3` (`text-muted-foreground`) | Captions, meta, help |
| `--fg-4` | `text-fg-4` | Disabled and decoration only; fails text contrast |

### Status

`--status-working`, `--status-needs-you`, `--status-done`, `--status-failed`, `--status-paused`. Utilities `text-status-*`, `bg-status-*`, `border-status-*`; use `/10` to `/12` opacity for tinted fills. A status is always a dot or glyph plus one word, never colour alone. Quota bar tones map calm to done, warm to needs-you, hot to failed.

### Owner palette

`--owner-1` to `--owner-6`: one slot per User in account order (`owners.js` `ownerTone`). Only User identity uses it.

### Type scale

One utility per style. Size, line height, weight and tracking come with the utility.

| Utility | Size / line | Use |
| --- | --- | --- |
| `text-headline` | 28 / 34, 600 | Page title (one per page) |
| `text-title` | 17 / 23, 600 | Section, card and row-group titles, empty and error lines |
| `text-body` | 15 / 22 | Row titles, values, body copy |
| `text-caption` | 12 / 16 | Meta, help, chips, tooltips |
| `text-numeric` | 28 / 28, 600 | Quota, cost and counts; figures are always tabular (set by the utility) |

Phones shrink only `StatCard` values (`max-sm:text-[1.5rem]`). Fonts: system sans, mono for code.

### Spacing

4 px base: Tailwind's `--spacing` is `0.25rem`, so every `p-`, `m-`, `gap-` and `size-` step is n x 4 px. Use steps 1, 2, 3, 4, 6, 8, 12, 16. Page gutters are `px-4` on phone and `md:px-6` on desktop; sections gap `gap-6`; card padding `p-4`; related controls `gap-1.5` to `gap-3`. Avoid arbitrary pixel values (`p-[13px]`).

### Radii

Unchanged: `rounded-lg` (controls, tiles), `rounded-xl` (callouts, popovers), `rounded-2xl` (cards), `rounded-full` (buttons, chips).

### Elevation

| Utility | Use |
| --- | --- |
| `shadow-elev-1` | Resting card |
| `shadow-elev-2` | Popover, tooltip |
| `shadow-elev-3` | Sheet, dialog |

### Motion

One easing, `--ease-smooth`; three durations. Utilities `motion-instant`, `motion-standard` and `motion-emphasis` set duration, easing and the `--tw-duration`/`--tw-ease` pair that `animate-in`/`animate-out` read, so transitions and enter/exit animations share one scale. Every unqualified `transition-*` utility (hover, focus, colour) defaults to `--dur-standard` and `--ease-smooth` through `--default-transition-duration`/`--default-transition-timing-function` in `app.css`; only the two other steps need a `motion-*` class. `animate-rise` (emphasis) is the mount entrance. No component carries its own `duration-*`, `ease-*` or hand-written milliseconds.

| Token | Value | Use |
| --- | --- | --- |
| `--dur-instant` | 100 ms | Press, tooltip |
| `--dur-standard` | 180 ms | Hover, colour, disclosure chevrons |
| `--dur-emphasis` | 320 ms | Entrances, bar growth, details height |

Reduced motion (`prefers-reduced-motion: reduce`): transitions and one-shot animations end at once and smooth scroll is off. Only spinners and `data-motion="essential"` elements keep moving, slowly. No decorative loops other than the Working indicator; Skeleton pulses with `motion-safe:` only. Streamed text keeps its own `stream-in` fade at the emphasis duration (`streamFade.js` `FADE_MS`, which must match the token).

### Merging classes

`cn()` in `ui/cn.js` knows the type and elevation utilities, so `cn('text-caption', 'text-fg-3')` keeps both. A `tv()` call that mixes them must pass `{ twMergeConfig }` from the same file (see `ui/chip.svelte`).

## Icon map

`src/lib/icons.js` names each domain concept once. Import `icons`; the same concept never has a second icon.

| Concept | `icons.` | lucide |
| --- | --- | --- |
| Session | `session` | `MessageSquare` |
| Run | `run` | `Layers` |
| Ticket | `ticket` | `Ticket` |
| Subagent | `subagent` | `Bot` |
| Steer | `steer` | `Signpost` |
| Follow-up | `followUp` | `ListEnd` |
| Report | `report` | `FileText` |
| Memory | `memory` | `Brain` |
| Quota | `quota` | `Gauge` |
| Provider | `provider` | `Plug` |
| Project | `project` | `FolderGit2` |
| Git | `git` | `GitBranch` |
| Compress | `compress` | `Shrink` |
| Copy | `copy` | `Copy` |
| Share | `share` | `Share2` |
| Edit | `edit` | `Pencil` |
| Retry (run a failed action again) | `retry` | `RotateCcw` |
| Dismiss | `dismiss` | `X` |
| Cancel (a queued message) | `cancel` | `Ban` |
| Refresh (re-read live data) | `refresh` | `RefreshCw` |
| Model | `model` | `Cpu` |

Status glyphs: `statusIcons` and `statusLabels` (working `Loader`, needs-you `CircleAlert`, done `CircleCheck`, failed `CircleX`, paused `CirclePause`). `providerGlyph(key)` gives one brand glyph per Provider inside lists; the Provider concept itself stays `icons.provider`. `areas.js` and `settingsIcons.js` read the map (ticket 01 rewired both). Everything else (Settings, Bell, User) is a one-off lucide import. `ui/primitives.test.js` guards uniqueness.

## Primitives

All in `src/lib/ui/`, built on `button`, `popover` and bits-ui, using only the tokens above.

| Primitive | File | Props and behaviour |
| --- | --- | --- |
| `Section` | `section.svelte` | `title`, `description` (one line), `icon`, `hint` (text or snippet, opens in an `InfoHint`), `actions` snippet, `collapsible` + `open` (native `<details>`; the hint button does not toggle it), `headingLevel` (cards inside a Section use one level lower). Heading row is 44 px under md. |
| `Row` | `row.svelte` | One structure for Session, Run and Project rows: `title`, `badges` snippet (chips beside the title), `meta` (one line), `icon`, `status` (status glyph from `statusIcons`, or `idle` hollow circle; an sr-only word follows unless `statusWord` is false), `unread` (dot), `attention` (needs-you tint), `current` (`aria-current`), `trailing` snippet, `selected`. A link with `href`, a button with `onclick`, else static. Truncates; 56 px under md. |
| `PageHeader` | `page-header.svelte` | The header of every area: optional phone menu button (from the shell context set by `App.svelte`, or `onMenu`), optional Back (`onBack`, `backLabel`, `backOn` = `always`, `phone` or `desktop`; a Back that shows on phone replaces the menu there), icon tile (desktop), the one `h1` (`text-headline` on desktop, `text-title` on phone), `subtitle`, scope chip (`scope`, `scopeIcon`: Global, a Project name, All projects), `meta` snippet, `hint` (`InfoHint`) and `actions` snippet pushed right. `size="compact"` is the chat header. |
| `StatCard` | `stat-card.svelte` | `label`, `value`, `unit`, `caption`, `tone` (status), `icon`. Tabular headline value. Three fit in a row at 390 px. |
| `Chip` | `chip.svelte` | `tone` (`neutral`, `outline`, or a status), `dot`, `icon`. One short word; truncates, never wraps. |
| `IconButton` | `icon-button.svelte` | `icon`, `label` (required: accessible name and tooltip), `variant`, `size`, `spin`. 44 px under md. |
| `Tooltip` | `tooltip.svelte` | `text`, `side`, `trigger` snippet. Used by `IconButton`; never the only name. |
| `EmptyState` | `empty-state.svelte` | `icon`, `title` (one line), `description`, `action` snippet, `compact`. Compact renders the same pattern as one row for a small region inside a page. |
| `ErrorState` | `error-state.svelte` | `title`, `detail`, `onRetry`, `busy`, `compact`. `role="alert"`, one Retry action; compact is the one-row form beside content that is still on screen. |
| `Skeleton` | `skeleton.svelte` | Decorative block (`aria-hidden`); wrap a group in `role="status"` with an `aria-label`. Loading is skeleton, not text. |
| `InfoHint` | `info-hint.svelte` | `label` (accessible name), children. Tap-to-reveal popover for help text. |
| `Panel` | `panel.svelte` | The grouped card: `rounded-2xl border bg-card shadow-elev-1`. `divided` (hairlines between children, default), `padded` (free content with `p-4 gap-3`), `tag` (`div` or `ul`). Settings, Memory lists and Profile put their rows in one Panel. |
| `SettingRow` | `setting-row.svelte` | One setting: `label`, `hint` (text or snippet, opens in an `InfoHint` named `About <label>`), `note` (a visible state line such as an environment lock or a warning; `noteTone` `default`, `warn` or `error`), `inline` (control stays beside the label on phone: switches, small buttons), `stack` (control under the label on desktop too: textareas), `width` (`narrow`, `field`, `wide`, `full` on desktop). Controls fill the row on phone. |
| `Segmented` | `segmented.svelte` | `items` (`{ value, label, icon?, count? }`), `value`, `label`, `mode`: `toggle` is a pressed-button group (`role="group"`, `aria-pressed`) for a view switch, `tabs` is a `tablist` (arrow, Home and End keys) that names its `panel` id with `aria-controls`. Segments are 44 px under md. |
| `BottomSheet` | `bottom-sheet.svelte` | A secondary editor or detail: bottom sheet under md, centred dialog above. `open`, `title`, `description` (sr-only, defaults to the title), `children`, `footer` snippet. The Runs and Ticket chain sheets, the Chain editor and the context meter open one on phone. |

Existing shadcn primitives (`Button`, `Badge`, `Dialog`, `Sheet`, `Select`, `Combobox`, ...) stay; `Chip` is for status and facts, `Badge` for legacy uses.

## Component map

- **Shell and pages:** `App.svelte` routes the shared `PageHeader` and `Rail`/`DrawerNav` through `DashboardPage`, `ProjectSettingsPage`, `RunsView`/`TicketPage`, `SettingsHub`/`SettingsPage`, `QuotaPage`, `ProvidersPage` and `MemoryPage`. `Root.svelte` owns the `AuthScreen` gate.
- **Chat:** `ChatMenu`, `ChatStatus`, `ContextMeter` and `WorkingIndicator` keep the header and composer compact; `Transcript`, `Card`, `MessageActions`, `ReportsTray`, `GroupCard` and `ToolStrip` own conversation content and reports.
- **Shared layout and states:** `PageHeader`, `Section`, `Row`, `StatCard`, `Chip`, `IconButton`, `Tooltip`, `EmptyState`, `ErrorState`, `Skeleton`, `InfoHint`, `Panel`, `SettingRow`, `Segmented` and `BottomSheet` are the reusable primitives above.
- `web/README.md` is the detailed source map and test inventory; the route layout sweep, Quota layout probe, shell layout and phone chat probes live in `e2e/tests/`.

## Rules

### Icon rule

Icons carry actions; words carry meaning. A common, well-known action (copy, share, edit, retry, dismiss, refresh, compress) is an `IconButton` with a tooltip and an accessible name. Status is a dot or glyph plus one short word. Concepts use the icon map. Never an icon-only button without `label`.

### Disclosure rule

At most one paragraph of help text is visible by default on any page. The rest sits behind an `InfoHint` or a collapsed `Section`. Data lines (updated 8s ago, resets in 1h) are not help text. Detail opens as a sheet or expandable section, not a wall of text.

### Responsive rule

Every design change specifies both layouts and says what phone omits. Phone (`<md`): single column, bottom-anchored primary action, sheets for secondary panels, 44 px touch targets (`max-md:` sizes), safe-area insets (`env(safe-area-inset-*)`), no hover-only controls. Desktop: persistent navigation, side panels, hover affordances with keyboard equivalents. Grids that sit beside the rail and second sidebar switch columns on the content column's width (container queries, `@container` + `@2xl:grid-cols-2`), not the viewport (`md:`), so 768 to 990 px viewports stay single column. `e2e/tests/layout-sweep.e2e.ts` verifies every routed page at 390 px and 1280 px; Quota also gets 800 px coverage. Every control remains keyboard reachable with a visible focus ring.

### State rule

Three patterns cover every screen, and no page renders a bare sentence for one of them. Loading is `Skeleton` inside `role="status"` with an `aria-label`; text like `Loading…` never stands in for content. Emptiness is `EmptyState` and failure is `ErrorState`, each with an icon, one line and one action. A state that fills the page (nothing loaded yet) is the full pattern; a state beside content that is still on screen (a failed refresh, a small region inside a card, a form's own error) is the `compact` row. Inline field validation attached to one control stays a one-line `role="alert"` beside that control.

## Shell

Rail (desktop) and drawer tabs (phone) carry a needs-you dot on an area that needs the User. The Project switcher is a compact popover with search. Rail and drawer rows use `Row`; Rename and Remove Project actions show on hover or focus on desktop and live in Project settings on phone. Tooltips open on keyboard focus, not on programmatic or pointer focus (`ignoreNonKeyboardFocus`). The Dashboard leads with two `StatCard`s (Needs you, Running), then Needs you rows, Running rows and Project rows.

## Chat chrome

The chat header is `PageHeader size="compact"`: title, one `ChatStatus` glyph (reconnecting alert, pulsing working dot, live dot; the word is the tooltip and accessible name), then icon buttons: the `ContextMeter` ring (percent in its name; tap opens a bottom sheet under `md`, an anchored panel above, with model, tokens and Compress) and Git on desktop (a dot marks changed files). The Project chip hides under `sm`. Everything shows on one line at 360 px.

On a phone the header is the title, the status glyph, the context ring and one `ChatMenu` button: its sheet (`Session`) holds New Session, Git, the model with its context line, and the Project row, which switches the same sheet to the Project scope list instead of stacking a second one. New Session and Git leave the header there. The drawer keeps the Project switcher, the Session list and the five area tabs.

The composer is icon controls only. While working, the mode switch is two icon toggles (Steer `icons.steer`, Follow-up `icons.followUp`; the active one is filled) and the send button wears the active mode's glyph (arrow for a plain prompt), so the mode reads without words; Enter sends. Remember is a toggle (`aria-pressed` while the draft carries `/remember`). The Working indicator is the spinner plus the running tool trimmed to 14 characters. The Compress banner is one line with Retry, Change compress model and Dismiss as `IconButton`s. The tool strip is collapsed to one summary line (status, one icon per tool kind, current tool or `N tools · Xs`, timer, chevron); the line toggles the stage and history, and a failed strip starts open.

## Transcript

One record, three shapes: the User speaks in a bubble on the right, the agent answers as plain full-width text on the left, and notices and tool cards span the width. Nothing else wears a bubble, so a report can never read as something the User said.

- **User message.** `bg-surface-2`, `rounded-2xl rounded-br-md`, at most `max-w-[85%]` (`sm:max-w-[75%]`), `text-body`. A non-plain mode shows its caption (Steer, Follow-up) in `text-fg-3`; the sender badge appears only on a real User prompt.
- **Queued.** Dashed `border-status-working/40`, transparent fill, `text-fg-2`, with a `Clock` glyph and `Queued · <mode>` above the bubble. **Canceled** is dashed `border-border`, dimmed, with a `Ban` glyph and the word Canceled. Delivered, queued and canceled read apart by shape and word, not colour alone.
- **Assistant message.** Full width, no bubble, `text-body`; the `MessageActions` trigger is an `IconButton` (Ellipsis) named `Message actions for <message>`: hidden until the card is hovered or focused on pointer devices that hover, always visible on phone and on a tablet with no hover.
- **Action surfaces.** Desktop: an anchored icon toolbar (Copy text, Copy Markdown, Share, Edit and resend / Edit, Cancel) with tooltips; Cancel wears the canceled row's `Ban` glyph, not Dismiss. Phone: the same group as a bottom sheet with a title row, a Close `IconButton`, a dimmed scrim and 44 px icon-over-label tiles. The card keeps its entrance animation on an inner wrapper so the fixed sheet is placed against the viewport, not a transformed row. Retry stays the Compress banner action: a message action never re-runs a turn.
- **Report notice.** A full-width `surface-1` notice: icon tile, the report title (`Subagent report` / `N subagent reports`), the group line `Subagents <group> finished` with a one-line summary, the counts (a `Chip` above `sm`, its own caption line on phone), a chevron for the member detail and a Dismiss `IconButton`. The parsed report rides the transcript block (`report`), so dismissal hides the notice and the App filter can restore it from the tray.
- **Reports tray.** Sticky above the transcript: one panel with a count `Chip` and `N in transcript`, then rows (group name, counts · summary, Dismissed `Chip`) that open to the body and a Restore button. The list caps at `max-h-[min(60dvh,24rem)]` and scrolls inside, so 1 and 10 reports never grow the page. A Session row keeps `Row`'s unread dot until the Session is opened.
- **Scroll to latest.** `↓ Latest` is sticky at the transcript's reserved bottom padding, so it clears the composer dock and never covers it. Its `data-testid` is `latest-jump`.

## Settings, Memory, Providers

Settings pages are `PageHeader`, one visible description line, then `Section`s of a `Panel` of `SettingRow`s. Every row's help sits behind its `InfoHint`; a `note` is only for state that must stay visible (environment lock, missing model, privacy line). The save bar is a floating pill above the safe area. The hub (`SettingsHub`) is a `Panel` of `Row`s per group, with an Admin chip on admin-only pages.

The Chain editor (`ChainEditor`) shows one role as a heading, an add `IconButton` and up to three entries. Desktop edits harness, model and effort inline in a grid. Phone lists each entry as a tap row (slot, model, harness, effort chip, image warning chip); a tap opens a `BottomSheet` with the three pickers plus move and remove.

Memory opens on three `StatCard`s (Pending, New, Rules) and the proposals that wait for approval; everything else (Memories with legacy observations and Episodes, Rules, Observations, Promotions, Profile, Status, Ledger, Purge) is a collapsed `Section`. Profile and Promotions take `embedded` to nest inside Memory as one collapsed `Section` each. Proposal evidence sits behind a native `details`.

Providers use the Quota card: same article, icon tile, `text-title` name and `QuotaWindows` (big percent, bar, reset countdown), so both pages read the same. A Provider card adds a `Chip` for Connected or Not connected, its connect actions and its login panels; the percent comes from the shared `quotaStore`, so the card shows usage with no extra request per card. Connect flows keep their behaviour; only the markup is new.

## Auth and first run

The Root gate never paints a blank page: while it probes the session it shows `Skeleton` rows under `role="status"`, and when the Daemon does not answer it shows an `ErrorState` with the `Try again` action. The sign-in screen is one centred column of `max-w-sm`: a 56 px brand tile, the `text-headline` question (sign in, create an account, create the admin account on an empty Daemon), one caption line, then the form. Under md the form is bare and full width with the page safe area; from md it sits in a bordered card (`rounded-2xl border bg-card p-6 shadow-elev-1`). The submit button is the only primary action and carries the busy spinner; the mode switch is a ghost button under the form and only appears when signup is open. A failed attempt is the `compact` `ErrorState` in the form, and the password rules stay in the one caption line.

## Phone shell and sheets

The shell (`.app-shell`) takes `--app-height`, published from `window.visualViewport` by `lib/viewport.js` on phones: the on-screen keyboard shrinks the visual viewport without resizing the layout viewport, so the shell follows it and the composer dock stays above the keyboard. `--kb-inset` is the covered height and `html[data-keyboard="open"]` drops `--safe-bottom` to `0px`, because the keyboard covers the home indicator; bottom sheets sit at `bottom: var(--kb-inset)`. Above `md` no variable is set and the shell keeps `100dvh`.

Every sheet shares one transient history entry (`lib/overlayBack.js`): system Back or the swipe-back gesture closes the top sheet, and a sheet closed from its own UI pops the entry it held, so Back never replays a closed sheet. `BottomSheet`, the context sheet, the message-action sheet and the Git panel take part; the phone drawer keeps the entry App already held for it. App leaves popstate routing to the module while a sheet is open, and a navigation takes the entry over. A sheet that opens another hands the entry across (`carryBack`) instead of closing and pushing.

## Checklist for a new page

1. One `PageHeader` with a `text-headline` title (title size on phone); the headline state (what is happening, what needs the User) first.
2. Sections and rows from the primitives; numbers in `text-numeric`.
3. Empty, loading and error states from `EmptyState`, `Skeleton`, `ErrorState`.
4. Tokens only; icons from the map; at most one visible help paragraph.
5. Checked at 390 px and 1280 px.
