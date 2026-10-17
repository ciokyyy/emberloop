# Designer agents

> Amended 2026-10-17: no design Reviewer seat. A `design` Ticket uses the spec and standards Reviewers; the standards Reviewer also judges the rendered result and gets only the `agent-browser skills get core` mention. Text below that names `reviewer-design` is superseded.

One spec, from the 2026-10-16 grilling Session. It adds Ticket types to Runs so visual work goes to a Designer on a strong model that can look at the rendered page, removes the grok Backend, and gives every agent a way to inspect local UI through the `agent-browser` CLI. Decision record: `docs/adr/0005-designer-tickets-agent-browser-cli.md`. Browser tooling research and measurements: `research-browser-tooling.md` (its Playwright MCP pick is superseded by the ADR).

## Problem Statement

1. Cheaper models implement backend logic well but cannot judge or direct UI. The same models review UI code and call any result good, so weak front ends merge.
2. Every Ticket gets the same Worker Chain and the same two Reviewers, so a User cannot route visual work to a stronger model.
3. No agent can see rendered output. There is no screenshot, snapshot or device-emulation tool; `read` tier 3 returns static HTML only.
4. The grok Backend has no path for images or for third-party tools, and the User no longer wants to support it. It adds a branch to every Backend, Chain and settings code path.
5. The Run preview and `to-tickets` have no notion of work that needs both logic and look.

## Solution

A Ticket has a type, `code` or `design`, set by a `**Type:**` header line (default `code`). A `design` Ticket is implemented by a Designer on its own `worker-design` Chain and reviewed by the spec, standards and design Reviewers, the last on its own `reviewer-design` Chain. `to-tickets` splits mixed work into a `code` Ticket and a `design` Ticket that it blocks; the rule ships as an Emberloop-owned skill override. The Run preview shows each Ticket's type and lets the User flip it before Start; the flip lives in the Run file.

Designers and design Reviewers see pages through the `agent-browser` CLI, a pinned release binary installed by `el deps` and driven from `bash` or `execute`. The Daemon gives each Session an isolated browser session, localhost-only domains, a screenshot directory and an idle timeout, closes the browser when the Session ends, and injects a short snapshot-first skill into Designer and design Reviewer prompts. Other roles get a one-line mention. The grok Backend and everything that exists only for it are removed.

## User Stories

1. As a User, I want to mark a Ticket `design`, so that a strong visual model implements it.
2. As a User, I want an unmarked Ticket to stay `code`, so that every existing spec runs as before.
3. As a User, I want a mistyped Type value to fail only that Ticket's preview row, so that one typo does not block the Run.
4. As a User, I want the Run preview to show each Ticket's type, so that I see what the Run will route before I start it.
5. As a User, I want to flip a Ticket's type in the Run preview, so that I fix a missed label without editing files.
6. As a User, I want a flipped type to survive a Daemon restart and apply when the Run starts, so that my correction is not lost.
7. As a User, I want the Ticket file left untouched by a flip, so that only the Run driver writes Ticket files.
8. As a User, I want a "Designer worker" Chain and a "Design reviewer" Chain in Settings > Agents, so that I choose models for visual work.
9. As a User, I want the same Chain and Switch behaviour on those roles as on the others, so that quota failures advance and I can pause and switch.
10. As a User, I want an unset design Chain to fall back to the worker or standards Chain with a visible warning, so that no Run blocks on a missing setting.
11. As a User, I want a warning when a design Chain entry's model takes no image input, so that I do not pick a model that cannot see the page.
12. As a User, I want a `design` Ticket to receive the spec, standards and design Reviewers, so that code quality and visual quality are both judged.
13. As a User, I want a `code` Ticket to keep exactly its two Reviewers, so that existing Runs are unchanged.
14. As a Designer, I want to open the Ticket's dev server in a browser from my worktree, so that I check my work by looking at it.
15. As a Designer, I want snapshot-first guidance, so that I use cheap text inspection and spend screenshot tokens only on visual checks.
16. As a Designer, I want phone and desktop widths and JPEG screenshots named in my brief, so that I check both layouts cheaply.
17. As a design Reviewer, I want to look at the rendered page at the Ticket tip, so that my verdict rests on what the page shows.
18. As a Worker or Reviewer of any role, I want a one-line mention that the browser CLI exists for local UI checks, so that I can use it when a Ticket needs it.
19. As a User, I want the browser limited to localhost by default, so that an agent cannot browse the open web through it, with an admin setting to widen the list.
20. As a User, I want each Session's browser closed when the Session ends or the Daemon stops, so that no browser outlives its work.
21. As a User, I want parallel Workers to use separate browser sessions and separate dev-server ports, so that they do not interfere.
22. As a User, I want `el deps` to install the browser CLI as an optional dependency, so that a missing binary warns instead of failing the install.
23. As a spec author, I want `to-tickets` to split logic and look into two sequential Tickets, so that the code Ticket's merged result gives the Designer a working page to restyle.
24. As a spec author, I want restyle-only work to produce one `design` Ticket and logic-only work one `code` Ticket, so that Tickets are not split for show.
25. As a User, I want the grok Backend removed with its settings, UI option and tests, so that no code path carries it.
26. As a User, I want existing Sessions that used grok to stay readable, so that I keep their history.
27. As a User, I want a saved Chain or role setting that names grok dropped on read with a notice, so that no Run starts a Backend that no longer exists.
28. As a User, I want an end-to-end probe of a code-then-design spec, so that I know the whole path works with real models.

