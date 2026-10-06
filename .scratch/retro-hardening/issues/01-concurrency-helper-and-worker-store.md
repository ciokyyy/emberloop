# 01: Worker store survives simultaneous sessions

**What to build:** Several sessions can spawn and finish workers at the same moment without losing an entry or reading a half-written store. This ticket also builds the reusable multi-process test helper the other concurrency tickets use: it starts N real helper processes against a fake herdr that records splits, closes and pane lists. Failing-first: the first test, written against the current code, loses an entry or reads a partial file; the fix (a cross-process lock and an atomic write for the store, using the lock helper wick already has) turns it green.

**Blocked by:** None (can start immediately)

**Status:** done

- [x] A helper runs several real processes that spawn and finish workers at once against a fake herdr, and the test fails on the unfixed code (record the failure in the ticket)
- [x] After the fix no entry is lost and no reader ever sees a partial store
- [x] The helper and fake herdr are reusable by the grid, reviewer and spacing tickets
- [x] Spec stories 34 and 37

Failing-first failure (unfixed code, `TestWorkerStoreSurvivesSimultaneousSessions`):
`a concurrent reader saw a partial worker store 4 times` — 6 helper processes
spawning at once against the file-backed fake herdr; readers looping `load()`
hit `worker store is corrupt` JSON errors from the non-atomic `os.WriteFile`.
The same race also loses entries (read-modify-write under an in-process mutex
only). Fix: `save()` via temp file + rename, and `Spawn`/`finishWorker` hold
`withFileLock("workers")` across the whole operation; `recall`/`workerAt` read
under the same lock. Helper: `wick-pig/concurrency_test.go`
(`runConcurrencyHelpers` + `TestConcurrencyHelperProcess` + file-backed fake
herdr with shared `herdr.json` and `calls.log` recording splits, closes and
pane lists).

Spec: `.scratch/retro-hardening/spec.md`. Read it for the decisions this ticket implements, and its Hard limits section, which binds you.
