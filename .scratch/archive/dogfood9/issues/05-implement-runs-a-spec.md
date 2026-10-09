# 05: el implement <spec> runs implement-spec on an integration branch

**What to build:** `el implement <spec>` takes a spec file whose tickets already exist. It creates `spec/<slug>` from the current HEAD, or reuses it if it exists (a resumed run), and records it as the base that spawn_worker branches workers from and finish_worker rebases onto and fast-forwards. Each worker record and brief carries the spec path alongside its ticket. The session's prompt is replaced by a pointer to `el://matt-pocock/implement-spec/SKILL.md`, the spec path, and a short el adapter:
- implementer subagent = spawn_worker; merger subagent = finish_worker (mechanical; no merger agent); exploration subagent = scout
- the integration branch already exists
- skip the skill's final code-review step; workers review themselves
- if finish_worker refuses a worker, tell_worker it to address its review and call code_review again
- if finish_worker returns kept:true (rebase conflict), tell_worker the worker to finish the rebase and resolve it keeping both sides' intent, wait for it once, finish again; a second conflict is reported, not retried (this is dogfood8 ticket 25, never landed)
- the session never edits and never reviews

The `implement` role gains scout. (review_worker was already removed from the role and from the old prompt.)

**Blocked by:** 03 (prompt, brief and launcher code)

**Status:** ready-for-agent

- [ ] Launching on a spec creates `spec/<slug>` from HEAD; a second launch reuses it.
- [ ] Workers spawned in that session branch from the integration tip; finish_worker merges into it, not the launch branch.
- [ ] The worker record and brief name the spec.
- [ ] The prompt names the implement-spec `el://` address and carries every adapter rule above; it contains no review step.
- [ ] `implement` serves read, find, spawn_worker, wait_workers, finish_worker, tell_worker, scout.
- [ ] A fake-exec test: a second finish_worker on a worker that conflicted and then committed merges.

## Hard limits (this ticket is built by the tool it changes)

- Spec: `.scratch/archive/dogfood9/spec.md`. Read it for the decisions this ticket implements.
- Edit only files under `el/` (and this ticket's named files). Never touch `~/.local/bin`, `~/.pig`, `~/.claude`, `~/.claude.json`, `~/.agents`, `~/.el`, or `scripts/`.
- Never run `go install`, never copy binaries anywhere. Build with `go build ./...` and test with `go test -skip TestLive ./...` from `el/`.
- Never run a `TestLive*` test. Never start el, pig, claude or bunx inside the work. Tests use a fake exec and temp dirs, never the network or the real home.
- Servers, watchers and tails go to the `background` tool, never bash.
- Keep `go vet ./...` and `go test -skip TestLive ./...` green.
