# 03: Reviewers, Passes and the finish gate

**What to build:** Each Ticket gets two persistent Reviewer Sessions, one for spec fit and one for repository standards, that keep context across Passes. When a Worker submits done, the Reviewers judge the tip. Findings go back to the same Worker, and the loop repeats until both Pass. The gate refuses the merge unless every required Reviewer has a Pass at the current tip; a stale Pass does not count. The Ticket row shows each verdict, whether it is at the current tip, and the last refusal reason. Reviewers use their settings roles with the same fallback as Workers.

**Blocked by:** 02

**Status:** ready-for-agent

- [ ] A submitted done spawns the spec Reviewer and the standards Reviewer for that Ticket, once
- [ ] Findings are delivered to the same Worker, and the Reviewers keep their context on the next Pass
- [ ] The merge is refused without both Passes at the current tip, and a new commit makes older Passes stale
- [ ] The Ticket row shows both verdicts, tip freshness and the last refusal
- [ ] A refused merge is told to the Worker unchanged and retried after the next submit
- [ ] Reviewer Sessions count against the live-Session cap
- [ ] API tests cover pass, fail, rework and stale Pass; a view test covers the verdict columns
