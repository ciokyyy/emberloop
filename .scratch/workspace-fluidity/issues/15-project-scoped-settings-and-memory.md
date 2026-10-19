# 15: Project-scoped Settings and Memory

**Type:** code

**What to build:** Settings inside a Project overrides model Chains and instructions for that Project only; unset falls back to global. Memory inside a Project shows that Project's notes and Rules first. Quota stays global. API and route work; follow the existing Project scope contract and Memory visibility rules.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] A Project Chain override applies to a new Session and Run in that Project and no other
- [ ] Clearing the override falls back to global
- [ ] Memory in a Project lists its notes and Rules first, and no private Memory of another User
- [ ] Quota routes stay global
- [ ] `.emberloop/check` passes

Spec: `../spec.md`.
