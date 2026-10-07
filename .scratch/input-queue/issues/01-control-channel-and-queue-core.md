# 01: Emberloop prompts reach a wrapped agent through one queue

**What to build:** The wrapper that owns an agent child also owns a control channel (a unix socket named by agent identity) and a queue, and is the only writer to the child PTY. The shared prompt-delivery helper tries the channel first and falls back to the existing herdr path when no wrapper runs. Each item is a steer (delivered at the next tool boundary) or a queued message (delivered after the turn ends); delivery counts as accepted only when the transcript shows the turn, and an item a startup dialog swallowed stays at the head of the queue. The strip shows `N pending` while items wait. Compress handoffs, reviewer follow-ups, steering and late prompts all use this path.

**Blocked by:** None (can start immediately)

**Status:** done

- [x] A prompt sent with the tell command to a wrapped agent arrives through the wrapper, never through herdr send-text
- [x] With no wrapper running, delivery takes the existing herdr path unchanged
- [x] Steer waits for the tool boundary; queue waits for turn end
- [x] An item not shown in the transcript as a user turn is retried and stays first in the queue
- [x] The strip shows the pending count and clears it when the queue is empty
- [x] Tests through the control channel and a fake child PTY with a fake clock: assert the exact bytes the PTY receives and the strip text
- [x] Spec stories 25, 28, 29, 4

Spec: `.scratch/input-queue/spec.md`. Read it for the decisions this ticket implements and its Out of Scope section.
