# 10: Retry policy and block state

**What to build:** Transient errors retry automatically. A credit or auth failure stops the Session with a banner and a Retry button. The old handle stays untouched.

**Blocked by:** 08

**Status:** done

- [x] Network, 5xx and rate-limit errors retry 3 times with 2s, 8s and 30s backoff, for the summarizer and for starting the new handle
- [x] Credit and auth errors (402, 401, quota) do not retry and block the Session
- [x] Other errors fall through the fallback chain
- [x] A blocked Session shows the banner "compress needs pig credits" with Retry and Change compress model
- [x] A `compress blocked` Event is appended and carries the same Retry action
- [x] Retry reruns the whole compress; a new prompt does not run while blocked until compress succeeds or the banner is dismissed
- [x] The old harness handle and resume token are unchanged after a block
- [x] Retry counts appear as fields on the `compressed` Event
- [x] Tests cover transient-then-success, credit block, Retry success and dismiss
- [x] Owning `AGENTS.md` files and root index updated; repo check passes
