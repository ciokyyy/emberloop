# 09: Spec branch core

**What to build:** Each Run creates or reuses `spec/<slug>` from master. Tickets branch from it and merge back into it under today's gate. The branch name and base SHA persist on the Run row and survive restart. A spec with no other live Run looks the same to the User.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] Ticket branches fork from the spec branch and finish into it, not master
- [ ] Branch name and base SHA persist and resume after restart
- [ ] Fake git models two branch levels and a moving master
- [ ] Run seam tests cover create, reuse, merge-in and restart; `.emberloop/check` passes
