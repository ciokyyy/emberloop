# 15: Designer Run live probe and closeout

**What to build:** A live probe runs a small spec with a `code` Ticket and a blocked `design` Ticket in a throwaway repo with real models and a trivial web page, and checks the whole path: the Run preview shows both types; the `code` Ticket uses `worker` and two Reviewers; the `design` Ticket uses `worker-design`, gets the injected Designer skill, runs the repo's `.emberloop/preview` on its `PORT`, takes a snapshot and a phone screenshot, merges after three Passes including the design Reviewer, and leaves no browser or preview process behind. Then close out the spec: ADR 0005 moves to `status: accepted`, `GLOSSARY.md` and every AGENTS.md on the route are current, the area `AGENTS.md` index lists `spec.md` and `issues/`, and the research file's superseded recommendation is marked. Follow `scripts/AGENTS.md` and `scripts/run-upgrade-two-specs*` for probe shape.

**Blocked by:** 01-remove-grok, 05-browser-live-probe, 07-type-badge-and-flip-ui, 11-browser-skills-and-mentions, 12-preview-port-and-cleanup, 13-design-chain-warnings, 14-to-tickets-override-library

**Status:** ready-for-agent

- [ ] `scripts/designer-run-probe.sh` runs against a real Daemon behind an `EL_LIVE_*` gate and reports pass or the first failing step
- [ ] The probe asserts routing, Reviewer set, Designer skill injection, screenshot taken, three Passes, merge, and no leftover browser or preview process
- [ ] ADR 0005 is `accepted`; the research file's recommendation line says it was superseded by the ADR
- [ ] `.scratch/designer-agents/AGENTS.md` lists `spec.md` and `issues/`, `scripts/AGENTS.md` lists both probes, and the root index is current
- [ ] No stale "grok" or "Playwright MCP plan" text remains in current docs; `.emberloop/check` passes
