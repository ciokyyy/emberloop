# 21: Jev decider

**What to build:** `jev` Decider: HTTP client for the Jev decision endpoint (`POST /api/v1/decisions`, `Authorization: Bearer`, body `model`, `state`, `questions`; answers carry `noul`, or `choice` with `probabilities` and `confidence`) and the TypeSafe `POST /v1/systemone` shape. Settings `jev.baseURL`, `jev.apiKey`, `jev.model` (default `jev-latest`), host-global, admin-only, off by default. Verify the request shape of a local Laya server (`laya[serve]`, `convaiinnovations/laya`) and add an adapter if it differs, so Laya can run as a local alternative. State is redacted and size-capped. Settings tell the admin that user turns leave the host when this is on.

**Blocked by:** 20

**Status:** ready-for-agent

- [ ] Noul, choice and score answers parse into the `Decider` result type; a 401, 422, timeout or malformed body maps to `unavailable`
- [ ] At most 6 questions per call and 2 to 8 choice options are enforced client-side
- [ ] The client sends only redacted text and never sends when the setting is off
- [ ] An env-gated live probe calls the real endpoint (`EL_LIVE_JEV=1`)
- [ ] A note in this ticket records the verified Laya server shape and whether an adapter was needed
- [ ] Fake HTTP server tests cover each answer type and each error
- [ ] Owning `AGENTS.md` files and root index updated; repo check passes
