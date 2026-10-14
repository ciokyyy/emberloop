# 30: Project switcher, Dashboard and scoped URLs

**What to build:** One Project switcher replaces the per-Project sidebar sections. Its "All projects" entry is the Dashboard at `/`. `/p/<projectId>` scopes the sidebar, Sessions and Runs; `/p/none` holds home-directory Sessions; `/s/<id>` stays canonical and sets the scope from the Session's Project. URLs use the stable Project id, so a rename breaks no link. `/quota`, `/providers`, Memory, Users and account Settings stay global. The switcher and Dashboard appear in the phone drawer (spec Parts 2 to 6, Project scope).

**Blocked by:** 27-quota-links-to-providers, 28-dashboard-payload (Providers lands before Project scope; both edit the Rail and `App.svelte`)

**Status:** ready-for-agent

- [ ] Project switcher with "All projects" replaces the per-Project sidebar sections
- [ ] `/` renders the Dashboard from the ticket 28 payload
- [ ] `/p/<id>` and `/p/none` scope the sidebar, Sessions and Runs; `/s/<id>` sets the scope
- [ ] Global pages ignore the scope
- [ ] Switcher and Dashboard work in the phone drawer
- [ ] Vitest router tests cover every URL in the spec table, scope from `/s/<id>`, `/p/none` and rename-stable links; repo check and `npm test` pass
