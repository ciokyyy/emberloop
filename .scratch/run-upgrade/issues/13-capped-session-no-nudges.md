# 13: Capped session no nudges

**What to build:** A Session classified as limit-capped is excluded from the submit gate reminder loop and shows a capped state instead.

**Blocked by:** failure-classifier-and-chain-settings (05)

**Status:** done

- [x] No "you have not submitted" nudge to a capped Session
- [x] Capped state visible on the Session
- [x] Run seam test with a scripted session-limit failure; `.emberloop/check` passes
