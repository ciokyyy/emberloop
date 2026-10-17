# Design system

"Calm density": dark only, quiet surfaces, confident numbers, status colour with a word. Tokens live in `src/app.css`; primitives live in `src/lib/ui/`; the icon map is `src/lib/icons.js`. Extend these. Never hardcode a colour, size, shadow or duration, and never pick an icon for a domain concept by hand.

The reference page is Quota (`src/lib/QuotaPage.svelte`): read it for every primitive in use at 390 px and 1280 px.
Quota provider cards sit in a container-query grid (single column below `@2xl`) and flow up to three usage windows across (`sm:grid-cols-3`) so row-mate cards stay even. `e2e/tests/quota-layout.e2e.ts` probes 390, 800 and 1280 px; 800 px is extra coverage for the rail plus sidebar layout.

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

One easing, `--ease-smooth`; three durations. Utilities `motion-instant`, `motion-standard` and `motion-emphasis` set duration and easing together; `animate-rise` (emphasis) is the mount entrance.

| Token | Value | Use |
| --- | --- | --- |
| `--dur-instant` | 100 ms | Press, tooltip |
| `--dur-standard` | 180 ms | Hover, colour, disclosure chevrons |
| `--dur-emphasis` | 320 ms | Entrances, bar growth, details height |

Reduced motion (`prefers-reduced-motion: reduce`): transitions and one-shot animations end at once and smooth scroll is off. Only spinners and `data-motion="essential"` elements keep moving, slowly. No decorative loops other than the Working indicator; Skeleton pulses with `motion-safe:` only. Streamed text keeps its own `stream-in` fade.

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
| `EmptyState` | `empty-state.svelte` | `icon`, `title` (one line), `description`, `action` snippet. |
| `ErrorState` | `error-state.svelte` | `title`, `detail`, `onRetry`, `busy`. `role="alert"`, one Retry action. |
| `Skeleton` | `skeleton.svelte` | Decorative block (`aria-hidden`); wrap a group in `role="status"` with an `aria-label`. Loading is skeleton, not text. |
| `InfoHint` | `info-hint.svelte` | `label` (accessible name), children. Tap-to-reveal popover for help text. |

Existing shadcn primitives (`Button`, `Badge`, `Dialog`, `Sheet`, `Select`, `Combobox`, ...) stay; `Chip` is for status and facts, `Badge` for legacy uses.

## Rules

### Icon rule

Icons carry actions; words carry meaning. A common, well-known action (copy, share, edit, retry, dismiss, refresh, compress) is an `IconButton` with a tooltip and an accessible name. Status is a dot or glyph plus one short word. Concepts use the icon map. Never an icon-only button without `label`.

### Disclosure rule

At most one paragraph of help text is visible by default on any page. The rest sits behind an `InfoHint` or a collapsed `Section`. Data lines (updated 8s ago, resets in 1h) are not help text. Detail opens as a sheet or expandable section, not a wall of text.

### Responsive rule

Every design change specifies both layouts and says what phone omits. Phone (`<md`): single column, bottom-anchored primary action, sheets for secondary panels, 44 px touch targets (`max-md:` sizes), safe-area insets (`env(safe-area-inset-*)`), no hover-only controls. Desktop: persistent navigation, side panels, hover affordances with keyboard equivalents. Grids that sit beside the rail and second sidebar switch columns on the content column's width (container queries, `@container` + `@2xl:grid-cols-2`), not the viewport (`md:`), so 768 to 990 px viewports stay single column. Verify at 390 px and 1280 px (and about 800 px for pages with the sidebar): no horizontal scroll, every control reachable by keyboard with a visible focus ring.

## Shell

Rail (desktop) and drawer tabs (phone) carry a needs-you dot on an area that needs the User. The Project switcher is a compact popover with search. Rail and drawer rows use `Row`; Rename and Remove Project actions show on hover or focus on desktop and live in Project settings on phone. Tooltips open on keyboard focus, not on programmatic or pointer focus (`ignoreNonKeyboardFocus`). The Dashboard leads with two `StatCard`s (Needs you, Running), then Needs you rows, Running rows and Project rows.

## Chat chrome

The chat header is `PageHeader size="compact"`: title, one `ChatStatus` glyph (reconnecting alert, pulsing working dot, live dot; the word is the tooltip and accessible name), then icon buttons: the `ContextMeter` ring (percent in its name; tap opens a bottom sheet under `md`, an anchored panel above, with model, tokens and Compress), New Session (phone) and Git (a dot marks changed files). The Project chip hides under `sm`. Everything shows on one line at 360 px.

The composer is icon controls only. While working, the mode switch is two icon toggles (Steer `icons.steer`, Follow-up `icons.followUp`; the active one is filled) and the send button wears the active mode's glyph (arrow for a plain prompt), so the mode reads without words; Enter sends. Remember is a toggle (`aria-pressed` while the draft carries `/remember`). The Working indicator is the spinner plus the running tool trimmed to 14 characters. The Compress banner is one line with Retry, Change compress model and Dismiss as `IconButton`s. The tool strip is collapsed to one summary line (status, one icon per tool kind, current tool or `N tools · Xs`, timer, chevron); the line toggles the stage and history, and a failed strip starts open.

## Checklist for a new page

1. One `PageHeader` with a `text-headline` title (title size on phone); the headline state (what is happening, what needs the User) first.
2. Sections and rows from the primitives; numbers in `text-numeric`.
3. Empty, loading and error states from `EmptyState`, `Skeleton`, `ErrorState`.
4. Tokens only; icons from the map; at most one visible help paragraph.
5. Checked at 390 px and 1280 px.
