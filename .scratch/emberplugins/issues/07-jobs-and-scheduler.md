# 07: Jobs and scheduler

**What to build:** A manifest declares cron jobs. Each job runs a script or Go binary in the plugin folder, a one-shot LLM step, or a Session with a prompt. The Daemon schedules them, respects the caps, and shows runs, status and logs on the plugin's page. A lead scraper can be a UI plus a scheduled job with no resident process.

**Blocked by:** 05 (Plugin Sessions), 06 (Storage)

**Status:** ready-for-agent

- [ ] Cron entries in the manifest parse and fire on schedule (fake clock in tests)
- [ ] Script jobs count against `max_plugins`; Session jobs count against `max_workers`
- [ ] A run skipped because the cap is full is logged as skipped and never queued forever
- [ ] Job runs, status, duration and logs are stored and listed through the API
- [ ] The plugin page in the UI shows job runs and logs, with a run-now button
- [ ] A job the user disables stops firing
- [ ] Jobs run only for enabled, approved plugins
- [ ] A `max_plugins` setting exists beside `max_workers`, with an env override like `EL_MAX_WORKERS`
- [ ] API-seam tests cover schedule, each job kind, caps, skip, and persistence of run history
