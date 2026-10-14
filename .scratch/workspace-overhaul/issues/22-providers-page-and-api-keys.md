# 22: Providers page with status and API keys

**What to build:** A global `/providers` page lists every pig provider with its connection status (stored key, environment variable, OAuth, or none). An admin pastes or removes an API key for any API-key provider (OpenRouter, OpenCode-Go, Gemini direct, DeepSeek, OpenAI), written through `AuthStorage.Set` into the host pig `auth.json`. A stored key is never sent to the browser, only a "set" flag. Other Users see status read-only with Connect locked. A Providers entry sits in the Quota second sidebar. Rows group as subscription logins, API keys and CLI-only, following `.scratch/settings-redesign/mock/index.html`, phone layout included (spec Parts 2 to 6, Providers).

**Blocked by:** 21-non-blocking-boot (Speed lands before Providers; both edit `App.svelte`)

**Status:** done

- [x] `/providers` route and Providers entry in the Quota second sidebar
- [x] The provider list comes from the pig SDK with the status source per row
- [x] Admin sets and removes an API key; no response body ever contains a key
- [x] Writes are admin-only; non-admin Users see status read-only
- [x] Go tests with a temp pig dir cover list, status sources, set, remove, no key in any response and admin-only writes; vitest covers page states and phone layout
- [x] Repo check and `npm test` pass
