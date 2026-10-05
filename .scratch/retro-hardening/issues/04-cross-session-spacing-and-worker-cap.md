# 04: Launches are spaced across sessions, and concurrent workers are capped

**What to build:** Launches from separate sessions keep the pause between launches (a shared timestamp under a lock, not a per-process one), and a setting caps concurrent workers, default three. A spawn call for more workers than the cap starts the cap and reports the rest as queued, then starts them as slots free. Add the setting to the settings file and the settings command, and mention the cap once in the orchestrator brief.

**Blocked by:** 01 Worker store survives simultaneous sessions (the multi-process helper)

**Status:** ready-for-agent

- [ ] Two processes launching at the same instant are separated by the configured pause (multi-process test)
- [ ] A spawn of more workers than the cap starts the cap, reports the queued ones, and starts them as earlier ones finish
- [ ] The cap is readable and settable through the settings command and file, default three
- [ ] Spec stories 36 and 44

Spec: `.scratch/retro-hardening/spec.md`. Read it for the decisions this ticket implements, and its Hard limits section, which binds you.
