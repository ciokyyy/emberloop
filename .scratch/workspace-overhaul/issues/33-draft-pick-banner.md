# 33: Draft project pick banner

**What to build:** When the Daemon suggests a Project, the draft shows "Run this in <Project>?". Yes creates the Session in that Project's path; no or no banner keeps today's behavior. A started Session never changes its working directory (spec Parts 2 to 6, Draft project pick).

**Blocked by:** 30-project-scope-routes, 32-draft-project-pick-api

**Status:** ready-for-agent

- [ ] The banner appears only when the Daemon returns a suggestion
- [ ] Yes starts the Session in the suggested Project's path; dismiss starts it as today
- [ ] Go test: a Session started through Yes runs in the chosen path; vitest covers the banner
- [ ] Repo check and `npm test` pass
