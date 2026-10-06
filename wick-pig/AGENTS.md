# AGENTS.md — wick-pig (main Go package)

## Purpose

The wick harness itself: pig and claude worker sessions, the spawn/grid
multiplexer plumbing, persistent reviewers, the vet-and-test gate, and the
`finish` merge. Builds and tests from this directory; the repo check runs
here and covers this module including `tools/`, `mcp/`, and `cmd/`.

## Ownership

Owns every `.go` file directly in this directory plus `cmd/` (the two
entry points). `tools/`, `mcp/`, `agenttypes/`, and `testdata/` are
separate durable boundaries with their own docs. File map (non-test file
owns its `_test.go` and the behaviour below):

- Sessions and harnesses: `harness.go` (pig/claude commands, system prompt),
  `models.go`, `resolve.go`, `background.go`, `exec.go`, `launch.go`
  (pane launch text), `commands.go`, `skills.go`
- Spawning and layout: `spawn.go`, `worker.go` (brief, worktree, lifecycle),
  `workerqueue.go` (worker-cap queue and drain), `grid.go`, `tile.go`,
  `background.go`, `orchestrate.go` (the spawn/finish/tell orchestration
  tools)
- Review and merge: `review.go` (packet, pass record), `submit.go`,
  `tell.go`, `deliver.go`, `integration.go`, `run.go`, `history.go`
- Tools served to agents: `tools/` (child doc); agent-visible shell surface
  is `background.go`, `exec.go`
- Transport: `mcp/` (child doc); binaries are `cmd/wick-pig/main.go` (harness
  CLI) and `cmd/wick-mcp/main.go` (tool server)
- Agent data: `agentstate.go`, `agenttype.go` (built-in types plus
  `.wick/agents` overrides), `transcript.go`, `usage.go`, `memory.go`,
  `archive.go`, `prune.go`
- Policy: `roles.go` (no role refuses to start), `readonly.go`,
  `stopgate.go`, `extension.go` (pig extension wiring, brief lines)
- Fixtures: `testdata/` (child doc)

## Local Contracts

- Every operation runs through the `Exec` seam (`exec.go`) so tests use a
  fake exec and temp directories; no test touches the network, the real
  home, or a live agent or harness.
- A wick MCP server with no role refuses to start; the full tool set needs
  an explicit full role.
- The tree stays gofmt-clean; formatting is a check result, never a
  judgement call.

## Work Guidance

- Build and test from this directory: `go build ./...`, `go vet ./...`,
  `go test -skip TestLive ./...`. Never run a live test and never start
  `wick-pig`, `pig`, or `claude` from inside the work.
- Behavioural change needs a test through the tool or command, not
  internals; expected values come from the spec or a worked example.

## Verification

`.wick/check` (`cd wick-pig && go vet ./... && go test -skip TestLive ./...`).

## Child DOX Index

- `wick-pig/tools/AGENTS.md` — the agent tools both hosts serve
- `wick-pig/mcp/AGENTS.md` — the MCP server transport over those tools
- `wick-pig/agenttypes/AGENTS.md` — the built-in worker, reviewer, and
  scout types
- `wick-pig/testdata/AGENTS.md` — captured fixtures tests parse
