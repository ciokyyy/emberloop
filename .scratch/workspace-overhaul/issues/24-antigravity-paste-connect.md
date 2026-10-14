# 24: Antigravity connect by pasted callback

**What to build:** An admin connects Antigravity even though the localhost callback page fails. The Daemon runs the existing prompt/callback flow in `pig-antigravity-auth/oauth.go`; the UI takes the pasted callback URL or code (spec Parts 2 to 6, Providers).

**Blocked by:** 22-providers-page-and-api-keys

**Status:** ready-for-agent

- [ ] Connect on the Antigravity row shows the login link and a field for the callback URL or code
- [ ] A pasted URL or bare code completes the login; a bad value shows an error and keeps the panel open
- [ ] Go tests with a local stub cover URL, bare code and a bad value; vitest covers the panel
- [ ] Repo check and `npm test` pass
