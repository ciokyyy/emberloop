# Browser tooling for Designer Workers and design Reviewers

Date: 2026-10-16. Question: how should a Ticket-scoped Worker/Reviewer SEE and
debug web UI in its git worktree? Today only `read` tier-3 exists
(`emberloop-web/el/tools/readurl.go`: one-shot `chrome-headless-shell`
`--dump-dom`, HTML only, no interaction, no pixels). Initial candidates
considered were a Daemon-owned screenshot tool with `.emberloop/preview`, and
a global browser MCP server; both were superseded by the ADR below.

Superseded by [ADR 0005](../../docs/adr/0005-designer-tickets-agent-browser-cli.md): the recommendation below is research history, not the current plan.

## Historical recommendation

**Winner: Daemon-owned, per-Session attachment — not a global shared server.
Wrap Playwright MCP (`@playwright/mcp`) as a second upstream server visible
only to the new Designer role; runner-up `agent-browser` (Vercel); use
`chrome-devtools-mcp` only for deep perf/debug; avoid Stagehand, Puppeteer
MCP, and a Go-native rewrite for now.**

Reasons:

1. A "global" server cannot work as one shared instance. Every serious
   candidate isolates by per-instance args (`--user-data-dir`, `--isolated`,
   `session`, debug port). Parallel Workers on different worktree dev servers
   need one browser context each, with localhost-only scope and cleanup —
   that is per-Session parametrization, which only the Daemon can do at
   Session start. What *is* global: the install (pin the npm package, prefetch
   the browser once).
2. `@playwright/mcp` has the exact knobs this job needs, all server-side:
   `--image-responses allow|omit|only` (token cap), `browser_snapshot`
   depth/element scoping ("better than screenshot", per its README),
   `--device`/`--mobile`/`--viewport-size`, `--allowed-hosts`,
   `--blocked-origins`, `--isolated`/`--user-data-dir`, `--output-dir` /
   `--output-max-size`, `--console-level`. No other candidate puts the token
   cap and the localhost restriction in one flag set.
3. It reuses Emberloop paths that already exist: el already speaks upstream
   MCP (`el/tools/mcpclient.go`, per-role enable/placement in
   `el/toolresolve.go`), el-mcp already returns MCP image blocks
   (`el/mcp/server.go`), and `read` already attaches local images
   (`el/tools/image.go`, 5 MB cap). No new browser-driving code to maintain.
4. `agent-browser` is a close second and beats Playwright on raw token
   efficiency (`snapshot --delta` incremental updates, `core` tool profile,
   annotated screenshots binding visual refs to AX refs). It loses on
   maturity risk: `vercel-labs` prefix, labs-grade API churn, v0.39.1.
   Revisit in ~6 months.
5. `chrome-devtools-mcp` (v1.11.0, released 2026-10-15, most active of the
   three) is the deepest debugger (perf traces + CrUX field data, console with
   source maps, CSS, `--slim` mode) but is the worst fit as *the* Designer
   tool: it officially supports only Google Chrome / Chrome for Testing (this
   host has `chromium`, not Chrome — a fresh ~150 MB install per machine),
   its default profile is shared (`~/.cache/chrome-devtools-mcp/`, one browser
   at a time), and usage statistics + update checks are on by default
   (opt-out flags exist). Keep it as an opt-in second tool for perf work.
6. Avoid: Stagehand (MIT SDK, but `act()` needs an LLM key and it targets
   Browserbase cloud — wrong shape, recurring cost); Puppeteer MCP (the
   official `modelcontextprotocol/servers` puppeteer server is archived;
   only scattered forks remain); Go-native chromedp/rod (both MIT and active
   — chromedp pushed 2026-10-11, rod 2026-08-11 — but a bespoke tool means
   owning AX-tree shaping, screenshot plumbing, orphan-Chrome reaping and its
   tests; do it only if node/npx in the agent path is unacceptable).
7. Keep `.emberloop/preview`: whatever drives the browser still needs the
   worktree dev-server URL, and a repo script printing it stays the right seam.

