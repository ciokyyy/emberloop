# 08: ChatGPT Plus quota card

**What to build:** The Quota page shows a ChatGPT Plus card with the 5-hour and weekly windows, read from the existing pig login so no new credential is added. A missing or expired login reads as not connected or expired, never as an error, so the user knows to sign in again.

**Blocked by:** 06-percent-contract

**Status:** done

- [x] Reads the `openai-codex` login from pig's auth store read-only (never refreshes it) and calls the usage endpoint with the account id header
- [x] Maps the primary window to 5-hour and the secondary window to Weekly (`used_percent`, `reset_at`); reproduces the observed shape (5-hour 4%, Weekly 4%, no extra credits)
- [x] Missing login yields `not_configured`; expired login yields `expired`
- [x] Go tests at the HTTP seam with a local stub and temp state dir; repo check passes
