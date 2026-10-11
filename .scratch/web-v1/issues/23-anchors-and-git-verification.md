# 23: Anchors and git verification

**What to build:** Anchors (paths, packages, `module@version` from `go.mod`) and the observed commit are stored on a Rule at approval. At injection the Daemon runs `git diff --stat <observed_commit>..HEAD -- <paths>` in the current worktree. Changed anchors give a verify flag; deleted anchors set the Rule `dormant`. Preferences have no anchors.

**Blocked by:** 15

**Status:** done

- [x] Tests use a temp git repo: unchanged paths inject clean, changed paths inject with the flag, deleted paths go dormant
- [x] A Rule observed on a branch that is not an ancestor of HEAD is flagged
- [x] A missing commit (rebased away) flags the Rule instead of failing the turn
- [x] Anchor extraction from a quote and the tool calls of its Session is deterministic and tested
- [x] Owning `AGENTS.md` files and root index updated; repo check passes
