# 06: Ladder storage, API and Chain entries

**Type:** code

**What to build:** Ladders and schedules are stored in Daemon settings and edited through an API: list, create, update, delete, plus the active rung per Ladder. A Chain entry may name `ladder/<name>`; the model validation, the paused-Ticket Switch and the Run snapshot accept it. The seeded Ladder exists on first run and survives upgrades. Deleting a Ladder that a Chain uses is refused with the reason.

**Blocked by:** 01-ladder-probe-and-resolve

**Status:** ready-for-agent

- [ ] Create, edit and delete a Ladder through the API
- [ ] A Chain entry with `ladder/deepseek-flash` starts a Session and a Run
- [ ] Deleting a used Ladder returns the reason
- [ ] The seeded Ladder is restored if absent
- [ ] `.emberloop/check` passes

Spec: `../spec.md`.
