# 05: Relace as a managed API-key provider

**Type:** code

**What to build:** Relace appears in the Providers page next to OpenRouter and OpenCode-Go with an editable key, and in the pig catalog with the models from its `/v1/models` catalog (prices, context, reasoning). DeepSeek's balance status shows there too. Never write the key into a repo file or a log.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] The Providers API lists Relace with keySet and keyEditable
- [ ] Setting and clearing the key through the API writes only the pig auth file
- [ ] The key never appears in logs or API responses
- [ ] `.emberloop/check` passes

Spec: `../spec.md`.
