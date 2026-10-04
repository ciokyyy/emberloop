# A reviewer agent that never touches the code

Type: build
Status: open — the earlier defer was reversed; build it.
Blocked by: nothing. 05 gated this, but herdr-as-the-executor is shipped and
running live, so 05 no longer blocks.

## Decisions — made, do not re-ask

A previous worker refused this ticket as deferred and asked. Answering, so the
next one does not stall:

- **Who calls the reviewer.** After every worker finishes, before its merge is
  offered. Per-worker coverage, not on demand — the whole point is that nobody
  has to remember.
- **Does a failing verdict block the merge.** No. It is surfaced to the merger
  and stops there. The human veto lives at the merge, which is the whole
  design's premise; a hard block here would add a second veto nobody asked
  for.
- **Revision round-trip.** None. `tell_worker` cannot deliver — see the
  constraints below — so a reviewer finding cannot bounce back to the worker
  automatically. The reviewer reports; a human decides what happens next.
- **Reviewer model.** Same model as the worker for now. Add a `reviewer:`
  field next to ticket 04's harness field when 04 lands; do not invent the
  model-assignment surface here.

## Why

Today `code-review` runs **inside the worker**, as step 3 of `workerSkills`:

```
implement  -> do the work
tdd        -> at the pre-agreed seams, while doing the work
code-review-> only after the work is done, before committing
```

The agent that wrote the diff reviews the diff. It cannot see what it got
wrong, because it is the thing that got it wrong. That is structural — no
prompt fixes it. The reviewer has to be a different agent in a different pane
with a different session.

## The mechanism (all verified to exist)

1. **Second pane.** `herdr pane split --pane <MERGER_PANE> --direction right`
   gives the reviewer its own pane beside the merger.
2. **No edit capability.** Launch with `pig --no-tools --tools read,grep,ls`.
   The reviewer can look but not touch. This is the enforcement, not a
   prompt — a reviewer that *can* edit will sometimes edit.
3. **Input is the diff, not the ticket.** The reviewer gets the worktree
   path and base ref, and reads them itself. Handing it the merger's summary
   would review the summary.
4. **Verdict returns to the merger** via `ctx.SendMessage`, which is
   available inside a tool handler. The merger owns the merge decision; the
   reviewer only reports.


## Effect on what exists

Building this makes the `code-review` entry in `workerSkills` redundant —
the worker no longer reviews itself. Delete that line in the same change
rather than leaving a self-review behind a real reviewer.

## Verification

- Reviewer is spawned with no write capability; assert a denied edit.
- Verdict arrives in the merger's transcript.
- A worker with a seeded defect produces a review naming that defect.

## Constraints — verified, do not re-derive

**`herdr agent prompt` cannot work. Do not build on it.** Verified this
session against herdr 0.9.1: `agent_not_ready: "agent <pane> is not an active
named agent"`, whether targeted by name or by pane, mid-turn or post-turn,
with or without a named session. herdr requires an `agent_session` it only
accepts from a first-party integration, and `ctx.GetSessionFile()` returns nil
even though pig does persist the session to disk. That is an SDK gap, not a
configuration mistake. The verdict must return through `ctx.SendMessage`
from the merger side.

**Build and verify:**

```bash
export PATH="$HOME/go/bin:$PATH"
cd wick && gofmt -w . && go vet ./... && go test ./... -count=1
pig install $PWD --validate-only --json    # must print valid: true
```

`go build` and `go mod tidy` do not work — PiG substitutes a version-matched
staged SDK at load time. Do not edit `go.mod`, `go.work` or `go.sum`.

**`pane run` executes argv[0] as the program** and types the joined argv into
the shell as one line. Env assignments must come first, multi-line prompts
arrive as one run-on sentence, and any argument containing spaces must be
pre-quoted — use the existing `shellArg()`, do not write another one.

**Only plain strings reach the model.** Returning a struct or map from a tool
handler renders as literally nothing; the model then reports "(no tool
output)". Use `fmt.Sprintf`, as `renderStatus` and `renderFinish` do.

**Skills are referenced by path, never copied.** `~/.agents/skills/<name>/SKILL.md`.
There are tests asserting the brief does not contain skill text.

**Cleanup:** `worktree remove` takes a workspace id, not a path, and
`worktree create` opens *two* workspaces — the worktree and the source
checkout. Close both; leaking them strands panes in `herdr workspace list`.

**Worker state** is `~/.pig/state/wick/workers.json`, briefs in
`~/.pig/state/wick/briefs/<id>.md`, guarded by `storeMu`.

## Current state

`go test ./... -count=1` — 34 passing. Six tools: `edit`, `read`,
`spawn_worker`, `wait_workers`, `tell_worker`, `finish_worker`. Plus the
`/implement` command. Do not regress any of them.