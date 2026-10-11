# 12: Tip short circuit and missing verdicts

**What to build:** A re-review is a new message in the same persistent Reviewer Session. If the tip equals the last verdict's tip, or `git patch-id` of the diff against base is unchanged, the driver reuses the verdict and sends nothing. A Reviewer turn that ended without a verdict at the current tip counts as no verdict and blocks merge.

**Blocked by:** reviewer-submit-tool (02)

**Status:** done

- [x] Same-tip and patch-identical rebase reuse the verdict with no Reviewer turn
- [x] Re-review reuses the existing Reviewer Session
- [x] Missing verdict blocks merge until one exists
- [x] Run seam tests; `.emberloop/check` passes
