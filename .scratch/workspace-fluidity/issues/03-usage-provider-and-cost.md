# 03: Turn usage records the serving provider and its cost

**Type:** code

**What to build:** The usage event of each turn carries the provider that served it and the cost at that provider's price (input, output, cache read). The transcript data exposes it so a design Ticket can show a badge such as "via Relace · peak". Prices come from the model entry in the pig model files.

**Blocked by:** 01-ladder-probe-and-resolve

**Status:** ready-for-agent

- [ ] A Ladder turn served by Relace and one by DeepSeek record different providers and costs for the same token counts
- [ ] Non-Ladder Sessions are unchanged
- [ ] `.emberloop/check` passes

Spec: `../spec.md`.
