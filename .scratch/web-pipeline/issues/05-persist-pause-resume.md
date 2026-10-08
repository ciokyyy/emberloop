# 05: Persist Runs, pause on restart, Resume

**What to build:** Each Run is saved as a file with one row per Ticket (state, Worker Session, verdicts at tip, last refusal, times), beside the existing group files, not in the core state database. On Daemon start, saved unfinished Runs load as paused and nothing spawns. The user clicks Resume, and the driver continues without starting a second Worker for a Ticket that already has one. The user can also pause an active Run. A finished Run stays listed with its final Ticket states.

**Blocked by:** 03

**Status:** done

- [x] Run state is written after every driver pass and reloaded on Daemon start
- [x] Reloaded unfinished Runs are paused and spawn nothing
- [x] Resume continues the Run; no Ticket with a Worker gets a second Worker
- [x] Pause stops new starts and leaves running Workers alone
- [x] Finished Runs remain listed and readable
- [x] API tests restart the Daemon mid-Run and resume; a view test covers the paused state and Resume button
