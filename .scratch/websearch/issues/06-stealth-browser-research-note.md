# Stealth browser research (ticket 06): low-RAM browser for `read` tier 3

Question: which browser should tier 3 use for pages tiers 1 (native fetch)
and 2 (Jina) cannot read — bot-challenged, JS-only, or thin pages?
Constraints from the spec: lowest RAM and startup cost wins (tier 3 fires
rarely, one page at a time), Go must drive it, install must be small and
licence-clean, and a missing install keeps returning the clear error.

Candidates: (A) `chrome-headless-shell` + a Go CDP driver,
(B) full Chrome/Chromium + `go-rod/stealth` evasions,
(C) Camoufox, (D) Patchright. (E) Playwright/Puppeteer with
`puppeteer-extra-plugin-stealth` was dropped early: same patch-and-decay
family as B with a Node runtime attached and no Go driver.

## Comparison against every criterion

### Peak RAM

- A: headless shell is the old headless implementation split out as a
  standalone binary precisely because it skips `//chrome` UI, GPU and
  compositing paths (Chrome for Developers, headless docs). Third-party
  measurements: ~80 MB base for headless Chromium vs ~150 MB headed, 0.5–1 s
  startup (webscraping.ai FAQ); ~250 MB for one render, returning to
  baseline after (ankushshandilya.com measurement, July 2026).
- B: full Chrome runs 800 MB+ in automation builds (Camoufox homepage
  comparison figure, camoufox.com). Same engine as A plus the browser UI
  layer, extensions and GPU process.
- C: Camoufox advertises ~200 MB vs Chrome's 800 MB+ (camoufox.com
  homepage) — a debloated Firefox build, but still a full browser with a
  fingerprinting stack, not a shell.
- D: Patchright drives full Chromium/Chrome — same footprint as B.

Winner: A, by roughly 3–4× over full-browser options.

### Cold-start time

- A: the shell exists for bulk screenshotting/PDF/scraping and minimal
  images where startup dominates (thebrowserlayer.com; Remotion and Quarto
  both switched to it for this reason). ~0.5–1 s vs 2–3 s headed
  (webscraping.ai FAQ). A persistent daemon (below) amortises even that.
- B/D: full-browser launch, seconds; Playwright adds a ~50 MB Node bridge
  over stdio per language binding (pkg.go.dev `patchright-go` note).
- C: full Firefox-based launch plus fingerprint-profile generation per
  context.

Winner: A.

### Bot-detection pass rate

- C is the lab champion: 88.58% success in the Zenrows benchmark (avg
  bypass 42.49 s — slow), 0% headless/stealth flags on CreepJS
  (ScrapingBee), and one of only 4/15 tools passing a strict lab referee
  (Proxycove, Aug 2026).
- Best live-Cloudflare result belongs to the nodriver pattern — stock
  Chrome driven over raw CDP with no WebDriver layer: 28/31 OK, 0 blocked
  on 31 live targets (Paterson benchmark, Aug 2026; Scalebrowser summary).
  Playwright-based tools (incl. Patchright) scored 24–26. Lesson: *how the
  browser is driven* matters more than patches; Playwright's driving layer
  is itself fingerprinted, and every Playwright fork fails the
  driving-layer check (roundproxies.com).
- A/B with `go-rod/stealth` (puppeteer-extra-stealth equivalents,
  pkg.go.dev): passes JS challenges and Turnstile-class gates that raw
  HTTP cannot (ScrapingBee Go/Cloudflare guide), but headless Chrome is
  detectable without patches and JS-injection patching is exactly the
  family DataDome dissects and detects (DataDome, 2022). No published
  head-to-head rate for go-rod/stealth exists — treat as weaker than C on
  hardened targets, comparable to D, and note the decay: anti-bots update
  faster than volunteer evasion patches (Scrapfly, 2026).
- D inherits Playwright's driving-layer fingerprint; undetected- variants
  need constant updates.

Winner on raw bypass: C, then the nodriver pattern (stock Chrome over raw
CDP — reproducible from Go with B), then B/D, then bare A. But tier 3 sees
only the trickle tiers 1–2 fail on, single pages, so a slightly lower pass
rate at 1/4 the RAM is the right trade — and the recommended shape (below)
keeps the full-Chrome upgrade path open.