## Implementation Decisions

- **Ticket type.** `**Type:**` sits beside `**Blocked by:**` and is parsed by the same Ticket header reader. Values are `code` and `design`; a missing line means `code`; any other value fails that Ticket's preview row and, as for a bad blocker, fails the Run's check-world pass. The Run snapshot and preview carry the effective type.
- **Type override.** The Run file stores an optional per-Ticket type override that wins over the file line. A flip is allowed until the Ticket's Worker starts. The Ticket file is never edited by a flip.
- **Chain roles.** Two new Chain roles, `worker-design` and `reviewer-design`, join the existing six, follow the same entry rules (at most three `{harness, model, effort}`, quota/credit/auth/session-limit advance, per-Ticket per-role position, Switch and Resume) and get Settings > Agents slots and `el settings role` support. An empty `worker-design` Chain makes a Designer use the `worker` Chain; an empty `reviewer-design` Chain makes the design Reviewer use the `reviewer-standards` Chain. Each fallback shows a warning on the Ticket and in the preview.
- **Reviewer set.** The required Reviewer types become a function of Ticket type: `code` gets spec and standards, `design` gets spec, standards and design. A Pass needs every required Reviewer at the current tip, as today. The design Reviewer judges the rendered result, not the diff, and follows the Reviewer submit contract.
- **Designer prompt.** A `design` Ticket's first prompt carries the Designer skill. The Designer implements on top of the merged `code` Ticket; its scope warning treats logic and test files as out of scope, which is advisory like the existing scope warning.
- **Browser CLI.** `el deps` installs a pinned `agent-browser` release binary from the vercel-labs GitHub release for the platform, verified against the release's sha256 digest, into the el bin dir, as an optional dependency (warns, never fails), idempotent. A `browser.enabled` setting (default on) gates the install and the Session environment. The binary uses system Chromium or Chrome through `AGENT_BROWSER_EXECUTABLE_PATH`.
- **Session environment.** For every harness Session the Daemon adds, to the environment of the el server the Backend launches, `AGENT_BROWSER_SESSION` (Session ID), `AGENT_BROWSER_NAMESPACE` (per Daemon), a per-Session `AGENT_BROWSER_SCREENSHOT_DIR`, `AGENT_BROWSER_ALLOWED_DOMAINS=localhost,127.0.0.1` (admin setting `browser.allowed_domains` widens it), `AGENT_BROWSER_MAX_OUTPUT`, `AGENT_BROWSER_CONTENT_BOUNDARIES`, `AGENT_BROWSER_IDLE_TIMEOUT_MS` of about five minutes, and the el bin dir on `PATH`. The `bash` and `execute` tools inherit that environment. This applies to the claude Backend and to the pig Backend, ACP and SDK paths alike.
- **Cleanup.** When a Session ends or its worktree is removed the Daemon runs `agent-browser close` for that Session's name; Daemon shutdown runs `close --all` in its namespace. The idle timeout is the backstop for a crash. Per-Session screenshot directories fall under the disk janitor.
- **Dev server.** The repo may provide `.emberloop/preview`, which reads `PORT` and prints the URL. The Daemon gives each Designer and design Reviewer Session a free `PORT` and makes sure no preview process outlives its Session. Without the script the Designer follows the dev command in the repo's AGENTS.md.
- **Skills.** A `designer` entry and a `reviewer-design` entry join the `run_skills` defaults: snapshot first with depth and element limits, screenshots only to judge visuals at 390 px and 1280 px in JPEG at a capped scale, never full-page by default, check console and `a11y` before `submit`, localhost only. Every other Worker and Reviewer brief and web Session prompt gains one line naming the CLI and `agent-browser skills get core`.
- **`to-tickets` override.** Emberloop ships a second skill library, installed beside `matt-pocock` by `el skills install`, whose `to-tickets` wins when both exist. It adds the split rule (a `code` Ticket delivering logic, API, data, tests and a plain-semantic-HTML UI that works; a `design` Ticket blocked by it that owns layout, styling, visual states and copy and may change markup but not logic; restyle-only work is one `design` Ticket; no-UI work is one `code` Ticket; a slice is complete only when both are done), the `**Type:**` line in its Ticket template, and a Type column in its quiz step.
- **Grok removal.** The grok Backend, its registration, its model and effort lists, its settings and Chain validation branches, its UI option and icon, its live probe and env vars (`GROK_HOME`, `EL_LIVE_GROK`) and its tests are deleted, and docs lose every mention (the ADR keeps one line saying grok was removed). A stored Session whose harness is `grok` stays readable (Events, transcript) and cannot be resumed or prompted: the Daemon refuses with a message that grok support was removed, and never falls through to the default Backend. A saved Chain entry or role setting naming grok is dropped on read; a Run's stored override or Chain position that referenced it is cleared, and a Ticket left with an empty role Chain uses the configured defaults. `PUT /api/settings` rejects grok with the existing "harness must be claude or pig" style error.

