# 11: Handoff compress on pig SDK and pig-over-ACP

**What to build:** Compress by Handoff restart on the pig SDK Backend, replacing the in-place compact, and on pig-over-ACP. Both report Sessions as compressible.

**Blocked by:** 06, 08

**Status:** done

- [x] A pig SDK Session compresses by Handoff restart; the in-place compact path is gone
- [x] A pig-over-ACP Session compresses by Handoff restart and reports compressible
- [x] Thinking level and model survive the new handle
- [x] The resume token rotates and a Daemon restart then resumes the new handle
- [x] Tests at the Daemon seam cover both Backends against their existing test doubles
- [x] Owning `AGENTS.md` files and root index updated; repo check passes
