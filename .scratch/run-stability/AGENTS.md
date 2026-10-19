# AGENTS.md — .scratch/run-stability

## Purpose

The spec and ticket area for Run stability in `emberloop-web`: deterministic fixes for stranded Runs (Part A), the Babysitter agent Session (Part B) and one global connection banner (Part C). `spec.md` holds the decisions every ticket implements; `issues/` holds one file per ticket with scope, blockers, status and acceptance checkboxes. Decision record for the Babysitter: `docs/adr/0006-babysitter-escalate-only.md`.

## Ownership

Owns `spec.md` and `issues/` in this directory. Code changes from these tickets land in `emberloop-web/` and `emberloop-web/el/`, owned by their docs. Live probes land in `scripts/`.

## Local Contracts

- The spec decides conflicts between a ticket and itself.
- Ticket files carry status and checkboxes; only the driver or a merge into the integration branch moves them.
- Ticket order follows the `Blocked by` lines.
- The spec rules out Reviewer round caps and any automatic interrupt, pause, Switch or Session end; no ticket adds one.

## Work Guidance

- Read the spec before the ticket.

## Verification

No check.
