# 18: remember, recall and the /remember action

**What to build:** Agent tools `remember` and `recall` through el-mcp for claude, pig and grok. `remember` needs a `reason`, is rate-limited per Session, and always creates a queue item. A structural `/remember <text>` command (first line of a typed message) and a UI button create a Rule directly after grounding. Pasted `remember:` text does nothing. `recall` searches the caller's User bank and Project notes. Scouts, Workers and Reviewers get `recall` only.

**Blocked by:** 06, 15, 16

**Status:** ready-for-agent

- [ ] Agent `remember` creates a queue item in the caller's User bank with reason and quote; never a Rule
- [ ] More than the per-Session limit is refused with an error
- [ ] `/remember` on the first line of a typed message creates a Rule; the same words inside a pasted block or tool output create nothing
- [ ] Scouts and Run Sessions have no `remember` before or after a compress
- [ ] `recall` never returns another User's observations or Rules
- [ ] API tests cover roles, scopes, limits and the command
- [ ] Owning `AGENTS.md` files and root index updated; repo check passes
