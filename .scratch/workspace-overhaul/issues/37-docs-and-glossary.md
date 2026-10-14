# 37: Docs, glossary and indexes

**What to build:** The docs describe the shipped workspace, so a fresh agent learns the behavior from docs alone. Update `emberloop-web/AGENTS.md`, `emberloop-web/web/README.md`, `GLOSSARY.md` (Provider, Dashboard and quota percent next to Project), `.scratch/settings-redesign/AGENTS.md` and the root index for Quota, Speed, Providers, Project scope, Draft pick, Settings and the test-suite guards (spec Parts 1 to 7). For Quota: the endpoint shape, the live `quota` topic, the subscriber-gated refresher, the percent-first cards and the provider set.

**Blocked by:** 09-openrouter-card, 11-refresh-min-interval, 14-quota-phone-drawer, 15-quota-live-patching, 18-runs-git-cache, 19-ticket-session-index, 26-cli-only-rows-and-redirect-base, 33-draft-pick-banner, 35-agents-chains-with-effort, 36-runs-and-memory-settings-pages (docs describe the shipped behavior)

**Status:** ready-for-agent

- [ ] `emberloop-web/AGENTS.md` covers the quota endpoint shape, the live topic, the subscriber-gated refresher and the new Runs, Providers, Project and Settings seams
- [ ] The web README Quota rows and the other area rows match the shipped behavior
- [ ] `GLOSSARY.md` defines Provider, Dashboard and quota percent
- [ ] Stale text is removed; every Child DOX Index entry resolves to a file; repo check passes
