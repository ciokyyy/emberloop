# 03: Web suite time target and an explicit check timeout

**What to build:** The healthy web suite has a measured wall time and a target, and the repo check can never run without a bound. Record the full-suite wall time and the ten slowest tests, set a target (for example under 10 minutes with `EL_TEST_RUN_PAR` at its default), and fix or split the tests that dominate it (spec Part 7).

**Blocked by:** 01-run-test-cleanup-never-holds-lock, 02-run-test-slot-watchdog-and-deadline

**Status:** ready-for-agent

- [ ] The full-suite wall time and the ten slowest tests are recorded in this ticket before any speed change
- [ ] A target is set and recorded; the tests that dominate the time are fixed or split until the suite meets it, and the new wall time is recorded
- [ ] `.emberloop/check` runs the web suite with an explicit `-timeout`
- [ ] Repo check passes