## Testing Decisions

- A good test drives external behaviour at the highest existing seam; it does not assert internals.
- **Run driver seam** (`runs_driver_test.go` with fake Backend and fake git): type routing, Reviewer set per type, Chain fallback and warning, restart persistence of the type override.
- **Ticket preview seam** (`ticketview` tests): Type line parse, default, bad value row failure.
- **Settings and Chains seam** (`chains_test.go`, `settings_test.go`, `SettingsPage.test.js`): the two new roles, migration, grok drop-on-read and PUT rejection.
- **Session environment seam**: Backend wire tests on the `mcpServersFor`/`mergedMCP` output, plus `execute` and `bash` inheriting it (one real tool call under a stub binary).
- **`el deps` seam**: fake release server and fake binary, as in existing deps tests; no test downloads for real.
- **Live probes** (never part of the repo check): one script proves the CLI end to end on claude and pig (env inheritance, screenshot through `read` reaches a vision model); one proves a code-then-design spec Run with real models.
- **UI seam** (`RunsView.test.js`, `router.test.js`): badge, flip and warning in the Run preview.
- **Prior art:** `runs_chain_test.go`, `runs_reviewers_test.go`, `deps_test.go`, `acpretry_test.go`, `harnessdirs_test.go`.

## Out of Scope

- Any image path for grok, or reviving it.
- A Playwright MCP, a global MCP, or a Daemon-owned `screenshot` tool; Playwright MCP stays the documented fallback if `agent-browser` churn becomes a cost.
- Inferring Ticket type from changed paths.
- Browser access beyond localhost by default.
- More Ticket types than `code` and `design`.
- Changing the Run's merge, check or Pass rules beyond adding the design Reviewer.

## Further Notes

- The ticket that removes grok runs first so later tickets do not carry the branch.
- The Designer and the design Reviewer need a vision-capable model. The warning is advisory because model metadata may not say.
- Two behaviours are decided here, not by the User in the grilling: stored grok Sessions stay readable but not resumable, and saved grok settings drop silently on read with a notice. Change them in ticket 01 before work starts if the User disagrees.
- `agent-browser` is a vercel-labs project at 0.x with frequent releases; bump the pin deliberately.
