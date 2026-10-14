# 18: Project Runs and Project settings shell

**What to build:** `/p/<projectId>/runs` lists that Project's Runs with no folder to type. `/p/<projectId>/settings` is a shell with rename, remove (existing behavior) and the editable gist; it has no override fields (spec Parts 2 to 6, Project scope).

**Blocked by:** 16-project-gist, 17-project-scope-routes

**Status:** ready-for-agent

- [ ] `/p/<id>/runs` lists the Project's Runs and starts a Run without a folder field
- [ ] `/p/<id>/settings` offers rename, remove and gist edit
- [ ] Vitest covers both routes; repo check and `npm test` pass
