# 09: A worker's submit ticks its ticket checkboxes

**What to build:** A worker's `submit` lists the ticket checkboxes it completed and the driver ticks them in the ticket files in the integration checkout, as the only writer to ticket files during a run. `finish` keeps marking the ticket done on merge. The run is correct without this: a missing or wrong list never blocks a merge. This ticket settles the field's name and whether a checkbox is named by its line text or by index.

**Blocked by:** 03 Driver happy path

**Status:** done

- [x] `submit` accepts a list of completed checkboxes and a submit without one still works
- [x] The driver ticks exactly those boxes in the ticket file and leaves the edit uncommitted
- [x] A list that names a checkbox that is not in the ticket is reported and does not fail the merge
- [x] Two workers submitting at once never write the same ticket file at the same time
- [x] Spec stories 27, 28

Spec: `.scratch/deterministic-driver/spec.md`. Read it for the decisions this ticket implements and its Testing Decisions, which bind you: test through the one driver seam (the spec and an injected world with a fake clock), assert on calls and run state, never on internals.
