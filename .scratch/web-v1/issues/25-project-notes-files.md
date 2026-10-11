# 25: Project notes as repo files

**What to build:** Project notes are Markdown files with frontmatter (`id`, `anchors`, `observed_commit`, `author`, `subject_key`) in `.emberloop/notes/`. The Daemon indexes the notes of each worktree at Session start into a temporary FTS5 index and includes them in recall. Notes obey anchors and verification like Rules. The Daemon never commits.

**Blocked by:** 15, 23

**Status:** done

- [x] Two worktrees on different branches see different note sets
- [x] A note with deleted anchors is skipped
- [x] A malformed frontmatter file is skipped with a notice, not an error
- [x] Recall merges notes and Rules without comparing BM25 scores across tables
- [x] Tests use a temp git repo with two worktrees
- [x] Owning `AGENTS.md` files and root index updated; repo check passes
