# 09: edit shows what it changed

**What to build:** A worker that edits a file sees the changed lines with a few lines of context on each side, and is told when a patch deleted far more lines than it added, so a wrong range is visible at once without re-reading the file. The patch format and tag semantics do not change.

**Blocked by:** None (can start immediately)

**Status:** done

- [x] The edit reply includes the changed region with context
- [x] A patch that deletes far more than it adds is flagged in the reply
- [x] Existing edit behaviour, tags and tests are unchanged
- [x] Spec stories 14 and 15

Spec: `.scratch/archive/retro-hardening/spec.md`. Read it for the decisions this ticket implements, and its Hard limits section, which binds you.
