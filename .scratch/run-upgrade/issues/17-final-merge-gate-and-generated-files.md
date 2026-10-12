# 17: Final merge gate and generated files

**What to build:** When every Ticket is merged or discarded, the driver rebases the spec branch if master moved, builds `web/dist` and lockfiles on it, runs the repo check on the merge result, then merges into master. A failing check or conflict makes the Run Needs-you with files listed, and master does not move.

**Blocked by:** spec-branch-core (09)

**Status:** done

- [x] Master moves only after a green check on the merge result
- [x] Conflict and red check keep master and show Needs-you with files
- [x] Generated files built only here, never per Ticket
- [x] Run seam tests; `.emberloop/check` passes
