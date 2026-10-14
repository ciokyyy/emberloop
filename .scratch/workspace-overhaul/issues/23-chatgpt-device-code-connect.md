# 23: ChatGPT connect by device code

**What to build:** An admin connects ChatGPT from a browser on any machine. The Daemon runs pig's device-code login as a short-lived login session; the UI shows the user code and the verification link, then the ChatGPT Quota card refreshes (spec Parts 2 to 6, Providers).

**Blocked by:** 22-providers-page-and-api-keys

**Status:** done

- [x] Connect on the ChatGPT row starts the device-code login and shows the code and the link
- [x] Success writes the pig login and refreshes the ChatGPT Quota card
- [x] Go tests with a local stub cover success, expiry and cancel; vitest covers the panel
- [x] Repo check and `npm test` pass
