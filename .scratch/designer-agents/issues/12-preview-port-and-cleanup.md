# 12: Dev server for design Sessions

**What to build:** A Designer or design Reviewer can start the Ticket's dev server without port clashes between parallel worktrees, and nothing it starts outlives its Session. The Daemon gives each of those Sessions a free `PORT` in its environment. The repo may provide `.emberloop/preview`, a script that reads `PORT`, starts the dev server for the worktree and prints the URL; otherwise the agent follows the dev command in the repo's `AGENTS.md`. When the Session ends, is paused, or its worktree is removed, processes the Session started under its worktree are stopped. Choose the mechanism (process group, cgroup-free cwd scan, or tracked children) and justify it in the ticket's closing notes. Re-read the Run worktree lifecycle (`endMergedWorker`, abandon, pause), `el/tools/bash.go` process-group handling and `claude.go` env wiring (ticket 04).

**Blocked by:** 04-browser-session-env-and-cleanup, 09-designer-worker-routing

**Status:** ready-for-agent

- [ ] Designer and design Reviewer Sessions receive a free `PORT`; two parallel Sessions never get the same port (test)
- [ ] A background preview started by a Session is stopped on Session end, pause, merge end and abandon; a test starts a fake long-running process and sees it gone
- [ ] No other role's environment gains `PORT`
- [ ] `.emberloop/preview` contract (reads `PORT`, prints the URL) is documented in `emberloop-web/AGENTS.md` and, for repo authors, in the area `AGENTS.md`
- [ ] The Emberloop repo itself gains a `.emberloop/preview` that serves its web UI for a worktree, so the flow can be exercised here
- [ ] `.emberloop/check` passes
