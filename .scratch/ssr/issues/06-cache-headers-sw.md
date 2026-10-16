# 06: Cache headers and service worker

**What to build:** SSR HTML sends `Cache-Control: private, no-store` and `Vary: Cookie`. `sw.js` stops caching navigation documents and keeps caching hashed assets and never touches `/api/*`.

**Blocked by:** 05

**Status:** open

- [ ] Header test on SSR and fallback responses
- [ ] Service worker test: no cached navigation document, assets still cached
- [ ] `web/README.md` Installable app text updated
