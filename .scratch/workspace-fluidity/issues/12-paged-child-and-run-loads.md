# 12: Paged child and Run history loads

**Type:** code

**What to build:** Group cards and Run pages load a child Session's tail with the same paging as the main view instead of the whole log, and load earlier on demand. Opening a Session shows its last turn within 300 ms from the paged tail. Check every other call that reads a full log and page it or justify it in the result.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] A child Session with a long log transfers one page on open
- [ ] Load earlier works in the Group card and the Run page
- [ ] No call without paging remains unless justified
- [ ] `.emberloop/check` passes

Spec: `../spec.md`.
