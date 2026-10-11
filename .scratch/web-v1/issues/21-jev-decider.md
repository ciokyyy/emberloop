# 21: Jev decider

**What to build:** `jev` Decider: HTTP client for the Jev decision endpoint (`POST /api/v1/decisions`, `Authorization: Bearer`, body `model`, `state`, `questions`; answers carry `noul`, or `choice` with `probabilities` and `confidence`) and the TypeSafe `POST /v1/systemone` shape. Settings `jev.baseURL`, `jev.apiKey`, `jev.model` (default `jev-latest`), host-global, admin-only, off by default. Verify the request shape of a local Laya server (`laya[serve]`, `convaiinnovations/laya`) and add an adapter if it differs, so Laya can run as a local alternative. State is redacted and size-capped. Settings tell the admin that user turns leave the host when this is on.

**Blocked by:** 20

**Status:** done

- [x] Noul, choice and score answers parse into the `Decider` result type; a 401, 422, timeout or malformed body maps to `unavailable`
- [x] At most 6 questions per call and 2 to 8 choice options are enforced client-side
- [x] The client sends only redacted text and never sends when the setting is off
- [x] An env-gated live probe calls the real endpoint (`EL_LIVE_JEV=1`)
- [x] A note in this ticket records the verified Laya server shape and whether an adapter was needed
- [x] Fake HTTP server tests cover each answer type and each error
- [x] Owning `AGENTS.md` files and root index updated; repo check passes

## Laya verification

`laya[serve]` (upstream repo `NandhaKishorM/laya`; `convaiinnovations/laya` is the HuggingFace checkpoint org) verified against its README ("Self-Hosting: HTTP Server (Jev-compatible)") and the laya 0.4.2 `serve.py` source: it exposes `POST /v1/systemone` only (plus `/batch`, `/models`, `/health`) — no `/api/v1/decisions` path — with the same `{model, state, questions}` body and schema-identical contracted answer keys (`noul` / `choice` / `score` plus input/output usage). **No adapter needed**: the client POSTs the endpoint URL verbatim and ignores unknown reply fields (Laya's `routing`, extended `usage`, `answer_confidence` extras), so Laya plugs in by `baseURL` alone (e.g. `http://localhost:8000/v1/systemone`, optional `Bearer` via `LAYA_API_KEY`). Caveats: `model` honours Laya checkpoint names (`jev-*` means the router auto-selects), and Laya `confidence` is 1-minus-normalised-entropy rather than Jev's formula, so callers gate on probability, never on confidence.
