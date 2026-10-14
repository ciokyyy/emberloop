# 29: Project gist

**What to build:** Each Project has a one-line gist derived from its `AGENTS.md` or README, cached and stored with the Project. An edited gist wins and is never overwritten by the derivation. The draft project pick reads it (spec Parts 2 to 6, Project scope and Draft project pick).

**Blocked by:** 03-suite-time-target-and-check-timeout (Part 7 goes first)

**Status:** done

- [x] The gist is derived from `AGENTS.md`, else README, and cached with the Project
- [x] An API sets the gist; an edited gist survives re-derivation
- [x] Go tests cover derivation from each source, the cache and edit precedence; repo check passes
