# el skills install

Type: build
Status: open
Blocked by: nothing

## What

`el skills install` fills the library `skillsRoot()/matt-pocock` from `mattpocock/skills` using the `skills` CLI. That CLI only installs into agent dirs, so run it in a temp git repo and move the result:

    in a temp dir, git init it
    run: bunx skills@latest add mattpocock/skills --skill '*' --agent universal --copy -y
    on failure: return its output, change nothing
    replace skillsRoot()/matt-pocock with the temp dir's .agents/skills
    keep skills-lock.json beside it
    remove the temp dir

`skillsRoot()` is `$EL_SKILLS_DIR` when set, else `~/.el/skills`. `skillsDir()` in `extension.go` becomes `skillsRoot()/matt-pocock`, so `skillPath` follows. Wire the subcommand in `cmd/el/main.go` beside `settings` and `retry`.

## Done when

- The install runs through the `Exec` seam so a test drives it with a fake that writes a skill tree into the temp dir.
- A failed bunx leaves the previous library untouched. A missing `bunx` returns a message that says to install bun.
- A second install replaces the first, with no leftover skills.
- Tests cover success, failure with the old library kept, and missing bunx.

## Hard limits (this ticket is built by the tool it changes)

- Edit only files under `el/` (and this ticket's named files). Never touch `~/.local/bin`, `~/.pig`, `~/.claude`, `~/.claude.json`, `~/.agents`, `~/.el`, or `scripts/`.
- Never run `go install`, never copy binaries anywhere. Build with `go build ./...` and test with `go test -skip TestLive ./...` from `el/`.
- Never run a `TestLive*` test. Never start el, pig, claude or bunx inside the work. Tests use a fake exec and temp dirs, never the network or the real home.
- Servers, watchers and tails go to the `background` tool, never bash.
- Keep `go test -skip TestLive ./...` green.
