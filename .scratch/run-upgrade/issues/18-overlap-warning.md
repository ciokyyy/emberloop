# 18: Overlap warning

**What to build:** At each Ticket merge, the driver compares the spec branch's changed files with other live spec branches' and publishes a warning Event on both Runs when they intersect.

**Blocked by:** spec-branch-core (09)

**Status:** done

- [x] Warning Event on both Runs with the shared files
- [x] No warning for disjoint specs
- [x] Two-Run seam test; `.emberloop/check` passes
