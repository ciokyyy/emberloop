# 05: Steer and Follow-up

**What to build:** While a Session works, the user can send a Steer (delivered at the next tool boundary) or a Follow-up (delivered after the turn), see the queued messages, and clear them. Pig uses its native queues. Claude uses the Daemon's own queue and degrades a Steer to a Follow-up, visibly, if mid-turn injection is not possible per the spike. There is no interrupt.

**Blocked by:** 03, 04

**Status:** done

- [x] The composer shows which mode a message will use and offers Steer and Follow-up while a Session works
- [x] A Steer reaches a pig Session at its next tool boundary
- [x] A Follow-up runs after the current turn on both harnesses
- [x] Queued messages are listed and can be cleared before delivery
- [x] A degraded Steer on claude is labelled as a Follow-up in the transcript
- [x] Seam tests with the fake Backend cover both modes and clearing
