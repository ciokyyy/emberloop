# 04: Quota cards in credits or windows

**Type:** code

**What to build:** Each quota card is `credits` (dollars left) or `windows` (percent), never both. OpenRouter, DeepSeek and Relace are `credits`. DeepSeek's balance comes from its balance endpoint (dollars, granted plus topped-up). OpenRouter shows dollars left. Relace shows "credits unknown, healthy until a 402" and turns to a warning state while flagged. OpenCode-Go, Claude, ChatGPT and Antigravity keep percent windows. The cache-only quota API and subscriber-gated refresher rules still hold.

**Blocked by:** 02-credit-flag-and-failover

**Status:** ready-for-agent

- [ ] The quota API returns a `kind` and dollar fields for credits cards
- [ ] A DeepSeek fake balance of 2.50 USD shows as 2.50 left
- [ ] No percentage appears on a credits card
- [ ] OpenCode-Go windows are unchanged
- [ ] `.emberloop/check` passes

Spec: `../spec.md`.
