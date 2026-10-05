# 08: finish marks the ticket done

**What to build:** After a successful merge, finish sets the ticket's status to done, ticks its checkboxes, and commits that on the target branch. A refused or conflicting merge leaves the ticket untouched. The worker entry gains the ticket path it was spawned from. The orchestrator brief stops asking the orchestrator to edit tickets; its allowed-paths ability stays for specs and notes.

**Blocked by:** 05 finish says exactly why it refuses (both change the finish path)

**Status:** ready-for-agent

- [ ] Merged worker: ticket status done, all boxes ticked, one commit on the target
- [ ] Refused or conflicting merge: ticket file unchanged
- [ ] The orchestrator brief no longer asks for the edit
- [ ] Spec stories 10, 11, 12, 13

Spec: `.scratch/retro-hardening/spec.md`. Read it for the decisions this ticket implements, and its Hard limits section, which binds you.
