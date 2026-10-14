# 01: Run test cleanup never holds the Daemon lock or a test slot

**What to build:** A panic inside a Run test or its cleanup can no longer hang the web suite. `stopRunsForTest` never leaves `d.mu` held, and a Run test slot is always freed, whatever else fails. The panic that hung the suite for 23 minutes is named and fixed at its source (spec Part 7). Re-read `emberloop-web/runs_driver_test.go` first: Run test helpers changed after the spec was written.

**Blocked by:** None (can start immediately)

**Status:** done

- [x] `stopRunsForTest` takes `d.mu` with `defer` or recovers a panic, reports the panic value through `t.Errorf` or `t.Logf`, and never fails a test by its own wait
- [x] The Run test slot is freed in its own `t.Cleanup`, registered first so it runs last, with no condition
- [x] The original panic is reproduced with `-run TestAbandonRunRemovesBranchesAndWorktrees|TestRebaseConflictNeedsYouWithFiles -count -race`, its cause is named in the commit message, and it is fixed at the source
- [x] A test that panics inside a Run test body (run as a subtest under a helper) frees its slot and leaves `d.mu` unlocked; the next test takes the slot
- [x] Repo check passes
