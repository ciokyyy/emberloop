# 02: Run test slot watchdog and per-test deadline

**What to build:** A stuck Run test names itself. When a test waits more than 60 seconds for a Run test slot, the suite logs which tests hold the slots. Each Run test has its own deadline, so one stuck test fails with its own name and goroutine dump instead of the whole package timing out (spec Part 7).

**Blocked by:** 01-run-test-cleanup-never-holds-lock (both change the Run test slot helpers)

**Status:** ready-for-agent

- [ ] A wait of more than 60 seconds for a Run test slot logs the names of the tests that hold slots
- [ ] A unit test drives the slot watchdog with a fake clock
- [ ] Run tests get a deadline derived from `t.Deadline()` or a context; a test past it fails with its own name and a goroutine dump
- [ ] Repo check passes
