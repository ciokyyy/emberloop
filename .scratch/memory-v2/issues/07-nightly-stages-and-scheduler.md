# 07: Nightly stages and scheduler fix

**What to build:** Fix `nightlyDue`: due when no `nightly_summary` ledger row exists since the most recent 03:00 local, at any hour, when the Daemon is idle. A busy due run writes one `nightly_skipped_busy` row per day and retries every minute; the last-run marker comes from the ledger. Add stages: catch-up ingest, reembed, fold (Episodes older than 12 h into at most 5 inferred Memories per Session, importance 0.6, mark `consolidated_at`), consolidate (merge/generalize proposals validated in Go; a valid `generalize` over 2+ Memories becomes a pending `rule` item), promote candidates (`mention_count >= 2` over 2+ Sessions, importance at least 0.7, kind preference/instruction/decision, no live Rule at cosine 0.88, cluster cosine 0.80), then the existing Profile proposals, anchor check and summary with the new counts.

**Blocked by:** 03, 04, 05

**Status:** todo

- [ ] A missed day runs the next time the Daemon is idle at any hour; a restart does not repeat a finished day
- [ ] A busy day writes exactly one skipped row
- [ ] Fold keeps originals and never re-folds a consolidated Episode
- [ ] Promote candidate creates a pending Rule with its quotes; nothing is auto-approved
- [ ] The old subject-key recurrence test is removed from candidate creation
- [ ] Owning `AGENTS.md` files and root index updated; repo check passes
