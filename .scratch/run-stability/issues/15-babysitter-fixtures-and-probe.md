# 15: Babysitter fixture corpus and live probe

**What to build:** A fixture corpus of recorded digests and git states of real stranded cases: a long Reviewer loop, a generated-bundle merge conflict, a dirty-master refusal and a lost finished submit. Each fixture, run against a fake model seam, asserts the expected action. One live probe in `scripts/` runs a real cheap model against a recorded Run and stays out of the repo check.

**Blocked by:** 11-babysitter-steer-and-escalate, 12-babysitter-hygiene-and-retry

**Status:** ready-for-agent

- [ ] Four fixtures, each asserting the expected named action
- [ ] The fixtures need no network and no real model
- [ ] The live probe runs one real model against one fixture and prints the diagnosis and action
- [ ] The probe is documented in `scripts/AGENTS.md` and excluded from `.emberloop/check`
- [ ] `.emberloop/check` passes