## Measured token comparison (reproduced locally, 2026-10-16)

Trivial settings-like page (`/tmp/uitest.html`), `chromium --headless
--no-sandbox`, method recorded so anyone can rerun:

| Output | Size | ≈ Tokens |
|---|---|---|
| ARIA snapshot (`playwright-core` `ariaSnapshot()`, what `browser_snapshot` returns) | 393 chars | ~100 (@4 ch/tok) |
| `--dump-dom` HTML (what `read` tier-3 returns today) | 631 chars | ~160 |
| Viewport screenshot 1280×800 PNG | 24 KB | **1334** visual tokens (Anthropic formula below) |
| Phone screenshot 390×844 PNG | 20 KB | **434** visual tokens |

Anthropic visual-token rule (primary: `platform.claude.com/docs/en/build-with-claude/vision`):
cost = `⌈w/28⌉ × ⌈h/28⌉`; standard tier downscales to ≤1568 px long edge /
≤1568 tokens, high-res tier (Claude 4.7+) to 2576 px / 4784 tokens. A full-HD
viewport shot costs the standard-tier maximum (~1560); a 4K shot costs 4784
on high-res models. Anthropic's own guidance: downsample before sending.

Caveat: the test page is tiny. Real app snapshots run 5–30 KB; that is why
snapshot-first discipline plus `depth`, element scoping, `-i/-c` compact flags
or `--delta` matters more than the per-shot choice. Microsoft's own README
says it outright: CLI+SKILLs beat MCP on tokens because MCP loads large tool
schemas and verbose AX trees into context (`microsoft/playwright-mcp`
README, "Playwright MCP vs Playwright CLI").

Working rule for the Designer skill: snapshot first (capped depth), screenshot
only to judge visuals (viewport, `css` scale, JPEG/WebP), full-page never by
default, console+network on failure.

## Comparison table

