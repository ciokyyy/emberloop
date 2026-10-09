# 09: Every pane is named after its agent type

**What to build:** Every pane el starts carries a readable name in herdr: the agent's type. The names are `orchestrator` for the user's top-level session, `implementer` for workers, `spec-reviewer` and `code-reviewer` for the two reviewers, and `scout` for scouts. Names need not be unique; several implementers can share the name. El sets the name when it places the pane and again when a pane is reused or its agent restarts, so a name never goes stale. The top-level session names its own pane at startup when it runs inside herdr. Failures to rename are silent and never block a spawn. The header strip's name chip uses the same type name, so the strip and the herdr sidebar agree.

**Blocked by:** None (can start immediately)

**Status:** done

- [x] Spawned workers, reviewers and scouts get their type name on their pane at placement
- [x] The top-level session names its own pane `orchestrator` at startup inside herdr, and does nothing outside herdr
- [x] A reused reviewer pane and a restarted agent keep the right name
- [x] A failed rename is silent and does not fail the spawn
- [x] Names come from one place, so the header strip chip can read the same value
- [x] Tests through the exec seam with a fake herdr assert the rename calls

Spec: `.scratch/archive/context-compress/spec.md`. Read it for the surrounding decisions and its Out of Scope section.