### How Go drives it (daemon or per-request, CDP or CLI)

- A/B: native Go, CDP, both shapes. chromedp launches per-request by
  default over `--remote-debugging-pipe` (no port) and attaches to a
  persistent daemon via `NewRemoteAllocator` + wsURL (chromedp docs,
  issues #438/#1400; `chromedp/headless-shell` Docker image exists for the
  daemon shape). rod's launcher auto-downloads a pinned browser, reuses one
  browser process across pages via a control URL, and `leakless` reaps it
  (pkg.go.dev `rod/lib/launcher`; go-rod custom-launch doc).
- C: driven via its Python/TypeScript launchers (MIT-licensed launchers,
  MPL-2.0 browser); no native Go driver — Go integration means a
  subprocess/CLI or a websocket shim, plus a Python runtime in the install.
- D: Python/Node first-class; the Go port (`patchright-go`) is an
  unofficial community bridge over a Node runtime.

Winner: A/B — pure-Go CDP, daemon or per-request, no sidecar runtime.

### Install size

- A: the smaller Chrome-for-Testing asset by design (Quarto: full Chromium
  bundle "significantly larger than what headless rendering needs";
  chromote docs). Exact MB varies per release/platform — read it off the
  CfT dashboard at install time.
- B/D: full Chrome/Chromium + driver + (for Patchright) Node bridge.
- C: `camoufox fetch` ~300 MB download; installed browser directory
  1.2 GB + 40 MB GeoIP DB (camoufox.com installation page; gotoolradar).

Winner: A, by an order of magnitude over C.

### Licence

- A/B engine: Chromium is BSD-3-Clause; note Chrome-branded CfT binaries
  carry Google's Chrome terms, so prefer the Chromium-flavoured shell
  (cf. `chromedp/headless-shell`, a modified Chromium build reporting a
  Chrome UA) for a clean BSD story.
- Go drivers: rod MIT, `go-rod/stealth` MIT, chromedp MIT (pkg.go.dev
  licence fields).
- C: browser + patches MPL-2.0, Python/TS launchers MIT, vendored Cursory
  trajectories LGPLv3-or-later (daijro/camoufox README) — heavier,
  copyleft-adjacent for vendoring.
- D: Apache-2.0 (libraries.io record for `patchright-fork`; Playwright
  itself is Apache-2.0).

Winner: A/B — MIT + BSD, no copyleft, no branded-binary terms.

## Recommendation

**Use `chrome-headless-shell` (Chromium flavour) driven from Go over CDP
as a persistent daemon, via `rod` + `go-rod/stealth`, with full Chrome as
the documented fallback.**

Why: it wins RAM, startup and install size outright, ties B on licence
and Go driving, and loses only raw bypass rate to Camoufox — acceptable
because tier 3 handles a trickle of single pages, and the daemon shape
reproduces the nodriver lesson (raw CDP, no WebDriver/Playwright layer),
which is the single biggest bypass factor on live Cloudflare targets.
Camoufox is the
stronger bypasser but costs 1.2 GB installed, a Python sidecar with no Go
driver, MPL/LGPL licensing, and 42 s average bypass times — wrong shape
for an occasional single-page fallback inside a Go tool.

## Integration shape (for ticket 07)

- wick-pig owns one daemon: launch `chrome-headless-shell
  --remote-debugging-port=<loopback>` at first tier-3 need (lazy), reuse
  across reads, kill on process exit; per-read opens a fresh CDP
  target/tab, navigates with the existing 20 s timeout, extracts rendered
  HTML, and feeds the existing tier-1 HTML→Markdown pipeline so shaping,
  300-line/overflow handling and headers stay identical except `tier 3`.
  NOTE: rod's launcher auto-downloads full Chromium, not the shell — ticket
  07 must point rod at the shell binary via `launcher.Bin`, or the RAM win
  is lost.
- Same safety rules as tiers 1–2: loopback/private/link-local block on
  every hop, ~10 MB cap, untrusted-content wrap, never send
  credential/token URLs anywhere new (no third party here at all —
  strictly better than Jina on secrecy).
- Fallback chain inside tier 3: shell first; on shell failure retry once
  with full Chrome/Chromium binary (covers the missing-`//chrome`
  features: extensions, rare UI flows), then the existing clear
  "not installed" error when no binary is present.
