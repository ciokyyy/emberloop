# 15: Quota docs and index updates

**What to build:** The docs say what the Quota page now does. The browser UI module doc, the Quota rows of the web README and the root index all describe the cached-at-once reads, background refresh over the live topic, the percent-first cards and the provider set, so a fresh agent learns the behaviour from docs alone.

**Blocked by:** 08-openrouter-card, 10-refresh-min-interval, 13-phone-drawer, 14-live-patching (docs describe the shipped behaviour)

**Status:** ready-for-agent

- [ ] Browser UI module doc covers the quota endpoint shape, the live topic and the subscriber-gated refresher
- [ ] Web README Quota rows match the shipped behaviour and provider set
- [ ] Root index points at the quota area correctly
- [ ] No code behaviour changes; repo check passes
