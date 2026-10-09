# 08: Running the driver again after a crash carries on

**What to build:** Starting `el implement <spec> --driver` again finds merged tickets, running workers and parked tickets from the tickets, the worker records and git alone, and carries on. It never starts a second worker for a ticket that already has one. The driver holds no state file of its own.

**Blocked by:** 03 Driver happy path

**Status:** done

- [x] After the driver process is killed mid-run, a new start finds each ticket's state and finishes the run
- [x] A ticket that already has a running worker gets no second worker
- [x] Merged and parked tickets are recognised
- [x] No private state file is written by the driver
- [x] Spec stories 31, 32, 33

Spec: `.scratch/archive/deterministic-driver/spec.md`. Read it for the decisions this ticket implements and its Testing Decisions, which bind you: test through the one driver seam (the spec and an injected world with a fake clock), assert on calls and run state, never on internals.
