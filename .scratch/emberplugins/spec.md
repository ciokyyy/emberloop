# Emberplugins

## Problem Statement

Emberloop users run coding-agent Sessions in the browser UI, but the UI is fixed. A user who builds a lead scraper, or a dashboard for a local app, has no place inside Emberloop to see it. Adding a page today means changing Emberloop source and rebuilding the UI. The user wants agents inside Emberloop to add pages, background work and small apps at runtime, using the harnesses and models Emberloop already drives.

## Solution

An **Emberplugin** is a folder in a global plugin directory under the Daemon state dir. It has a manifest and optionally a UI, scheduled jobs, storage and a server process. An agent writes it inside a Session, following a docs page, using Go and Svelte on a prebuilt kit. The Daemon notices the folder (hot reload) and adds a sidebar entry. The page runs in a sandboxed iframe on its own origin. It calls the Daemon API with a per-plugin token, so it can run LLM calls and Sessions on the user's harnesses. Permissions come from the manifest and the user approves them once. Idle plugins release their RAM.

## User Stories

1. As a user, I want to ask a Session "add a page that shows X", so that the page appears in Emberloop without me touching source code.
2. As a user, I want Emberplugins to be global, so that one plugin works whichever project I have open.
3. As a user, I want a new Emberplugin to show in the sidebar without restarting the Daemon, so that the agent's work is visible at once.
4. As a user, I want an Emberplugin page to look like the rest of Emberloop, including dark and light theme, so that apps feel native.
5. As a user, I want to disable or delete an Emberplugin from the UI, so that I stay in control of what runs.
6. As a user, I want to see each Emberplugin's status, RAM and logs, so that I can find a misbehaving one.
7. As a user, I want a one-time approval screen listing the permissions an Emberplugin asks for, so that an agent cannot widen its rights silently.
8. As a user, I want the approval screen to say plainly that plugin processes run with my OS rights, so that I know the limit of the permission system.
9. As a user, I want sudo and file-write permissions to always need my approval, so that default grants stay safe.
10. As a user, I want every Session an Emberplugin starts to show in the main Session list tagged `plugin:<name>`, so that no agent activity is hidden.
11. As a user, I want to stop any Session or job an Emberplugin runs from the main UI, so that a runaway plugin is easy to stop.
12. As a user, I want a lead scraper Emberplugin that fetches leads on a schedule and shows them in a table, so that I can review them in Emberloop.
13. As a user, I want a lead scraper step that calls an LLM to score a lead, so that the plugin uses Emberloop's models directly.
14. As a user, I want a dashboard for a local, undeployed app, so that I can watch it in Emberloop.
15. As a user, I want an Emberplugin to notify me through Web Push or an in-app banner, so that "found 12 new leads" reaches me.
16. As a user, I want notifications rate-limited and tagged with the plugin name, so that one plugin cannot spam me.
17. As a user, I want tapping a notification to open the plugin page, so that I land where the news is.
18. As a user, I want idle Emberplugin servers to stop after a timeout, so that RAM stays low.
19. As a user, I want an Emberplugin page to unmount when I leave it, so that closed pages cost no browser RAM.
20. As a user, I want a `keepAlive` plugin, such as a scheduled scraper, to keep running when idle, so that schedules fire.
21. As a user, I want Emberplugin processes counted against their own cap `max_plugins`, so that plugins never starve my Sessions.
22. As a user, I want a Session job to count against `max_workers`, so that the global live-Session limit still holds.
23. As a user, I want a skipped job run (cap full) to be logged and not queued forever, so that the system stays predictable.
24. As a user, I want a crashing plugin server to restart with backoff and be disabled after three strikes, so that a bad plugin cannot loop forever.
25. As an agent, I want a docs page `el://docs/plugins` with manifest format, kit, API, templates and a worked example, so that I can build an Emberplugin without guessing.
26. As an agent, I want a CLI (`emberloop-web plugin new|build|logs|status`) that I run through bash, so that scaffolding and building are one command on every harness.
27. As an agent, I want a prebuilt kit with every shadcn-svelte component in Emberloop's style, plus page templates, so that I compose a page and build very little.
28. As an agent, I want one shared build toolchain, so that I never run a slow per-plugin install.
29. As an agent, I want per-plugin storage (KV and SQLite) from the Daemon, so that a plugin needs no server to keep data.
30. As an agent, I want cron jobs in the manifest that run a script, an LLM step or a Session, so that a UI-only plugin can still work in the background.
31. As an agent, I want an optional Go server process with a documented env-var contract, so that I can handle cases storage and jobs cannot.
32. As an agent, I want an OpenAPI file and generated SDKs for the plugin-callable routes, so that I can discover every call and call it typed.
33. As an agent, I want a plugin token whose permissions I declare in the manifest, so that the Daemon enforces what the plugin may call.
34. As an agent, I want build errors and runtime logs readable through the CLI, so that I can fix my own plugin.
35. As a developer of Emberloop, I want the Emberplugin term in the glossary and the origin and trust decisions in an ADR, so that later work stays consistent.

## Implementation Decisions

