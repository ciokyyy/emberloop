# 08: el runs .el/check before every review

**What to build:** Before each code_review, el runs the single shell command in `.el/check` at the repo root, in the worker's tree, with a timeout inside the tool budget, and puts its exit code and trimmed output into every reviewer's packet, so reviewers judge real results rather than the worker's word. With no `.el/check`, the packet says "no checks configured" and review proceeds. Add this repo's `.el/check` running `go vet ./...` and `go test -skip TestLive ./...` from `el/`. This is on trial: the first dogfood run judges whether the output earns its place.

**Blocked by:** 06

**Status:** ready-for-agent

- [ ] Check output (exit code and trimmed output) appears in both reviewers' packets.
- [ ] Missing file: packet says "no checks configured"; review still runs.
- [ ] A check that times out is reported as such, not as a pass.
- [ ] `.el/check` exists at the repo root (this ticket may write it, outside `el/`).
- [ ] Covered over the fake exec.

## Hard limits (this ticket is built by the tool it changes)

- Spec: `.scratch/archive/dogfood9/spec.md`. Read it for the decisions this ticket implements.
- Edit only files under `el/` (and this ticket's named files). Never touch `~/.local/bin`, `~/.pig`, `~/.claude`, `~/.claude.json`, `~/.agents`, `~/.el`, or `scripts/`.
- Never run `go install`, never copy binaries anywhere. Build with `go build ./...` and test with `go test -skip TestLive ./...` from `el/`.
- Never run a `TestLive*` test. Never start el, pig, claude or bunx inside the work. Tests use a fake exec and temp dirs, never the network or the real home.
- Servers, watchers and tails go to the `background` tool, never bash.
- Keep `go vet ./...` and `go test -skip TestLive ./...` green.
