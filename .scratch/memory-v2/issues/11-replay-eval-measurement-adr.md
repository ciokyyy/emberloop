# 11: Replay eval arm, measurements and ADR 0004

**What to build:** Add a recall-with-Memories arm to the web v1 replay eval (A/B recall-off switch stays). Surface production proxies from the ledger on the Memory page: Memories per day, forgotten within 7 days (target under 20 percent), promoted share, injection hit rate, contradiction counter, and daily extractor plus embedder cost with an alert above $0.05. Write `docs/adr/0004-...` for the narrowed human gate (Memories automatic; Rules, Profile and Project notes approved) and list it in the docs index.

**Blocked by:** 06, 07, 09

**Status:** done

- [x] The replay eval reports the Memories arm next to the existing arms
- [x] Forget rate and cost figures match the ledger rows in a test fixture
- [x] ADR 0004 states the decision, the rejected alternative (approve every Memory) and the consequence
- [x] Owning `AGENTS.md` files and root index updated; repo check passes
