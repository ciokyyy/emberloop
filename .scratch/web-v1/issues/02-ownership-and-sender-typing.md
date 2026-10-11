# 02: Ownership and sender typing

**What to build:** Record `owner` on Sessions and Runs, inherit it for Subagents, Workers and Reviewers, hide ownerless data, and type every prompt Event with `sender` (`user` or `system`) and `user` ID. Daemon-made prompts (grouped results, sudo results) are `sender=system`. No-Project Sessions open in the User home dir.

**Blocked by:** 01

**Status:** done

- [x] Create records the cookie User as `owner`; Subagents, Workers and Reviewers inherit it
- [x] Ownerless Sessions and Runs are hidden from lists and 404 on GET
- [x] Prompt, steer and follow-up Events carry `sender` and, for users, `user`; Daemon-generated prompts carry `sender=system`
- [x] The Handoff prompt Event from a Compress carries `sender=system`, and `owner` is unchanged across the swap (assertions added to the existing Handoff tests from ticket 05)
- [x] Any User can prompt, steer, interrupt, cancel and end any Session
- [x] A create with no Project uses `<stateDir>/enderbloom/users/<name>/home`, created on first use
- [x] Go tests at the API seam cover owner, inheritance, hiding, sender typing and home dir
- [x] Owning `AGENTS.md` files and root index updated; repo check passes
