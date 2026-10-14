# 17: Runs list returns summaries; Run detail loads on open

**What to build:** The Runs page opens without waiting on `git` for every Run. `GET /api/runs` returns summaries only (id, spec, status, counts, repo, Project id, updated time). Ticket bodies and acceptance text load from a per-Run endpoint when a Run opens. `App.svelte`, `RunsSidebar.svelte` and `RunsView.svelte` follow the new shape (spec Part 2, Speed).

**Blocked by:** 03-suite-time-target-and-check-timeout (Part 7 goes first)

**Status:** ready-for-agent

- [ ] `GET /api/runs`, Run-list render and cold boot are measured BEFORE any change; the numbers are recorded in this ticket
- [ ] `GET /api/runs` returns the summary shape; a per-Run endpoint returns Ticket bodies and acceptance text
- [ ] The Runs sidebar and Runs page render from summaries and load detail when a Run opens
- [ ] Go tests at the `/api/runs` seam cover the summary shape and the detail endpoint; a bounded-time test runs on a fixture with many Runs
- [ ] The new `GET /api/runs` time is recorded beside the baseline; repo check passes
