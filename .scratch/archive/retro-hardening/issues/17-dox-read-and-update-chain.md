# 17: Agents read the DOX chain and keep it current; reviewers enforce it

**What to build:** Workers read the root-to-target chain before editing and update the closest owning doc after a meaningful change, per the framework. Both built-in reviewer types are told to flag a change that alters a documented contract or structure without updating the owning doc, and to flag a stale index. The implementer is not asked to police this; review is where it is enforced.

**Blocked by:** 16 The repo has its DOX tree

**Status:** done

- [x] Worker brief and reviewer types carry the read-before-edit and update-after rules (brief and type tests)
- [x] A reviewer fixture where a contract changed and no doc did produces the flag
- [x] Spec stories 18, 19, 20

Spec: `.scratch/archive/retro-hardening/spec.md`. Read it for the decisions this ticket implements, and its Hard limits section, which binds you.
