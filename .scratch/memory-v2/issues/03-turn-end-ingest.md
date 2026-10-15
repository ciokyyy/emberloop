# 03: Turn-end ingest trigger

**What to build:** On a Session idle status Event, enqueue a scan: debounce 3 s, at most one scan in flight per Session, off the turn path. Reuse the existing cursor. Select only `sender=user` prompt, steer and follow-up Events; drop `sender=system`, slash commands and text under 12 characters; cap 4 KB per Event and 15 KB per scan (keep newest); redact. Store one `episodes` row per kept Event (importance 0.65) even when extraction is off or fails. Call the extractor with the last assistant message (at most 1 KB, redacted, labelled context only). Write one `ingest` ledger row per scan with the counts, model, latency and cost.

**Blocked by:** 01, 02

**Status:** done

- [x] A steer burst becomes one scan
- [x] Injection, handoff, Subagent result and sudo result Events are never scanned
- [x] The assistant context is never accepted as a quote
- [x] A steer from User B inside User A's Session lands in B's bank only
- [x] An extractor failure still stores Episodes and writes a ledger row; the turn is never blocked
- [x] Owning `AGENTS.md` files and root index updated; repo check passes
