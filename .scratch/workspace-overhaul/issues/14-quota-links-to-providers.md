# 14: Quota cards link to Providers and refresh after connect

**What to build:** A "not connected" Quota card links to its Providers row, so the user fixes it in one click, and a successful connect on any flow refreshes the matching Quota card at once (spec Parts 2 to 6, Providers).

**Blocked by:** 10-chatgpt-device-code-connect, 11-antigravity-paste-connect, 12-claude-pty-connect

**Status:** ready-for-agent

- [ ] Every not-connected Quota card links to its Providers row
- [ ] A successful connect refreshes the matching Quota card
- [ ] Vitest covers the link and the refresh; repo check and `npm test` pass
