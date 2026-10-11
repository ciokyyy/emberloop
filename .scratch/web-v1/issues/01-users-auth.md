# 01: Users and auth

**What to build:** User store, signup, login, logout, `me`, cookie sessions, first User admin, `signup_enabled` setting, `user set-password` CLI, and a 401 gate on every `/api` route and stream. See the spec, section Users and Sessions.

**Blocked by:** none

**Status:** done

- [x] First signup becomes admin; later signups are plain Users; users have stable IDs
- [x] Login sets an HttpOnly SameSite=Lax cookie; logout clears it
- [x] Every `/api` route and SSE stream is 401 without a cookie; the static shell stays public
- [x] Only the admin can write `signup_enabled`; off means signup is refused
- [x] `emberloop-web user set-password <name>` resets a password
- [x] Go tests at the API seam cover all of the above
- [x] Owning `AGENTS.md` files and root index updated; repo check passes
