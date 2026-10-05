# 11: The spawn wait bound is a named setting and the reply states it

**What to build:** The wait for blocking agents keeps its current bound, but the bound becomes a named setting, the 'still running' reply states the wait in seconds and that the result arrives as a notice, and the worker and orchestrator briefs mention it once. A test covers the bound so a change is deliberate.

**Blocked by:** 04 Launches are spaced across sessions, and concurrent workers are capped (both change spawn)

**Status:** ready-for-agent

- [ ] A fake slow reviewer outlasts the bound; the reply names the wait in seconds and says results arrive as notices
- [ ] The bound is readable and settable through the settings command and file
- [ ] Spec stories 29 and 30

Spec: `.scratch/retro-hardening/spec.md`. Read it for the decisions this ticket implements, and its Hard limits section, which binds you.
