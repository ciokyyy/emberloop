# 11: In-place event store and tail-only regroup

**Type:** code

**What to build:** Make the repro green. Update the event store in place and flush on a frame boundary instead of copying the whole array. Merge the tail on every event kind so a non-chunk event no longer forces a full regroup. Settled cards keep identity. Scroll stability and the stick-to-bottom behavior do not change.

**Blocked by:** 10-stream-lag-repro

**Status:** ready-for-agent

- [ ] The repro passes within the budget
- [ ] Existing transcript, scroll and grouping tests pass unchanged
- [ ] A live check with `agent-browser` under CPU throttle shows no dropped-frame stall at the repro rate
- [ ] `.emberloop/check` passes

Spec: `../spec.md`.
