# Retry an agent turn that ended in a provider error

Type: build
Status: open
Blocked by: 17

## What

A provider error (for example pig's "Provider returned an empty response") is the last record in the session transcript. The pane goes idle with no reply. `wait_workers` then treats the worker as settled, and `WaitReply` waits its whole 10 minutes. Pig already retries 3 times internally, so this is what is left after that.

## Done when

- `Harness` gains `TurnState(path) (state string, detail string)` with state one of `done`, `failed`, `running`. Pig: last record is an assistant message with `stopReason` `error` means failed, with its `errorMessage` as detail. Claude: last record is an assistant error or an API error entry. Unknown shape means running, never failed.
- The worker store record keeps the session id and session path, set at spawn.
- One helper `Nudge(run, pane, text)` waits for the pane to be idle, then `herdr agent prompt`. `WaitReply` and `wait_workers` use it: on failed, wait a backoff (5s, then 20s), nudge with "continue" (workers) or the original prompt (reviewers), at most 2 nudges.
- After the last nudge a still-failed worker is reported as **failed** with the error text in `wait_workers` output, and `finish_worker` refuses to merge it unless called with `merge` forced. A failed reviewer returns the error text as the tool error.
- Tests with a fake exec and transcripts: error then recovery, error three times (failed reported, no merge), a running turn is never nudged.
## Hard limits (this ticket is built by the tool it changes)

- Edit only files under `emberloop/` (and this ticket's named files). Never touch `~/.local/bin`, `~/.pig`, `~/.claude`, `~/.claude.json`, or `scripts/`.
- Never run `go install`, never copy binaries anywhere. Build with `go build ./...` and test with `go test -skip TestLive ./...` from `emberloop/`.
- Never run a `TestLive*` test. Never start emberloop, pig or claude inside the work.
- Servers, watchers and tails go to the `background` tool, never bash.
- Keep `go test -skip TestLive ./...` green.
