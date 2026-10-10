# 14: E2E lead scraper

**What to build:** One real fixture Emberplugin, a lead scraper, proves the whole path in a real browser against a throwaway Daemon: sidebar entry, styled iframe, token call, LLM call on the fake Backend, storage write and read, a scheduled job that adds leads, and a notification that opens the plugin page.

**Blocked by:** 07 (Jobs and scheduler), 10 (Notify)

**Status:** ready-for-agent

- [ ] Fixture plugin lives with the e2e suite and installs into the throwaway Daemon's plugin dir
- [ ] Browser test covers sidebar, iframe render with kit styles, approval, LLM call, storage and a job run adding rows
- [ ] Browser test covers a notification and that opening it lands on the plugin page
- [ ] Browser test covers disable removing the sidebar entry
- [ ] Runs through `npm run test:e2e` with no model and no API key
