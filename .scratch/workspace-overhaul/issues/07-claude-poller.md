# 07: Claude read-only host-login poller

**What to build:** The Daemon reads Claude quota from the host login with no prompt and no model call, so the browser shows it without any terminal pane running — and checking it never costs usage. The poller can never log the user out of Claude Code: it never refreshes, rewrites or deletes the credentials file, never starts the harness, and never sends a prompt. The push-based statusline file stays as the fallback when the host login is absent, and the terminal header strip works unchanged.

**Blocked by:** 06-percent-contract

**Status:** ready-for-agent

- [ ] Polls the OAuth usage endpoint with the host login's access token, the required `User-Agent` (version read from the installed binary with a fallback constant) and the OAuth beta header; never a model call
- [ ] Maps the 5-hour window to 5-hour and the 7-day window to Weekly (`utilization` is already percent used); plan name comes from the credentials subscription type; reproduces the observed shape (5-hour 36%, Weekly 8%, Pro plan)
- [ ] Cache TTL 60-300s; a rate-limit reply backs off for its `Retry-After` duration
- [ ] A 401 or an expired token timestamp yields `expired` with last values kept; other failures yield `stale` with their age and never blank the card
- [ ] Test asserts the poller never writes the credentials file; HTTP-seam tests with local stubs and temp state dir; repo check passes
