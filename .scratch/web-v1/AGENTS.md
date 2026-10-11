# AGENTS.md — .scratch/web-v1

## Purpose

The spec and ticket area for web v1 in `emberloop-web`: Handoff compress, Users, and Memory with Profile, Rules and Deciders. `spec.md` holds the decisions every ticket implements; `issues/` holds one file per ticket with scope, blockers, status and acceptance checkboxes; `share/` holds the research report behind the Memory design.

## Ownership

Owns `spec.md`, `issues/` and `share/` in this directory. Code changes from these tickets land in `emberloop/` and `emberloop-web/`, owned by their docs. Live probes land in `scripts/`.

## Local Contracts

- The spec decides conflicts between a ticket and itself.
- Ticket files carry status and checkboxes; only the driver or a merge into the integration branch moves them.
- Ticket order follows the `Blocked by` lines. Tickets 12 and 13 have boxes checked from the old Handoff spec; verify them against the code before closing.
- The terminal compress in `emberloop/` is out of scope; do not change it.
- ADR `docs/adr/0003-in-process-memory-engine-with-grounded-rules.md` records the Memory architecture.

## Work Guidance

- Read the spec before the ticket.
- Prior art: `.scratch/archive/context-compress/spec.md` for compress.

## Verification

No check.
