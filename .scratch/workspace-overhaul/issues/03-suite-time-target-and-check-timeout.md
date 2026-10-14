# 03: Web suite time target and an explicit check timeout

**What to build:** The healthy web suite has a measured wall time and a target, and the repo check can never run without a bound. Record the full-suite wall time and the ten slowest tests, set a target (for example under 10 minutes with `EL_TEST_RUN_PAR` at its default), and fix or split the tests that dominate it (spec Part 7).

**Blocked by:** 01-run-test-cleanup-never-holds-lock, 02-run-test-slot-watchdog-and-deadline

**Status:** ready-for-agent

## Baseline (before speed changes)

- Command: `cd emberloop-web && go test -json -count=1 -timeout 15m -skip TestLive ./...` (`EL_TEST_RUN_PAR` unset; default 2).
- Full-suite wall time: 154s (Go test package total: 132.792s).
- Ten slowest top-level tests: `TestKilledDaemonKeepsManualPause` 20.73s; `TestRunReviewerFallbackChainFailureClasses` 20.14s; `TestRunFallbackStartOtherFailureNeedsYouNotExhausted` 7.80s; `TestRunWorkerFallbackDoneReusesSameTipReviewerPasses` 6.76s; `TestPauseStopsNewStartsAndResumeContinues` 6.65s; `TestRunFallbackTaskResumeFailureNeedsYou` 6.01s; `TestRunNeedsYouReasonSameRefusal` 6.00s; `TestNeedsYouIdleAlertSurfaced` 5.87s; `TestRunResumePreservesStartedAndUnstartedTicketPauses` 5.53s; `TestStallNudgesEscalateToNeedsYou` 5.28s.

## Target and final measurement

- Target: under 10 minutes with the default `EL_TEST_RUN_PAR=2`.
- Final full-suite wall time: 94s (Go test package total: 90.685s), measured with the baseline command above. The 154s baseline already met the target, so no test speed changes were necessary to reach it.
- `.emberloop/check` runs the web suite with `-timeout 15m`; `sh .emberloop/check` passed (120s).

- [ ] The full-suite wall time and the ten slowest tests are recorded in this ticket before any speed change
- [ ] A target is set and recorded; the tests that dominate the time are fixed or split until the suite meets it, and the new wall time is recorded
- [ ] `.emberloop/check` runs the web suite with an explicit `-timeout`
- [ ] Repo check passes
