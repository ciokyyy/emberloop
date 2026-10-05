# 02: Finished tickets are kept in history

**What to build:** When a ticket is finished (merged, discarded, or kept after a refused merge), a history entry is written under its run before the worker entry is deleted: ticket, spec, outcome, merge commit, branch, harness, model, effort, start and end times, both reviewer verdicts with their findings summary, and the agent ids of the worker and its reviewers (so usage can be summed later).

**Blocked by:** 01

**Status:** ready-for-agent

- [ ] History entry written for merged, discarded and kept outcomes
- [ ] Reviewer verdicts captured before reviewers are forgotten
- [ ] Exported read function lists a run's history
- [ ] Test: spawn → review → finish through the existing fake herdr; history survives worker deletion
