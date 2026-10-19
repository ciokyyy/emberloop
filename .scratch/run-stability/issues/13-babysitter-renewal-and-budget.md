# 13: Babysitter renewal, budget and log

**What to build:** At about 100k tokens the Babysitter finishes its current turn, then renews by Handoff into a fresh Session. Renewal never cuts an investigation mid-turn. A Run-wide budget of about 1M tokens ends its activity with one timeline note. A per-Run Babysitter log of diagnoses and actions feeds the Handoff and the User's review.

**Blocked by:** 08-babysitter-wake-and-session

**Status:** ready-for-agent

- [ ] Crossing the token threshold renews the Session only at a turn boundary
- [ ] The renewed Session starts from the Babysitter log and the Run summary
- [ ] At the Run budget the Babysitter goes silent and says so once on the timeline
- [ ] The log records each wake, diagnosis, action, drop and renewal with times
- [ ] `.emberloop/check` passes
