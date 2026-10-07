# emberloop:// addresses in the read tool, with an index

Type: build
Status: open
Blocked by: 22

## What

`readTool.Execute` (`emberloop/tools/files.go`) accepts `emberloop://<namespace>/<path>`:

    namespace maps to a dir under the skills root
        (the tools package cannot import emberloop: read $EMBERLOOP_SKILLS_DIR, else ~/.emberloop/skills)
    reject any path that resolves outside that namespace dir, including ".." and symlinks out
    if the path names a directory, return an index:
        one line per skill dir: name, then the description from its SKILL.md frontmatter
    otherwise read the file as usual
    unknown namespace: an error that lists the namespaces that exist

`emberloop://matt-pocock` therefore returns the index of every installed skill. Relative references inside a skill (for example `grilling/agents/`) resolve under the same scheme.

## Done when

- Tests use a temp skills root: index of two skills, a file read, a missing namespace, a `..` escape, a symlink escape, and a skill with no frontmatter description.
- Plain paths and `EMBERLOOP_READ_ALLOW` behave exactly as before.

## Hard limits (this ticket is built by the tool it changes)

- Edit only files under `emberloop/` (and this ticket's named files). Never touch `~/.local/bin`, `~/.pig`, `~/.claude`, `~/.claude.json`, `~/.agents`, `~/.emberloop`, or `scripts/`.
- Never run `go install`, never copy binaries anywhere. Build with `go build ./...` and test with `go test -skip TestLive ./...` from `emberloop/`.
- Never run a `TestLive*` test. Never start emberloop, pig, claude or bunx inside the work. Tests use a fake exec and temp dirs, never the network or the real home.
- Servers, watchers and tails go to the `background` tool, never bash.
- Keep `go test -skip TestLive ./...` green.
