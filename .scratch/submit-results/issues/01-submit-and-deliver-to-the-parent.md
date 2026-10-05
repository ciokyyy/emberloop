# 01: Agent types, `spawn` and `submit`, with scouts moved over

**What to build:** Scouts are started through one `spawn` tool by **type**, several in one call, all in parallel, and each answers with `submit`.

- **Types.** A type is a markdown file with frontmatter. The fields are `name`, `description`, `tools`, `checkout` (`worktree` / `none` / `parent`), `blocking`, `statuses`, `spawns`, `persistent`, `requires_review`, and optional `harness`/`model`. The body is the brief or prompt template.
- **Built-in types.** wick embeds `scout` (`none`, blocking, read-only tools, status `answered`). Files in `.wick/agents/*.md` add types or override built-ins by name. They are validated at MCP server start, and an invalid file is an error naming the file and field.
- **Who may spawn what.** A top-level role says which types it may spawn (`implement` and `plan`: scout for now; ticket 06 adds worker). A spawned agent may spawn its type's `spawns`.
- **`spawn`.** It takes a list of `{type, prompt, …}`. Every entry starts in parallel, with launches spaced as today, and placed as ticket-tabs places read-only agents. Blocking entries are awaited together, and their reports come back in one result, in list order. The tool description lists the types the caller may spawn. The `scout` tool is removed. Until ticket 06, a blocking spawn that outlasts the tool budget keeps today's "still running, call again" answer.
- **`submit`.** Every spawned agent gets `submit`, with `status` from its type's `statuses`. A blocking type submits a `report` (free text, no length limit). A non-blocking type submits a one-line `summary` of at most ~300 characters, used from ticket 06 on. A wrong status, a missing field, or an over-long or multi-line summary is rejected with a clear error. A submit is recorded in the agent's state as its last submit and as "submitted this turn", and the waiting `spawn` returns the report.

**Blocked by:** The ticket-tabs spec (`.scratch/ticket-tabs/`), merged and installed. Within this spec: none.

**Status:** ready-for-agent

- [ ] The built-in `scout` loads. A repo file adds a type, and a repo file named like a built-in overrides it. A file with a missing or invalid field fails loading, naming the file and field.
- [ ] `tools/list` has `spawn`, whose description lists only the caller's spawnable types, and no `scout` tool. `spawn` rejects a type the caller may not spawn.
- [ ] A fake-exec test: one `spawn` with three scouts launches them in parallel, and returns their three reports together, in list order.
- [ ] A scout runs in the repo with read-only tools only, and its pane is placed by the ticket-tabs grid.
- [ ] `submit` rejects a status outside the type's statuses, a missing `report`/`summary`, and an over-long or multi-line `summary`. A multi-line `report` is accepted and returned intact.
- [ ] The agent's state records the last submit and "submitted this turn", ready for the stop gate in ticket 03 to read.
- [ ] Existing scout tests move to `spawn` and keep their assertions.

## Hard limits (this ticket is built by the tool it changes)

- Spec: `.scratch/submit-results/spec.md`. Read it for the decisions this ticket implements.
- Edit only files under `wick-pig/` (and this ticket's named files). Never touch `~/.local/bin`, `~/.pig`, `~/.claude`, `~/.claude.json`, `~/.agents`, `~/.wick-pig`, or `scripts/`.
- Never run `go install`, never copy binaries anywhere. Build with `go build ./...` and test with `go test -skip TestLive ./...` from `wick-pig/`.
- Never run a `TestLive*` test. Never start wick-pig, pig, claude or bunx inside the work. Tests use a fake exec and temp dirs, never the network or the real home.
- Servers, watchers and tails go to the `background` tool, never bash.
- Keep `go vet ./...` and `go test -skip TestLive ./...` green.