- **Term**: Emberplugin. Not plugin, extension, addon or miniapp (harnesses own those words). Add to `GLOSSARY.md`.
- **Location**: one global plugin directory under the state dir beside the other `enderbloom/` state. Each Emberplugin is a folder with a manifest, source, `dist/`, a data dir for storage, and logs.
- **Discovery**: the Daemon watches the plugin directory. New, changed or removed manifests update the registry and the sidebar without restart. Enable and disable state persists.
- **Manifest**: name, title, icon, sidebar entry, permissions, optional jobs (cron plus job kind), optional server (command, `keepAlive`), optional config fields (for example a project working directory).
- **UI embed**: each Emberplugin is served by the Daemon on its own origin and rendered in an iframe with `sandbox` that omits `allow-same-origin` toward the main UI. The page mounts only while open and unmounts on leave. Plugin state lives in storage, not in the iframe.
- **Kit**: the Daemon serves a prebuilt kit at `/plugin-kit/`: design tokens, base CSS, every shadcn-svelte component styled the Emberloop way, page templates (dashboard, table plus detail, form plus results), theme sync with the host, and an API client. The kit is versioned with Emberloop. No plugin ships its own design system.
- **Build**: `plugin build` runs one shared Vite and Svelte toolchain installed once under the state dir. Plugin folders hold only source and `dist/`. Node or bun must exist on the machine.
- **Token and permissions**: each Emberplugin gets a per-plugin token. The Daemon checks the manifest permission groups on every route (for example `llm`, `sessions`, `runs`, `files`, `sudo`, `notify`). Default grant is read plus LLM. The user approves the full set once in the UI; a changed set needs a new approval. Sudo and file writes always need approval.
- **Trust**: Emberplugin code is trusted agent code with no OS sandbox. Daemon permissions gate the Daemon API only. Scripts and servers run with user rights. The approval screen says so. Record in an ADR.
- **Emberloop access**: through the Daemon API with the token, from the iframe and from the plugin's server and jobs alike. A one-shot LLM completion route exists (no Session). Plugins can create and drive full Sessions and Subagents, and read Runs, Sessions, models and quota.
- **Plugin-created Sessions**: appear in the main Session list tagged `plugin:<name>`, never hidden, stoppable from the main UI, read-only to the plugin after finish.
- **Storage**: per-plugin KV and SQLite file in the plugin data dir, offered through the Daemon API.
- **Jobs**: cron entries in the manifest. A job runs a script or Go binary in the plugin folder, or an Emberloop step (one-shot LLM call or Session with a prompt). Script jobs use the `max_plugins` budget. Session jobs use `max_workers`. A run skipped for a full cap is logged, never queued forever. Job runs, status and logs show on the plugin's page in the UI.
- **Server process**: optional. The Daemon picks a loopback port and passes it, the token, the Daemon URL and the data dir through env vars. It proxies `/plugins/<name>/api/*` to the process. Stdout and stderr go to a log file shown in the UI. Crashes restart with backoff; three strikes disable the plugin. Start on first request; stop after 10 minutes idle unless `keepAlive`.
- **Caps and RAM**: `max_plugins` caps live plugin processes, separate from `max_workers`, settable in settings like `max_workers`. The plugin list shows RAM per process. The Daemon's in-place restart treats a running plugin process like a live agent process.
- **Notifications**: a `notify` permission feeds the existing Web Push and an in-app banner like Needs-you. Rate-limited, tagged with the plugin name, opens the plugin page.
- **Agent surface**: docs page `docs/agent/plugins.md` (read as `el://docs/plugins`, refreshed by `el update`), and a CLI in the Daemon binary run through bash. No MCP tool, no new MCP surface.
- **API discoverability**: a generated OpenAPI file served at `/api/openapi.json`, linked from the docs, with a permission group on each route. Start with plugin-callable routes only (sessions, prompt, events, spawn, runs, models, quota, one-shot LLM, storage, notify). SDKs for Go and TypeScript are generated from it with an off-the-shelf generator; the generator is chosen in the SDK ticket.
- **Modules**: a plugin registry and watcher; a plugin origin server and token issuer with a permission middleware; a job scheduler; a process supervisor; a storage service; the kit and its build; the toolchain installer and CLI; the settings and UI views for the plugin list, approval screen, status and logs; OpenAPI generation.
- **Docs ownership**: new `AGENTS.md` for the area; update `emberloop-web/AGENTS.md`, `web/README.md` (design rules apply to the kit), root `AGENTS.md` index, `GLOSSARY.md`, and add an ADR.

## Testing Decisions

- A good test drives external behavior only: HTTP and SSE API calls, rendered UI, files on disk. It does not assert internal structure.
- **Primary seam**: the Daemon HTTP and SSE API with the fake Backend and a temp plugin directory, as `api_test.go` and `lifecycle_test.go` do. Fixture plugin folders sit in test data. Registry reload, token and permission checks, storage, job runs, caps, supervisor restart and strikes are all tested here with fake clocks where time matters.
- **Secondary seam**: Vitest component tests for the kit and plugin list views, as `RunsView.test.js` does.
- **Browser seam**: the `web/e2e/` suite proves one real plugin end to end: sidebar entry, iframe render with kit styles, token call, LLM call on the fake Backend. Not in the main check.
- Real harness calls stay in live probes, never in the main check.
- Prior art: `api_test.go`, `lifecycle_test.go`, `spawn_test.go`, `quota_test.go` (local HTTP stubs), `runs_persist_test.go` (persistence and reload), `RunsView.test.js`, `web/e2e/`.

## Out of Scope

- Hooks into existing views (Session transcript cards, Composer buttons, Runs widgets).
- Install from a git URL, sharing between machines, a registry, versioned updates.
- Plugin-to-plugin calls.
- An OS-level sandbox for plugin processes.
- A declarative, code-free page spec.
- A new MCP tool.

## Further Notes

- Slices build on each other: first the thin path (manifest, sidebar, iframe, kit, token, LLM call), then storage and jobs, then the server process, then approval UI, notify, CLI and docs, then OpenAPI and SDKs.
- The sibling-checkout rule holds: the work lands in `emberloop-web/` and may touch core `emberloop/` only where a shared route needs it; core never imports `emberloop-web`.
- Open detail for the SDK ticket: pick the OpenAPI SDK generator (candidates to compare: openapi-generator, oapi-codegen for Go, a TypeScript generator).
