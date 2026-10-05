# 07: Stale passes are rechecked, dead reviewers replaced

**What to build:** A pass is tied to a commit. When a reviewer passed at commit A and the worker has since moved HEAD (e.g. fixing the other reviewer's findings), the next code_review also sends that reviewer the `A..HEAD` diff with "check this did not break what you approved", even though it has no open findings. When a reviewer's pane is gone (crashed, closed by hand), wick starts a replacement with the full diff and a summary of its predecessor's findings, and the code_review reply tells the worker it did so.

**Blocked by:** 06

**Status:** ready-for-agent

- [ ] A reviewer with a pass older than HEAD receives exactly the `pass..HEAD` diff on the next call.
- [ ] A reviewer with a pass at HEAD is not called.
- [ ] A dead pane is replaced; the new reviewer's prompt carries the summary; the reply says it was replaced.
- [ ] Covered over the fake exec.

## Hard limits (this ticket is built by the tool it changes)

- Spec: `.scratch/dogfood9/spec.md`. Read it for the decisions this ticket implements.
- Edit only files under `wick-pig/` (and this ticket's named files). Never touch `~/.local/bin`, `~/.pig`, `~/.claude`, `~/.claude.json`, `~/.agents`, `~/.wick-pig`, or `scripts/`.
- Never run `go install`, never copy binaries anywhere. Build with `go build ./...` and test with `go test -skip TestLive ./...` from `wick-pig/`.
- Never run a `TestLive*` test. Never start wick-pig, pig, claude or bunx inside the work. Tests use a fake exec and temp dirs, never the network or the real home.
- Servers, watchers and tails go to the `background` tool, never bash.
- Keep `go vet ./...` and `go test -skip TestLive ./...` green.
