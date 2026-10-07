# 09: finish_worker merges only after both reviewers pass; review_worker deleted

**What to build:** finish_worker merges a worker only when both reviewers have `pass` recorded at the worker's current HEAD. Otherwise it refuses without touching anything and names the unsatisfied reviewer and the commit ("spec reviewer: changes at abc123"; "no review yet"). Discarding (`merge=false`) is always allowed. Both merge and discard close the worker's reviewer panes and clear their state. The `review_worker` tool and the `Review` function are deleted (it is already gone from every role); the shared diff-review path stays as the two-reviewer path.

**Blocked by:** 05 (integration-branch merging), 06 (reviewer state)

**Status:** ready-for-agent

- [ ] Refuses with no review, with a `changes` verdict, and with a pass older than HEAD; the message names reviewer and commit.
- [ ] Merges when both passed at HEAD.
- [ ] Discard works without passes.
- [ ] Merge and discard close the reviewer panes (fake exec shows the close calls).
- [ ] `review_worker` and `Review` no longer exist; no prompt mentions them.

## Hard limits (this ticket is built by the tool it changes)

- Spec: `.scratch/dogfood9/spec.md`. Read it for the decisions this ticket implements.
- Edit only files under `emberloop/` (and this ticket's named files). Never touch `~/.local/bin`, `~/.pig`, `~/.claude`, `~/.claude.json`, `~/.agents`, `~/.emberloop`, or `scripts/`.
- Never run `go install`, never copy binaries anywhere. Build with `go build ./...` and test with `go test -skip TestLive ./...` from `emberloop/`.
- Never run a `TestLive*` test. Never start emberloop, pig, claude or bunx inside the work. Tests use a fake exec and temp dirs, never the network or the real home.
- Servers, watchers and tails go to the `background` tool, never bash.
- Keep `go vet ./...` and `go test -skip TestLive ./...` green.
