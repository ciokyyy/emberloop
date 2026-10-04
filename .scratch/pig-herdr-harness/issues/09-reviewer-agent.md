# A reviewer agent that never touches the code

Type: build
Status: deferred — user declined; `/implement` in the wick extension is the
executor for now, no reviewer agent. Revisit only if self-review actually
produces bad merges.
Blocked by: 05

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

## Decisions still open

- **Who calls the reviewer.** The merger after every `finish_worker`, or the
  human on demand? Per-worker is more coverage; on-demand is fewer tokens.
- **Does a failed verdict block the merge?** If it does, the human has to
  unblock it, which is the veto the whole design keeps anyway.
- **Round trip.** Does the merger send a revision request back via
  `tell_worker`, and how many rounds before it stops asking?
- **Model.** A reviewer on the same model reviews the same blind spot. Worth
  a different model, which puts a `reviewer:` field next to the worker
  harness field from ticket 04.

## Effect on what exists

Building this makes the `code-review` entry in `workerSkills` redundant —
the worker no longer reviews itself. Delete that line in the same change
rather than leaving a self-review behind a real reviewer.

## Verification

- Reviewer is spawned with no write capability; assert a denied edit.
- Verdict arrives in the merger's transcript.
- A worker with a seeded defect produces a review naming that defect.