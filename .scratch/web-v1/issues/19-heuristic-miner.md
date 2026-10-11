# 19: Heuristic miner and cursor

**What to build:** Scan `events.jsonl` per Session from a byte-offset cursor (stored with file size and a hash of the first 4 KB), only up to the last newline, only for settled Sessions, only `sender=user` prompt, steer and follow-up Events. Apply the ordered heuristic pass ported from `claude-reflect` (MIT). Store each hit with its quote, Event ID and the corrected tool call or diff hunk. Run redaction first. Dedup keeps negation tokens and assigns duplicate, refines or contradicts relations. Duplicates raise `evidence_count`.

**Blocked by:** 07, 15, 16

**Status:** done

- [x] A truncated or rewritten file resets the cursor; a torn last line is not read
- [x] Handoff text, injection blocks and `sender=system` Events are never mined
- [x] Golden cases: guardrail phrases accepted; `no problem`, `never mind`, `no worries` rejected; `Perfect! Now let's add X` rejected; bare praise under 25 chars rejected; CJK corrections accepted
- [x] A steer from User B inside User A's Session lands in B's bank only
- [x] "always return wrapped errors" and "never return wrapped errors" are related as contradicts, not merged
- [x] Each observation passes the grounding check and is redacted before storage
- [x] Tests drive the miner with canned `events.jsonl` files
- [x] Owning `AGENTS.md` files and root index updated; repo check passes
