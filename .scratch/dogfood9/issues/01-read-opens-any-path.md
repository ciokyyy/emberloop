# 01: Read opens any path, except credentials

**What to build:** `read` opens any absolute path, so a worker can read its own brief under emberloop's state dir and a scout or reviewer can read logs, state and transcripts outside the repo. A short deny list in code refuses credential paths (`~/.ssh`, pig's agent auth file, claude's config and credentials files, `.env*` anywhere) with an error that says it is a credential path. The `EMBERLOOP_READ_ALLOW` mechanism goes away; reviewers still reach their prompt and diff files because read now opens any path. edit and write stay confined to the workspace.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] Reading an absolute path outside the workspace returns the file, for any role.
- [ ] Each deny-list path is refused, and the error names the reason.
- [ ] `EMBERLOOP_READ_ALLOW` is no longer set by launchers nor read by the tool; reviewer launches still work (covered by the existing readonly tests).
- [ ] edit and write still refuse paths outside the workspace.
- [ ] Tests over the read tool's Execute, temp dirs only.

## Hard limits (this ticket is built by the tool it changes)

- Spec: `.scratch/dogfood9/spec.md`. Read it for the decisions this ticket implements.
- Edit only files under `emberloop/` (and this ticket's named files). Never touch `~/.local/bin`, `~/.pig`, `~/.claude`, `~/.claude.json`, `~/.agents`, `~/.emberloop`, or `scripts/`.
- Never run `go install`, never copy binaries anywhere. Build with `go build ./...` and test with `go test -skip TestLive ./...` from `emberloop/`.
- Never run a `TestLive*` test. Never start emberloop, pig, claude or bunx inside the work. Tests use a fake exec and temp dirs, never the network or the real home.
- Servers, watchers and tails go to the `background` tool, never bash.
- Keep `go vet ./...` and `go test -skip TestLive ./...` green.
