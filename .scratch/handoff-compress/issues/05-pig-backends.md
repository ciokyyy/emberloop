# 05: Handoff compress on pig SDK and pig-over-ACP

**What to build:** Compress by Handoff restart on the pig SDK Backend, replacing the in-place compact, and on pig-over-ACP, which could not compress before. Both report Sessions as compressible.

**Blocked by:** 02

**Status:** ready-for-agent

- [ ] A pig SDK Session compresses by Handoff restart; the in-place compact path is gone
- [ ] A pig-over-ACP Session compresses by Handoff restart and reports compressible
- [ ] Thinking level and model survive the new handle
- [ ] The resume token rotates and a Daemon restart then resumes the new handle
- [ ] Tests at the Daemon seam cover both Backends against their existing test doubles
