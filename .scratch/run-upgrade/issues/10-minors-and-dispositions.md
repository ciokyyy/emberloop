# 10: Minors and dispositions

**What to build:** The Worker receives every finding of a recorded verdict, minors included, marked blocking or not, and answers each with a Disposition (`fixed`, `declined` with a reason, `deferred` with a Ticket). A `done` submit missing a Disposition for a blocker or major is refused.

**Blocked by:** reviewer-submit-tool (02)

**Status:** ready-for-agent

- [ ] Worker prompt lists all findings with blocking marks
- [ ] Worker `submit` carries Dispositions; missing blocker or major Disposition is refused
- [ ] Dispositions recorded on the Ticket and shown in the Run view
- [ ] Run seam tests; `.emberloop/check` passes
