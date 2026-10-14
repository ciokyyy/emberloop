# 28: Dashboard payload

**What to build:** The Daemon serves the "All projects" Dashboard data: only running and needs-you Sessions and Runs, grouped by Project. A Run belongs to the deepest Project that contains its repo path; Sessions in the User's home directory group under the built-in `none` scope (spec Parts 2 to 6, Project scope).

**Blocked by:** 21-non-blocking-boot (Project scope depends on Speed)

**Status:** ready-for-agent

- [ ] A Dashboard endpoint returns running and needs-you Sessions and Runs grouped by Project id; idle history is absent
- [ ] Runs map to the deepest containing Project; home-directory Sessions map to `none`
- [ ] Go tests at the HTTP seam cover grouping, the filter and `none`; repo check passes
