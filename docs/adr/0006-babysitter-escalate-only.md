---
status: accepted
---

# The Babysitter is a constrained agent Session that escalates and never stops agents

## Decision

Each Run gets one Babysitter: a cheap agent Session started lazily on the first trouble signal and woken by Follow-up afterwards. It investigates with read-only tools (`read`, `find`, and an allowlisted `git` tool for `status`, `diff`, `log`, `show`, `merge-base`, `ls-files`) and acts only through named actions the driver validates and executes: `steer_worker`, `steer_reviewer`, `retry`, `hygiene(name)` and `escalate(note)`. An action queues and is consumed by a driver pass under the per-Ticket lock; it is dropped when the tip, round or state changed since the Babysitter looked. The Babysitter never interrupts, pauses, Switches or ends a Session, never waives a Reviewer finding, never edits code and never touches the master checkout. A Reviewer round has no cap; a loop ends because the Babysitter Steers, and when Steering twice fails it escalates through needs-you with a diagnosis. The Session renews by Handoff at about 100k tokens, only at a turn boundary, with a Run-wide budget of about 1M tokens after which it goes silent. Its model comes from a `babysitter` role in Agents settings (host-global, admin-only) with a mode `off | shadow | active`, default `shadow`. The driver never waits on it: when it fails or hangs the Run continues on the existing nudges and needs-you alerts.

## Context

Runs lose most time to terminal states that need a person: refused final merges, red bases, Reviewer loops of 46 and 114 rounds, lost finished turns. The driver already has stall nudges, dead-turn detection, ACP respawn and Chains, so another timer adds little. What is missing is diagnosis: whether a repeated finding is the same topic, why a merge is refused, what a stuck Worker is waiting on. That is judgement over files and git state, not a fixed digest.

## Rejected alternatives

- Hard caps and auto-actions (a Reviewer round cap, turn deadlines that interrupt and Switch): the User wants no automatic stops of agents, and a cap drops real findings with the noise.
- A one-shot structured call on a digest: cheaper and simpler, but a merge refusal needs the Babysitter to look at git state and files the digest cannot predict.
- A free-shell agent: can repair more, but can also damage worktrees or the master checkout, and is hard to test.

## Consequences

- A Babysitter Session costs tokens only when a Run has trouble; throttles (one wake per Ticket per five minutes) and the Run budget bound it.
- Shadow mode keeps read tools real and makes action tools log only, so diagnosis quality is judged on real Runs before actions run.
- Deterministic causes (dirty master, `web/dist` rebase conflicts, red base at Start, stale `implement/*` branches) are fixed by driver code first (Part A of the spec) and only reach the Babysitter when that code gives up.
