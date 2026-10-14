# 21: Settings hub and shell

**What to build:** `/settings` becomes a hub with one page per concern, each at its own URL. Workspace: Agents, Runs, Context and names, Memory, Users and access. You: Profile, Notifications, Sound and voice. Each page carries a scope pill (Global, Admin, You). A save bar appears on the first change with Revert and Save. On phones `/settings` shows the hub list and each page a "< Settings" link. Shell and colors follow `.scratch/settings-redesign/mock/index.html` with tokens only from `src/app.css` (spec Parts 2 to 6, Settings).

**Blocked by:** 18-project-runs-and-settings-shell (Settings runs after Project scope; both edit the Rail and `App.svelte`)

**Status:** ready-for-agent

- [ ] Hub and one route per page, grouped Workspace and You
- [ ] Scope pill on each page
- [ ] Save bar with Revert and Save appears on the first change
- [ ] Phone hub list and "< Settings" links; 44px touch targets and 16px inputs
- [ ] Vitest covers the hub routes, pills and save bar; repo check and `npm test` pass