| Criterion | Playwright MCP 0.0.83 (2026-09-28, Apache-2.0, Microsoft) | Playwright CLI (repo `microsoft/playwright-cli`; npm name/TBD — `playwright-cli@0.262.0` on npm is dated 2026-02 and may be a different package, **verify**) | chrome-devtools-mcp 1.11.0 (2026-10-15, Apache-2.0, Google) | agent-browser 0.39.1 (2026-10-13, Apache-2.0, vercel-labs) | chromedp / rod (Go, MIT, pushed Oct/Aug 2026) | Stagehand / Puppeteer MCP |
|---|---|---|---|---|---|---|
| Inspection tokens | AX `browser_snapshot` + `depth`/`boxes`/element scope; every action omits the snapshot unless asked (`--snapshot-mode none`); `--image-responses omit` kills images server-side | Same engine, but no tool-schema load per call; snapshot printed after each command; `snapshot "#main"`, depth limit. MS claims CLI is the token-efficient shape | `take_snapshot` (AX + UID refs) to file or inline; `--slim` shrinks tool surface | `snapshot -i -c -d5 -s SEL`, **`--delta`** (full once, then structural diffs); `core` MCP profile keeps context small | Whatever you build (full control, but you build it) | SDK-shaped, no snapshot discipline of its own / archived |
| Visual judgement | `browser_take_screenshot`: viewport/element/`fullPage`, png/jpeg/webp, `scale: css` (small) vs `device`; `--caps=vision` adds coordinate actions. No computed-style/contrast tool (use `browser_evaluate`) | Same via `screenshot` cmd; `console`, `requests` cmds | `take_screenshot` (page/element, fullPage param); `emulate` (viewport, geolocation…); `get_css_styles` via snapshot UIDs; perf traces | `screenshot --annotate` (numbered labels bound to AX refs — best vision↔text bridge); `mobile` profile for devices; a11y audit; React tree | CDP screenshot + emulation free; computed-style/contrast is hand-rolled JS | Screenshots yes; nothing distinctive |
| Debugging | `browser_console_messages` (+ `--console-level`), `browser_network_requests`, `browser_evaluate`, `--caps=devtools`, tracing | `console [min-level]`, `requests`, `eval`, trace/video cmds | Deepest: console w/ source-mapped stacks, network list + detail, all DevTools domains, **perf traces + CrUX field data**, PWA/memory tools | `console`, `network` routes + HAR, tracing/profiling/recording, `diff`, dashboard | CDP gives everything; you write the tools | Good, but cloud-coupled / dead upstream |
| Parallel isolation | `--isolated` (in-memory profile), `--user-data-dir <per-session>`, `--storage-state`; HTTP/SSE multi-client shares context unless isolated; 1 h idle shutdown (headless) | Sessions (`PLAYWRIGHT_CLI_SESSION`), 1 h idle shutdown; one CLI call = one shot, no server state to leak | Default profile shared, **one browser at a time**; `--isolated` temp profile; `pageId` routing when one server is shared; `--browser-url` to attach to per-worktree Chromes on distinct ports | `--session <id>` / `AGENT_BROWSER_SESSION`, `--namespace` (daemon sockets), `--profile <path>` | Per-Session `user-data-dir` + context cancel in Go; you own reaping | Cloud sessions solve it with money / N/A |
| Headless Linux + install | `npx @playwright/mcp`, needs a Playwright browser **but** `--executable-path /usr/bin/chromium` reuses system Chrome (this host already has chromium + cached `ms-playwright` builds). **Headed by default — must pass `--headless`.** | Same engine, headless by default | `npx chrome-devtools-mcp`, needs **official Chrome or Chrome-for-Testing** (system chromium unsupported); `--headless`, headless viewport cap 3840×2160 | `npm i -g agent-browser` + `agent-browser install` (fetches Chrome; `--with-deps` for Linux libs) | Zero new runtime if Chrome exists; Go dep only | Needs LLM key + cloud (Stagehand) / ad-hoc (forks) |
| Security | `--allowed-hosts`, `--blocked-origins`, fs locked to workspace roots (`--allow-unrestricted-file-access` off by default). README warns allow/block lists are **not a security boundary** | Same engine flags via config file | `blockedUrlPattern`, `--file-navigations` (new: reject `file:` URLs); remote-debug port warning documented; stats + update checks default-on (disable via `--no-usage-statistics`, `CHROME_DEVTOOLS_MCP_NO_UPDATE_CHECKS`) | Per-tool `allowedDomains` + `--allowed-domains` with launch-mode containment | Localhost-only guard is ~10 lines of Go (cf. `read` SSRF guard in `el/tools/AGENTS.md`) | Cloud exfil surface / unmaintained |
| Maintenance | MS org, versioned with Playwright, README updated for the CLI split | New repo, direction MS endorses for coding agents | Hottest: v1.11.0 dated **yesterday** | Vercel Labs, rapid releases, experimental prefix | Both libs active (see push dates above) | Active-but-cloud / archived |

## Per-Backend integration (what must change)

Common prerequisite (all backends): a `designer` role in `el/roles.go` +
`el/toolresolve.go` (base tools + browser tools, no Daemon management tools),
a chain entry in `emberloop-web/chains.go`, and skill text in
`emberloop-web/runs_inject.go` (snapshot-first discipline, localhost-only,
viewport list). The `.emberloop/preview` script stays: it prints the
worktree dev-server URL the browser tool navigates to.

- **claude via ACP.** `ClaudeBackend.mcpServersFor(sessionID, role)`
  (`emberloop-web/claude.go:1041`) returns the single `el` server passed as
  `mcpServers` in `session/new`. Change: append a second `acpMCPServer`
  (`command: npx`, `args: [@playwright/mcp, --headless, --isolated or
  --user-data-dir <per-session dir>, --output-dir <session dir>,
  --viewport-size …, --image-responses allow, --allowed-hosts localhost …]`)
  only when `role == "designer"`. ~20 lines + wire-test. Images: el-mcp
  already emits MCP `image` blocks (`el/mcp/server.go`); whether the claude
  ACP adapter forwards tool-result images to the model is **unverified —
  live probe required** (caution: upstream reports of MCP image blocks being
  dropped exist, e.g. `anthropics/claude-code#54737`).
