# 01: Grid placement for read-only agents in the caller's tab

**What to build:** Scouts and other read-only agents appear side by side in the caller's tab, in rows of three, at most 9 panes counting the caller. The 10th opens a `scouts` tab. This is done by a new shared grid module that every later ticket also uses. Given a tab and an overflow label, it places one new pane and returns the pane id and tab id. Because herdr splits panes, not regions, the grid is three columns, each split into rows: panes 2 and 3 split `right`, and pane k (k ≥ 4) splits `down` from pane k−3, which reads row by row (`1 2 3` / `4 5 6` / `7 8 9`). After placing a pane, sizes are evened with the existing tiling logic. The module can also place a replacement in a given dead pane's slot instead of appending, without re-flowing the rest of the grid. The process-wide split lock becomes one lock per tab: splits in different tabs run concurrently, and splits in one tab are serialized. Overflow tab creation happens under the full tab's lock.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] A fake split-tree tab (extending the tiling tests' fake) shows that 9 placements land in rows of three with equal sizes, with the caller as pane 1.
- [ ] The 10th placement creates a tab with the given overflow label and puts the pane there. Further placements fill that tab by the same rules.
- [ ] Placing a replacement into a dead pane's slot puts it at that position. Other panes keep their positions, and only sizes are evened.
- [ ] Read-only launches no longer split `--current --direction down`. They go through the grid into the caller's tab, with overflow label `scouts`.
- [ ] Concurrent launches into two different tabs overlap their splits. Concurrent launches into one tab never overlap their splits but do overlap their waits (extends the existing concurrent read-only launch test).
- [ ] Tiling stays cosmetic: a tab that won't even out never fails a launch.

## Hard limits (this ticket is built by the tool it changes)

- Spec: `.scratch/ticket-tabs/spec.md`. Read it for the decisions this ticket implements.
- Edit only files under `wick-pig/` (and this ticket's named files). Never touch `~/.local/bin`, `~/.pig`, `~/.claude`, `~/.claude.json`, `~/.agents`, `~/.wick-pig`, or `scripts/`.
- Never run `go install`, never copy binaries anywhere. Build with `go build ./...` and test with `go test -skip TestLive ./...` from `wick-pig/`.
- Never run a `TestLive*` test. Never start wick-pig, pig, claude or bunx inside the work. Tests use a fake exec and temp dirs, never the network or the real home.
- Servers, watchers and tails go to the `background` tool, never bash.
- Keep `go vet ./...` and `go test -skip TestLive ./...` green.
