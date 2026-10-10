# 13: OpenAPI and SDKs

**What to build:** An agent discovers every plugin-callable route from `/api/openapi.json`, each route marked with its permission group, and calls them through generated Go and TypeScript SDKs. The first spec covers plugin-callable routes only: sessions, prompt, events, spawn, runs, models, quota, one-shot LLM, storage and notify. The ticket compares SDK generators (openapi-generator, oapi-codegen, a TypeScript generator) and picks one.

**Blocked by:** 07 (Jobs and scheduler), 08 (Server process supervisor), 10 (Notify)

**Status:** ready-for-agent

- [ ] `/api/openapi.json` is generated from the routes, not hand-written, and stays in sync (a test fails on drift)
- [ ] Each route carries its permission group
- [ ] A generator is chosen with a short reason, and Go and TypeScript SDKs are generated from the spec
- [ ] The kit API client and the Go server skeleton use the SDKs
- [ ] The docs page links the spec and the SDKs
- [ ] Tests cover spec content for a sample of routes and the drift check
