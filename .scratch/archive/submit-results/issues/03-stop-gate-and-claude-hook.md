# 03: Stop gate and the claude `Stop` hook

**What to build:** A spawned claude agent, of any type, cannot end its turn without calling `submit`. One stop-gate decision, shared by every harness, takes: whether a submit was recorded this turn, how many times this turn has already been blocked, and whether the turn ended in a provider error. It returns one of these:
- **allow** when a submit was recorded, the turn ended in a provider error, or the agent has auto-backgrounded children that still owe a result (it is waiting for their notices; ticket 06 records them)
- **block** with reminder n of 3
- **block and pin `submit`** on the 3rd attempt, only for harnesses that can pin
- **give up** after 3 blocks: allow the stop, and send `<system-notice> <type> <id> stopped without submitting: <excerpt of last message></system-notice>` the way that agent's submit would go (returned to a waiting blocking `spawn`, or delivered to the parent through ticket 06's delivery)

The count resets when a new turn starts. On claude, the `Stop` hook input's `stop_hook_active` tells the two apart: false means a fresh turn (a `tell`, a delivered notice), true means this stop follows a block. The reminder follows omp's wording (see the spec): every turn ends with a tool call; keep working is the default and not a stop order; submit a final status only when the work is genuinely complete and `blocked` only for a real, nameable blocker; never invent a blocker; never end a turn with text only. A `el stop-gate` subcommand reads claude `Stop` hook JSON on stdin, asks the decision, and writes `{"decision":"block","reason":<reminder>}` or nothing. claude cannot pin a tool, so its 3rd block says it is the last. Every claude agent launch passes the `Stop` hook inline with `--settings`, which still applies with `--setting-sources ""`. The `Harness` interface gains a gate. pig gets its own in ticket 04. A harness that cannot have a gate is out of scope and is decided when one is added.

**Blocked by:** 06 (Workers on `spawn`, delivery to the parent, auto-background)

**Status:** ready-for-agent

- [ ] The claude agent command carries `--settings` with a `Stop` hook that runs `el stop-gate`, and the rest of the stripped launch is unchanged (extends `TestClaudeRunsStripped`).
- [ ] stop-gate subcommand over stdin/stdout: no output when a submit is recorded this turn; reminders 1, 2, 3 (as block JSON) on successive stops without a submit; no output on the 4th stop. For a non-blocking agent, the "stopped without submitting" notice is delivered to the parent through the fake exec. For a blocking agent, it becomes that agent's result in the waiting `spawn`.
- [ ] No output, and nothing delivered, when the turn ended in a provider error, or when the agent has backgrounded children still owing a result.
- [ ] A stop with `stop_hook_active` false starts the count from zero.
- [ ] The decision function returns "pin" on the 3rd attempt only for a harness that can pin, so pig can use it in ticket 04.

## Hard limits (this ticket is built by the tool it changes)

- Spec: `.scratch/archive/submit-results/spec.md`. Read it for the decisions this ticket implements.
- Edit only files under `el/` (and this ticket's named files). Never touch `~/.local/bin`, `~/.pig`, `~/.claude`, `~/.claude.json`, `~/.agents`, `~/.el`, or `scripts/`.
- Never run `go install`, never copy binaries anywhere. Build with `go build ./...` and test with `go test -skip TestLive ./...` from `el/`.
- Never run a `TestLive*` test. Never start el, pig, claude or bunx inside the work. Tests use a fake exec and temp dirs, never the network or the real home.
- Servers, watchers and tails go to the `background` tool, never bash.
- Keep `go vet ./...` and `go test -skip TestLive ./...` green.
