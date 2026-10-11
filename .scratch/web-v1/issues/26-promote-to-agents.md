# 26: Promote to AGENTS.md

**What to build:** A queue item of kind `promotion` that proposes an edit to the nearest `AGENTS.md` (DOX) or a new note file in the Session's worktree. The Memory page and a chat action promote a Rule. Approval writes the edit to the worktree; the Daemon never commits and the Reviewer loop reviews it as a normal change.

**Blocked by:** 22, 25

**Status:** done

- [x] Promoting a Rule produces a proposed diff against the nearest `AGENTS.md` shown for approval
- [x] Approval writes the file in the Session's worktree and leaves it uncommitted
- [x] The promoted Rule is marked as promoted and keeps its provenance
- [x] Tests use a temp git repo
- [x] Owning `AGENTS.md` files and root index updated; repo check passes
