# 07: Shared session.Idle()

**What to build:** Define idle once in the Daemon: no running turn, no outstanding Group, no queued input, and a minimum quiet time. Auto-compress, the miner and the nightly job all call it.

**Blocked by:** 05

**Status:** done

- [x] `session.Idle()` and a Daemon-wide idle check exist with one definition, documented in `emberloop-web/AGENTS.md`
- [x] Tests drive it with a fake clock, running turn, Group and queue
- [x] No other idle check remains in the Daemon
- [x] Owning `AGENTS.md` files and root index updated; repo check passes
