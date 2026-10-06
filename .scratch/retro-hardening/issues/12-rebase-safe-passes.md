# 12: A review pass survives a clean rebase

**What to build:** A pass is also recorded against the identity of the worker's own change (its diff from its base, hashed so it does not depend on where the branch sits), beside the content snapshot. After a clean rebase onto other merged tickets the identity is the same and the pass holds; after a conflict, an edit or a different change it differs and the pass is stale. The gate accepts a pass when either the content or the patch identity matches. When only the base moved, a follow-up is the worker's own change against the new base, never other tickets' work.

**Blocked by:** 10 Reviewers are told when the diff covers less than the tree (both change the review gate and prompt)

**Status:** done

- [x] A pass survives a clean rebase onto an unrelated merged change and the gate accepts it (test with a real temp git repo)
- [x] A conflicting or content-changing rebase makes the pass stale
- [x] A follow-up after a rebase shows only the worker's own change
- [x] Spec stories 38 and 39

Spec: `.scratch/retro-hardening/spec.md`. Read it for the decisions this ticket implements, and its Hard limits section, which binds you.
