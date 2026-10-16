# 09: Memory page tabs and actions

**What to build:** Add a Memories tab: newest first, filter by kind and status, search, importance, mention count, source quote and Session link. Actions: Forget, Park, Pin, Promote to Rule (pending `rule` item with the Memory as source), Edit (new superseding Memory). Read-only tabs for Legacy observations (with a promote-to-Memory action) and Episodes. Header counter "N new since last visit", queue size and oldest pending age. New API endpoints for each action.

**Blocked by:** 01, 06

**Status:** done

- [x] Forget hides a Memory from recall at once and keeps the row until purge
- [x] Edit creates a new row and supersedes the old one; the old text is unchanged
- [x] Promote creates a pending Rule only; it does not approve
- [x] The counter resets on visit
- [x] Browser and API tests cover all actions
- [x] Owning `AGENTS.md` files and root index updated; repo check passes
