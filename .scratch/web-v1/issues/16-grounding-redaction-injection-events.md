# 16: Grounding check, redactor and injection Events

**What to build:** Three small shared pieces. (a) Grounding: given an Event ID and a quote, verify the Event exists, has `sender=user` and contains the quote verbatim. (b) Redactor ported from `deja-vu` (MIT): cloud keys, provider tokens, JWTs, connection URLs, `Bearer`, CLI password flags, multilingual key-value forms, PEM blocks. (c) `injection` Event type with Rule IDs and versions, `sender=system`.

**Blocked by:** 02, 15

**Status:** done

- [x] Grounding rejects missing Events, `sender=system` Events and quotes that are not substrings
- [x] Redactor golden tests cover every pattern family including non-English key words and CLI flags
- [x] `injection` Events are appended on every injection and are never read by the miner
- [x] The redactor is the only path to the distill LLM, the Decider LLM and the Jev client (a test fails if a call site bypasses it)
- [x] Owning `AGENTS.md` files and root index updated; repo check passes
