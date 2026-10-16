# 08: Profile patch notification and inline approval

**What to build:** Each pending Profile patch raises a push notification with Approve and Reject (reuse the `push.go` sudo flow) and an inline card in the open Session. Approval records `approved_by`; rejection keeps the item as a label. A pending item older than 48 hours raises one reminder. Add the opt-in daily digest user preference (off by default).

**Blocked by:** 07

**Status:** done

- [x] Approving from the notification and from the card give the same result and record `approved_by`
- [x] A rejected patch is kept as a label and not proposed again unchanged
- [x] One reminder per item after 48 hours, never more
- [x] Digest is off by default and sends only when enabled
- [x] API and browser tests cover both paths
- [x] Owning `AGENTS.md` files and root index updated; repo check passes
