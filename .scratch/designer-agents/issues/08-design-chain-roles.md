# 08: Chain roles `worker-design` and `reviewer-design`

**What to build:** Two new Chain roles join the six: "Designer worker" (`worker-design`) and "Design reviewer" (`reviewer-design`). They follow the same entry rules as every role (at most three `{harness, model, effort}`, effort choices from each model's metadata), appear in Settings > Agents beside their siblings, work with `el settings role`, migrate cleanly from existing settings (absent means empty), and take part in Run Chain state (`runRoles`, per-Ticket per-role positions, Switch, Resume). Empty Chains stay empty here; the fallback behaviour lands with routing in tickets 09 and 10. Re-read `chains.go` (`agentChainRoles`, `chainRoleGroups`, `canonicalAgentChains`), `runs_chain.go` (`runRoles`), `cmd/el/settings.go` and `web/src/lib/SettingsPage.svelte`.

**Blocked by:** 01-remove-grok

**Status:** done

- [x] `GET/PUT /api/settings` serve and validate both roles with the existing entry rules; an old settings file without them loads with both empty
- [x] Settings > Agents lists both roles in the Run group with copy that says what they are for, with the existing save bar and effort lists
- [x] `el settings role worker-design|reviewer-design harness|model|effort <value>` works
- [x] `runRoles` includes both so ChainPositions, Switch, Pause and Resume handle them; a Run test with a scripted quota failure on each advances its Chain
- [x] Chain tests cover migration, aliases, the three-entry cap and the legacy-role mirror untouched for existing roles
- [x] `SettingsPage.test.js`, `chains_test.go`, `settings_test.go`, `runs_chain_test.go` cover the new roles; `emberloop-web/AGENTS.md` role list is updated; `.emberloop/check`, `npm test` and `npm run build` pass