- **pig via ACP.** `pi-acp` ignores `session/new` `mcpServers`, so the el
  server travels via the per-Session `mcp.json` written by
  `PigACPBackend.mergedMCP` (`emberloop-web/pigacp.go:581`, `exposure:
  direct`, 600 s timeout, `EL_READY_FILE` gating). Change: add the browser
  server entry to that map for the designer role, same per-Session args.
  Do not merge the user's own `mcp.json` (herdr tools — existing rule).
  Images: pig SDK consumes `ai.ImageContent` tool results (`pigtools.go`,
  `pigtools_image_test.go`), but pig Sessions run through pi-acp, so the same
  forwarding probe is needed.

## Cited claims (primary sources)

- Snapshot/screenshot semantics, flags, isolation, security caveats:
  `microsoft/playwright-mcp` README (raw GitHub, read 2026-10-16).
- CLI-vs-MCP token claim + sessions/headless/console: `microsoft/playwright-cli`
  README (raw GitHub, read 2026-10-16).
- Tool reference (take_snapshot/take_screenshot/console/network/emulate),
  `--isolated`/pageId/`--browser-url`, stats + update-check opt-outs:
  `ChromeDevTools/chrome-devtools-mcp` README, `docs/tool-reference.md`,
  `docs/configuration.md`, `docs/advanced-usage.md` (raw GitHub).
- Sessions, `allowedDomains`, tool profiles, snapshot filters/`--delta`,
  annotated screenshots, install: `vercel-labs/agent-browser` README.
- Visual-token formula, tier caps, downsample guidance:
  `platform.claude.com/docs/en/build-with-claude/vision`.
- Versions/install dates: `npm view` 2026-10-16 (`@playwright/mcp` 0.0.83 /
  2026-09-28; `chrome-devtools-mcp` 1.11.0 / 2026-10-15; `agent-browser`
  0.39.1 / 2026-10-13; all Apache-2.0). CHANGELOG head confirms
  chrome-devtools-mcp v1.11.0 (2026-10-15) incl. `--file-navigations`.
- chromedp/rod MIT + push dates: GitHub API 2026-10-16. Stagehand MIT +
  SDK shape: repo README. Puppeteer MCP archived:
  `modelcontextprotocol/servers-archived`.
- Backend wiring: `emberloop-web/claude.go:1041` (`mcpServersFor`),
  `pigacp.go:21-22,581-603` (`mergedMCP`), `el/mcp/server.go` (image blocks),
  `el/tools/image.go` (`Multimodal`), `pigtools.go` (`ai.ImageContent`),
  `el/toolresolve.go` (per-role server placement), `el/tools/settings.go` (`mcp_servers`).
- Local numbers: reproduced on this host (chromium headless, trivial page,
  `playwright-core` `ariaSnapshot()`; method above). Real-app pages will show
  larger snapshots — rerun against a worktree dev server before sizing caps.

## Open questions

1. Do tool-result image blocks reach the model on claude-via-ACP and
   pig-via-ACP? Needs one live probe per backend (screenshot tool → vision
   model describes it). If either drops images, that backend's Designer is
   snapshot-only until the adapter is fixed.
2. Exact npm package/name for Microsoft's Playwright CLI (verify before
   pinning) and whether `el deps` (`el/deps.go`) should own the browser
   install.
3. Orphan-Chrome reaping under parallel Runs (idle-timeout suffices, or
   Daemon-side kill on Session end?) and per-Session `--output-dir` hygiene
   (wire into `disk.go` janitor).
4. Snapshot cap numbers against the real Emberloop UI (rerun §Measured
   against a worktree dev server; pick `depth`, `browser_snapshot` vs
   `--delta`, JPEG quality from data).
5. `chrome-devtools-mcp` needs official Chrome: acceptable extra install, or
   does `--executable-path`-style reuse (Playwright) decide it?
