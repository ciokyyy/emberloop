# 08: A harness override drops the other harness's model and effort

**What to build:** Found in the post-finish review of global-settings. The resolver picks harness, model and effort per field independently, so a layer that changes the harness still inherits model and effort from a lower layer meant for another harness. Reproduced: settings `code-review = {pig, deepseek/deepseek-flash, minimal}` with `WICK_READONLY_HARNESS=claude` resolves to `{claude, deepseek/deepseek-flash, minimal}`, and claude is launched with a pig model and an effort it doesn't have. The same happens with `--claude` against a pig orchestrator slot, or an agent type whose frontmatter sets only `harness`. It matters more once agy is a third harness.

Rule: model and effort come only from layers whose harness matches the resolved harness. A layer that sets a model or effort without a harness counts as matching (it inherits the harness resolved above it). When the harness came from a higher layer than the slot, the slot's model and effort are dropped and the harness's own defaults apply.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] Resolver table test: harness override from flag, env (`WICK_READONLY_HARNESS`), and type/spawn layer each drop a lower layer's model and effort for a different harness
- [ ] A flag that sets only model (no harness) still combines with the slot's harness
- [ ] Same harness at both layers keeps the lower layer's model/effort when the higher one sets none
- [ ] Launch tests: `--claude` with a pig orchestrator slot launches claude with no `--model`/`--effort`
