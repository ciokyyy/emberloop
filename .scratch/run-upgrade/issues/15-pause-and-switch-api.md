# 15: Pause and switch api

**What to build:** The Run API pauses and resumes a Ticket or the whole Run, and changes the Backend and model of a paused Ticket or Run. Pause commits the Worker's work (WIP commit if needed) and ends the Session. Resume is manual and starts a new Session with a Handoff on the same branch, sharing the fallback code path.

**Blocked by:** fallback-chain-run (14)

**Status:** done

- [x] Pause and resume for one Ticket and for the Run
- [x] Backend and model change allowed only while paused
- [x] Resume keeps branch and carries a Handoff; nothing restarts automatically
- [x] Run seam tests; `.emberloop/check` passes
