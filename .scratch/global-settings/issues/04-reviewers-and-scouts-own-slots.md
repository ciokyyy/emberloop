# 04: Reviewers and scouts resolve their own slots

**What to build:** The code reviewer uses the `code-review` slot, the spec reviewer the `spec-review` slot, scouts the `scout` slot, through the resolver. Reviewers stop copying the worker's model. A non-pig reviewer keeps its model and effort (the rule that dropped it goes). `EMBERLOOP_READONLY_HARNESS` still works, treated like a flag.

**Blocked by:** 03

**Status:** done

- [x] Code and spec reviewers launch with their own slot's harness, model and effort (fake-herdr review tests)
- [x] Scouts launch with the scout slot unless the agent type sets its own
- [x] A claude reviewer receives the configured model
- [x] `EMBERLOOP_READONLY_HARNESS` overrides the slot harness
- [x] Reviewer records store harness, model and effort
