# Long tools must return before the harness cuts the call

Type: build
Status: open
Blocked by: 15

## What

Pig (and claude) abort an MCP tool call at 60s. `wait_workers`, `review_worker` and `scout` block much longer. The caller sees "MCP request timed out after 60000ms", calls again, and `review_worker`/`scout` start a brand new reviewer pane each time. Ticket 12 capped `wait_workers` at 5 minutes; that is still above the limit.

## Done when

- One package constant `ToolBudget` = 50 seconds. `wait_workers` clamps to it (replace the 5 minute `WaitCap`), and its result says it was clamped and to call again.
- `review_worker` and `scout` start the reviewer at most once per key (worker id, or a hash of the scout question), keep it in a process-level map, and wait at most `ToolBudget`. If it is not done they return "still reviewing <key>, call review_worker again" and leave the reviewer running. A later call with the same key joins the running reviewer and returns its reply when done. The entry is removed once the reply is returned.
- Tool descriptions and `implementPrompt` tell the caller to expect "still reviewing" and to call again, not to restart.
- Tests with a fake exec: a call that outlives the budget returns the still-running text; the second call does not start a second reviewer and returns the reply; the clamp.
## Hard limits (this ticket is built by the tool it changes)

- Edit only files under `wick-pig/` (and this ticket's named files). Never touch `~/.local/bin`, `~/.pig`, `~/.claude`, `~/.claude.json`, or `scripts/`.
- Never run `go install`, never copy binaries anywhere. Build with `go build ./...` and test with `go test -skip TestLive ./...` from `wick-pig/`.
- Never run a `TestLive*` test. Never start wick-pig, pig or claude inside the work.
- Servers, watchers and tails go to the `background` tool, never bash.
- Keep `go test -skip TestLive ./...` green.
