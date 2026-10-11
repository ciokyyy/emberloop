# 13: Capped session no nudges

**What to build:** A Session classified as limit-capped is excluded from the submit gate reminder loop and shows a capped state instead.

**Blocked by:** failure-classifier-and-chain-settings (05)

**Status:** ready-for-agent

- [ ] No "you have not submitted" nudge to a capped Session
- [ ] Capped state visible on the Session
- [ ] Run seam test with a scripted session-limit failure; `.emberloop/check` passes
