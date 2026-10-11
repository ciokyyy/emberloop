# 02: Reviewer submit tool

**What to build:** Reviewers report through a `submit` tool with `verdict`, `tip`, `ran` and `findings`. The Daemon refuses a `pass` with empty `ran`, records a verdict whose `tip` is not the branch tip as stale, and accepts minors on a `pass` without blocking. `changes` needs a blocker or major. The text parser stays only as a fallback for a Backend without the tool.

**Blocked by:** None (can start immediately)

**Status:** done

- [x] Fake Backend can script a Reviewer `submit`
- [x] `pass` with empty `ran` is refused with a tool error
- [x] Stale-tip verdict is recorded and does not count
- [x] `changes` with only minors is refused
- [x] Reviewer prompts describe the tool and drop the text format
- [x] Run seam tests cover each case; `.emberloop/check` passes
