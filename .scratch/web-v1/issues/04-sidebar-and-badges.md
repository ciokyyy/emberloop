# 04: Sidebar grouping and sender badges

**What to build:** Rebuild the Session list to match `.scratch/mockup/user-sidebar.html`: User groups, filters, Project headers, nesting, Run tag, online dot, plus a sender badge on prompts and the creator in the Session header.

**Blocked by:** 02, 03

**Status:** ready-for-agent

- [x] Own group first and open; other groups collapsed with count and online dot
- [x] Colored left bar per owner; Project headers; Subagents nested; Runs tagged
- [x] Filters Mine, Everyone and Project work
- [x] Each user prompt shows a sender badge; system prompts show none; the Session header shows the creator
- [x] Layout follows `web/README.md` design rules and works on `<md` screens
- [x] vitest covers grouping, filters and badges
- [ ] Owning `AGENTS.md` files and root index updated; repo check passes
