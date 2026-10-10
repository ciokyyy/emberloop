# Web overhaul, round 2: what is left

Done this round (see `emberloop-web/AGENTS.md` Local Contracts): inline images stored as Session images with real thumbnails, claude reasoning effort picker, glide-to-bottom transcript scroll, persistent voice Cleanup toggle, colored tool output (`lib/highlight.js`), "Writing tool call…" placeholder, dual rail (`Rail.svelte`, `RunsSidebar.svelte`), Runs graph pan/zoom on `@panzoom/panzoom`.

## Open items

1. **Pig reasoning effort.** The picker shows for claude only. Find how the pi-acp adapter or `pig` takes a thinking level, add it to `pigacp.go` and `pig.go`, show the picker for pig, then update the AGENTS note. Verify claude really honors the SDK `effort` option (live probe under `scripts/`); it is unconfirmed.
2. **Steer card snap.** The pending card (`pending-<key>`) swaps to the real prompt Event card (`<seq>` key), which remounts and replays `animate-rise`. Give both the same key or skip the entrance animation on the swap. Also check the tool strip height growth against the new glide scroll in a real browser.
3. **Runs rail, spec list.** The spec list and "Find specs" form still live inside `RunsView.svelte`. Move spec discovery (repo directory, spec list, preview pick) into the second sidebar per `.scratch/mockup/layout.html`, keep the Run table/graph in the main area.
4. **runs-actions** (from `web-overhaul.md` item 4): needs-you banner triggers (same refusal x3, 10 min idle, 3 stall nudges), per-ticket Steer, Retry, Park, Discard routes and buttons, Run list picker, and a live probe under `scripts/` that starts a 2-ticket Run through `/api/runs`.
5. **project-init-and-validate** (`web-overhaul.md` item 3): user must say "go" first.
6. **check-green** (`web-overhaul.md` item 5): fix known failures (`TestExecuteDescriptionExamplesFollowTheToolList`, `TestImplementDriverRunStartsWorkersThroughTheLiveLoop`, `TestDoxTreeIndexIsComplete`, a push API test, flaky combobox windowing tests). `TestPigSteerWithImagesQueuesNatively` and `TestPigSteer…` style TempDir cleanup flakes ("directory not empty") are the same known race: background image or log writes outlive the test. Fix by waiting for the Session to settle or closing the Backend before test end. Run `.emberloop/check` once end to end.
7. **Tool output colors, polish.** Optional: swap the regex tokenizer for Shiki if language accuracy matters; strip ANSI in saved output; add per-tool header colors.
8. **Docs.** `web/README.md` "Design rules" should name the rail and the `tk-*` token classes. Run `npm run test:e2e` once after the rail change.
9. **Prompt image delivery.** The agent now gets file paths only (no native vision bytes). Confirm pig and claude both read the files; if vision quality drops, pass bytes inline as well while keeping stored ids for display.
