# Retry an agent turn that ended in a provider error

Type: build
Status: open
Blocked by: 17

## What

A provider error (for example pig's "Provider returned an empty response") is the last record in the session transcript. The pane goes idle with no reply. `wait_workers` then treats the worker as settled, and `WaitReply` waits its whole 10 minutes. Pig already retries 3 times internally, so this is what is left after that.

## Done when

- `Harness` gains `TurnState(path) (state string, detail string)` with state one of `done`, `failed`, `running`. Pig: last record is an assistant message with `stopReason` `error` means failed, with its `errorMessage` as detail. Claude: last record is an assistant error or an API error entry. Unknown shape means running, never failed.
- The worker store record keeps the session id and session path, set at spawn.
- One helper `Nudge(run, pane, text)` waits for the pane to be idle, then sends the text. Use `herdr agent prompt`; when herdr says the pane is not an active named agent (the launcher's merger pane is not one), fall back to `herdr pane send-text <pane> <text>` followed by `herdr pane send-keys <pane> Enter`. `WaitReply` and `wait_workers` use it: on failed, wait a backoff (10s, 45s, then 120s), nudge with "continue" (workers) or the original prompt (reviewers), at most 3 nudges. The backoff list is a package variable so tests run it at zero.
- Why the long backoff: the failure is the upstream provider (OpenRouter, the default `stealth/space-bunny-alpha`) returning an empty body in bursts that clear after a few minutes. Direct calls with no pig reproduce it, and pig does not retry it.
- Manual retry: `wick-pig retry <worker-id|pane-id>` looks the pane up (workers.json for an id), checks `TurnState`, and nudges with "continue" if failed. It prints what it did, and exits non-zero when the turn is not failed.
- The launcher's late-prompt goroutine in `cmd/wick-pig` watches the merger's own transcript with the same loop, so a merger that fails on its first request is retried without a human.
- `spawn_worker` called several times in a row waits 3 seconds between launches, so a run does not open every request at once.
- Optional fallback: if the settings (`tools.Settings`) hold a `FallbackModel`, a worker still failed after the last nudge is relaunched once with that model. Unset means fail loudly. Do not add a settings command for it in this ticket.
- After the last nudge a still-failed worker is reported as **failed** with the error text in `wait_workers` output, and `finish_worker` refuses to merge it unless called with `merge` forced. A failed reviewer returns the error text as the tool error.
- Tests with a fake exec and transcripts: error then recovery, error three times (failed reported, no merge), a running turn is never nudged.
## Hard limits (this ticket is built by the tool it changes)

- Edit only files under `wick-pig/` (and this ticket's named files). Never touch `~/.local/bin`, `~/.pig`, `~/.claude`, `~/.claude.json`, or `scripts/`.
- Never run `go install`, never copy binaries anywhere. Build with `go build ./...` and test with `go test -skip TestLive ./...` from `wick-pig/`.
- Never run a `TestLive*` test. Never start wick-pig, pig or claude inside the work.
- Servers, watchers and tails go to the `background` tool, never bash.
- Keep `go test -skip TestLive ./...` green.
