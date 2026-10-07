# Point sessions at the emberloop:// library

Type: build
Status: open
Blocked by: 22, 23

## What

`systemPrompt` (`harness.go`) gains one sentence: skills are listed by reading `emberloop://matt-pocock`; when the user asks for one by name (grill, research, wayfinder, tdd, to-spec), read it and follow it exactly. Every emberloop session carries it, pig and claude. `skillPrompt` (`commands.go`) and the worker brief (`writeBrief`) tell the agent to read `emberloop://matt-pocock/<skill>/SKILL.md` for claude sessions; pig keeps the absolute path it gets through `--skill`.

## Done when

- A test asserts the system prompt names `emberloop://matt-pocock` for both harnesses.
- A test asserts the brief and skill prompt use the `emberloop://` address under claude and the absolute path under pig.
- No code outside this ticket's files changes.

## Hard limits (this ticket is built by the tool it changes)

- Edit only files under `emberloop/` (and this ticket's named files). Never touch `~/.local/bin`, `~/.pig`, `~/.claude`, `~/.claude.json`, `~/.agents`, `~/.emberloop`, or `scripts/`.
- Never run `go install`, never copy binaries anywhere. Build with `go build ./...` and test with `go test -skip TestLive ./...` from `emberloop/`.
- Never run a `TestLive*` test. Never start emberloop, pig, claude or bunx inside the work. Tests use a fake exec and temp dirs, never the network or the real home.
- Servers, watchers and tails go to the `background` tool, never bash.
- Keep `go test -skip TestLive ./...` green.
