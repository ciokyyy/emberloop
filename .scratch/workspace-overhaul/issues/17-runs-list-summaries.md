# 17: Runs list returns summaries; Run detail loads on open

**What to build:** The Runs page opens without waiting on `git` for every Run. `GET /api/runs` returns summaries only (id, spec, status, counts, repo, Project id, updated time). Ticket bodies and acceptance text load from a per-Run endpoint when a Run opens. `App.svelte`, `RunsSidebar.svelte` and `RunsView.svelte` follow the new shape (spec Part 2, Speed).

**Blocked by:** 03-suite-time-target-and-check-timeout (Part 7 goes first)

**Status:** ready-for-agent

- [x] `GET /api/runs`, Run-list render and cold boot are measured BEFORE any change; the numbers are recorded in this ticket
  Baseline (before implementation): `GET /api/runs` p50 2.53s (5 timed requests after 2 warmups) for a fixture of 40 Runs × 8 Tickets; Vitest/jsdom list render 226.4ms for 40 Runs; cold boot to the Runs sidebar shell 516.2ms and to all 40 rows 610.7ms. The UI measurements use the same 40-Run, 8-Ticket fixture.
  After summaries: `GET /api/runs` p50 548.8µs, p95 833.1µs (5 timed requests after 2 warmups) for the same 40-Run, 8-Ticket fixture; every request stayed under the 750ms test bound.
- [x] `GET /api/runs` returns the summary shape; a per-Run endpoint returns Ticket bodies and acceptance text
- [x] The Runs sidebar and Runs page render from summaries and load detail when a Run opens
- [x] Go tests at the `/api/runs` seam cover the summary shape and the detail endpoint; a bounded-time test runs on a fixture with many Runs
- [x] The new `GET /api/runs` time is recorded beside the baseline; repo check passes
