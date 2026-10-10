# 04: Retry policy and block state

**What to build:** Transient errors retry automatically. A credit or auth failure stops the Session with a banner and a Retry button. The old handle stays untouched, so a block loses nothing.

**Blocked by:** 02

**Status:** ready-for-agent

- [ ] Network, 5xx and rate-limit errors retry 3 times with backoff of 2s, 8s and 30s, for the summarizer and for starting the new handle
- [ ] Credit and auth errors (402, 401, quota) do not retry automatically and block the Session
- [ ] Other errors fall through the fallback chain
- [ ] A blocked Session shows the banner "compress needs pig credits" with Retry and Change compress model
- [ ] A `compress blocked` Event is appended and carries the same Retry action
- [ ] Retry reruns the whole compress from the start
- [ ] A new prompt does not run while blocked, until compress succeeds or the user dismisses the banner
- [ ] The old harness handle and resume token are unchanged after a block
- [ ] Retry counts appear as fields on the `compressed` Event
- [ ] Tests cover transient-then-success, credit block, Retry success and dismiss
