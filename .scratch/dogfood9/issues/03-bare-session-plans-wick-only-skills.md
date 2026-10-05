# 03: A bare session plans; skills come only from wick://

**What to build:** `wick-pig` with no subcommand starts a `plan` session instead of `full`. `plan` serves read, edit, write, bash, find, background, scout, and no longer code_review. `full` is reachable only through an explicit role flag on the launcher. No wick session ever loads global skills: the pig harness keeps `--no-skills` and drops the `--skill <abs path>` loop; every harness, and every prompt and worker brief, refers to skills only by `wick://` address (the brief names `wick://` for implement and tdd). The harness-dependent skill reference and the session's skill list go away.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] A bare launch builds a `plan` session; the role flag can select `full`.
- [ ] `plan` serves exactly read, edit, write, bash, find, background, scout.
- [ ] pig commands carry no `--skill`; claude keeps its isolation flags.
- [ ] Prompts and briefs contain `wick://` skill addresses and no absolute skill paths, for both harnesses.
- [ ] Tests over RoleTools, harness Command and the brief/prompt text.

## Hard limits (this ticket is built by the tool it changes)

- Spec: `.scratch/dogfood9/spec.md`. Read it for the decisions this ticket implements.
- Edit only files under `wick-pig/` (and this ticket's named files). Never touch `~/.local/bin`, `~/.pig`, `~/.claude`, `~/.claude.json`, `~/.agents`, `~/.wick-pig`, or `scripts/`.
- Never run `go install`, never copy binaries anywhere. Build with `go build ./...` and test with `go test -skip TestLive ./...` from `wick-pig/`.
- Never run a `TestLive*` test. Never start wick-pig, pig, claude or bunx inside the work. Tests use a fake exec and temp dirs, never the network or the real home.
- Servers, watchers and tails go to the `background` tool, never bash.
- Keep `go vet ./...` and `go test -skip TestLive ./...` green.
