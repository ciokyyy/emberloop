# 25: Project notes as repo files

**What to build:** Project notes are Markdown files with frontmatter (`id`, `anchors`, `observed_commit`, `author`, `subject_key`) in `.emberloop/notes/`. The Daemon indexes the notes of each worktree at Session start into a temporary FTS5 index and includes them in recall. Notes obey anchors and verification like Rules. The Daemon never commits.

**Blocked by:** 15, 23

**Status:** ready-for-agent

- [ ] Two worktrees on different branches see different note sets
- [ ] A note with deleted anchors is skipped
- [ ] A malformed frontmatter file is skipped with a notice, not an error
- [ ] Recall merges notes and Rules without comparing BM25 scores across tables
- [ ] Tests use a temp git repo with two worktrees
- [ ] Owning `AGENTS.md` files and root index updated; repo check passes
