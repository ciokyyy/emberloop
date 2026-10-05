# 07: wick-pig stats prune

**What to build:** `wick-pig stats prune` deletes archived data on purpose: by age (`--older-than`) or by run (`--run <slug>`), with a dry run that shows what would go and how much space it frees. Blobs no longer referenced are removed. Nothing is ever deleted automatically.

**Blocked by:** 05, 06

**Status:** ready-for-agent

- [ ] Dry run by default or via `--dry-run` (pick one, document it), listing runs and size
- [ ] Removes transcripts, usage logs, memory logs, history for the selection; orphaned blobs removed
- [ ] Never touches a running run
- [ ] Tests on a fixture archive