- Install: `wick-pig deps` (ticket 19) fetches the pinned shell from the
  Chrome-for-Testing JSON API into the wick state dir, prints a proof
  line, and the tier-3 stub keeps returning the clear error until then.
  Pin the revision (rod does the same by default) so behaviour is
  consistent across machines.

## Install steps (operator)

1. `wick-pig deps` (after ticket 19 lands) — installs + verifies the
   pinned `chrome-headless-shell` build. Manual equivalent: download the
   `chrome-headless-shell` asset for the platform from
   `googlechromelabs.github.io/chrome-for-testing` (JSON API) into the
   wick state dir.
2. `go get go-rod/rod go-rod/stealth` in `wick-pig` (both MIT).
3. No Python, Node, or 1 GB browser download. Optional: set the full-Chrome
   path for the tier-3 fallback; otherwise the shell alone suffices.

## Open risks

1. Evasion decay: `go-rod/stealth` is JS-injection patching, the family
   DataDome already detects; pass rates will rot as anti-bots update.
   Mitigation: raw-CDP driving (no WebDriver layer) is the durable part;
   re-run a live probe (Cloudflare Turnstile + DataDome target) in
   ticket 07 and on each pinned-revision bump.
2. Shell gaps: headless-shell drops `//chrome` features (extensions, some
   flows); a page needing them fails the shell and burns the fallback.
   Mitigation: the full-Chrome retry inside tier 3.
3. No CAPTCHA/Turnstile solving: interactive or proof-of-work gates that
   need a human still fail — tier 3 reports unreachable, same as today.
4. Daemon lifecycle: a leaked browser process repeats the known
   orphaned-`chrome-headless-shell` failure (10 GB RAM leak report,
   claude-code #50783). Mitigation: `leakless`-style reaping, kill on
   wick-mcp exit, and a `deps`/health check that refuses a stale daemon.
5. Fingerprint consistency: shell + stock evasions do not rotate
   fingerprints per request (unlike Camoufox/Browserforge profiles);
   high-volume use from one IP will still be rate-limited — out of scope,
   tier 3 is low-volume by construction.

## Sources (primary first)

- Chrome headless docs (old headless → `chrome-headless-shell` since
  Chrome 132): https://developer.chrome.com/docs/chromium/headless
- Chrome-for-Testing assets/JSON API:
  https://googlechromelabs.github.io/chrome-for-testing/
- `chromedp/headless-shell` image (Chromium build, same UA):
  https://github.com/chromedp/docker-headless-shell
- chromedp remote allocator / pipe default:
  https://github.com/chromedp/chromedp (+ issues #438, #1400)
- rod launcher auto-download + leakless:
  https://pkg.go.dev/github.com/go-rod/rod/lib/launcher
- rod / stealth licences (MIT):
  https://pkg.go.dev/github.com/go-rod/rod,
  https://pkg.go.dev/github.com/go-rod/stealth
- Camoufox stealth approach + licence + sizes:
  https://camoufox.com/stealth/, https://github.com/daijro/camoufox,
  https://camoufox.com/python/installation/
- Benchmarks: Paterson 31-target set (nodriver 28/31, 0 blocked),
  https://ianlpaterson.com/blog/anti-detect-browser-benchmark-patchright-nodriver-curl-cffi/;
  Zenrows (Camoufox 88.58%, 42.49 s),
  https://www.zenrows.com/blog/top-stealth-browser-tools;
  Scrapfly (patch-vs-build analysis),
  https://scrapfly.io/blog/posts/best-stealth-browsers;
  ScrapingBee Go/Cloudflare + CreepJS 0%,
  https://www.scrapingbee.com/blog/bypass-cloudflare-golang/,
  https://www.scrapingbee.com/blog/how-to-scrape-with-camoufox-to-bypass-antibot-technology/
- DataDome on puppeteer-extra-stealth detectability:
  https://datadome.co/bot-management-protection/detecting-headless-chrome-puppeteer-extra-plugin-stealth/
- RAM/startup figures:
  https://webscraping.ai/faq/headless-chromium/what-is-the-difference-between-headless-chromium-and-regular-chrome,
  https://webscraping.ai/faq/headless-chromium/how-can-i-make-headless-chromium-use-less-cpu-and-memory,
  https://ankushshandilya.com/headless-chrome-memory-cost-small-server/
