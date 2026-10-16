# 01: npm dependencies to latest

**What to build:** Every dependency in `emberloop-web/web/` and `web/e2e/` is at its latest version. Vite 8 and `@sveltejs/vite-plugin-svelte` 7 move together. `svelte` becomes an explicit dependency.

**Blocked by:** None (can start immediately)

**Status:** open

- [ ] `npm outdated` is empty in `web/` and `web/e2e/`
- [ ] `svelte` is listed in `package.json`
- [ ] `npm test`, `npm run build` and `npm run test:e2e` pass
- [ ] `uiSourceHash` still matches `hashWebSource`
- [ ] `web/README.md` reflects any changed command or version
