# 27: Quota cards link to Providers and refresh after connect

**What to build:** A "not connected" Quota card links to its Providers row, so the user fixes it in one click, and a successful connect on any flow refreshes the matching Quota card at once (spec Parts 2 to 6, Providers).

**Blocked by:** 23-chatgpt-device-code-connect, 24-antigravity-paste-connect, 25-claude-pty-connect

**Status:** done

- [x] Every not-connected Quota card links to its Providers row
- [x] A successful connect refreshes the matching Quota card
- [x] Vitest covers the link and the refresh; repo check and `npm test` pass
