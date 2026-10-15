# 34: Settings hub and shell

**What to build:** `/settings` becomes a hub with one page per concern, each at its own URL. Workspace: Agents, Runs, Context and names, Memory, Users and access. You: Profile, Notifications, Sound and voice. Each page carries a scope pill (Global, Admin, You). A save bar appears on the first change with Revert and Save. On phones `/settings` shows the hub list and each page a "< Settings" link. Shell and colors follow `.scratch/settings-redesign/mock/index.html` with tokens only from `src/app.css` (spec Parts 2 to 6, Settings).

**Blocked by:** 31-project-runs-and-settings-shell (Settings runs after Project scope; both edit the Rail and `App.svelte`)

**Status:** done

- [x] Hub and one route per page, grouped Workspace and You
- [x] Scope pill on each page
- [x] Save bar with Revert and Save appears on the first change
- [x] Phone hub list and "< Settings" links; 44px touch targets and 16px inputs
- [x] Vitest covers the hub routes, pills and save bar; repo check and `npm test` pass
