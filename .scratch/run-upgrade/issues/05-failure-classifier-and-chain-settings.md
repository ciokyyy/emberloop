# 05: Failure classifier and chain settings

**What to build:** Run settings hold a Chain of up to three `{backend, model}` entries per role, and one classifier labels a failure as quota, credit, auth, session-limit or other, shared with the Compress block code.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] Settings store, validate and return per-role Chains
- [ ] Classifier extracted from the Compress block path and used by both
- [ ] Unit and API tests cover each class and the settings round trip
- [ ] `.emberloop/check` passes
