# 04: pig gate

**What to build:** A spawned pig agent, of any type, cannot settle without calling `submit`, and its last chance is forced at the API level. The pig extension that wick loads into agents (the one that already reports to herdr) handles `agent_before_settle`: it asks ticket 03's stop-gate decision. When the answer is block, it continues the agent with the reminder instead of letting it settle. When the answer is give up, it lets the agent settle, and the decision sends the "stopped without submitting" notice. On the attempt the decision marks as "pin", the extension also handles `before_provider_request` for that one request and sets `tool_choice` to the `submit` tool in the request payload. No other request is changed. pig's harness reports that it can pin, so it gets the 3rd-attempt pin that claude can't have.

**Blocked by:** 03 (Stop gate and the claude `Stop` hook)

**Status:** ready-for-agent

- [ ] Calling the extension's `agent_before_settle` handler directly, with no submit recorded: it continues with reminder 1, then 2, then 3, and on the 4th settle it lets the agent settle and the fallback notice is delivered.
- [ ] With a submit recorded this turn, or a provider-error turn, the handler lets the agent settle at once.
- [ ] `before_provider_request` sets `tool_choice` to `submit` only for the request that follows the 3rd reminder, and leaves every other request payload byte-for-byte unchanged.
- [ ] Every pig agent launch still loads the extension, and the reporting it already does to herdr is unchanged (existing reporter tests stay green).

## Hard limits (this ticket is built by the tool it changes)

- Spec: `.scratch/submit-results/spec.md`. Read it for the decisions this ticket implements.
- Edit only files under `wick-pig/` and `herdr-report/` (and this ticket's named files). Never touch `~/.local/bin`, `~/.pig`, `~/.claude`, `~/.claude.json`, `~/.agents`, `~/.wick-pig`, or `scripts/`.
- Never run `go install`, never copy binaries anywhere. Build with `go build ./...` and test with `go test -skip TestLive ./...` from `wick-pig/` (and from `herdr-report/` when it changes).
- Never run a `TestLive*` test. Never start wick-pig, pig, claude or bunx inside the work. Tests use a fake exec and temp dirs, never the network or the real home.
- Servers, watchers and tails go to the `background` tool, never bash.
- Keep `go vet ./...` and `go test -skip TestLive ./...` green.
