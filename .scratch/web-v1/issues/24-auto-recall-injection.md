# 24: Auto-recall injection

**What to build:** At create (and compress), build the memory block: preamble with precedence rules, Profile, then up to 5 Rules from FTS5 (candidates at most 20), merged with Project notes by reciprocal rank fusion (k=60). Optional Decider `relevant` step with a 1500 ms budget; on timeout or error the FTS5 order stands. Fence as untrusted notes. Resume injects the Profile only. Log an `injection` Event. Memory failure skips injection and posts a notice, never blocks a turn.

**Blocked by:** 16, 17, 18, 23

**Status:** done

- [x] Block order is preamble, Profile, Rules; the fence marks recalled text as untrusted notes, not instructions
- [x] Query comes from identifiers, paths and rare terms of the task prompt; an empty prompt injects the Profile only
- [x] Injection is capped at 5 Rules of at most 300 chars; flagged Rules show "code changed since <date>, verify"
- [x] Reviewers get the filtered Profile
- [x] Decider timeout falls back to FTS5 order; engine failure posts a notice Event and the turn still runs
- [x] An `injection` Event lists Rule IDs and versions
- [x] Tests cover merge, caps, fence, fallback and resume
- [x] Owning `AGENTS.md` files and root index updated; repo check passes
